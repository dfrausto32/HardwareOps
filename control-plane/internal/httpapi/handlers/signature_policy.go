package handlers

import (
	"encoding/json"

	"github.com/parcel/control-plane/internal/artifacttrust"
	"github.com/parcel/control-plane/internal/store"
)

type ArtifactSignaturePolicy struct {
	Store                 store.Store
	VerificationMode      string
	AllowedSigningKeyIDs  []string
	AllowedSignatureTypes []string
	Require               bool
	EnforceIngest         bool
	KeyID                 string
	Hardened              bool
	KeylessOpts           artifacttrust.KeylessVerifyOptions
}

func (p ArtifactSignaturePolicy) EffectiveGlobalPolicy() (store.ArtifactTrustPolicy, error) {
	return artifacttrust.ResolveGlobalPolicy(p.Store, artifacttrust.DefaultPolicyConfig{
		HardenedProfile:         p.Hardened,
		VerificationMode:        p.VerificationMode,
		AllowedSigningKeyIDs:    p.AllowedSigningKeyIDs,
		AllowedSignatureTypes:   p.AllowedSignatureTypes,
		RequireSignatureDefault: p.Require,
		EnforceIngest:           p.EnforceIngest,
		RequiredKeyID:           p.KeyID,
	})
}

func (p ArtifactSignaturePolicy) ResolvePolicy(raw json.RawMessage) (artifacttrust.ResolvedPolicy, error) {
	defaults, err := p.EffectiveGlobalPolicy()
	if err != nil {
		return artifacttrust.ResolvedPolicy{}, err
	}
	return artifacttrust.ResolvePolicy(defaults, raw)
}

func (p ArtifactSignaturePolicy) MergeApplyPolicy(raw json.RawMessage) (json.RawMessage, error) {
	defaults, err := p.EffectiveGlobalPolicy()
	if err != nil {
		return raw, err
	}
	return artifacttrust.MergeApplyPolicy(defaults, raw)
}

func (p ArtifactSignaturePolicy) ValidateDesiredArtifact(artifact store.Artifact, raw json.RawMessage) error {
	return p.ValidateDesiredArtifactWithAttestations(artifact, nil, raw)
}

func (p ArtifactSignaturePolicy) ValidateDesiredArtifactWithAttestations(artifact store.Artifact, attestations []store.AttestationRecord, raw json.RawMessage) error {
	policy, err := p.ResolvePolicy(raw)
	if err != nil {
		return err
	}
	return artifacttrust.ArtifactAllowedByPolicy(artifacttrust.ArtifactPolicyInput{
		Artifact:     artifact,
		Attestations: attestations,
	}, policy)
}
