package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeTarGz builds a tar.gz at a temp path from the given entries.
func writeTarGz(t *testing.T, entries []tar.Header, contents map[string][]byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, hdr := range entries {
		h := hdr
		body := contents[h.Name]
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(body))
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestExtractTarGz_RegularFilesAndDirs(t *testing.T) {
	src := writeTarGz(t, []tar.Header{
		{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "dir/file.txt", Typeflag: tar.TypeReg, Mode: 0o644},
		{Name: "manifest.json", Typeflag: tar.TypeReg, Mode: 0o644},
	}, map[string][]byte{
		"dir/file.txt":  []byte("hello"),
		"manifest.json": []byte(`{}`),
	})

	dest := t.TempDir()
	if err := ExtractTarGz(src, dest); err != nil {
		t.Fatalf("extract: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "dir", "file.txt"))
	if err != nil || string(data) != "hello" {
		t.Fatalf("expected extracted file content, got %q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "manifest.json")); err != nil {
		t.Fatalf("manifest.json missing: %v", err)
	}
}

func TestExtractTarGz_RejectsPathTraversal(t *testing.T) {
	for _, name := range []string{"../evil.txt", "/abs.txt", "a/../../evil.txt"} {
		src := writeTarGz(t, []tar.Header{
			{Name: name, Typeflag: tar.TypeReg, Mode: 0o644},
		}, map[string][]byte{name: []byte("x")})
		dest := t.TempDir()
		if err := ExtractTarGz(src, dest); err == nil {
			t.Fatalf("expected traversal rejection for %q", name)
		}
	}
}

func TestExtractTarGz_SkipsSymlinks(t *testing.T) {
	src := writeTarGz(t, []tar.Header{
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		{Name: "ok.txt", Typeflag: tar.TypeReg, Mode: 0o644},
	}, map[string][]byte{"ok.txt": []byte("safe")})

	dest := t.TempDir()
	if err := ExtractTarGz(src, dest); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "link")); !os.IsNotExist(err) {
		t.Fatal("symlink should have been skipped")
	}
	if _, err := os.Stat(filepath.Join(dest, "ok.txt")); err != nil {
		t.Fatalf("regular file should have been extracted: %v", err)
	}
}

func TestExtractTarGz_NotGzip(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "plain-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("just a plain file, e.g. firmware blob"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := ExtractTarGz(f.Name(), t.TempDir()); !errors.Is(err, ErrNotTarGz) {
		t.Fatalf("expected ErrNotTarGz, got %v", err)
	}
}

func TestExtractTarGz_GzipButNotTar(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "gz-*")
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write([]byte("gzipped but not a tarball")); err != nil {
		t.Fatal(err)
	}
	gz.Close()
	f.Close()

	if err := ExtractTarGz(f.Name(), t.TempDir()); !errors.Is(err, ErrNotTarGz) {
		t.Fatalf("expected ErrNotTarGz, got %v", err)
	}
}

func TestExtractToTempDir_CleansUpOnNotTarGz(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "plain-*")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("not an archive")
	f.Close()

	dir, err := ExtractToTempDir(f.Name(), "archive-test-*")
	if !errors.Is(err, ErrNotTarGz) {
		t.Fatalf("expected ErrNotTarGz, got %v (dir=%q)", err, dir)
	}
	if dir != "" {
		t.Fatalf("expected empty dir on error, got %q", dir)
	}
}
