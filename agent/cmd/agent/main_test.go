package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/parcel/agent/internal/artifacts"
	"github.com/parcel/agent/internal/client"
	"github.com/parcel/agent/internal/config"
	"github.com/parcel/agent/internal/logging"
	"github.com/parcel/agent/internal/state"
)

func TestReenrollDeviceWritesReturnedCACert(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "device.key")
	certPath := filepath.Join(tmpDir, "device.crt")
	caPath := filepath.Join(tmpDir, "ca.crt")
	writeTestPrivateKey(t, keyPath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/devices/reenroll" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deviceId":"device-1","certPem":"CERT-PEM","caCertPem":"CA-PEM"}`))
	}))
	defer server.Close()

	cfg := config.Config{
		DeviceKeyPath:  keyPath,
		DeviceCertPath: certPath,
		CACertPath:     caPath,
	}
	st := state.State{DeviceID: "device-1"}
	logger := logging.New(logging.Error, "agent", st.DeviceID, nil)

	if err := reenrollDevice(client.New(server.URL), cfg, &st, logger); err == nil {
		t.Fatal("expected invalid CA PEM error")
	}

	// Retry with a valid CA PEM to verify the file is updated.
	validCAPEM := testSelfSignedCertPEM(t)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deviceId":"device-1","certPem":"CERT-PEM","caCertPem":` + quoteJSON(validCAPEM) + `}`))
	})

	if err := reenrollDevice(client.New(server.URL), cfg, &st, logger); err != nil {
		t.Fatalf("reenrollDevice error: %v", err)
	}

	gotCert, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read device cert: %v", err)
	}
	if string(gotCert) != "CERT-PEM" {
		t.Fatalf("unexpected device cert contents: %q", string(gotCert))
	}

	gotCA, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("read CA cert: %v", err)
	}
	if strings.TrimSpace(string(gotCA)) != strings.TrimSpace(validCAPEM) {
		t.Fatalf("unexpected CA cert contents: %q", string(gotCA))
	}
}

func writeTestPrivateKey(t *testing.T, path string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(path, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func testSelfSignedCertPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	template := testCertificateTemplate(t)
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func testCertificateTemplate(t *testing.T) *x509.Certificate {
	t.Helper()
	serial := make([]byte, 16)
	if _, err := rand.Read(serial); err != nil {
		t.Fatalf("serial: %v", err)
	}
	n := new(big.Int).SetBytes(serial)
	return &x509.Certificate{
		SerialNumber:          n,
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
	}
}

func quoteJSON(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestShouldImmediateRecheck(t *testing.T) {
	if shouldImmediateRecheck(nil) {
		t.Fatal("expected nil response to be false")
	}
	if shouldImmediateRecheck(&client.CheckinResponse{}) {
		t.Fatal("expected default response to be false")
	}
	if !shouldImmediateRecheck(&client.CheckinResponse{ImmediateRecheckin: true}) {
		t.Fatal("expected immediate recheck response to be true")
	}
}

// ── F1 transport seam ─────────────────────────────────────────────────────────
// A stub (non-HTTP) Transport drives the full apply dispatch — metadata fetch,
// presign, content download, and apply-result reporting — proving the dispatch
// is transport-agnostic (Phase F1 acceptance).

type stubTransport struct {
	artifact     client.ArtifactResponse
	content      []byte
	presigned    string
	applyResults []client.ApplyResultRequest
	downloads    []string
}

func (s *stubTransport) CheckIn(st state.State, capabilities any) (*client.CheckinResponse, error) {
	return &client.CheckinResponse{}, nil
}

func (s *stubTransport) GetArtifact(artifactID string) (*client.ArtifactResponse, error) {
	a := s.artifact
	return &a, nil
}

func (s *stubTransport) PresignArtifact(artifactID string) (*client.PresignResponse, error) {
	return &client.PresignResponse{DownloadURL: s.presigned}, nil
}

func (s *stubTransport) PostApplyResult(deviceID string, req client.ApplyResultRequest) error {
	s.applyResults = append(s.applyResults, req)
	return nil
}

func (s *stubTransport) DownloadArtifact(url string) (io.ReadCloser, error) {
	s.downloads = append(s.downloads, url)
	return io.NopCloser(bytes.NewReader(s.content)), nil
}

func stubArtifactBundle(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	manifest := []byte(`{"name":"stub-app","version":"2.0.0","type":"app_bundle"}`)
	for _, f := range []struct {
		name string
		body []byte
	}{
		{"manifest.json", manifest},
		{"app.txt", []byte("stub payload")},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestApplyDispatchThroughStubTransport(t *testing.T) {
	bundle := stubArtifactBundle(t)
	sum := sha256.Sum256(bundle)

	stub := &stubTransport{
		artifact: client.ArtifactResponse{
			ArtifactID: "stub-artifact-1",
			Name:       "stub-app",
			Version:    "2.0.0",
			Type:       "app_bundle",
			SHA256:     hex.EncodeToString(sum[:]),
			SizeBytes:  int64(len(bundle)),
		},
		content:   bundle,
		presigned: "stub://artifacts/stub-artifact-1",
	}

	st := state.State{DeviceID: "stub-device", AgentVersion: "test"}
	logger := logging.New(logging.Error, "agent", "stub-device", nil)
	desired := map[string]client.DesiredComponent{
		"app_bundle": {
			ArtifactID:      "stub-artifact-1",
			SoftwareVersion: "2.0.0",
		},
	}

	root := t.TempDir()
	err := applyDesiredComponents(root, stub, desired, &st, logger,
		artifacts.ApplyOptions{VerificationMode: "allow_unsigned"}, 0)
	if err != nil {
		t.Fatalf("apply through stub transport: %v", err)
	}

	if len(stub.downloads) != 1 || stub.downloads[0] != "stub://artifacts/stub-artifact-1" {
		t.Fatalf("expected one download via the stub locator, got %v", stub.downloads)
	}
	if len(stub.applyResults) != 1 || stub.applyResults[0].Status != "success" {
		t.Fatalf("expected one success apply result through the stub, got %+v", stub.applyResults)
	}
	if st.Components["app_bundle"].CurrentVersion != "2.0.0" {
		t.Fatalf("component state not updated: %+v", st.Components["app_bundle"])
	}
}
