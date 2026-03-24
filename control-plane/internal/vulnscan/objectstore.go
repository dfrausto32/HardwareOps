package vulnscan

import (
	"context"
	"fmt"
	"io"
	"os"
)

// ObjectStore is the subset of the MinIO/S3 client the scanners need.
type ObjectStore interface {
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

// downloadToTemp streams an object from the store to a local temp file and
// returns the file path. The caller is responsible for removing the file.
func downloadToTemp(ctx context.Context, store ObjectStore, bucket, key string) (string, error) {
	rc, err := store.GetObject(ctx, bucket, key)
	if err != nil {
		return "", fmt.Errorf("get object %s/%s: %w", bucket, key, err)
	}
	defer rc.Close()

	f, err := os.CreateTemp("", "hwops-vuln-scan-*")
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
