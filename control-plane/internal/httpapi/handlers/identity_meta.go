package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

const cloneIPSwitchWindow = 10 * time.Minute

type cloneSuspicion struct {
	Reasons             []string
	PreviousSourceIP    string
	CurrentSourceIP     string
	PreviousCapHash     string
	CurrentCapHash      string
	SuspectedCloneCount int
}

func updateIdentityMeta(meta []byte, sourceIP string, capabilities json.RawMessage, previousSeen, now time.Time) ([]byte, bool, *cloneSuspicion) {
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

	prevSource, _ := identity["lastSourceIP"].(string)
	prevCapHash, _ := identity["capabilityHash"].(string)
	suspectedCount := intFromAny(identity["suspectedCloneCount"])
	currentCapHash := capabilitySignalHash(capabilities)

	reasons := []string{}
	if prevSource != "" && sourceIP != "" && prevSource != sourceIP {
		if !previousSeen.IsZero() && now.Sub(previousSeen) <= cloneIPSwitchWindow {
			reasons = append(reasons, "source_ip_changed_too_fast")
		}
	}
	if prevCapHash != "" && currentCapHash != "" && prevCapHash != currentCapHash {
		reasons = append(reasons, "capabilities_changed")
	}

	changed := false
	if sourceIP != "" && sourceIP != prevSource {
		identity["lastSourceIP"] = sourceIP
		changed = true
	}
	if currentCapHash != "" && currentCapHash != prevCapHash {
		identity["capabilityHash"] = currentCapHash
		changed = true
	}

	var signal *cloneSuspicion
	if len(reasons) > 0 {
		suspectedCount++
		identity["suspectedCloneCount"] = suspectedCount
		identity["lastSuspectedCloneAt"] = now.UTC().Format(time.RFC3339)
		identity["lastSuspectedCloneReasons"] = reasons
		changed = true
		signal = &cloneSuspicion{
			Reasons:             reasons,
			PreviousSourceIP:    prevSource,
			CurrentSourceIP:     sourceIP,
			PreviousCapHash:     prevCapHash,
			CurrentCapHash:      currentCapHash,
			SuspectedCloneCount: suspectedCount,
		}
	}

	if !changed {
		return nil, false, nil
	}
	hwops["identity"] = identity
	obj["hwops"] = hwops
	merged, err := json.Marshal(obj)
	if err != nil {
		return nil, false, nil
	}
	return merged, true, signal
}

func capabilitySignalHash(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return ""
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:])
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}
