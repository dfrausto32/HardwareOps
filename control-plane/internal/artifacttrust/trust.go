package artifacttrust

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/store"
)

const (
	VerificationModeAllowUnsigned = "allow_unsigned"
	VerificationModeWarnUnsigned  = "warn_unsigned"
	VerificationModeRequire       = "require_verified"

	VerificationStatusUnsigned  = "unsigned"
	VerificationStatusLegacy    = "legacy"
	VerificationStatusVerified  = "verified"
	VerificationStatusFailed    = "failed"
	VerificationStatusUntrusted = "untrusted"

	SignatureTypeEd25519 = "ed25519"
	SignatureTypeCosign  = "cosign"

	KeyStateActive  = "active"
	KeyStateRetired = "retired"
)

type DefaultPolicyConfig struct {
	HardenedProfile         bool
	VerificationMode        string
	AllowedSigningKeyIDs    []string
	AllowedSignatureTypes   []string
	RequireSignatureDefault bool
	EnforceIngest           bool
	RequiredKeyID           string
}

type PolicyOverride struct {
	VerificationMode      string   `json:"verificationMode,omitempty"`
	AllowedSigningKeyIDs  []string `json:"allowedSigningKeyIds,omitempty"`
	AllowedSignatureTypes []string `json:"allowedSignatureTypes,omitempty"`
	RequireSignature      *bool    `json:"requireSignature,omitempty"`
	SigningKeyID          string   `json:"signingKeyId,omitempty"`
}

type ResolvedPolicy struct {
	VerificationMode      string
	AllowedSigningKeyIDs  []string
	AllowedSignatureTypes []string
}

type SigningTrustKey struct {
	KeyID        string `json:"keyId"`
	Algorithm    string `json:"algorithm"`
	PublicKeyPEM string `json:"publicKeyPem"`
}

type SigningTrustBundle struct {
	UpdatedAt time.Time         `json:"updatedAt,omitempty"`
	Keys      []SigningTrustKey `json:"keys"`
}

type VerificationResult struct {
	Status         string
	Error          string
	VerifiedAt     time.Time
	SignatureType  string
	SignatureKeyID string
}

func DeriveDefaultPolicy(cfg DefaultPolicyConfig) store.ArtifactTrustPolicy {
	mode := VerificationModeWarnUnsigned
	if strings.TrimSpace(cfg.VerificationMode) != "" {
		mode = strings.TrimSpace(cfg.VerificationMode)
	} else if cfg.HardenedProfile || cfg.RequireSignatureDefault || cfg.EnforceIngest {
		mode = VerificationModeRequire
	}
	policy := store.ArtifactTrustPolicy{
		VerificationMode: mode,
	}
	allowedKeyIDs := normalizeStringSlice(cfg.AllowedSigningKeyIDs)
	allowedTypes := normalizeStringSlice(cfg.AllowedSignatureTypes)
	if len(allowedKeyIDs) == 0 && strings.TrimSpace(cfg.RequiredKeyID) != "" {
		allowedKeyIDs = []string{strings.TrimSpace(cfg.RequiredKeyID)}
	}
	if len(allowedTypes) == 0 && len(allowedKeyIDs) > 0 {
		allowedTypes = []string{SignatureTypeEd25519}
	}
	if len(allowedKeyIDs) > 0 {
		policy.AllowedSigningKeyIDsJSON = MustEncodeStringArray(allowedKeyIDs)
	}
	if len(allowedTypes) > 0 {
		policy.AllowedSignatureTypesJSON = MustEncodeStringArray(allowedTypes)
	}
	return policy
}

func ResolveGlobalPolicy(st store.Store, cfg DefaultPolicyConfig) (store.ArtifactTrustPolicy, error) {
	if st != nil {
		if policy, ok, err := st.GetArtifactTrustPolicy(); err != nil {
			return store.ArtifactTrustPolicy{}, err
		} else if ok {
			return EnforcePolicyFloor(policy, cfg.HardenedProfile)
		}
	}
	return EnforcePolicyFloor(DeriveDefaultPolicy(cfg), cfg.HardenedProfile)
}

func EnforcePolicyFloor(policy store.ArtifactTrustPolicy, hardened bool) (store.ArtifactTrustPolicy, error) {
	mode, err := NormalizeVerificationMode(policy.VerificationMode)
	if err != nil {
		return store.ArtifactTrustPolicy{}, err
	}
	if hardened && policyStrictness(mode) < policyStrictness(VerificationModeRequire) {
		return store.ArtifactTrustPolicy{}, fmt.Errorf("hardened profile requires verificationMode=%s", VerificationModeRequire)
	}
	policy.VerificationMode = mode
	if normalized, err := normalizeEncodedStringArray(policy.AllowedSigningKeyIDsJSON, false, nil); err != nil {
		return store.ArtifactTrustPolicy{}, err
	} else {
		policy.AllowedSigningKeyIDsJSON = normalized
	}
	if normalized, err := normalizeEncodedStringArray(policy.AllowedSignatureTypesJSON, false, NormalizeSignatureType); err != nil {
		return store.ArtifactTrustPolicy{}, err
	} else {
		policy.AllowedSignatureTypesJSON = normalized
	}
	return policy, nil
}

func ResolvePolicy(defaults store.ArtifactTrustPolicy, raw json.RawMessage) (ResolvedPolicy, error) {
	baseMode, err := NormalizeVerificationMode(defaults.VerificationMode)
	if err != nil {
		return ResolvedPolicy{}, err
	}
	baseKeyIDs, err := DecodeStringArray(defaults.AllowedSigningKeyIDsJSON, false, nil)
	if err != nil {
		return ResolvedPolicy{}, err
	}
	baseTypes, err := DecodeStringArray(defaults.AllowedSignatureTypesJSON, false, NormalizeSignatureType)
	if err != nil {
		return ResolvedPolicy{}, err
	}
	resolved := ResolvedPolicy{
		VerificationMode:      baseMode,
		AllowedSigningKeyIDs:  baseKeyIDs,
		AllowedSignatureTypes: baseTypes,
	}
	if len(raw) == 0 || string(raw) == "null" {
		return resolved, nil
	}
	var override PolicyOverride
	if err := json.Unmarshal(raw, &override); err != nil {
		return ResolvedPolicy{}, fmt.Errorf("invalid apply policy")
	}
	if strings.TrimSpace(override.VerificationMode) != "" {
		mode, err := NormalizeVerificationMode(override.VerificationMode)
		if err != nil {
			return ResolvedPolicy{}, err
		}
		if policyStrictness(mode) < policyStrictness(baseMode) {
			return ResolvedPolicy{}, fmt.Errorf("verificationMode cannot be weaker than global default")
		}
		resolved.VerificationMode = mode
	} else if override.RequireSignature != nil && *override.RequireSignature {
		if policyStrictness(VerificationModeRequire) < policyStrictness(baseMode) {
			return ResolvedPolicy{}, fmt.Errorf("requireSignature cannot be weaker than global default")
		}
		resolved.VerificationMode = VerificationModeRequire
	}
	if len(override.AllowedSigningKeyIDs) > 0 {
		resolved.AllowedSigningKeyIDs = normalizeStringSlice(override.AllowedSigningKeyIDs)
	} else if keyID := strings.TrimSpace(override.SigningKeyID); keyID != "" {
		resolved.AllowedSigningKeyIDs = []string{keyID}
	}
	if len(override.AllowedSignatureTypes) > 0 {
		types := make([]string, 0, len(override.AllowedSignatureTypes))
		for _, sigType := range override.AllowedSignatureTypes {
			norm, err := NormalizeSignatureType(sigType)
			if err != nil {
				return ResolvedPolicy{}, err
			}
			types = append(types, norm)
		}
		resolved.AllowedSignatureTypes = uniqueStrings(types)
	}
	return resolved, nil
}

func MergeApplyPolicy(defaults store.ArtifactTrustPolicy, raw json.RawMessage) (json.RawMessage, error) {
	resolved, err := ResolvePolicy(defaults, raw)
	if err != nil {
		return nil, err
	}
	obj := map[string]any{
		"verificationMode": resolved.VerificationMode,
	}
	if len(resolved.AllowedSigningKeyIDs) > 0 {
		obj["allowedSigningKeyIds"] = resolved.AllowedSigningKeyIDs
	}
	if len(resolved.AllowedSignatureTypes) > 0 {
		obj["allowedSignatureTypes"] = resolved.AllowedSignatureTypes
	}
	if resolved.VerificationMode == VerificationModeRequire {
		obj["requireSignature"] = true
	}
	if len(resolved.AllowedSigningKeyIDs) == 1 {
		obj["signingKeyId"] = resolved.AllowedSigningKeyIDs[0]
	}
	out, _ := json.Marshal(obj)
	return out, nil
}

func ArtifactAllowedByPolicy(artifact store.Artifact, policy ResolvedPolicy) error {
	if policy.VerificationMode == VerificationModeRequire && artifact.VerificationStatus != VerificationStatusVerified {
		return fmt.Errorf("artifact %s must be verified for strict policy", artifact.ArtifactID)
	}
	if len(policy.AllowedSigningKeyIDs) > 0 && artifact.SignatureKeyID != "" && !slices.Contains(policy.AllowedSigningKeyIDs, artifact.SignatureKeyID) {
		return fmt.Errorf("artifact %s signed with disallowed key", artifact.ArtifactID)
	}
	if len(policy.AllowedSignatureTypes) > 0 && artifact.SignatureType != "" && !slices.Contains(policy.AllowedSignatureTypes, artifact.SignatureType) {
		return fmt.Errorf("artifact %s uses disallowed signature type", artifact.ArtifactID)
	}
	return nil
}

func NormalizeVerificationMode(val string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(val)) {
	case "", VerificationModeWarnUnsigned:
		return VerificationModeWarnUnsigned, nil
	case VerificationModeAllowUnsigned:
		return VerificationModeAllowUnsigned, nil
	case VerificationModeRequire:
		return VerificationModeRequire, nil
	default:
		return "", fmt.Errorf("invalid verificationMode: %s", val)
	}
}

func NormalizeSignatureType(val string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(val)) {
	case "", SignatureTypeEd25519:
		return SignatureTypeEd25519, nil
	case SignatureTypeCosign:
		return SignatureTypeCosign, nil
	default:
		return "", fmt.Errorf("invalid signatureType: %s", val)
	}
}

func NormalizeKeyAlgorithm(val string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(val)) {
	case SignatureTypeEd25519:
		return SignatureTypeEd25519, nil
	case SignatureTypeCosign:
		return SignatureTypeCosign, nil
	default:
		return "", fmt.Errorf("invalid algorithm: %s", val)
	}
}

func ComputeKeyIDFromPublicKeyPEM(publicKeyPEM string) (string, error) {
	pub, _, err := ParsePublicKeyPEM(publicKeyPEM)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func ValidateTrustedKey(algorithm, publicKeyPEM string) error {
	algo, err := NormalizeKeyAlgorithm(algorithm)
	if err != nil {
		return err
	}
	pub, _, err := ParsePublicKeyPEM(publicKeyPEM)
	if err != nil {
		return err
	}
	if algo == SignatureTypeEd25519 {
		if _, ok := pub.(ed25519.PublicKey); !ok {
			return errors.New("ed25519 trusted key requires an Ed25519 public key")
		}
	}
	return nil
}

func BuildSigningTrustBundle(keys []store.TrustedSigningKey) SigningTrustBundle {
	bundle := SigningTrustBundle{Keys: make([]SigningTrustKey, 0, len(keys))}
	for _, key := range keys {
		if key.State != KeyStateActive {
			continue
		}
		if key.CreatedAt.After(bundle.UpdatedAt) {
			bundle.UpdatedAt = key.CreatedAt
		}
		bundle.Keys = append(bundle.Keys, SigningTrustKey{
			KeyID:        key.KeyID,
			Algorithm:    key.Algorithm,
			PublicKeyPEM: key.PublicKeyPEM,
		})
	}
	return bundle
}

func VerifyArtifact(signatureType, signature, publicKeyPEM string, shaHex string, blob []byte) error {
	if strings.TrimSpace(signature) == "" {
		return errors.New("artifact signature required but missing")
	}
	sigBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	sigType, err := NormalizeSignatureType(signatureType)
	if err != nil {
		return err
	}
	pub, _, err := ParsePublicKeyPEM(publicKeyPEM)
	if err != nil {
		return err
	}
	switch sigType {
	case SignatureTypeEd25519:
		key, ok := pub.(ed25519.PublicKey)
		if !ok {
			return errors.New("ed25519 signature requires an Ed25519 public key")
		}
		sum, err := hex.DecodeString(shaHex)
		if err != nil {
			return fmt.Errorf("invalid sha256 for signature verification")
		}
		if !ed25519.Verify(key, sum, sigBytes) {
			return errors.New("signature verification failed")
		}
		return nil
	case SignatureTypeCosign:
		if len(blob) == 0 {
			return errors.New("cosign verification requires artifact bytes")
		}
		sum := sha256.Sum256(blob)
		switch key := pub.(type) {
		case *ecdsa.PublicKey:
			if !ecdsa.VerifyASN1(key, sum[:], sigBytes) {
				return errors.New("signature verification failed")
			}
			return nil
		case *rsa.PublicKey:
			if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sigBytes); err == nil {
				return nil
			}
			if err := rsa.VerifyPSS(key, crypto.SHA256, sum[:], sigBytes, nil); err == nil {
				return nil
			}
			return errors.New("signature verification failed")
		case ed25519.PublicKey:
			if !ed25519.Verify(key, blob, sigBytes) {
				return errors.New("signature verification failed")
			}
			return nil
		default:
			return errors.New("unsupported public key type for cosign verification")
		}
	default:
		return fmt.Errorf("invalid signatureType: %s", sigType)
	}
}

func ParsePublicKeyPEM(publicKeyPEM string) (any, []byte, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, nil, errors.New("invalid public key pem")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return pub, block.Bytes, nil
}

func ReadAllAndSHA256(r io.Reader) ([]byte, string, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), nil
}

func DecodeStringArray(raw []byte, emptyNil bool, normalize func(string) (string, error)) ([]string, error) {
	if len(raw) == 0 {
		if emptyNil {
			return nil, nil
		}
		return []string{}, nil
	}
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("invalid string array")
	}
	items = normalizeStringSlice(items)
	if normalize != nil {
		out := make([]string, 0, len(items))
		for _, item := range items {
			norm, err := normalize(item)
			if err != nil {
				return nil, err
			}
			out = append(out, norm)
		}
		items = uniqueStrings(out)
	}
	if len(items) == 0 && emptyNil {
		return nil, nil
	}
	return items, nil
}

func MustEncodeStringArray(items []string) []byte {
	items = uniqueStrings(normalizeStringSlice(items))
	if len(items) == 0 {
		return []byte("[]")
	}
	out, _ := json.Marshal(items)
	return out
}

func VerificationOutcomeForIngest(policy ResolvedPolicy, signature, signatureType, signatureKeyID string, key *store.TrustedSigningKey, verifyErr error) (VerificationResult, error) {
	result := VerificationResult{
		SignatureType:  strings.TrimSpace(signatureType),
		SignatureKeyID: strings.TrimSpace(signatureKeyID),
	}
	if strings.TrimSpace(signature) == "" {
		result.Status = VerificationStatusUnsigned
		if policy.VerificationMode == VerificationModeRequire {
			return result, errors.New("artifact signature required but missing")
		}
		return result, nil
	}
	if key == nil {
		result.Status = VerificationStatusUntrusted
		result.Error = "trusted signing key not found"
		if policy.VerificationMode == VerificationModeRequire {
			return result, errors.New(result.Error)
		}
		return result, nil
	}
	if verifyErr != nil {
		result.Status = VerificationStatusFailed
		result.Error = verifyErr.Error()
		if policy.VerificationMode == VerificationModeRequire {
			return result, verifyErr
		}
		return result, nil
	}
	result.Status = VerificationStatusVerified
	result.VerifiedAt = time.Now().UTC()
	return result, nil
}

func normalizeEncodedStringArray(raw []byte, emptyNil bool, normalize func(string) (string, error)) ([]byte, error) {
	items, err := DecodeStringArray(raw, emptyNil, normalize)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		if emptyNil {
			return nil, nil
		}
		return []byte("[]"), nil
	}
	out, _ := json.Marshal(items)
	return out, nil
}

func normalizeStringSlice(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func uniqueStrings(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func policyStrictness(mode string) int {
	switch mode {
	case VerificationModeAllowUnsigned:
		return 0
	case VerificationModeWarnUnsigned:
		return 1
	case VerificationModeRequire:
		return 2
	default:
		return -1
	}
}
