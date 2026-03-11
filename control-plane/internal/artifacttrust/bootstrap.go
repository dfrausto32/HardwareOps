package artifacttrust

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/hardwareops/control-plane/internal/store"
)

type BootstrapConfig struct {
	StaticFile    string
	StaticJSON    string
	AWSSecretID   string
	AWSRegion     string
	DefaultPolicy DefaultPolicyConfig
}

type bootstrapTrustedSigningKey struct {
	KeyID        string `json:"keyId"`
	DisplayName  string `json:"displayName"`
	Algorithm    string `json:"algorithm"`
	PublicKeyPEM string `json:"publicKeyPem"`
	Notes        string `json:"notes"`
}

type bootstrapTrustedSigningKeyEnvelope struct {
	Keys []bootstrapTrustedSigningKey `json:"keys"`
}

type awsSecretsManagerClient interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

var loadAWSConfig = awsconfig.LoadDefaultConfig
var newAWSSecretsManagerClient = func(cfg aws.Config) awsSecretsManagerClient {
	return secretsmanager.NewFromConfig(cfg)
}

func LoadBootstrapTrustedSigningKeys(filePath, inlineJSON string) ([]store.TrustedSigningKey, error) {
	filePath = strings.TrimSpace(filePath)
	inlineJSON = strings.TrimSpace(inlineJSON)
	raw := inlineJSON
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read trusted signing keys file: %w", err)
		}
		raw = strings.TrimSpace(string(data))
	}
	if raw == "" {
		return nil, nil
	}
	return parseBootstrapTrustedSigningKeysRaw(raw)
}

func LoadBootstrapTrustedSigningKeysFromAWSSecretManager(ctx context.Context, secretID, region string) ([]store.TrustedSigningKey, error) {
	secretID = strings.TrimSpace(secretID)
	region = strings.TrimSpace(region)
	if secretID == "" {
		return nil, nil
	}
	loadOpts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(region))
	}
	awsCfg, err := loadAWSConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("aws config load: %w", err)
	}
	client := newAWSSecretsManagerClient(awsCfg)
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretID,
	})
	if err != nil {
		return nil, fmt.Errorf("aws secretsmanager get-secret-value: %w", err)
	}
	raw := strings.TrimSpace(valueOrEmpty(out.SecretString))
	if raw == "" && len(out.SecretBinary) > 0 {
		raw = strings.TrimSpace(string(out.SecretBinary))
	}
	if raw == "" {
		return nil, fmt.Errorf("secret %q has empty SecretString", secretID)
	}
	return parseBootstrapTrustedSigningKeysRaw(raw)
}

func MergeBootstrapTrustedSigningKeys(sets ...[]store.TrustedSigningKey) []store.TrustedSigningKey {
	merged := map[string]store.TrustedSigningKey{}
	for _, set := range sets {
		for _, key := range set {
			if strings.TrimSpace(key.KeyID) == "" {
				continue
			}
			merged[key.KeyID] = key
		}
	}
	if len(merged) == 0 {
		return nil
	}
	out := make([]store.TrustedSigningKey, 0, len(merged))
	for _, key := range merged {
		out = append(out, key)
	}
	return out
}

func SeedBootstrapState(ctx context.Context, st store.Store, cfg BootstrapConfig) (store.ArtifactTrustPolicy, []store.TrustedSigningKey, error) {
	staticKeys, err := LoadBootstrapTrustedSigningKeys(cfg.StaticFile, cfg.StaticJSON)
	if err != nil {
		return store.ArtifactTrustPolicy{}, nil, err
	}
	awsKeys, err := LoadBootstrapTrustedSigningKeysFromAWSSecretManager(ctx, cfg.AWSSecretID, cfg.AWSRegion)
	if err != nil {
		return store.ArtifactTrustPolicy{}, nil, err
	}
	bootstrapKeys := MergeBootstrapTrustedSigningKeys(staticKeys, awsKeys)
	now := time.Now().UTC()
	for i := range bootstrapKeys {
		key := bootstrapKeys[i]
		existing, ok, err := st.GetTrustedSigningKey(key.KeyID)
		if err != nil {
			return store.ArtifactTrustPolicy{}, nil, err
		}
		if ok {
			bootstrapKeys[i] = existing
			continue
		}
		if key.CreatedAt.IsZero() {
			key.CreatedAt = now
		}
		key.State = KeyStateActive
		saved, err := st.UpsertTrustedSigningKey(key)
		if err != nil {
			return store.ArtifactTrustPolicy{}, nil, err
		}
		bootstrapKeys[i] = saved
	}
	allKeys, err := st.ListTrustedSigningKeys(true)
	if err != nil {
		return store.ArtifactTrustPolicy{}, nil, err
	}
	activeStoreKeys, err := st.ListTrustedSigningKeys(false)
	if err != nil {
		return store.ArtifactTrustPolicy{}, nil, err
	}

	policy, ok, err := st.GetArtifactTrustPolicy()
	if err != nil {
		return store.ArtifactTrustPolicy{}, bootstrapKeys, err
	}
	if !ok {
		policy, err = EnforcePolicyFloor(DeriveDefaultPolicy(cfg.DefaultPolicy), cfg.DefaultPolicy.HardenedProfile)
		if err != nil {
			return store.ArtifactTrustPolicy{}, bootstrapKeys, err
		}
		if policy.VerificationMode == VerificationModeRequire && len(activeStoreKeys) == 0 {
			return store.ArtifactTrustPolicy{}, bootstrapKeys, fmt.Errorf("strict artifact trust policy requires at least one active trusted signing key")
		}
		policy, err = st.SetArtifactTrustPolicy(policy)
		if err != nil {
			return store.ArtifactTrustPolicy{}, bootstrapKeys, err
		}
	} else {
		policy, err = EnforcePolicyFloor(policy, cfg.DefaultPolicy.HardenedProfile)
		if err != nil {
			return store.ArtifactTrustPolicy{}, bootstrapKeys, err
		}
	}

	if err := validateAllowedKeyCoverage(st, policy); err != nil {
		return store.ArtifactTrustPolicy{}, bootstrapKeys, err
	}

	return policy, allKeys, nil
}

func parseBootstrapTrustedSigningKeysRaw(raw string) ([]store.TrustedSigningKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var list []bootstrapTrustedSigningKey
	if strings.HasPrefix(raw, "{") {
		var envelope bootstrapTrustedSigningKeyEnvelope
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			return nil, fmt.Errorf("parse trusted signing keys json: %w", err)
		}
		list = envelope.Keys
	} else {
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			return nil, fmt.Errorf("parse trusted signing keys json: %w", err)
		}
	}
	out := make([]store.TrustedSigningKey, 0, len(list))
	for _, item := range list {
		key, err := normalizeBootstrapTrustedSigningKey(item)
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, nil
}

func normalizeBootstrapTrustedSigningKey(item bootstrapTrustedSigningKey) (store.TrustedSigningKey, error) {
	item.DisplayName = strings.TrimSpace(item.DisplayName)
	item.Algorithm = strings.TrimSpace(item.Algorithm)
	item.PublicKeyPEM = strings.TrimSpace(item.PublicKeyPEM)
	item.Notes = strings.TrimSpace(item.Notes)
	if item.Algorithm == "" {
		item.Algorithm = SignatureTypeEd25519
	}
	algorithm, err := NormalizeKeyAlgorithm(item.Algorithm)
	if err != nil {
		return store.TrustedSigningKey{}, err
	}
	if item.PublicKeyPEM == "" {
		return store.TrustedSigningKey{}, fmt.Errorf("trusted signing key publicKeyPem required")
	}
	if err := ValidateTrustedKey(algorithm, item.PublicKeyPEM); err != nil {
		return store.TrustedSigningKey{}, err
	}
	computedKeyID, err := ComputeKeyIDFromPublicKeyPEM(item.PublicKeyPEM)
	if err != nil {
		return store.TrustedSigningKey{}, err
	}
	keyID := strings.TrimSpace(item.KeyID)
	if keyID == "" {
		keyID = computedKeyID
	} else if !strings.EqualFold(keyID, computedKeyID) {
		return store.TrustedSigningKey{}, fmt.Errorf("trusted signing key keyId does not match public key")
	}
	if item.DisplayName == "" {
		item.DisplayName = keyID
	}
	return store.TrustedSigningKey{
		KeyID:        keyID,
		DisplayName:  item.DisplayName,
		Algorithm:    algorithm,
		PublicKeyPEM: item.PublicKeyPEM,
		State:        KeyStateActive,
		Notes:        item.Notes,
	}, nil
}

func validateAllowedKeyCoverage(st store.Store, policy store.ArtifactTrustPolicy) error {
	allowedKeyIDs, err := DecodeStringArray(policy.AllowedSigningKeyIDsJSON, false, nil)
	if err != nil {
		return err
	}
	for _, keyID := range allowedKeyIDs {
		if _, ok, err := st.GetTrustedSigningKey(keyID); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("artifact trust policy references unknown signing key %q", keyID)
		}
	}
	return nil
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
