package bootstrap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	ModeApproval = "approval"

	StateBootstrapInit       = "bootstrap_init"
	StateBootstrapBlocked    = "bootstrap_blocked"
	StateKeypairReady        = "keypair_ready"
	StateRequestSubmit       = "request_submit"
	StatePendingApproval     = "pending_approval"
	StateBootstrapBackoff    = "bootstrap_backoff"
	StateBootstrapDenied     = "bootstrap_denied"
	StateBootstrapConflict   = "bootstrap_conflict"
	StateMaterializeIdentity = "materialize_identity"
	StateActive              = "active"
	CurrentStateVersion      = 1
)

type State struct {
	Version         int       `json:"version"`
	Mode            string    `json:"mode"`
	State           string    `json:"state"`
	RequestID       string    `json:"requestId,omitempty"`
	ClaimToken      string    `json:"claimToken,omitempty"`
	HardwareID      string    `json:"hardwareId,omitempty"`
	KeyPath         string    `json:"keyPath,omitempty"`
	CertPath        string    `json:"certPath,omitempty"`
	DeviceIDPath    string    `json:"deviceIdPath,omitempty"`
	IssuedDeviceID  string    `json:"issuedDeviceId,omitempty"`
	IssuedCertPEM   string    `json:"issuedCertPem,omitempty"`
	IssuedCACertPEM string    `json:"issuedCaCertPem,omitempty"`
	ExpiresAt       time.Time `json:"expiresAt,omitempty"`
	DeniedReason    string    `json:"deniedReason,omitempty"`
	LastError       string    `json:"lastError,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func Load(path string) (State, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, false, nil
		}
		return State{}, false, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, false, err
	}
	if st.Version == 0 {
		st.Version = CurrentStateVersion
	}
	return st, true, nil
}

func Save(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	st.Version = CurrentStateVersion
	st.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func Clear(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
