package artifacts

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplySuccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tarPath, sum := createTestBundle(t, root, "1.0.0", "app_bundle")

	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Dir(tarPath))))
	t.Cleanup(server.Close)

	desired := Desired{
		ArtifactID:      "artifact-1",
		SoftwareVersion: "1.0.0",
		DownloadURL:     server.URL + "/" + filepath.Base(tarPath),
	}
	meta := ArtifactMeta{
		ArtifactID: "artifact-1",
		SHA256:     sum,
		Version:    "1.0.0",
		Type:       "app_bundle",
	}

	outcome, err := Apply(root, desired, meta, server.Client(), nil, ApplyOptions{})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if outcome.PreApplyStatus != "skipped" {
		t.Fatalf("expected preapply skipped, got %s", outcome.PreApplyStatus)
	}

	current := filepath.Join(root, "current")
	target, err := os.Readlink(current)
	if err != nil {
		t.Fatalf("readlink current: %v", err)
	}
	if filepath.Base(target) != "1.0.0" {
		t.Fatalf("expected current version 1.0.0, got %s", target)
	}

	appliedFile := filepath.Join(root, "versions", "1.0.0", "files", "app.txt")
	if _, err := os.Stat(appliedFile); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestApplyUnsupportedTypeFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tarPath, sum := createTestBundle(t, root, "1.0.0", "firmware")

	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Dir(tarPath))))
	t.Cleanup(server.Close)

	desired := Desired{
		ArtifactID:      "artifact-2",
		SoftwareVersion: "1.0.0",
		DownloadURL:     server.URL + "/" + filepath.Base(tarPath),
	}
	meta := ArtifactMeta{
		ArtifactID: "artifact-2",
		SHA256:     sum,
		Version:    "1.0.0",
		Type:       "firmware",
	}

	_, err := Apply(root, desired, meta, server.Client(), nil, ApplyOptions{})
	if err == nil {
		t.Fatalf("expected error for unsupported type")
	}
	var notImpl ErrApplyNotImplemented
	if !errors.As(err, &notImpl) {
		t.Fatalf("expected ErrApplyNotImplemented, got %v", err)
	}
}

func TestApplyUnsupportedTypeAllowed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tarPath, sum := createTestBundle(t, root, "2.0.0", "firmware")

	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Dir(tarPath))))
	t.Cleanup(server.Close)

	desired := Desired{
		ArtifactID:      "artifact-3",
		SoftwareVersion: "2.0.0",
		DownloadURL:     server.URL + "/" + filepath.Base(tarPath),
	}
	meta := ArtifactMeta{
		ArtifactID: "artifact-3",
		SHA256:     sum,
		Version:    "2.0.0",
		Type:       "firmware",
	}

	outcome, err := Apply(root, desired, meta, server.Client(), nil, ApplyOptions{AllowUnsupported: true})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if outcome.PreApplyStatus != "skipped" {
		t.Fatalf("expected preapply skipped, got %s", outcome.PreApplyStatus)
	}
	current := filepath.Join(root, "current")
	target, err := os.Readlink(current)
	if err != nil {
		t.Fatalf("readlink current: %v", err)
	}
	if filepath.Base(target) != "2.0.0" {
		t.Fatalf("expected current version 2.0.0, got %s", target)
	}
}

func TestApplySignatureRequiredMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tarPath, sum := createTestBundle(t, root, "1.0.0", "app_bundle")

	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Dir(tarPath))))
	t.Cleanup(server.Close)

	desired := Desired{
		ArtifactID:      "artifact-4",
		SoftwareVersion: "1.0.0",
		DownloadURL:     server.URL + "/" + filepath.Base(tarPath),
	}
	meta := ArtifactMeta{
		ArtifactID: "artifact-4",
		SHA256:     sum,
		Version:    "1.0.0",
		Type:       "app_bundle",
	}

	_, err := Apply(root, desired, meta, server.Client(), nil, ApplyOptions{RequireSignature: true})
	if err == nil || !strings.Contains(err.Error(), "signature required") {
		t.Fatalf("expected signature required error, got %v", err)
	}
}

func TestApplySignatureVerificationSuccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tarPath, sum := createTestBundle(t, root, "1.0.0", "app_bundle")

	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Dir(tarPath))))
	t.Cleanup(server.Close)

	pubPath, keyID, sig := createSignatureFixture(t, sum)

	desired := Desired{
		ArtifactID:      "artifact-5",
		SoftwareVersion: "1.0.0",
		DownloadURL:     server.URL + "/" + filepath.Base(tarPath),
	}
	meta := ArtifactMeta{
		ArtifactID:     "artifact-5",
		SHA256:         sum,
		Version:        "1.0.0",
		Type:           "app_bundle",
		Signature:      sig,
		SignatureKeyID: keyID,
	}

	_, err := Apply(root, desired, meta, server.Client(), nil, ApplyOptions{
		SigningPublicKeyPath: pubPath,
		SigningKeyID:         keyID,
		RequireSignature:     true,
	})
	if err != nil {
		t.Fatalf("expected signature verification success, got %v", err)
	}
}

func TestApplySignatureVerificationFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tarPath, sum := createTestBundle(t, root, "1.0.0", "app_bundle")

	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Dir(tarPath))))
	t.Cleanup(server.Close)

	pubPath, keyID, _ := createSignatureFixture(t, sum)

	desired := Desired{
		ArtifactID:      "artifact-6",
		SoftwareVersion: "1.0.0",
		DownloadURL:     server.URL + "/" + filepath.Base(tarPath),
	}
	meta := ArtifactMeta{
		ArtifactID:     "artifact-6",
		SHA256:         sum,
		Version:        "1.0.0",
		Type:           "app_bundle",
		Signature:      base64.StdEncoding.EncodeToString([]byte("bad-sig")),
		SignatureKeyID: keyID,
	}

	_, err := Apply(root, desired, meta, server.Client(), nil, ApplyOptions{
		SigningPublicKeyPath: pubPath,
		SigningKeyID:         keyID,
		RequireSignature:     true,
	})
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected signature verification failure, got %v", err)
	}
}

func TestRollbackToVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	v1 := filepath.Join(root, "versions", "v1")
	v2 := filepath.Join(root, "versions", "v2")
	if err := os.MkdirAll(v1, 0o755); err != nil {
		t.Fatalf("mkdir v1: %v", err)
	}
	if err := os.MkdirAll(v2, 0o755); err != nil {
		t.Fatalf("mkdir v2: %v", err)
	}

	current := filepath.Join(root, "current")
	if err := os.Symlink(v2, current); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	if err := RollbackToVersion(root, "v1"); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	target, err := os.Readlink(current)
	if err != nil {
		t.Fatalf("readlink current: %v", err)
	}
	if target != v1 {
		t.Fatalf("expected rollback to %s, got %s", v1, target)
	}
}

func createTestBundle(t *testing.T, root, version, atype string) (string, string) {
	t.Helper()

	bundleDir := filepath.Join(root, "bundle")
	filesDir := filepath.Join(bundleDir, "files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		t.Fatalf("mkdir files: %v", err)
	}

	filePath := filepath.Join(filesDir, "app.txt")
	content := []byte("hello hardwareops")
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	fileSum, err := fileSHA256(filePath)
	if err != nil {
		t.Fatalf("file sha: %v", err)
	}

	manifest := Manifest{
		Name:      "agent",
		Version:   version,
		Type:      atype,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Files: []ManifestFile{
			{
				Path:   "files/app.txt",
				SHA256: fileSum,
				Size:   int64(len(content)),
			},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	tarPath := filepath.Join(root, "bundle.tar.gz")
	if err := writeTarGz(tarPath, bundleDir); err != nil {
		t.Fatalf("write tar: %v", err)
	}
	sum, err := sha256File(tarPath)
	if err != nil {
		t.Fatalf("tar sha: %v", err)
	}
	return tarPath, sum
}

func createSignatureFixture(t *testing.T, shaHex string) (string, string, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal pub key: %v", err)
	}
	pubPem := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	pubPath := filepath.Join(t.TempDir(), "ed25519.pub")
	if err := os.WriteFile(pubPath, pubPem, 0o644); err != nil {
		t.Fatalf("write pub key: %v", err)
	}
	sumBytes, err := hex.DecodeString(shaHex)
	if err != nil {
		t.Fatalf("decode sha: %v", err)
	}
	sig := ed25519.Sign(priv, sumBytes)
	sigB64 := base64.StdEncoding.EncodeToString(sig)
	sum := sha256.Sum256(der)
	keyID := "sha256:" + hex.EncodeToString(sum[:])
	return pubPath, keyID, sigB64
}

func writeTarGz(dest, sourceDir string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, in); err != nil {
				in.Close()
				return err
			}
			in.Close()
		}
		return nil
	})
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
