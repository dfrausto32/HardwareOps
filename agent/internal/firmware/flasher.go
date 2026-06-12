package firmware

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Flasher is the device-specific flashing seam. FileSlotFlasher covers
// devices whose firmware lives at writable file or block-device paths;
// vendor-specific flashers (SPI programmers, DFU, etc.) implement the same
// interface and are wired via NewWithFlasher.
type Flasher interface {
	// Slots returns the active and inactive slot names ("A"/"B").
	Slots() (active, inactive string, err error)
	// Flash writes the image at imagePath to the named slot.
	Flash(slot, imagePath string) error
	// ReadBack returns the sha256 (hex) of the slot's current content,
	// re-read from the flash target — not from any cached copy.
	ReadBack(slot string) (string, error)
	// SetBootSlot commits the boot selection and records version/hash
	// bookkeeping. version/hash may be empty on rollback re-selection.
	SetBootSlot(slot, version, sha256hex string) error
	// Version returns the firmware version the device currently boots,
	// or "" if unknown (never flashed through this agent).
	Version() (string, error)
}

const (
	slotA      = "A"
	slotB      = "B"
	slotSingle = "single"
)

// flasherState is the JSON persisted at Config.StateFile.
type flasherState struct {
	Active string                `json:"active"`
	Slots  map[string]slotRecord `json:"slots"`
}

type slotRecord struct {
	Version string `json:"version,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

// FileSlotFlasher implements Flasher over file/block-device paths with a
// JSON state file for boot-slot selection and version bookkeeping. The
// optional BootSwitchCmd hook propagates the slot switch to the real
// bootloader (e.g. writing u-boot environment variables).
type FileSlotFlasher struct {
	cfg Config
}

func (f *FileSlotFlasher) slotPath(slot string) (string, error) {
	switch slot {
	case slotA:
		return f.cfg.SlotAPath, nil
	case slotB:
		return f.cfg.SlotBPath, nil
	case slotSingle:
		return f.cfg.SingleImagePath, nil
	default:
		return "", fmt.Errorf("unknown slot %q", slot)
	}
}

func (f *FileSlotFlasher) loadState() (flasherState, error) {
	st := flasherState{Active: slotA, Slots: map[string]slotRecord{}}
	raw, err := os.ReadFile(f.cfg.StateFile)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("firmware state file corrupt: %w", err)
	}
	if st.Active == "" {
		st.Active = slotA
	}
	if st.Slots == nil {
		st.Slots = map[string]slotRecord{}
	}
	return st, nil
}

func (f *FileSlotFlasher) saveState(st flasherState) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.cfg.StateFile), 0o755); err != nil {
		return err
	}
	tmp := f.cfg.StateFile + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.cfg.StateFile)
}

func (f *FileSlotFlasher) Slots() (string, string, error) {
	st, err := f.loadState()
	if err != nil {
		return "", "", err
	}
	if st.Active == slotB {
		return slotB, slotA, nil
	}
	return slotA, slotB, nil
}

func (f *FileSlotFlasher) Flash(slot, imagePath string) error {
	target, err := f.slotPath(slot)
	if err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("no path configured for slot %s", slot)
	}
	return copyFileSync(imagePath, target)
}

func (f *FileSlotFlasher) ReadBack(slot string) (string, error) {
	target, err := f.slotPath(slot)
	if err != nil {
		return "", err
	}
	return fileSHA256(target)
}

func (f *FileSlotFlasher) SetBootSlot(slot, version, sha256hex string) error {
	st, err := f.loadState()
	if err != nil {
		return err
	}
	if _, err := f.slotPath(slot); err != nil {
		return err
	}
	prev := st.Active
	st.Active = slot
	if version != "" || sha256hex != "" {
		st.Slots[slot] = slotRecord{Version: version, SHA256: sha256hex}
	}
	if err := f.saveState(st); err != nil {
		return err
	}
	if f.cfg.BootSwitchCmd != "" {
		cmd := exec.Command("sh", "-c", f.cfg.BootSwitchCmd)
		cmd.Env = append(os.Environ(),
			"FIRMWARE_BOOT_SLOT="+slot,
			"FIRMWARE_PREV_SLOT="+prev,
			"FIRMWARE_VERSION="+version,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			// The bootloader switch failed: revert the state file so agent
			// bookkeeping matches the device's actual boot selection.
			st.Active = prev
			_ = f.saveState(st)
			return fmt.Errorf("boot switch hook failed: %v (%s)", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func (f *FileSlotFlasher) Version() (string, error) {
	st, err := f.loadState()
	if err != nil {
		return "", err
	}
	return st.Slots[st.Active].Version, nil
}
