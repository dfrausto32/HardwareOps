// Package vulnscan provides artifact-level (Grype/Trivy) and device-level
// (Nessus) vulnerability scanning integrations.
package vulnscan

import "context"

// Scanner is implemented by GrypeScanner and TrivyScanner.
type Scanner interface {
	Type() string
	Scan(ctx context.Context, req ScanRequest) (ScanResult, error)
}

// ScanRequest describes the artifact to scan.
type ScanRequest struct {
	ArtifactID string
	ObjectKey  string // MinIO object key
	SHA256     string
}

// ScanResult holds the output of a successful scan.
type ScanResult struct {
	ScannerVersion string
	Findings       []VulnerabilityFinding
	SeverityCounts SeverityCounts
}

// VulnerabilityFinding is one CVE entry from scanner output.
type VulnerabilityFinding struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Package     string `json:"package"`
	Version     string `json:"version"`
	FixedIn     string `json:"fixedIn,omitempty"`
	Description string `json:"description,omitempty"`
}

// SeverityCounts is the rolled-up count per severity level.
type SeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
}

func countSeverities(findings []VulnerabilityFinding) SeverityCounts {
	var c SeverityCounts
	for _, f := range findings {
		switch f.Severity {
		case "critical", "Critical", "CRITICAL":
			c.Critical++
		case "high", "High", "HIGH":
			c.High++
		case "medium", "Medium", "MEDIUM":
			c.Medium++
		case "low", "Low", "LOW":
			c.Low++
		default:
			c.Unknown++
		}
	}
	return c
}
