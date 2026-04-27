package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/parcel/control-plane/internal/artifacttrust"
	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

type trustedSigningKeyResponse struct {
	KeyID       string     `json:"keyId"`
	DisplayName string     `json:"displayName"`
	Algorithm   string     `json:"algorithm"`
	PublicKeyPEM string    `json:"publicKeyPem"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"createdAt"`
	RetiredAt   *time.Time `json:"retiredAt,omitempty"`
	Notes       string     `json:"notes,omitempty"`
}

type trustedSigningKeyRequest struct {
	KeyID       string `json:"keyId"`
	DisplayName string `json:"displayName"`
	Algorithm   string `json:"algorithm"`
	PublicKeyPEM string `json:"publicKeyPem"`
	Notes       string `json:"notes"`
}

type updateTrustedSigningKeyRequest struct {
	DisplayName string `json:"displayName"`
	Notes       string `json:"notes"`
}

type provenancePolicyRequest struct {
	RequireProvenance     bool   `json:"requireProvenance"`
	RequiredPredicateType string `json:"requiredPredicateType,omitempty"`
	RequiredBuilderID     string `json:"requiredBuilderId,omitempty"`
	RequiredBuilderIssuer string `json:"requiredBuilderIssuer,omitempty"`
}

type artifactTrustPolicyRequest struct {
	VerificationMode      string                  `json:"verificationMode"`
	AllowedSigningKeyIDs  []string                `json:"allowedSigningKeyIds,omitempty"`
	AllowedSignatureTypes []string                `json:"allowedSignatureTypes,omitempty"`
	Provenance            *provenancePolicyRequest `json:"provenance,omitempty"`
}

type artifactTrustPolicyResponse struct {
	VerificationMode      string                  `json:"verificationMode"`
	AllowedSigningKeyIDs  []string                `json:"allowedSigningKeyIds,omitempty"`
	AllowedSignatureTypes []string                `json:"allowedSignatureTypes,omitempty"`
	Provenance            *provenancePolicyRequest `json:"provenance,omitempty"`
	UpdatedAt             time.Time               `json:"updatedAt"`
	UpdatedByUserID       string                  `json:"updatedByUserId,omitempty"`
}

func ListTrustedSigningKeys(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		includeRetired := strings.TrimSpace(r.URL.Query().Get("includeRetired")) == "1"
		keys, err := st.ListTrustedSigningKeys(includeRetired)
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.keys.list", "artifact_trust", ""), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		resp := make([]trustedSigningKeyResponse, 0, len(keys))
		for _, key := range keys {
			resp = append(resp, trustedSigningKeyToResponse(key))
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.keys.list", "artifact_trust", "")
		event.MetadataJSON = auditJSON(map[string]any{"count": len(resp), "includeRetired": includeRetired})
		writeAudit(logger, st, event, nil)
		writeJSON(w, map[string]any{"items": resp})
	}
}

func PutTrustedSigningKey(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req trustedSigningKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.DisplayName = strings.TrimSpace(req.DisplayName)
		req.PublicKeyPEM = strings.TrimSpace(req.PublicKeyPEM)
		req.Notes = strings.TrimSpace(req.Notes)
		if req.DisplayName == "" || req.PublicKeyPEM == "" || req.Algorithm == "" {
			http.Error(w, "displayName, algorithm, publicKeyPem required", http.StatusBadRequest)
			return
		}
		algo, err := artifacttrust.NormalizeKeyAlgorithm(req.Algorithm)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := artifacttrust.ValidateTrustedKey(algo, req.PublicKeyPEM); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		keyID := strings.TrimSpace(req.KeyID)
		computedKeyID, err := artifacttrust.ComputeKeyIDFromPublicKeyPEM(req.PublicKeyPEM)
		if err != nil {
			http.Error(w, "invalid public key pem", http.StatusBadRequest)
			return
		}
		if keyID == "" {
			keyID = computedKeyID
		} else if !strings.EqualFold(keyID, computedKeyID) {
			http.Error(w, "keyId does not match public key", http.StatusBadRequest)
			return
		}
		key, err := st.UpsertTrustedSigningKey(store.TrustedSigningKey{
			KeyID:        keyID,
			DisplayName:  req.DisplayName,
			Algorithm:    algo,
			PublicKeyPEM: req.PublicKeyPEM,
			State:        artifacttrust.KeyStateActive,
			CreatedAt:    time.Now().UTC(),
			Notes:        req.Notes,
		})
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.key.upsert", "artifact_trust_key", keyID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.key.upsert", "artifact_trust_key", key.KeyID)
		event.AfterJSON = auditJSON(trustedSigningKeyToResponse(key))
		writeAudit(logger, st, event, nil)
		writeJSON(w, trustedSigningKeyToResponse(key))
	}
}

func PatchTrustedSigningKey(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keyID := strings.TrimSpace(chi.URLParam(r, "keyId"))
		if keyID == "" {
			http.Error(w, "keyId required", http.StatusBadRequest)
			return
		}
		var req updateTrustedSigningKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		key, ok, err := st.GetTrustedSigningKey(keyID)
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.key.update", "artifact_trust_key", keyID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "trusted signing key not found", http.StatusNotFound)
			return
		}
		key.DisplayName = strings.TrimSpace(req.DisplayName)
		key.Notes = strings.TrimSpace(req.Notes)
		updated, err := st.UpsertTrustedSigningKey(key)
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.key.update", "artifact_trust_key", keyID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.key.update", "artifact_trust_key", keyID)
		event.AfterJSON = auditJSON(trustedSigningKeyToResponse(updated))
		writeAudit(logger, st, event, nil)
		writeJSON(w, trustedSigningKeyToResponse(updated))
	}
}

func RetireTrustedSigningKey(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keyID := strings.TrimSpace(chi.URLParam(r, "keyId"))
		if keyID == "" {
			http.Error(w, "keyId required", http.StatusBadRequest)
			return
		}
		key, err := st.RetireTrustedSigningKey(keyID, time.Now().UTC())
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.key.retire", "artifact_trust_key", keyID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.key.retire", "artifact_trust_key", keyID)
		event.AfterJSON = auditJSON(trustedSigningKeyToResponse(key))
		writeAudit(logger, st, event, nil)
		writeJSON(w, trustedSigningKeyToResponse(key))
	}
}

func GetArtifactTrustPolicy(logger *log.Logger, st store.Store, trustProxy bool, sigPolicy ArtifactSignaturePolicy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		policy, err := sigPolicy.EffectiveGlobalPolicy()
		if err != nil {
			http.Error(w, "policy error", http.StatusInternalServerError)
			return
		}
		resp, err := artifactTrustPolicyToResponse(policy)
		if err != nil {
			http.Error(w, "policy error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.policy.read", "artifact_trust", "global")
		writeAudit(logger, st, event, nil)
		writeJSON(w, resp)
	}
}

func PutArtifactTrustPolicy(logger *log.Logger, st store.Store, trustProxy bool, sigPolicy ArtifactSignaturePolicy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req artifactTrustPolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		mode, err := artifacttrust.NormalizeVerificationMode(req.VerificationMode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		keyIDs := artifacttrust.MustEncodeStringArray(req.AllowedSigningKeyIDs)
		types := make([]string, 0, len(req.AllowedSignatureTypes))
		for _, sigType := range req.AllowedSignatureTypes {
			norm, err := artifacttrust.NormalizeSignatureType(sigType)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			types = append(types, norm)
		}
		var provJSON []byte
		if req.Provenance != nil {
			provJSON = artifacttrust.EncodeProvenancePolicy(store.ProvenancePolicy{
				RequireProvenance:     req.Provenance.RequireProvenance,
				RequiredPredicateType: req.Provenance.RequiredPredicateType,
				RequiredBuilderID:     req.Provenance.RequiredBuilderID,
				RequiredBuilderIssuer: req.Provenance.RequiredBuilderIssuer,
			})
		}
		policy, err := artifacttrust.EnforcePolicyFloor(store.ArtifactTrustPolicy{
			VerificationMode:          mode,
			AllowedSigningKeyIDsJSON:  keyIDs,
			AllowedSignatureTypesJSON: artifacttrust.MustEncodeStringArray(types),
			ProvenancePolicyJSON:      provJSON,
			UpdatedByUserID:           currentUserID(r),
		}, sigPolicy.Hardened)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := st.SetArtifactTrustPolicy(policy)
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.policy.update", "artifact_trust", "global"), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		resp, err := artifactTrustPolicyToResponse(saved)
		if err != nil {
			http.Error(w, "policy error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact_trust.policy.update", "artifact_trust", "global")
		event.AfterJSON = auditJSON(resp)
		writeAudit(logger, st, event, nil)
		writeJSON(w, resp)
	}
}

func artifactTrustPolicyToResponse(policy store.ArtifactTrustPolicy) (artifactTrustPolicyResponse, error) {
	keyIDs, err := artifacttrust.DecodeStringArray(policy.AllowedSigningKeyIDsJSON, false, nil)
	if err != nil {
		return artifactTrustPolicyResponse{}, err
	}
	types, err := artifacttrust.DecodeStringArray(policy.AllowedSignatureTypesJSON, false, artifacttrust.NormalizeSignatureType)
	if err != nil {
		return artifactTrustPolicyResponse{}, err
	}
	resp := artifactTrustPolicyResponse{
		VerificationMode:      policy.VerificationMode,
		AllowedSigningKeyIDs:  keyIDs,
		AllowedSignatureTypes: types,
		UpdatedAt:             policy.UpdatedAt,
		UpdatedByUserID:       policy.UpdatedByUserID,
	}
	if len(policy.ProvenancePolicyJSON) > 0 {
		prov, err := artifacttrust.DecodeProvenancePolicy(policy.ProvenancePolicyJSON)
		if err != nil {
			return artifactTrustPolicyResponse{}, err
		}
		resp.Provenance = &provenancePolicyRequest{
			RequireProvenance:     prov.RequireProvenance,
			RequiredPredicateType: prov.RequiredPredicateType,
			RequiredBuilderID:     prov.RequiredBuilderID,
			RequiredBuilderIssuer: prov.RequiredBuilderIssuer,
		}
	}
	return resp, nil
}

func trustedSigningKeyToResponse(key store.TrustedSigningKey) trustedSigningKeyResponse {
	return trustedSigningKeyResponse{
		KeyID:        key.KeyID,
		DisplayName:  key.DisplayName,
		Algorithm:    key.Algorithm,
		PublicKeyPEM: key.PublicKeyPEM,
		State:        key.State,
		CreatedAt:    key.CreatedAt,
		RetiredAt:    timePtr(key.RetiredAt),
		Notes:        key.Notes,
	}
}

func currentUserID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if user, ok := auth.UserFromContext(r.Context()); ok {
		return user.UserID
	}
	return ""
}
