package sbom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/vulnscan"
)

// vexStore is the narrow interface the VEX job needs from the store.
type vexStore interface {
	GetArtifact(artifactID string) (store.Artifact, bool, error)
	GetLatestArtifactVulnScan(artifactID string) (store.ArtifactVulnerabilityScan, bool, error)
	ListVexAssertions(artifactID string) ([]store.VexAssertion, error)
	SetArtifactVexObjectKey(artifactID, vexObjectKey string) error
}

// VexJob generates CycloneDX VEX documents for artifacts and stores them in MinIO.
// It runs after a vulnerability scan completes and the SBOM is available.
type VexJob struct {
	objectStore ObjectStore
	bucket      string
	store       vexStore
	logger      *log.Logger
}

// NewVexJob creates a VexJob.
func NewVexJob(objectStore ObjectStore, bucket string, st vexStore, logger *log.Logger) *VexJob {
	return &VexJob{
		objectStore: objectStore,
		bucket:      bucket,
		store:       st,
		logger:      logger,
	}
}

// Trigger runs VEX generation for the given artifact in a background goroutine.
func (j *VexJob) Trigger(artifactID string) {
	go j.run(artifactID)
}

func (j *VexJob) run(artifactID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	artifact, ok, err := j.store.GetArtifact(artifactID)
	if err != nil || !ok {
		j.logger.Printf("vex: get artifact %s: %v ok=%v", artifactID, err, ok)
		return
	}
	if artifact.SBOMObjectKey == "" {
		// SBOM not yet generated; the job will be re-triggered after the SBOM job runs.
		j.logger.Printf("vex: artifact %s has no SBOM yet, skipping", artifactID)
		return
	}

	scan, ok, err := j.store.GetLatestArtifactVulnScan(artifactID)
	if err != nil || !ok || scan.ScanStatus != "completed" {
		j.logger.Printf("vex: artifact %s scan not ready (ok=%v status=%s): %v", artifactID, ok, scan.ScanStatus, err)
		return
	}

	var findings []vulnscan.VulnerabilityFinding
	if len(scan.FindingsJSON) > 0 {
		if err := json.Unmarshal(scan.FindingsJSON, &findings); err != nil {
			j.logger.Printf("vex: parse findings for %s: %v", artifactID, err)
			return
		}
	}

	manualAssertions, err := j.store.ListVexAssertions(artifactID)
	if err != nil {
		j.logger.Printf("vex: list assertions for %s: %v", artifactID, err)
		return
	}

	vexData, err := buildVexDocument(artifact, findings, manualAssertions)
	if err != nil {
		j.logger.Printf("vex: build document for %s: %v", artifactID, err)
		return
	}

	vexKey := "sboms/" + artifactID + ".vex.json"
	if _, err := j.objectStore.PutObject(ctx, j.bucket, vexKey,
		bytes.NewReader(vexData), int64(len(vexData)), "application/vnd.cyclonedx+json"); err != nil {
		j.logger.Printf("vex: upload for %s: %v", artifactID, err)
		return
	}

	if err := j.store.SetArtifactVexObjectKey(artifactID, vexKey); err != nil {
		j.logger.Printf("vex: save key for %s: %v", artifactID, err)
		return
	}

	j.logger.Printf("vex: generated for artifact %s key=%s", artifactID, vexKey)
}

// ─── CycloneDX VEX document types (minimal subset) ──────────────────────────

type vexDocument struct {
	BOMFormat       string           `json:"bomFormat"`
	SpecVersion     string           `json:"specVersion"`
	Version         int              `json:"version"`
	SerialNumber    string           `json:"serialNumber"`
	Metadata        vexMetadata      `json:"metadata"`
	Vulnerabilities []vexEntry       `json:"vulnerabilities"`
}

type vexMetadata struct {
	Timestamp string            `json:"timestamp"`
	Component vexComponent      `json:"component"`
}

type vexComponent struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type vexEntry struct {
	ID       string        `json:"id"`
	Analysis vexAnalysis   `json:"analysis"`
	Affects  []vexAffects  `json:"affects,omitempty"`
}

type vexAnalysis struct {
	State         string `json:"state"`
	Justification string `json:"justification,omitempty"`
}

type vexAffects struct {
	Ref string `json:"ref"`
}

// buildVexDocument produces a CycloneDX VEX JSON document.
// Manual assertions override scanner-derived defaults for matching CVE/component pairs.
func buildVexDocument(artifact store.Artifact, findings []vulnscan.VulnerabilityFinding, manual []store.VexAssertion) ([]byte, error) {
	// Index manual assertions by "cveID|componentName" for O(1) lookup.
	manualIdx := make(map[string]store.VexAssertion, len(manual))
	for _, a := range manual {
		manualIdx[a.CVEID+"|"+a.ComponentName] = a
	}

	// Deduplicate CVE IDs from findings, keeping the highest-severity finding per CVE.
	type cveKey struct{ cveID, component string }
	seen := map[cveKey]vulnscan.VulnerabilityFinding{}
	for _, f := range findings {
		k := cveKey{f.ID, f.Package}
		if _, exists := seen[k]; !exists {
			seen[k] = f
		}
	}

	var entries []vexEntry
	for k, f := range seen {
		idxKey := k.cveID + "|" + k.component
		state := "under_investigation"
		justification := ""

		if ma, ok := manualIdx[idxKey]; ok {
			state = normalizeAssertionState(ma.Assertion)
			justification = ma.Justification
		} else if strings.TrimSpace(f.FixedIn) != "" {
			state = "fixed"
		}

		entry := vexEntry{
			ID: k.cveID,
			Analysis: vexAnalysis{
				State:         state,
				Justification: justification,
			},
		}
		if k.component != "" {
			entry.Affects = []vexAffects{{Ref: k.component + "@" + f.Version}}
		}
		entries = append(entries, entry)
	}

	// Add manual assertions for CVEs not in scanner findings.
	for _, ma := range manual {
		k := cveKey{ma.CVEID, ma.ComponentName}
		if _, alreadyIn := seen[k]; !alreadyIn {
			entry := vexEntry{
				ID: ma.CVEID,
				Analysis: vexAnalysis{
					State:         normalizeAssertionState(ma.Assertion),
					Justification: ma.Justification,
				},
			}
			if ma.ComponentName != "" {
				entry.Affects = []vexAffects{{Ref: ma.ComponentName}}
			}
			entries = append(entries, entry)
		}
	}

	doc := vexDocument{
		BOMFormat:   "CycloneDX",
		SpecVersion: "1.5",
		Version:     1,
		SerialNumber: "urn:uuid:" + uuid.NewString(),
		Metadata: vexMetadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Component: vexComponent{
				Type:    "application",
				Name:    artifact.Name,
				Version: artifact.Version,
			},
		},
		Vulnerabilities: entries,
	}

	return json.MarshalIndent(doc, "", "  ")
}

func normalizeAssertionState(s string) string {
	switch strings.ToLower(s) {
	case "affected":
		return "affected"
	case "not_affected":
		return "not_affected"
	case "fixed":
		return "fixed"
	default:
		return "under_investigation"
	}
}

// GenerateVexForArtifact builds and returns a VEX document without persisting it.
// Used by the QMS package bundler (G5) to include VEX inline.
func GenerateVexForArtifact(artifact store.Artifact, findings []vulnscan.VulnerabilityFinding, manual []store.VexAssertion) ([]byte, error) {
	return buildVexDocument(artifact, findings, manual)
}

// vexKeyForArtifact returns the canonical MinIO key for an artifact's VEX file.
func vexKeyForArtifact(artifactID string) string {
	return fmt.Sprintf("sboms/%s.vex.json", artifactID)
}
