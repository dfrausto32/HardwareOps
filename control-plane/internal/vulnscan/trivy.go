package vulnscan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// TrivyScanner runs the Trivy CLI against an artifact downloaded from object
// storage.
type TrivyScanner struct {
	binPath     string
	objectStore ObjectStore
	bucket      string
}

// NewTrivyScanner creates a TrivyScanner. binPath defaults to "trivy" when empty.
func NewTrivyScanner(binPath string, objectStore ObjectStore, bucket string) *TrivyScanner {
	if binPath == "" {
		binPath = "trivy"
	}
	return &TrivyScanner{binPath: binPath, objectStore: objectStore, bucket: bucket}
}

func (t *TrivyScanner) Type() string { return "trivy" }

func (t *TrivyScanner) Scan(ctx context.Context, req ScanRequest) (ScanResult, error) {
	tmp, err := downloadToTemp(ctx, t.objectStore, t.bucket, req.ObjectKey)
	if err != nil {
		return ScanResult{}, fmt.Errorf("trivy: download artifact: %w", err)
	}
	defer os.Remove(tmp)

	scanPath, cleanup, err := prepareScanPath(tmp)
	if err != nil {
		return ScanResult{}, fmt.Errorf("trivy: %w", err)
	}
	defer cleanup()

	out, err := exec.CommandContext(ctx, t.binPath, "fs", "--format", "json", scanPath).Output()
	if err != nil {
		if len(out) == 0 {
			return ScanResult{}, fmt.Errorf("trivy: run: %w", err)
		}
	}

	return parseTrivyOutput(out)
}

// trivyOutput is the top-level JSON structure emitted by `trivy fs --format json`.
type trivyOutput struct {
	SchemaVersion int    `json:"SchemaVersion"`
	ArtifactName  string `json:"ArtifactName"`
	Results       []struct {
		Vulnerabilities []struct {
			VulnerabilityID  string `json:"VulnerabilityID"`
			PkgName          string `json:"PkgName"`
			InstalledVersion string `json:"InstalledVersion"`
			FixedVersion     string `json:"FixedVersion"`
			Severity         string `json:"Severity"`
			Description      string `json:"Description"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

func parseTrivyOutput(raw []byte) (ScanResult, error) {
	var t trivyOutput
	if err := json.Unmarshal(raw, &t); err != nil {
		return ScanResult{}, fmt.Errorf("trivy: parse output: %w", err)
	}
	var findings []VulnerabilityFinding
	for _, res := range t.Results {
		for _, v := range res.Vulnerabilities {
			findings = append(findings, VulnerabilityFinding{
				ID:          v.VulnerabilityID,
				Severity:    v.Severity,
				Package:     v.PkgName,
				Version:     v.InstalledVersion,
				FixedIn:     v.FixedVersion,
				Description: v.Description,
			})
		}
	}
	return ScanResult{
		Findings:       findings,
		SeverityCounts: countSeverities(findings),
	}, nil
}
