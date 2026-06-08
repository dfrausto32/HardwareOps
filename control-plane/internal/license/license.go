package license

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

type Payload struct {
	IssuedTo   string     `json:"issuedTo"`
	MaxDevices int        `json:"maxDevices"`
	NotBefore  *time.Time `json:"notBefore,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	// Variant gates medical-profile features. Valid values: "standard" (default), "medical".
	Variant    string     `json:"variant,omitempty"`
}

type File struct {
	Payload   Payload `json:"payload"`
	Signature string  `json:"signature"`
	KeyID     string  `json:"keyId,omitempty"`
}

type Info struct {
	Enabled  bool      `json:"enabled"`
	Valid    bool      `json:"valid"`
	Error    string    `json:"error,omitempty"`
	Payload  Payload   `json:"payload,omitempty"`
	KeyID    string    `json:"keyId,omitempty"`
	LoadedAt time.Time `json:"loadedAt,omitempty"`
	Source   string    `json:"source,omitempty"`
}

type Manager struct {
	mu         sync.RWMutex
	path       string
	publicKey  ed25519.PublicKey
	enforce    bool
	cacheTTL   time.Duration
	lastLoaded time.Time
	lastMtime  time.Time
	lastInfo   Info
	lastErr    error
}

func NewManager(path, publicKeyPEMOrB64, publicKeyPath string, enforce bool, cacheTTL time.Duration) (*Manager, error) {
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Second
	}
	pub, err := loadPublicKey(publicKeyPEMOrB64, publicKeyPath)
	if enforce && err != nil {
		return nil, err
	}
	return &Manager{
		path:      strings.TrimSpace(path),
		publicKey: pub,
		enforce:   enforce,
		cacheTTL:  cacheTTL,
	}, nil
}

func (m *Manager) Enabled() bool {
	return m != nil && m.enforce
}

// IsMedical returns true when the loaded license explicitly grants the medical variant.
// Returns false when license enforcement is disabled (so a non-enforced dev environment
// never silently becomes a medical deployment — DEPLOYMENT_PROFILE is the primary gate).
func (m *Manager) IsMedical() bool {
	if m == nil || !m.enforce {
		return false
	}
	info := m.Snapshot()
	return info.Valid && info.Payload.Variant == "medical"
}

func (m *Manager) Snapshot() Info {
	if m == nil || !m.enforce {
		return Info{Enabled: false}
	}
	if err := m.reloadIfNeeded(); err != nil {
		m.mu.RLock()
		defer m.mu.RUnlock()
		info := m.lastInfo
		info.Enabled = true
		info.Valid = false
		info.Error = err.Error()
		return info
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	info := m.lastInfo
	info.Enabled = true
	return info
}

func (m *Manager) reloadIfNeeded() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	if !m.lastLoaded.IsZero() && now.Sub(m.lastLoaded) < m.cacheTTL {
		if !m.lastMtime.IsZero() && m.path != "" {
			if stat, err := os.Stat(m.path); err == nil && stat.ModTime().Equal(m.lastMtime) {
				return m.lastErr
			}
		} else {
			return m.lastErr
		}
	}
	info, err := m.loadLocked()
	m.lastLoaded = now
	m.lastInfo = info
	m.lastErr = err
	return err
}

func (m *Manager) loadLocked() (Info, error) {
	if strings.TrimSpace(m.path) == "" {
		return Info{Enabled: true, Valid: false}, errors.New("license path not configured")
	}
	stat, err := os.Stat(m.path)
	if err != nil {
		return Info{Enabled: true, Valid: false}, err
	}
	m.lastMtime = stat.ModTime()
	raw, err := os.ReadFile(m.path)
	if err != nil {
		return Info{Enabled: true, Valid: false}, err
	}
	var lf File
	if err := json.Unmarshal(raw, &lf); err != nil {
		return Info{Enabled: true, Valid: false}, err
	}
	if lf.Signature == "" {
		return Info{Enabled: true, Valid: false}, errors.New("license signature missing")
	}
	if m.publicKey == nil {
		return Info{Enabled: true, Valid: false}, errors.New("license public key not configured")
	}
	sig, err := base64.StdEncoding.DecodeString(lf.Signature)
	if err != nil {
		return Info{Enabled: true, Valid: false}, errors.New("license signature invalid (base64)")
	}
	payloadBytes, err := json.Marshal(lf.Payload)
	if err != nil {
		return Info{Enabled: true, Valid: false}, err
	}
	if !ed25519.Verify(m.publicKey, payloadBytes, sig) {
		return Info{Enabled: true, Valid: false}, errors.New("license signature verification failed")
	}
	if lf.Payload.MaxDevices <= 0 {
		return Info{Enabled: true, Valid: false}, errors.New("license maxDevices must be > 0")
	}
	now := time.Now().UTC()
	if lf.Payload.NotBefore != nil && now.Before(*lf.Payload.NotBefore) {
		return Info{Enabled: true, Valid: false}, errors.New("license not yet valid")
	}
	if lf.Payload.ExpiresAt != nil && now.After(*lf.Payload.ExpiresAt) {
		return Info{Enabled: true, Valid: false}, errors.New("license expired")
	}
	return Info{
		Enabled:  true,
		Valid:    true,
		Payload:  lf.Payload,
		KeyID:    lf.KeyID,
		LoadedAt: now,
		Source:   m.path,
	}, nil
}

func loadPublicKey(inline, path string) (ed25519.PublicKey, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return parsePublicKey(string(data))
	}
	inline = strings.TrimSpace(inline)
	if inline == "" {
		return nil, errors.New("license public key not configured")
	}
	return parsePublicKey(inline)
}

func parsePublicKey(raw string) (ed25519.PublicKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("license public key empty")
	}
	if strings.Contains(raw, "BEGIN PUBLIC KEY") {
		block, _ := pem.Decode([]byte(raw))
		if block == nil {
			return nil, errors.New("invalid PEM public key")
		}
		pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		pub, ok := pubAny.(ed25519.PublicKey)
		if !ok {
			return nil, errors.New("public key is not ed25519")
		}
		return pub, nil
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		if b, errRaw := base64.RawStdEncoding.DecodeString(raw); errRaw == nil {
			data = b
		} else {
			return nil, err
		}
	}
	if len(data) != ed25519.PublicKeySize {
		return nil, errors.New("public key has unexpected length")
	}
	return ed25519.PublicKey(data), nil
}
