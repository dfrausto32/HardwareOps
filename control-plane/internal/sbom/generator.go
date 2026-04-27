// Package sbom generates CycloneDX SBOMs for artifacts using the Trivy CLI
// and stores them in object storage alongside the artifact.
package sbom

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/parcel/control-plane/internal/store"
)

// ObjectStore is the subset of the MinIO/S3 client the generator needs.
type ObjectStore interface {
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (int64, error)
}

// ArtifactSBOMJob generates CycloneDX SBOMs for artifacts in the background.
type ArtifactSBOMJob struct {
	binPath     string
	objectStore ObjectStore
	bucket      string
	store       store.Store
	logger      *log.Logger
}

// NewArtifactSBOMJob creates an ArtifactSBOMJob. binPath defaults to "trivy".
func NewArtifactSBOMJob(binPath string, objectStore ObjectStore, bucket string, st store.Store, logger *log.Logger) *ArtifactSBOMJob {
	if binPath == "" {
		binPath = "trivy"
	}
	return &ArtifactSBOMJob{
		binPath:     binPath,
		objectStore: objectStore,
		bucket:      bucket,
		store:       st,
		logger:      logger,
	}
}

// Trigger starts a background SBOM generation for the given artifact.
func (j *ArtifactSBOMJob) Trigger(artifactID, objectKey, sha256 string) {
	go j.run(artifactID, objectKey, sha256)
}

func (j *ArtifactSBOMJob) run(artifactID, objectKey, sha256 string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	tmp, err := j.downloadToTemp(ctx, objectKey)
	if err != nil {
		j.logger.Printf("sbom: download artifact %s: %v", artifactID, err)
		return
	}
	defer os.Remove(tmp)

	sbomData, err := j.generate(ctx, tmp)
	if err != nil {
		j.logger.Printf("sbom: generate for artifact %s: %v", artifactID, err)
		return
	}

	sbomKey := "sboms/" + artifactID + ".cdx.json"
	reader := bytes.NewReader(sbomData)
	if _, err := j.objectStore.PutObject(ctx, j.bucket, sbomKey, reader, int64(len(sbomData)), "application/vnd.cyclonedx+json"); err != nil {
		j.logger.Printf("sbom: upload for artifact %s: %v", artifactID, err)
		return
	}

	if err := j.store.SetArtifactSBOMObjectKey(artifactID, sbomKey); err != nil {
		j.logger.Printf("sbom: save object key for artifact %s: %v", artifactID, err)
		return
	}

	j.logger.Printf("sbom: generated for artifact %s key=%s sha256=%s", artifactID, sbomKey, sha256)
}

func (j *ArtifactSBOMJob) generate(ctx context.Context, path string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, j.binPath, "fs", "--format", "cyclonedx", "--quiet", path).Output()
	if err != nil {
		if len(out) == 0 {
			return nil, fmt.Errorf("trivy: %w", err)
		}
		// trivy may exit non-zero when vulnerabilities are found; output is still valid
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("trivy: empty output")
	}
	return out, nil
}

func (j *ArtifactSBOMJob) downloadToTemp(ctx context.Context, key string) (string, error) {
	rc, err := j.objectStore.GetObject(ctx, j.bucket, key)
	if err != nil {
		return "", fmt.Errorf("get object %s: %w", key, err)
	}
	defer rc.Close()

	f, err := os.CreateTemp("", "hwops-sbom-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err := io.Copy(f, rc); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}
	f.Close()
	return f.Name(), nil
}
