package vulnscan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// GrypeScanner runs the Grype CLI against an artifact downloaded from object
// storage.
type GrypeScanner struct {
	binPath     string
	objectStore ObjectStore
	bucket      string
}

// NewGrypeScanner creates a GrypeScanner. binPath defaults to "grype" when empty.
func NewGrypeScanner(binPath string, objectStore ObjectStore, bucket string) *GrypeScanner {
	if binPath == "" {
		binPath = "grype"
	}
	return &GrypeScanner{binPath: binPath, objectStore: objectStore, bucket: bucket}
}

func (g *GrypeScanner) Type() string { return "grype" }

func (g *GrypeScanner) Scan(ctx context.Context, req ScanRequest) (ScanResult, error) {
	tmp, err := downloadToTemp(ctx, g.objectStore, g.bucket, req.ObjectKey)
	if err != nil {
		return ScanResult{}, fmt.Errorf("grype: download artifact: %w", err)
	}
	defer os.Remove(tmp)

	out, err := exec.CommandContext(ctx, g.binPath, tmp, "-o", "json").Output()
	if err != nil {
		// Grype exits non-zero when findings are present; try to parse anyway.
		if len(out) == 0 {
			return ScanResult{}, fmt.Errorf("grype: run: %w", err)
		}
	}

	return parseGrypeOutput(out)
}

// grypeOutput is the top-level JSON structure emitted by `grype -o json`.
type grypeOutput struct {
	Descriptor struct {
		Version string `json:"version"`
	} `json:"descriptor"`
	Matches []struct {
		Vulnerability struct {
			ID          string `json:"id"`
			Severity    string `json:"severity"`
			Description string `json:"description"`
			Fix         struct {
				Versions []string `json:"versions"`
			} `json:"fix"`
		} `json:"vulnerability"`
		Artifact struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"artifact"`
	} `json:"matches"`
}

func parseGrypeOutput(raw []byte) (ScanResult, error) {
	var g grypeOutput
	if err := json.Unmarshal(raw, &g); err != nil {
		return ScanResult{}, fmt.Errorf("grype: parse output: %w", err)
	}
	findings := make([]VulnerabilityFinding, 0, len(g.Matches))
	for _, m := range g.Matches {
		fixedIn := ""
		if len(m.Vulnerability.Fix.Versions) > 0 {
			fixedIn = m.Vulnerability.Fix.Versions[0]
		}
		findings = append(findings, VulnerabilityFinding{
			ID:          m.Vulnerability.ID,
			Severity:    m.Vulnerability.Severity,
			Package:     m.Artifact.Name,
			Version:     m.Artifact.Version,
			FixedIn:     fixedIn,
			Description: m.Vulnerability.Description,
		})
	}
	return ScanResult{
		ScannerVersion: g.Descriptor.Version,
		Findings:       findings,
		SeverityCounts: countSeverities(findings),
	}, nil
}
