package bootstrap

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadClear(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bootstrap-state.json")
	want := State{
		Mode:         ModeApproval,
		State:        StatePendingApproval,
		RequestID:    "req-1",
		ClaimToken:   "claim-1",
		HardwareID:   "hw-1",
		KeyPath:      "/tmp/device.key",
		CertPath:     "/tmp/device.crt",
		DeviceIDPath: "/tmp/device-id",
		ExpiresAt:    time.Now().UTC().Add(5 * time.Minute).Round(time.Second),
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, ok, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok {
		t.Fatalf("expected state to exist")
	}
	if got.Version != CurrentStateVersion {
		t.Fatalf("version=%d", got.Version)
	}
	if got.Mode != want.Mode || got.State != want.State || got.RequestID != want.RequestID || got.ClaimToken != want.ClaimToken {
		t.Fatalf("unexpected state: %#v", got)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatalf("expected updatedAt to be set")
	}

	if err := Clear(path); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, ok, err := Load(path); err != nil {
		t.Fatalf("load after clear: %v", err)
	} else if ok {
		t.Fatalf("expected state to be removed")
	}
}
