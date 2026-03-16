package state

import (
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type State struct {
	DeviceID            string                    `json:"deviceId"`
	AgentVersion        string                    `json:"agentVersion"`
	CurrentVersion      string                    `json:"currentVersion"`
	CurrentConfigRev    string                    `json:"currentConfigRev"`
	PreviousVersion     string                    `json:"previousVersion"`
	LastApplyStatus     string                    `json:"lastApplyStatus"`
	LastApplyError      string                    `json:"lastApplyError"`
	LastApplyAt         time.Time                 `json:"lastApplyAt"`
	LastApplyArtifactID string                    `json:"lastApplyArtifactId"`
	LastPreApplyStatus  string                    `json:"lastPreApplyStatus"`
	LastPreApplyError   string                    `json:"lastPreApplyError"`
	LastPreApplyAt      time.Time                 `json:"lastPreApplyAt"`
	SigningTrustUpdatedAt time.Time               `json:"signingTrustUpdatedAt,omitempty"`
	SigningTrustKeys     []SigningTrustKey        `json:"signingTrustKeys,omitempty"`
	Components          map[string]ComponentState `json:"components,omitempty"`
}

type SigningTrustKey struct {
	KeyID        string `json:"keyId"`
	Algorithm    string `json:"algorithm"`
	PublicKeyPEM string `json:"publicKeyPem"`
}

type ComponentState struct {
	CurrentVersion      string    `json:"currentVersion,omitempty"`
	CurrentConfigRev    string    `json:"currentConfigRev,omitempty"`
	PreviousVersion     string    `json:"previousVersion,omitempty"`
	LastApplyStatus     string    `json:"lastApplyStatus,omitempty"`
	LastApplyError      string    `json:"lastApplyError,omitempty"`
	LastApplyAt         time.Time `json:"lastApplyAt,omitempty"`
	LastApplyArtifactID string    `json:"lastApplyArtifactId,omitempty"`
	LastPreApplyStatus  string    `json:"lastPreApplyStatus,omitempty"`
	LastPreApplyError   string    `json:"lastPreApplyError,omitempty"`
	LastPreApplyAt      time.Time `json:"lastPreApplyAt,omitempty"`
	ConsecutiveFailures int       `json:"consecutiveFailures,omitempty"`
}

func Load(path string) (State, error) {
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		var st State
		if err := json.NewDecoder(f).Decode(&st); err != nil {
			return State{}, err
		}
		st.ensureComponents()
		return st, nil
	}

	st := State{DeviceID: uuid.NewString(), AgentVersion: "0.1.0"}
	st.ensureComponents()
	if err := Save(path, st); err != nil {
		return State{}, err
	}
	return st, nil
}

func Save(path string, st State) error {
	st.ensureComponents()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(st)
}

func (st *State) ensureComponents() {
	if st.Components == nil {
		st.Components = map[string]ComponentState{}
	}
	if _, ok := st.Components["agent_bundle"]; !ok {
		st.Components["agent_bundle"] = ComponentState{
			CurrentVersion: st.AgentVersion,
		}
	}
}

func (st *State) EnsureComponents() {
	st.ensureComponents()
}

// LoadSigned loads state from path, verifying the HMAC sidecar at path+".hmac"
// if hmacKey is non-nil. If the sidecar is absent the state is loaded without
// verification (migration path). If the sidecar is present but the HMAC does
// not match, an error is returned.
func LoadSigned(path string, hmacKey []byte) (State, error) {
	if hmacKey == nil {
		return Load(path)
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		st := State{DeviceID: uuid.NewString(), AgentVersion: "0.1.0"}
		st.ensureComponents()
		if err := SaveSigned(path, st, hmacKey); err != nil {
			return State{}, err
		}
		return st, nil
	}
	if err != nil {
		return State{}, err
	}

	sidecarPath := path + ".hmac"
	sidecarData, sidecarErr := os.ReadFile(sidecarPath)
	if sidecarErr == nil {
		// Sidecar exists: verify HMAC.
		want := strings.TrimSpace(string(sidecarData))
		got := hex.EncodeToString(computeHMAC(hmacKey, data))
		if !hmac.Equal([]byte(want), []byte(got)) {
			return State{}, errors.New("state file integrity check failed: HMAC mismatch")
		}
	}
	// If sidecar absent: allow load without verification (migration from unsigned state).

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	st.ensureComponents()
	return st, nil
}

// SaveSigned writes state to path and writes its HMAC-SHA256 to path+".hmac".
func SaveSigned(path string, st State, hmacKey []byte) error {
	if hmacKey == nil {
		return Save(path, st)
	}
	st.ensureComponents()
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	// Append newline to match json.Encoder output format.
	data = append(data, '\n')

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	f.Close()

	mac := hex.EncodeToString(computeHMAC(hmacKey, data))
	sidecarPath := path + ".hmac"
	return os.WriteFile(sidecarPath, []byte(mac+"\n"), 0o600)
}
