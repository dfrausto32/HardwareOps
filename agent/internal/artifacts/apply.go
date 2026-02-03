package artifacts

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Desired struct {
	ArtifactID      string
	SoftwareVersion string
	ConfigRev       string
	DownloadURL     string
}

type ArtifactMeta struct {
	ArtifactID string
	SHA256     string
	SizeBytes  int64
	Version    string
}

type Manifest struct {
	Name      string         `json:"name"`
	Version   string         `json:"version"`
	CreatedAt string         `json:"createdAt"`
	Files     []ManifestFile `json:"files"`
}

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func Apply(root string, desired Desired, meta ArtifactMeta, httpClient *http.Client) error {
	downloads := filepath.Join(root, "downloads")
	versions := filepath.Join(root, "versions")
	current := filepath.Join(root, "current")

	if err := os.MkdirAll(downloads, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(versions, 0o755); err != nil {
		return err
	}

	archivePath := filepath.Join(downloads, desired.ArtifactID+".tar.gz")
	if err := downloadAndVerify(httpClient, desired.DownloadURL, archivePath, meta.SHA256); err != nil {
		return err
	}

	versionDir := filepath.Join(versions, desired.SoftwareVersion)
	tmpDir := versionDir + ".tmp-" + fmt.Sprintf("%d", time.Now().UnixNano())
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}

	if err := extractTarGz(archivePath, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}

	if err := verifyManifest(tmpDir, desired.SoftwareVersion); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}

	// Replace target directory
	if err := os.RemoveAll(versionDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}
	if err := os.Rename(tmpDir, versionDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}

	_, err := switchSymlink(current, versionDir)
	if err != nil {
		return err
	}

	return nil
}

func downloadAndVerify(client *http.Client, url, dest, expectedSHA string) error {
	if url == "" {
		return fmt.Errorf("missing download url")
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	mw := io.MultiWriter(f, h)
	if _, err := io.Copy(mw, resp.Body); err != nil {
		return err
	}

	sum := hex.EncodeToString(h.Sum(nil))
	if expectedSHA != "" && !strings.EqualFold(sum, expectedSHA) {
		return fmt.Errorf("sha256 mismatch: expected %s got %s", expectedSHA, sum)
	}
	return nil
}

func extractTarGz(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Name == "" {
			continue
		}
		if strings.Contains(hdr.Name, "..") || strings.HasPrefix(hdr.Name, "/") {
			return fmt.Errorf("invalid path in archive: %s", hdr.Name)
		}
		path := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func verifyManifest(root, expectedVersion string) error {
	manifestPath := filepath.Join(root, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest.json missing: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	if expectedVersion != "" && m.Version != "" && m.Version != expectedVersion {
		return fmt.Errorf("manifest version mismatch: %s != %s", m.Version, expectedVersion)
	}

	for _, f := range m.Files {
		filePath := filepath.Join(root, f.Path)
		info, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("missing file %s", f.Path)
		}
		if f.Size > 0 && info.Size() != f.Size {
			return fmt.Errorf("size mismatch for %s", f.Path)
		}
		if f.SHA256 != "" {
			sum, err := fileSHA256(filePath)
			if err != nil {
				return err
			}
			if !strings.EqualFold(sum, f.SHA256) {
				return fmt.Errorf("sha256 mismatch for %s", f.Path)
			}
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
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

func switchSymlink(linkPath, target string) (string, error) {
	prev := ""
	if info, err := os.Lstat(linkPath); err == nil && (info.Mode()&os.ModeSymlink) != 0 {
		if p, err := os.Readlink(linkPath); err == nil {
			prev = p
		}
	}

	tmpLink := linkPath + ".tmp"
	_ = os.Remove(tmpLink)
	if err := os.Symlink(target, tmpLink); err != nil {
		return prev, err
	}
	if err := os.Rename(tmpLink, linkPath); err != nil {
		return prev, err
	}
	return prev, nil
}
