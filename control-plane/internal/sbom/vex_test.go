package sbom

import (
	"encoding/json"
	"testing"

	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/vulnscan"
)

func TestBuildVexDocument_EmptyFindings(t *testing.T) {
	artifact := store.Artifact{ArtifactID: "a1", Name: "myapp", Version: "1.0.0"}
	data, err := buildVexDocument(artifact, nil, nil)
	if err != nil {
		t.Fatalf("buildVexDocument: %v", err)
	}
	var doc vexDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.BOMFormat != "CycloneDX" {
		t.Errorf("bomFormat = %q", doc.BOMFormat)
	}
	if doc.Metadata.Component.Name != "myapp" {
		t.Errorf("component.name = %q", doc.Metadata.Component.Name)
	}
	if len(doc.Vulnerabilities) != 0 {
		t.Errorf("expected 0 vulnerabilities, got %d", len(doc.Vulnerabilities))
	}
}

func TestBuildVexDocument_FindingsDefaultUnderInvestigation(t *testing.T) {
	artifact := store.Artifact{ArtifactID: "a1", Name: "app", Version: "1.0"}
	findings := []vulnscan.VulnerabilityFinding{
		{ID: "CVE-2024-1001", Severity: "high", Package: "libssl", Version: "1.1.1"},
		{ID: "CVE-2024-1002", Severity: "medium", Package: "zlib", Version: "1.2.11"},
	}
	data, err := buildVexDocument(artifact, findings, nil)
	if err != nil {
		t.Fatalf("buildVexDocument: %v", err)
	}
	var doc vexDocument
	_ = json.Unmarshal(data, &doc)
	if len(doc.Vulnerabilities) != 2 {
		t.Fatalf("expected 2 vulnerabilities, got %d", len(doc.Vulnerabilities))
	}
	for _, v := range doc.Vulnerabilities {
		if v.Analysis.State != "under_investigation" {
			t.Errorf("CVE %s: state = %q, want under_investigation", v.ID, v.Analysis.State)
		}
	}
}

func TestBuildVexDocument_FixedInSetsFixed(t *testing.T) {
	artifact := store.Artifact{ArtifactID: "a1", Name: "app", Version: "1.0"}
	findings := []vulnscan.VulnerabilityFinding{
		{ID: "CVE-2024-2000", Package: "openssl", Version: "3.0.0", FixedIn: "3.0.1"},
	}
	data, _ := buildVexDocument(artifact, findings, nil)
	var doc vexDocument
	_ = json.Unmarshal(data, &doc)
	if len(doc.Vulnerabilities) != 1 {
		t.Fatalf("expected 1, got %d", len(doc.Vulnerabilities))
	}
	if doc.Vulnerabilities[0].Analysis.State != "fixed" {
		t.Errorf("state = %q, want fixed", doc.Vulnerabilities[0].Analysis.State)
	}
}

func TestBuildVexDocument_ManualAssertionOverridesScanner(t *testing.T) {
	artifact := store.Artifact{ArtifactID: "a1", Name: "app", Version: "1.0"}
	findings := []vulnscan.VulnerabilityFinding{
		{ID: "CVE-2024-3000", Package: "curl", Version: "7.80.0"},
	}
	manual := []store.VexAssertion{
		{
			ArtifactID:    "a1",
			CVEID:         "CVE-2024-3000",
			ComponentName: "curl",
			Assertion:     "not_affected",
			Justification: "using safe subset of API",
		},
	}
	data, _ := buildVexDocument(artifact, findings, manual)
	var doc vexDocument
	_ = json.Unmarshal(data, &doc)
	if len(doc.Vulnerabilities) != 1 {
		t.Fatalf("expected 1, got %d", len(doc.Vulnerabilities))
	}
	v := doc.Vulnerabilities[0]
	if v.Analysis.State != "not_affected" {
		t.Errorf("state = %q, want not_affected", v.Analysis.State)
	}
	if v.Analysis.Justification != "using safe subset of API" {
		t.Errorf("justification = %q", v.Analysis.Justification)
	}
}

func TestBuildVexDocument_ManualOnlyAssertion(t *testing.T) {
	artifact := store.Artifact{ArtifactID: "a1", Name: "app", Version: "1.0"}
	// No scanner findings — manual assertion for a CVE the scanner didn't find.
	manual := []store.VexAssertion{
		{ArtifactID: "a1", CVEID: "CVE-2023-9999", ComponentName: "", Assertion: "affected"},
	}
	data, _ := buildVexDocument(artifact, nil, manual)
	var doc vexDocument
	_ = json.Unmarshal(data, &doc)
	if len(doc.Vulnerabilities) != 1 {
		t.Fatalf("expected 1 manual entry, got %d", len(doc.Vulnerabilities))
	}
	if doc.Vulnerabilities[0].Analysis.State != "affected" {
		t.Errorf("state = %q, want affected", doc.Vulnerabilities[0].Analysis.State)
	}
}

func TestNormalizeAssertionState(t *testing.T) {
	cases := map[string]string{
		"affected":            "affected",
		"AFFECTED":            "affected",
		"not_affected":        "not_affected",
		"NOT_AFFECTED":        "not_affected",
		"fixed":               "fixed",
		"under_investigation": "under_investigation",
		"UNKNOWN":             "under_investigation",
		"":                    "under_investigation",
	}
	for input, want := range cases {
		got := normalizeAssertionState(input)
		if got != want {
			t.Errorf("normalizeAssertionState(%q) = %q, want %q", input, got, want)
		}
	}
}
