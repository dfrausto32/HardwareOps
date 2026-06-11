package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/vulnscan"
)

// ── narrow interfaces ─────────────────────────────────────────────────────────

type qmsStore interface {
	GetArtifact(artifactID string) (store.Artifact, bool, error)
	GetChangeRecordForArtifact(artifactID string) (store.ChangeRecord, bool, error)
	GetLatestArtifactVulnScan(artifactID string) (store.ArtifactVulnerabilityScan, bool, error)
	ListAttestations(artifactID string) ([]store.AttestationRecord, error)
	ListAuditEvents(filter store.AuditEventFilter) ([]store.AuditEvent, error)
	CreateAuditEvent(event store.AuditEvent) error
}

type qmsObjectStore interface {
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (int64, error)
	PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error)
}

// ── prerequisite types ────────────────────────────────────────────────────────

type QMSPrerequisite struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

type QMSPrerequisites struct {
	SBOM         QMSPrerequisite `json:"sbom"`
	VEX          QMSPrerequisite `json:"vex"`
	ChangeRecord QMSPrerequisite `json:"changeRecord"`
	VulnScan     QMSPrerequisite `json:"vulnScan"`
}

func (p QMSPrerequisites) allReady() bool {
	return p.SBOM.Ready && p.VEX.Ready && p.ChangeRecord.Ready && p.VulnScan.Ready
}

// ── handler ───────────────────────────────────────────────────────────────────

// PostQMSPackage handles POST /api/v1/medical/artifacts/{artifactId}/qms-package.
// Generates an ISO 13485 evidence bundle ZIP on demand and returns a presigned URL.
func PostQMSPackage(logger *log.Logger, st qmsStore, objStore qmsObjectStore, bucket string, presignExpires time.Duration, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		actorID := changeRecordActorID(r)

		artifact, ok, err := st.GetArtifact(artifactID)
		if err != nil {
			logger.Printf("qms: get artifact %s: %v", artifactID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}

		changeRec, crOK, err := st.GetChangeRecordForArtifact(artifactID)
		if err != nil {
			logger.Printf("qms: get change record %s: %v", artifactID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		scan, scanOK, err := st.GetLatestArtifactVulnScan(artifactID)
		if err != nil {
			logger.Printf("qms: get vuln scan %s: %v", artifactID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		prereqs := checkQMSPrerequisites(artifact, changeRec, crOK, scan, scanOK)
		if !prereqs.allReady() {
			auditQMSPackage(logger, st, r, trustProxy, actorID, artifactID, false)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":         "prerequisites not met",
				"prerequisites": prereqs,
			})
			return
		}

		attestations, err := st.ListAttestations(artifactID)
		if err != nil {
			logger.Printf("qms: list attestations %s: %v", artifactID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		crAudit, err := st.ListAuditEvents(store.AuditEventFilter{
			TargetType: "change_record",
			TargetID:   changeRec.RecordID,
			Limit:      500,
		})
		if err != nil {
			logger.Printf("qms: change record audit %s: %v", artifactID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		// Collect desired-state upsert events from both group and device history.
		deployAudit, err := collectDeploymentAudit(st, 500)
		if err != nil {
			logger.Printf("qms: deployment audit %s: %v", artifactID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		// Fetch SBOM and VEX from object store.
		sbomData, err := fetchObject(r.Context(), objStore, bucket, artifact.SBOMObjectKey)
		if err != nil {
			logger.Printf("qms: fetch SBOM %s: %v", artifact.SBOMObjectKey, err)
			http.Error(w, "storage error fetching SBOM", http.StatusInternalServerError)
			return
		}
		vexData, err := fetchObject(r.Context(), objStore, bucket, artifact.VexObjectKey)
		if err != nil {
			logger.Printf("qms: fetch VEX %s: %v", artifact.VexObjectKey, err)
			http.Error(w, "storage error fetching VEX", http.StatusInternalServerError)
			return
		}

		zipData, err := buildQMSZip(artifact, changeRec, scan, attestations, crAudit, deployAudit, sbomData, vexData)
		if err != nil {
			logger.Printf("qms: build zip %s: %v", artifactID, err)
			http.Error(w, "package generation error", http.StatusInternalServerError)
			return
		}

		packageKey := fmt.Sprintf("qms/%s/package-%d.zip", artifactID, time.Now().UTC().UnixMilli())
		if _, err := objStore.PutObject(r.Context(), bucket, packageKey, bytes.NewReader(zipData), int64(len(zipData)), "application/zip"); err != nil {
			logger.Printf("qms: upload zip %s: %v", artifactID, err)
			http.Error(w, "package upload error", http.StatusInternalServerError)
			return
		}

		url, err := objStore.PresignGet(r.Context(), bucket, packageKey, presignExpires)
		if err != nil {
			logger.Printf("qms: presign %s: %v", packageKey, err)
			http.Error(w, "presign error", http.StatusInternalServerError)
			return
		}

		auditQMSPackage(logger, st, r, trustProxy, actorID, artifactID, true)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"url":        url,
			"packageKey": packageKey,
		})
	}
}

// ── prerequisite check ────────────────────────────────────────────────────────

func checkQMSPrerequisites(artifact store.Artifact, changeRec store.ChangeRecord, crOK bool, scan store.ArtifactVulnerabilityScan, scanOK bool) QMSPrerequisites {
	p := QMSPrerequisites{}

	if artifact.SBOMObjectKey != "" {
		p.SBOM = QMSPrerequisite{Ready: true}
	} else {
		p.SBOM = QMSPrerequisite{Ready: false, Reason: "SBOM not yet generated"}
	}

	if artifact.VexObjectKey != "" {
		p.VEX = QMSPrerequisite{Ready: true}
	} else {
		p.VEX = QMSPrerequisite{Ready: false, Reason: "VEX document not yet generated"}
	}

	if crOK && changeRec.Status == "approved" {
		p.ChangeRecord = QMSPrerequisite{Ready: true}
	} else if !crOK {
		p.ChangeRecord = QMSPrerequisite{Ready: false, Reason: "no change record exists"}
	} else {
		p.ChangeRecord = QMSPrerequisite{Ready: false, Reason: fmt.Sprintf("change record status is %q; must be approved", changeRec.Status)}
	}

	if scanOK && scan.ScanStatus == "completed" {
		p.VulnScan = QMSPrerequisite{Ready: true}
	} else if !scanOK {
		p.VulnScan = QMSPrerequisite{Ready: false, Reason: "no vulnerability scan exists"}
	} else {
		p.VulnScan = QMSPrerequisite{Ready: false, Reason: fmt.Sprintf("vulnerability scan status is %q; must be completed", scan.ScanStatus)}
	}

	return p
}

// ── ZIP builder ───────────────────────────────────────────────────────────────

type qmsManifest struct {
	ArtifactID    string   `json:"artifactId"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Type          string   `json:"type"`
	SafetyClass   string   `json:"safetyClass,omitempty"`
	SBOMKey       string   `json:"sbomObjectKey"`
	VEXKey        string   `json:"vexObjectKey"`
	AttestationIDs []string `json:"attestationIds"`
	EosDate       string   `json:"eosDate,omitempty"`
	GeneratedAt   string   `json:"generatedAt"`
}

type vulnScanSummary struct {
	ScanID         string                        `json:"scanId"`
	ScannerType    string                        `json:"scannerType"`
	ScannerVersion string                        `json:"scannerVersion"`
	Status         string                        `json:"status"`
	ScannedAt      time.Time                     `json:"scannedAt"`
	SeverityCounts json.RawMessage               `json:"severityCounts"`
	CriticalHigh   []vulnscan.VulnerabilityFinding `json:"criticalHighFindings"`
}

func buildQMSZip(
	artifact store.Artifact,
	changeRec store.ChangeRecord,
	scan store.ArtifactVulnerabilityScan,
	attestations []store.AttestationRecord,
	crAudit []store.AuditEvent,
	deployAudit []store.AuditEvent,
	sbomData, vexData []byte,
) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	attestIDs := make([]string, 0, len(attestations))
	for _, a := range attestations {
		attestIDs = append(attestIDs, a.AttestationID)
	}

	eosDate := ""
	if !artifact.EosDate.IsZero() {
		eosDate = artifact.EosDate.Format("2006-01-02")
	}

	manifest := qmsManifest{
		ArtifactID:     artifact.ArtifactID,
		Name:           artifact.Name,
		Version:        artifact.Version,
		Type:           artifact.Type,
		SafetyClass:    artifact.SafetyClass,
		SBOMKey:        artifact.SBOMObjectKey,
		VEXKey:         artifact.VexObjectKey,
		AttestationIDs: attestIDs,
		EosDate:        eosDate,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	if err := zipAddJSON(zw, "manifest.json", manifest); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if err := zipAddBytes(zw, "sbom.cdx.json", sbomData); err != nil {
		return nil, fmt.Errorf("sbom: %w", err)
	}
	if err := zipAddBytes(zw, "sbom.vex.json", vexData); err != nil {
		return nil, fmt.Errorf("vex: %w", err)
	}
	if err := zipAddJSON(zw, "attestations.json", attestations); err != nil {
		return nil, fmt.Errorf("attestations: %w", err)
	}
	if err := zipAddJSON(zw, "change-record.json", changeRec); err != nil {
		return nil, fmt.Errorf("change-record: %w", err)
	}
	if err := zipAddJSON(zw, "change-record-audit-trail.json", crAudit); err != nil {
		return nil, fmt.Errorf("change-record-audit-trail: %w", err)
	}
	if err := zipAddJSON(zw, "vuln-scan-summary.json", buildVulnScanSummary(scan)); err != nil {
		return nil, fmt.Errorf("vuln-scan-summary: %w", err)
	}
	if err := zipAddJSON(zw, "deployment-audit-trail.json", deployAudit); err != nil {
		return nil, fmt.Errorf("deployment-audit-trail: %w", err)
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return buf.Bytes(), nil
}

func buildVulnScanSummary(scan store.ArtifactVulnerabilityScan) vulnScanSummary {
	var all []vulnscan.VulnerabilityFinding
	if len(scan.FindingsJSON) > 0 {
		_ = json.Unmarshal(scan.FindingsJSON, &all)
	}
	var critHigh []vulnscan.VulnerabilityFinding
	for _, f := range all {
		sev := strings.ToLower(f.Severity)
		if sev == "critical" || sev == "high" {
			critHigh = append(critHigh, f)
		}
	}
	if critHigh == nil {
		critHigh = []vulnscan.VulnerabilityFinding{}
	}
	return vulnScanSummary{
		ScanID:         scan.ScanID,
		ScannerType:    scan.ScannerType,
		ScannerVersion: scan.ScannerVersion,
		Status:         scan.ScanStatus,
		ScannedAt:      scan.ScannedAt,
		SeverityCounts: json.RawMessage(scan.SeverityCountsJSON),
		CriticalHigh:   critHigh,
	}
}

func collectDeploymentAudit(st qmsStore, limit int) ([]store.AuditEvent, error) {
	groups, err := st.ListAuditEvents(store.AuditEventFilter{Action: "desired_state_group.upsert", Limit: limit})
	if err != nil {
		return nil, err
	}
	devices, err := st.ListAuditEvents(store.AuditEventFilter{Action: "desired_state_device.upsert", Limit: limit})
	if err != nil {
		return nil, err
	}
	combined := make([]store.AuditEvent, 0, len(groups)+len(devices))
	combined = append(combined, groups...)
	combined = append(combined, devices...)
	return combined, nil
}

// ── ZIP helpers ───────────────────────────────────────────────────────────────

func zipAddJSON(zw *zip.Writer, name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return zipAddBytes(zw, name, data)
}

func zipAddBytes(zw *zip.Writer, name string, data []byte) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

func fetchObject(ctx context.Context, objStore qmsObjectStore, bucket, key string) ([]byte, error) {
	rc, err := objStore.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", key, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// ── audit helper ──────────────────────────────────────────────────────────────

func auditQMSPackage(logger *log.Logger, st qmsStore, r *http.Request, trustProxy bool, actorID, artifactID string, success bool) {
	event := buildAuditEvent(r, trustProxy, actorUser(actorID), "qms_package.generated", "artifact", artifactID)
	if !success {
		event.Status = "error"
		event.Error = "prerequisites_not_met"
	}
	writeChangeRecordAudit(logger, st, event)
}
