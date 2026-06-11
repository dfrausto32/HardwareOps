// Package archive provides safe extraction of artifact tar.gz bundles so
// scanners (trivy, grype) can inspect bundle contents rather than the opaque
// compressed blob.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotTarGz is returned when the input is not a gzip-compressed tarball.
// Callers fall back to handling the raw blob (e.g. firmware images).
var ErrNotTarGz = errors.New("not a gzip-compressed tar archive")

const (
	// MaxExtractedBytes caps the total decompressed size to defuse
	// decompression bombs. Artifact bundles are configuration and app
	// payloads; 2 GiB is far above any legitimate bundle.
	MaxExtractedBytes = int64(2 << 30)
	// MaxExtractedFiles caps the entry count.
	MaxExtractedFiles = 100_000
)

// ExtractTarGz unpacks the tar.gz at srcPath into destDir.
//
// Only regular files and directories are extracted; symlinks, hardlinks, and
// device nodes are skipped so a hostile bundle cannot link outside destDir.
// Entry paths are confined to destDir (path traversal entries are rejected),
// and the total decompressed size and file count are capped.
func ExtractTarGz(srcPath, destDir string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return ErrNotTarGz
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var totalBytes int64
	var totalFiles int
	sawEntry := false

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if !sawEntry {
				return ErrNotTarGz
			}
			return fmt.Errorf("read tar: %w", err)
		}
		sawEntry = true

		totalFiles++
		if totalFiles > MaxExtractedFiles {
			return fmt.Errorf("archive exceeds %d entries", MaxExtractedFiles)
		}

		target, err := securePath(destDir, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			if totalBytes+hdr.Size > MaxExtractedBytes {
				return fmt.Errorf("archive exceeds %d decompressed bytes", MaxExtractedBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("mkdir parent of %s: %w", hdr.Name, err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return fmt.Errorf("create %s: %w", hdr.Name, err)
			}
			// LimitReader as a second line of defense: the tar header size is
			// what tr enforces, but cap against the remaining global budget too.
			n, err := io.Copy(out, io.LimitReader(tr, MaxExtractedBytes-totalBytes+1))
			out.Close()
			if err != nil {
				return fmt.Errorf("write %s: %w", hdr.Name, err)
			}
			totalBytes += n
			if totalBytes > MaxExtractedBytes {
				return fmt.Errorf("archive exceeds %d decompressed bytes", MaxExtractedBytes)
			}
		default:
			// Symlinks, hardlinks, devices, FIFOs: skipped by design.
			continue
		}
	}
	return nil
}

// securePath joins name under destDir and rejects entries that would escape it.
func securePath(destDir, name string) (string, error) {
	cleaned := filepath.Clean(name)
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry %q escapes extraction directory", name)
	}
	target := filepath.Join(destDir, cleaned)
	// Belt and braces: re-verify containment after the join.
	rel, err := filepath.Rel(destDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry %q escapes extraction directory", name)
	}
	return target, nil
}

// ExtractToTempDir extracts the tar.gz at srcPath into a fresh temp directory
// and returns its path. The caller must os.RemoveAll it. Returns ErrNotTarGz
// (with no leftover directory) when the input is not a tar.gz.
func ExtractToTempDir(srcPath, pattern string) (string, error) {
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	if err := ExtractTarGz(srcPath, dir); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}
