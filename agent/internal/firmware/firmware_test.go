package firmware

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/parcel/agent/internal/artifacts"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// writeBundleDir lays out an extracted firmware bundle (firmware.json +
// image) in a temp dir, as the apply pipeline would after extraction.
func writeBundleDir(t *testing.T, spec Spec, image []byte) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "firmware.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	imgRel := spec.Image
	if imgRel == "" {
		imgRel = defaultImagePath
	}
	imgPath := filepath.Join(dir, filepath.FromSlash(imgRel))
	if err := os.MkdirAll(filepath.Dir(imgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imgPath, image, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newABApplier builds an Applier over temp file slots, returning the applier
// and the slot/state paths.
func newABApplier(t *testing.T, mutate func(*Config)) (*Applier, Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		SlotAPath: filepath.Join(dir, "slotA.img"),
		SlotBPath: filepath.Join(dir, "slotB.img"),
		StateFile: filepath.Join(dir, "state.json"),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a, cfg
}

func applyCtx(versionDir, version string) artifacts.ApplyContext {
	return artifacts.ApplyContext{
		Root:       filepath.Dir(versionDir),
		VersionDir: versionDir,
		Desired:    artifacts.Desired{ArtifactID: "fw-1", SoftwareVersion: version},
	}
}

func readState(t *testing.T, cfg Config) flasherState {
	t.Helper()
	raw, err := os.ReadFile(cfg.StateFile)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var st flasherState
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

// ── A/B happy path ────────────────────────────────────────────────────────────

func TestApplyABSlots_FlashVerifyCommit(t *testing.T) {
	image := []byte("firmware-image-v2")
	a, cfg := newABApplier(t, nil)
	dir := writeBundleDir(t, Spec{Checksum: sha(image)}, image)

	if err := a.ApplyFirmware(applyCtx(dir, "2.0.0")); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Active slot was A (default) → image must be in B, boot switched to B.
	slotB, err := os.ReadFile(cfg.SlotBPath)
	if err != nil || !bytes.Equal(slotB, image) {
		t.Fatalf("slot B not flashed: err=%v", err)
	}
	if _, err := os.Stat(cfg.SlotAPath); !os.IsNotExist(err) {
		t.Fatalf("slot A (active) must remain untouched")
	}
	st := readState(t, cfg)
	if st.Active != "B" || st.Slots["B"].Version != "2.0.0" || st.Slots["B"].SHA256 != sha(image) {
		t.Fatalf("unexpected state after commit: %+v", st)
	}
}

func TestApplyABSlots_SecondApplyAlternatesSlot(t *testing.T) {
	a, cfg := newABApplier(t, nil)

	img1 := []byte("v1")
	dir1 := writeBundleDir(t, Spec{Checksum: sha(img1)}, img1)
	if err := a.ApplyFirmware(applyCtx(dir1, "1.0.0")); err != nil {
		t.Fatal(err)
	}
	img2 := []byte("v2")
	dir2 := writeBundleDir(t, Spec{Checksum: sha(img2)}, img2)
	if err := a.ApplyFirmware(applyCtx(dir2, "2.0.0")); err != nil {
		t.Fatal(err)
	}

	st := readState(t, cfg)
	if st.Active != "A" {
		t.Fatalf("expected second apply to land in slot A, state: %+v", st)
	}
	slotA, _ := os.ReadFile(cfg.SlotAPath)
	if !bytes.Equal(slotA, img2) {
		t.Fatalf("slot A content wrong: %q", slotA)
	}
	// Previous firmware still intact in B for bootloader fallback.
	slotB, _ := os.ReadFile(cfg.SlotBPath)
	if !bytes.Equal(slotB, img1) {
		t.Fatalf("slot B (previous) content wrong: %q", slotB)
	}
}

// ── preflight gates ───────────────────────────────────────────────────────────

func TestPreflight_ModelMismatch(t *testing.T) {
	image := []byte("img")
	a, _ := newABApplier(t, func(c *Config) { c.DeviceModel = "pi-4b" })
	dir := writeBundleDir(t, Spec{Checksum: sha(image), DeviceModel: "pi-zero"}, image)

	err := a.ApplyFirmware(applyCtx(dir, "1.0.0"))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("device model mismatch")) {
		t.Fatalf("expected model mismatch error, got %v", err)
	}
}

func TestPreflight_HWRevisionMismatch(t *testing.T) {
	image := []byte("img")
	a, _ := newABApplier(t, func(c *Config) { c.HWRevision = "rev2" })
	dir := writeBundleDir(t, Spec{Checksum: sha(image), HWRevision: "rev1"}, image)

	if err := a.ApplyFirmware(applyCtx(dir, "1.0.0")); err == nil {
		t.Fatal("expected hw revision mismatch")
	}
}

func TestPreflight_CurrentMinVersion(t *testing.T) {
	a, cfg := newABApplier(t, nil)

	// Install 1.2.0 first.
	img1 := []byte("v1.2.0")
	dir1 := writeBundleDir(t, Spec{Checksum: sha(img1)}, img1)
	if err := a.ApplyFirmware(applyCtx(dir1, "1.2.0")); err != nil {
		t.Fatal(err)
	}

	// Firmware requiring >= 2.0.0 must be rejected.
	img2 := []byte("v3")
	dir2 := writeBundleDir(t, Spec{Checksum: sha(img2), CurrentMinVersion: "2.0.0"}, img2)
	err := a.ApplyFirmware(applyCtx(dir2, "3.0.0"))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("below required minimum")) {
		t.Fatalf("expected min-version rejection, got %v", err)
	}
	// State unchanged.
	if st := readState(t, cfg); st.Slots[st.Active].Version != "1.2.0" {
		t.Fatalf("state must be unchanged after rejected preflight: %+v", st)
	}

	// Numeric (not lexicographic) comparison: 1.10.0 >= 1.9.0.
	img3 := []byte("v1.10")
	dir3 := writeBundleDir(t, Spec{Checksum: sha(img3), CurrentMinVersion: "1.2.0"}, img3)
	if err := a.ApplyFirmware(applyCtx(dir3, "1.10.0")); err != nil {
		t.Fatalf("1.2.0 satisfies min 1.2.0: %v", err)
	}
	img4 := []byte("v1.10.1")
	dir4 := writeBundleDir(t, Spec{Checksum: sha(img4), CurrentMinVersion: "1.9.0"}, img4)
	if err := a.ApplyFirmware(applyCtx(dir4, "1.10.1")); err != nil {
		t.Fatalf("1.10.0 must satisfy min 1.9.0 (numeric compare): %v", err)
	}
}

func TestPreflight_HookFailureBlocksApply(t *testing.T) {
	image := []byte("img")
	a, cfg := newABApplier(t, func(c *Config) { c.PreflightCmd = "exit 1" })
	dir := writeBundleDir(t, Spec{Checksum: sha(image)}, image)

	if err := a.ApplyFirmware(applyCtx(dir, "1.0.0")); err == nil {
		t.Fatal("expected preflight hook failure")
	}
	if _, err := os.Stat(cfg.SlotBPath); !os.IsNotExist(err) {
		t.Fatal("no slot may be written after failed preflight")
	}
}

// ── stage/verify failures ─────────────────────────────────────────────────────

func TestStage_ChecksumMismatchBlocksFlash(t *testing.T) {
	image := []byte("real-image")
	a, cfg := newABApplier(t, nil)
	dir := writeBundleDir(t, Spec{Checksum: sha([]byte("different"))}, image)

	err := a.ApplyFirmware(applyCtx(dir, "1.0.0"))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("checksum mismatch")) {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	if _, err := os.Stat(cfg.SlotBPath); !os.IsNotExist(err) {
		t.Fatal("flash must not happen on checksum mismatch")
	}
}

// corruptingFlasher simulates a bad flash: written content does not match on
// read-back (e.g. failing NAND).
type corruptingFlasher struct {
	*FileSlotFlasher
}

func (c *corruptingFlasher) Flash(slot, imagePath string) error {
	if err := c.FileSlotFlasher.Flash(slot, imagePath); err != nil {
		return err
	}
	target, _ := c.FileSlotFlasher.slotPath(slot)
	return os.WriteFile(target, []byte("corrupted-by-hardware"), 0o600)
}

func TestVerify_ReadbackMismatchDoesNotCommit(t *testing.T) {
	image := []byte("good-image")
	dir := t.TempDir()
	cfg := Config{
		SlotAPath: filepath.Join(dir, "slotA.img"),
		SlotBPath: filepath.Join(dir, "slotB.img"),
		StateFile: filepath.Join(dir, "state.json"),
	}
	a := NewWithFlasher(cfg, &corruptingFlasher{&FileSlotFlasher{cfg: cfg}})
	bundle := writeBundleDir(t, Spec{Checksum: sha(image)}, image)

	err := a.ApplyFirmware(applyCtx(bundle, "1.0.0"))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("verify failed")) {
		t.Fatalf("expected verify failure, got %v", err)
	}
	// Boot selection must still be the untouched active slot.
	if _, err := os.Stat(cfg.StateFile); err == nil {
		var st flasherState
		raw, _ := os.ReadFile(cfg.StateFile)
		_ = json.Unmarshal(raw, &st)
		if st.Active == "B" {
			t.Fatal("boot slot must not switch after failed verify")
		}
	}
}

// ── rollback paths ────────────────────────────────────────────────────────────

func TestHealthFailure_RollsBackBootSlot(t *testing.T) {
	image := []byte("img-v2")
	a, cfg := newABApplier(t, func(c *Config) { c.HealthCmd = "exit 1" })

	// Seed slot A as the running firmware.
	if err := os.WriteFile(cfg.SlotAPath, []byte("img-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := writeBundleDir(t, Spec{Checksum: sha(image)}, image)

	err := a.ApplyFirmware(applyCtx(dir, "2.0.0"))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("health check failed")) {
		t.Fatalf("expected health failure, got %v", err)
	}
	if st := readState(t, cfg); st.Active != "A" {
		t.Fatalf("boot slot must be rolled back to A, state: %+v", st)
	}
}

func TestBootSwitchHookFailure_RevertsState(t *testing.T) {
	image := []byte("img")
	a, cfg := newABApplier(t, func(c *Config) { c.BootSwitchCmd = "exit 1" })
	dir := writeBundleDir(t, Spec{Checksum: sha(image)}, image)

	if err := a.ApplyFirmware(applyCtx(dir, "1.0.0")); err == nil {
		t.Fatal("expected commit failure when boot switch hook fails")
	}
	if st := readState(t, cfg); st.Active != "A" {
		t.Fatalf("state must match real boot selection (A), got: %+v", st)
	}
}

func TestSingleImage_RollbackOnHealthFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "firmware.img")
	prev := []byte("running-v1")
	if err := os.WriteFile(target, prev, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		SingleImagePath: target,
		StateFile:       filepath.Join(dir, "state.json"),
		HealthCmd:       "exit 1",
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	image := []byte("new-v2")
	bundle := writeBundleDir(t, Spec{Checksum: sha(image), InstallType: InstallTypeSingleImage}, image)
	if err := a.ApplyFirmware(applyCtx(bundle, "2.0.0")); err == nil {
		t.Fatal("expected health failure")
	}
	restored, _ := os.ReadFile(target)
	if !bytes.Equal(restored, prev) {
		t.Fatalf("single image must be restored to previous content, got %q", restored)
	}
}

func TestSingleImage_HappyPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "firmware.img")
	cfg := Config{
		SingleImagePath: target,
		StateFile:       filepath.Join(dir, "state.json"),
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	image := []byte("single-v1")
	bundle := writeBundleDir(t, Spec{Checksum: sha(image), InstallType: InstallTypeSingleImage}, image)
	if err := a.ApplyFirmware(applyCtx(bundle, "1.0.0")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, image) {
		t.Fatalf("image not written: %q", got)
	}
}

// ── integration: full artifacts.Apply pipeline ───────────────────────────────

type bundleDownloader struct{ content []byte }

func (d bundleDownloader) DownloadArtifact(url string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(d.content)), nil
}

func buildFirmwareBundle(t *testing.T, version string, image []byte) []byte {
	t.Helper()
	spec := Spec{Checksum: sha(image), InstallType: InstallTypeABSlots, RebootRequired: true}
	specRaw, _ := json.Marshal(spec)
	manifest := []byte(`{"name":"demo-fw","version":"` + version + `","type":"firmware"}`)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		body []byte
	}{
		{"manifest.json", manifest},
		{"firmware.json", specRaw},
		{"files/firmware.bin", image},
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

// TestFirmwareApplyThroughArtifactsPipeline drives a firmware artifact
// through the full artifacts.Apply flow (download, extract, manifest, type
// dispatch) into a configured Applier — the F2 acceptance path.
func TestFirmwareApplyThroughArtifactsPipeline(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		SlotAPath: filepath.Join(dir, "slotA.img"),
		SlotBPath: filepath.Join(dir, "slotB.img"),
		StateFile: filepath.Join(dir, "fw-state.json"),
	}
	applier, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	artifacts.SetFirmwareApplier(applier)

	image := []byte("pipeline-firmware-image")
	bundle := buildFirmwareBundle(t, "5.0.0", image)
	bundleSum := sha(bundle)

	root := t.TempDir()
	_, err = artifacts.Apply(root,
		artifacts.Desired{ArtifactID: "fw-pipe-1", SoftwareVersion: "5.0.0", DownloadURL: "stub://fw"},
		artifacts.ArtifactMeta{ArtifactID: "fw-pipe-1", SHA256: bundleSum, Version: "5.0.0", Type: "firmware"},
		bundleDownloader{content: bundle}, nil,
		artifacts.ApplyOptions{VerificationMode: "allow_unsigned"})
	if err != nil {
		t.Fatalf("apply through pipeline: %v", err)
	}

	flashed, err := os.ReadFile(cfg.SlotBPath)
	if err != nil || !bytes.Equal(flashed, image) {
		t.Fatalf("firmware not flashed to inactive slot: err=%v", err)
	}
	raw, _ := os.ReadFile(cfg.StateFile)
	var st flasherState
	_ = json.Unmarshal(raw, &st)
	if st.Active != "B" || st.Slots["B"].Version != "5.0.0" {
		t.Fatalf("boot state wrong after pipeline apply: %+v", st)
	}
}
