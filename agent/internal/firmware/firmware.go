// Package firmware implements the agent-resident firmware apply path
// (Phase F2): preflight → stage → flash → verify → commit, with A/B slots
// and rollback. It satisfies artifacts.FirmwareApplier and is wired in via
// artifacts.SetFirmwareApplier when the agent is configured with slot paths.
//
// The flash target is abstracted behind the Flasher interface; the built-in
// FileSlotFlasher writes whole images to file or block-device paths, which
// covers Linux-class devices that flash themselves (e.g. /dev/mmcblk0p3) as
// well as the simulated flasher used in tests (temp files).
package firmware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/parcel/agent/internal/artifacts"
)

// Spec is the required firmware.json carried in a firmware artifact bundle
// (schema from docs/development/artifact-apply-roadmap.md, "Firmware Apply
// Plan (v2)").
type Spec struct {
	DeviceModel       string `json:"deviceModel"`
	HWRevision        string `json:"hwRevision"`
	CurrentMinVersion string `json:"currentMinVersion"`
	Checksum          string `json:"checksum"`    // sha256 of the firmware image
	InstallType       string `json:"installType"` // "ab_slots" | "single_image"
	RebootRequired    bool   `json:"rebootRequired"`
	// Image is the bundle-relative path of the firmware image.
	// Defaults to files/firmware.bin.
	Image string `json:"image,omitempty"`
}

const (
	InstallTypeABSlots     = "ab_slots"
	InstallTypeSingleImage = "single_image"

	defaultImagePath = "files/firmware.bin"
)

// LoadSpec reads and validates firmware.json from an extracted bundle dir.
func LoadSpec(versionDir string) (Spec, error) {
	raw, err := os.ReadFile(filepath.Join(versionDir, "firmware.json"))
	if err != nil {
		return Spec{}, fmt.Errorf("firmware.json: %w", err)
	}
	var spec Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return Spec{}, fmt.Errorf("firmware.json: %w", err)
	}
	spec.Checksum = strings.ToLower(strings.TrimSpace(spec.Checksum))
	if spec.Checksum == "" {
		return Spec{}, fmt.Errorf("firmware.json: checksum is required")
	}
	switch spec.InstallType {
	case InstallTypeABSlots, InstallTypeSingleImage:
	case "":
		spec.InstallType = InstallTypeABSlots
	default:
		return Spec{}, fmt.Errorf("firmware.json: installType must be %q or %q", InstallTypeABSlots, InstallTypeSingleImage)
	}
	if spec.Image == "" {
		spec.Image = defaultImagePath
	}
	return spec, nil
}

// Config describes the device the applier flashes. Slot paths may be plain
// files or block devices.
type Config struct {
	DeviceModel string
	HWRevision  string

	// A/B install
	SlotAPath string
	SlotBPath string
	// single_image install
	SingleImagePath string

	// StateFile persists the active slot and per-slot version/hash bookkeeping.
	StateFile string
	// StagingDir holds the verified image before flashing.
	StagingDir string

	// PreflightCmd, BootSwitchCmd, and HealthCmd are optional device-specific
	// hooks (run via sh -c). PreflightCmd gates the apply (battery/idle
	// checks); BootSwitchCmd commits the slot switch to the real bootloader
	// (e.g. u-boot env); HealthCmd validates boot health after commit and
	// triggers rollback on failure.
	PreflightCmd  string
	BootSwitchCmd string
	HealthCmd     string
}

// Applier implements artifacts.FirmwareApplier.
type Applier struct {
	cfg     Config
	flasher Flasher
}

// New builds an Applier with the built-in FileSlotFlasher.
func New(cfg Config) (*Applier, error) {
	hasAB := cfg.SlotAPath != "" && cfg.SlotBPath != ""
	if !hasAB && cfg.SingleImagePath == "" {
		return nil, fmt.Errorf("firmware apply not configured: set FIRMWARE_SLOT_A_PATH+FIRMWARE_SLOT_B_PATH or FIRMWARE_SINGLE_IMAGE_PATH")
	}
	if cfg.StateFile == "" {
		return nil, fmt.Errorf("firmware apply: state file path required")
	}
	return &Applier{cfg: cfg, flasher: &FileSlotFlasher{cfg: cfg}}, nil
}

// NewWithFlasher builds an Applier around a custom (vendor) flasher.
func NewWithFlasher(cfg Config, flasher Flasher) *Applier {
	return &Applier{cfg: cfg, flasher: flasher}
}

// ApplyFirmware runs preflight → stage → flash → verify → commit. With A/B
// slots the running image is never touched until the verified flash commits,
// so failures before commit need no rollback; a post-commit health failure
// rolls the boot slot back. single_image installs back up the current image
// and restore it on failure.
func (a *Applier) ApplyFirmware(ctx artifacts.ApplyContext) error {
	spec, err := LoadSpec(ctx.VersionDir)
	if err != nil {
		return err
	}
	imagePath := filepath.Join(ctx.VersionDir, filepath.FromSlash(spec.Image))

	// ── Preflight ───────────────────────────────────────────────────────────
	if err := a.preflight(spec, ctx); err != nil {
		return fmt.Errorf("preflight: %w", err)
	}

	// ── Stage: verify checksum, copy to staging ─────────────────────────────
	staged, err := a.stage(spec, imagePath)
	if err != nil {
		return fmt.Errorf("stage: %w", err)
	}
	defer os.Remove(staged)

	version := ctx.Desired.SoftwareVersion
	if version == "" {
		version = ctx.Meta.Version
	}

	switch spec.InstallType {
	case InstallTypeSingleImage:
		return a.applySingleImage(spec, staged, version, ctx)
	default:
		return a.applyABSlots(spec, staged, version, ctx)
	}
}

func (a *Applier) preflight(spec Spec, ctx artifacts.ApplyContext) error {
	if spec.DeviceModel != "" && a.cfg.DeviceModel != "" && spec.DeviceModel != a.cfg.DeviceModel {
		return fmt.Errorf("device model mismatch: firmware targets %q, device is %q", spec.DeviceModel, a.cfg.DeviceModel)
	}
	if spec.HWRevision != "" && a.cfg.HWRevision != "" && spec.HWRevision != a.cfg.HWRevision {
		return fmt.Errorf("hardware revision mismatch: firmware targets %q, device is %q", spec.HWRevision, a.cfg.HWRevision)
	}
	if spec.CurrentMinVersion != "" {
		current, err := a.flasher.Version()
		if err != nil {
			return fmt.Errorf("read current firmware version: %w", err)
		}
		// A device with no recorded firmware version is treated as flashable
		// (first-time install).
		if current != "" && compareVersions(current, spec.CurrentMinVersion) < 0 {
			return fmt.Errorf("current firmware %q is below required minimum %q", current, spec.CurrentMinVersion)
		}
	}
	if a.cfg.PreflightCmd != "" {
		if err := runHook(a.cfg.PreflightCmd, ctx, "preflight"); err != nil {
			return err
		}
	}
	return nil
}

func (a *Applier) stage(spec Spec, imagePath string) (string, error) {
	sum, err := fileSHA256(imagePath)
	if err != nil {
		return "", err
	}
	if sum != spec.Checksum {
		return "", fmt.Errorf("image checksum mismatch: got %s want %s", sum, spec.Checksum)
	}
	stagingDir := a.cfg.StagingDir
	if stagingDir == "" {
		stagingDir = filepath.Join(filepath.Dir(a.cfg.StateFile), "staging")
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", err
	}
	staged := filepath.Join(stagingDir, "firmware-"+sum[:16]+".img")
	if err := copyFileSync(imagePath, staged); err != nil {
		return "", err
	}
	return staged, nil
}

func (a *Applier) applyABSlots(spec Spec, staged, version string, ctx artifacts.ApplyContext) error {
	active, inactive, err := a.flasher.Slots()
	if err != nil {
		return fmt.Errorf("determine slots: %w", err)
	}
	logf(ctx, "firmware apply: flashing inactive slot %s (active %s remains untouched)", inactive, active)

	// ── Flash inactive slot ─────────────────────────────────────────────────
	if err := a.flasher.Flash(inactive, staged); err != nil {
		return fmt.Errorf("flash slot %s: %w", inactive, err)
	}

	// ── Verify: read back and compare ───────────────────────────────────────
	readback, err := a.flasher.ReadBack(inactive)
	if err != nil {
		return fmt.Errorf("verify readback slot %s: %w", inactive, err)
	}
	if readback != spec.Checksum {
		return fmt.Errorf("verify failed: slot %s readback %s does not match %s (active slot untouched)", inactive, readback, spec.Checksum)
	}
	logf(ctx, "firmware verify ok: slot %s sha256=%s", inactive, readback)

	// ── Commit: switch boot slot ────────────────────────────────────────────
	if err := a.flasher.SetBootSlot(inactive, version, spec.Checksum); err != nil {
		return fmt.Errorf("commit boot slot %s: %w", inactive, err)
	}
	logf(ctx, "firmware commit: boot slot switched %s → %s version=%s", active, inactive, version)

	// ── Post-commit health: roll back the slot switch on failure ───────────
	if a.cfg.HealthCmd != "" {
		if err := runHook(a.cfg.HealthCmd, ctx, "health"); err != nil {
			if rbErr := a.flasher.SetBootSlot(active, "", ""); rbErr != nil {
				return fmt.Errorf("health check failed: %v; ROLLBACK FAILED: %v", err, rbErr)
			}
			logf(ctx, "firmware rollback: boot slot reverted to %s after failed health check", active)
			return fmt.Errorf("health check failed (rolled back to slot %s): %w", active, err)
		}
	}
	if spec.RebootRequired {
		logf(ctx, "firmware applied; reboot required to activate slot %s", inactive)
	}
	return nil
}

func (a *Applier) applySingleImage(spec Spec, staged, version string, ctx artifacts.ApplyContext) error {
	target := a.cfg.SingleImagePath
	if target == "" {
		return fmt.Errorf("single_image install requires FIRMWARE_SINGLE_IMAGE_PATH")
	}
	// Back up the current image so a failed flash/verify can be restored.
	backup := ""
	if _, err := os.Stat(target); err == nil {
		backup = target + ".prev"
		if err := copyFileSync(target, backup); err != nil {
			return fmt.Errorf("backup current image: %w", err)
		}
	}
	restore := func() error {
		if backup == "" {
			return nil
		}
		return copyFileSync(backup, target)
	}

	if err := copyFileSync(staged, target); err != nil {
		if rbErr := restore(); rbErr != nil {
			return fmt.Errorf("flash failed: %v; ROLLBACK FAILED: %v", err, rbErr)
		}
		return fmt.Errorf("flash (rolled back): %w", err)
	}
	readback, err := fileSHA256(target)
	if err != nil || readback != spec.Checksum {
		if rbErr := restore(); rbErr != nil {
			return fmt.Errorf("verify failed (readback=%s err=%v); ROLLBACK FAILED: %v", readback, err, rbErr)
		}
		return fmt.Errorf("verify failed (rolled back): readback %s does not match %s", readback, spec.Checksum)
	}
	if err := a.flasher.SetBootSlot(slotSingle, version, spec.Checksum); err != nil {
		return fmt.Errorf("record installed version: %w", err)
	}
	if a.cfg.HealthCmd != "" {
		if err := runHook(a.cfg.HealthCmd, ctx, "health"); err != nil {
			if rbErr := restore(); rbErr != nil {
				return fmt.Errorf("health check failed: %v; ROLLBACK FAILED: %v", err, rbErr)
			}
			return fmt.Errorf("health check failed (rolled back): %w", err)
		}
	}
	logf(ctx, "firmware single-image apply ok version=%s", version)
	return nil
}

// runHook executes a device-specific hook command with apply context in env.
func runHook(command string, ctx artifacts.ApplyContext, phase string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(),
		"FIRMWARE_PHASE="+phase,
		"FIRMWARE_ARTIFACT_ID="+ctx.Desired.ArtifactID,
		"FIRMWARE_VERSION="+ctx.Desired.SoftwareVersion,
		"FIRMWARE_VERSION_DIR="+ctx.VersionDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s hook failed: %v (%s)", phase, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func logf(ctx artifacts.ApplyContext, format string, args ...any) {
	if ctx.Logger != nil {
		ctx.Logger.Infof(format, args...)
	}
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

// copyFileSync copies src to dst and fsyncs the destination — flashing must
// not report success on data still in the page cache.
func copyFileSync(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// compareVersions compares dotted version strings numerically segment by
// segment ("1.10.0" > "1.9.9"), falling back to string comparison for
// non-numeric segments. Returns -1, 0, or 1.
func compareVersions(a, b string) int {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv string
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		ai, aErr := strconv.Atoi(av)
		bi, bErr := strconv.Atoi(bv)
		switch {
		case aErr == nil && bErr == nil:
			if ai != bi {
				if ai < bi {
					return -1
				}
				return 1
			}
		default:
			if av != bv {
				if av < bv {
					return -1
				}
				return 1
			}
		}
	}
	return 0
}
