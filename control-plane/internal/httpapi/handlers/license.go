package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/parcel/control-plane/internal/license"
	"github.com/parcel/control-plane/internal/store"
)

var errLicenseLimitExceeded = errors.New("device limit exceeded")
var errLicenseInvalid = errors.New("license invalid")

type LicenseStatus struct {
	Enabled    bool       `json:"enabled"`
	Valid      bool       `json:"valid"`
	Error      string     `json:"error,omitempty"`
	MaxDevices int        `json:"maxDevices,omitempty"`
	Devices    int        `json:"devices,omitempty"`
	Remaining  int        `json:"remaining,omitempty"`
	IssuedTo   string     `json:"issuedTo,omitempty"`
	NotBefore  *time.Time `json:"notBefore,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	KeyID      string     `json:"keyId,omitempty"`
	Source     string     `json:"source,omitempty"`
	LoadedAt   *time.Time `json:"loadedAt,omitempty"`
	KeyMode    string     `json:"keyMode,omitempty"`
}

func GetLicenseStatus(logger *log.Logger, st store.Store, mgr *license.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := LicenseStatus{Enabled: false, Valid: true}
		if mgr == nil || !mgr.Enabled() {
			resp.KeyMode = "disabled"
			if st != nil {
				if count, err := st.CountDevices(); err == nil {
					resp.Devices = count
				}
			}
			writeJSON(w, resp)
			return
		}
		info := mgr.Snapshot()
		resp.Enabled = info.Enabled
		resp.Valid = info.Valid
		resp.Error = info.Error
		resp.KeyMode = "embedded"
		if info.Source != "" {
			resp.KeyMode = "env"
		}
		resp.IssuedTo = info.Payload.IssuedTo
		resp.MaxDevices = info.Payload.MaxDevices
		resp.NotBefore = info.Payload.NotBefore
		resp.ExpiresAt = info.Payload.ExpiresAt
		resp.KeyID = info.KeyID
		resp.Source = info.Source
		if !info.LoadedAt.IsZero() {
			t := info.LoadedAt
			resp.LoadedAt = &t
		}
		if st != nil {
			if count, err := st.CountDevices(); err == nil {
				resp.Devices = count
				if resp.MaxDevices > 0 {
					resp.Remaining = resp.MaxDevices - count
					if resp.Remaining < 0 {
						resp.Remaining = 0
					}
				}
			} else if logger != nil {
				logger.Printf("license status device count error: %v", err)
			}
		}
		writeJSON(w, resp)
	}
}

func enforceLicense(st store.Store, mgr *license.Manager) error {
	if mgr == nil || !mgr.Enabled() {
		return nil
	}
	info := mgr.Snapshot()
	if !info.Valid {
		return errLicenseInvalid
	}
	if info.Payload.MaxDevices <= 0 {
		return errLicenseInvalid
	}
	count, err := st.CountDevices()
	if err != nil {
		return err
	}
	if count >= info.Payload.MaxDevices {
		return errLicenseLimitExceeded
	}
	return nil
}

func enrollmentMaxDevices(mgr *license.Manager) (int, error) {
	if mgr == nil || !mgr.Enabled() {
		return 0, nil
	}
	info := mgr.Snapshot()
	if !info.Valid {
		return 0, errLicenseInvalid
	}
	if info.Payload.MaxDevices <= 0 {
		return 0, errLicenseInvalid
	}
	return info.Payload.MaxDevices, nil
}
