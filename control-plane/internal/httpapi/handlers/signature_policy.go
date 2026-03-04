package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ArtifactSignaturePolicy struct {
	Require       bool
	EnforceIngest bool
	KeyID         string
}

type applyPolicyEnvelope struct {
	RequireSignature *bool  `json:"requireSignature,omitempty"`
	SigningKeyID     string `json:"signingKeyId,omitempty"`
}

func (p ArtifactSignaturePolicy) ValidateIngest(signature, signatureKeyID string) error {
	if !p.EnforceIngest {
		return nil
	}
	sig := strings.TrimSpace(signature)
	keyID := strings.TrimSpace(signatureKeyID)
	if p.Require && sig == "" {
		return fmt.Errorf("artifact signature required by policy")
	}
	if strings.TrimSpace(p.KeyID) != "" && sig != "" && !strings.EqualFold(strings.TrimSpace(p.KeyID), keyID) {
		return fmt.Errorf("signatureKeyId must match policy key id")
	}
	return nil
}

func (p ArtifactSignaturePolicy) MergeApplyPolicy(raw json.RawMessage) json.RawMessage {
	if !p.Require && strings.TrimSpace(p.KeyID) == "" {
		return raw
	}
	out := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &out); err != nil {
			return raw
		}
	}
	if p.Require {
		if _, ok := out["requireSignature"]; !ok {
			out["requireSignature"] = true
		}
	}
	if keyID := strings.TrimSpace(p.KeyID); keyID != "" {
		if _, ok := out["signingKeyId"]; !ok {
			out["signingKeyId"] = keyID
		}
	}
	if len(out) == 0 {
		return raw
	}
	merged, err := json.Marshal(out)
	if err != nil {
		return raw
	}
	return merged
}
