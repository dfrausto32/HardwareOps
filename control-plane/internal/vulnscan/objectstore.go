package vulnscan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/parcel/control-plane/internal/archive"
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

// prepareScanPath returns the path a CLI scanner should be pointed at for the
// downloaded artifact blob at tmp. Bundle artifacts are tar.gz, which neither
// trivy nor grype unpack — so the bundle is extracted to a temp dir and that
// dir is scanned. Non-tarball blobs (e.g. firmware images) are scanned as-is.
// The returned cleanup must always be called.
func prepareScanPath(tmp string) (string, func(), error) {
	dir, err := archive.ExtractToTempDir(tmp, "hwops-vuln-scan-extract-*")
	if err == nil {
		return dir, func() { os.RemoveAll(dir) }, nil
	}
	if errors.Is(err, archive.ErrNotTarGz) {
		return tmp, func() {}, nil
	}
	return "", func() {}, fmt.Errorf("extract artifact: %w", err)
}
