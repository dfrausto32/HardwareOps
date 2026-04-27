package handlers

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/parcel/control-plane/internal/store"
)

const (
	deviceIdentityModeDisabled = "disabled"
	deviceIdentityModeAudit    = "audit"
	deviceIdentityModeEnforce  = "enforce"
)

var deviceIdentityIDPattern = regexp.MustCompile(`^[a-z0-9._:-]{8,128}$`)

type DeviceIdentityPolicy struct {
	Mode             string
	RequireOnEnroll  bool
	RequireOnCheckin bool
}

type hardwareIdentity struct {
	ID     string
	Source string
}

type identityViolation struct {
	Reasons          []string
	HardwareID       string
	StoredHardwareID string
	ConflictDeviceID string
}

func (p DeviceIdentityPolicy) normalized() DeviceIdentityPolicy {
	mode := strings.ToLower(strings.TrimSpace(p.Mode))
	switch mode {
	case deviceIdentityModeDisabled, deviceIdentityModeAudit, deviceIdentityModeEnforce:
	default:
		mode = deviceIdentityModeAudit
	}
	return DeviceIdentityPolicy{
		Mode:             mode,
		RequireOnEnroll:  p.RequireOnEnroll,
		RequireOnCheckin: p.RequireOnCheckin,
	}
}

func parseHardwareIdentity(capabilities json.RawMessage) hardwareIdentity {
	trimmed := strings.TrimSpace(string(capabilities))
	if trimmed == "" || trimmed == "null" {
		return hardwareIdentity{}
	}
	var caps map[string]any
	if err := json.Unmarshal(capabilities, &caps); err != nil {
		return hardwareIdentity{}
	}
	if caps == nil {
		return hardwareIdentity{}
	}

	identity := hardwareIdentity{
		ID:     normalizeHardwareID(stringField(caps, "hardwareId")),
		Source: strings.TrimSpace(stringField(caps, "hardwareSource")),
	}
	if identity.ID == "" {
		if hw, ok := caps["hw"].(map[string]any); ok {
			if idv, ok := hw["identity"].(map[string]any); ok {
				identity.ID = normalizeHardwareID(stringField(idv, "id"))
				if identity.Source == "" {
					identity.Source = strings.TrimSpace(stringField(idv, "source"))
				}
			}
		}
	}
	if identity.ID == "" {
		return hardwareIdentity{}
	}
	if identity.Source == "" {
		identity.Source = "unknown"
	}
	return identity
}

func normalizeHardwareID(raw string) string {
	id := strings.ToLower(strings.TrimSpace(raw))
	if !deviceIdentityIDPattern.MatchString(id) {
		return ""
	}
	return id
}

func hardwareIDFromMetadata(meta []byte) string {
	if len(meta) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(meta, &obj); err != nil {
		return ""
	}
	hwops, _ := obj["hwops"].(map[string]any)
	identity, _ := hwops["identity"].(map[string]any)
	return strings.TrimSpace(stringField(identity, "hardwareId"))
}

func upsertHardwareIdentityMeta(meta []byte, hw hardwareIdentity, now time.Time) ([]byte, bool) {
	if hw.ID == "" {
		return nil, false
	}
	var obj map[string]any
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	hwops, _ := obj["hwops"].(map[string]any)
	if hwops == nil {
		hwops = map[string]any{}
	}
	identity, _ := hwops["identity"].(map[string]any)
	if identity == nil {
		identity = map[string]any{}
	}

	changed := false
	if existing := strings.TrimSpace(stringField(identity, "hardwareId")); existing == "" {
		identity["hardwareId"] = hw.ID
		identity["hardwareFirstSeenAt"] = now.UTC().Format(time.RFC3339)
		changed = true
	} else if existing != hw.ID {
		identity["hardwareId"] = hw.ID
		changed = true
	}
	if existing := strings.TrimSpace(stringField(identity, "hardwareSource")); existing != hw.Source {
		identity["hardwareSource"] = hw.Source
		changed = true
	}
	lastSeen := now.UTC().Format(time.RFC3339)
	if existing := strings.TrimSpace(stringField(identity, "hardwareLastSeenAt")); existing != lastSeen {
		identity["hardwareLastSeenAt"] = lastSeen
		changed = true
	}
	if !changed {
		return nil, false
	}
	hwops["identity"] = identity
	obj["hwops"] = hwops
	merged, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return merged, true
}

func checkinIdentityViolation(st store.Store, deviceID string, existingMeta []byte, incoming hardwareIdentity, policy DeviceIdentityPolicy) (*identityViolation, error) {
	policy = policy.normalized()
	if policy.Mode == deviceIdentityModeDisabled {
		return nil, nil
	}

	storedID := hardwareIDFromMetadata(existingMeta)
	reasons := []string{}
	conflictDeviceID := ""

	if policy.Mode == deviceIdentityModeEnforce && policy.RequireOnCheckin && incoming.ID == "" {
		reasons = append(reasons, "hardware_identity_missing")
	}
	if incoming.ID != "" && storedID != "" && incoming.ID != storedID {
		reasons = append(reasons, "hardware_identity_mismatch")
	}
	if incoming.ID != "" {
		existingDevice, ok, err := st.GetDeviceByHardwareID(incoming.ID)
		if err != nil {
			return nil, err
		}
		if ok && existingDevice.DeviceID != "" && existingDevice.DeviceID != deviceID {
			reasons = append(reasons, "hardware_identity_reused")
			conflictDeviceID = existingDevice.DeviceID
		}
	}

	if len(reasons) == 0 {
		return nil, nil
	}
	return &identityViolation{
		Reasons:          reasons,
		HardwareID:       incoming.ID,
		StoredHardwareID: storedID,
		ConflictDeviceID: conflictDeviceID,
	}, nil
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	val, _ := m[key]
	s, _ := val.(string)
	return s
}
