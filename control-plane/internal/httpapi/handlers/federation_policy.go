package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
)

type federationPolicyStore interface {
	UpsertGlobalPolicyCache(policy store.GlobalPolicyCache) error
	ListGlobalPolicyCaches() ([]store.GlobalPolicyCache, error)
	DeleteGlobalPolicyCache(groupID string) error
}

// federationPolicyPayload is the shape pushed by the global plane.
type federationPolicyPayload struct {
	GroupID          string `json:"groupId"`
	GroupName        string `json:"groupName"`
	SelectorJSON     []byte `json:"selectorJson"`
	ArtifactID       string `json:"artifactId"`
	DesiredVersion   string `json:"desiredVersion"`
	DesiredConfigRev string `json:"desiredConfigRev"`
	PolicyJSON       []byte `json:"policyJson"`
	ComponentsJSON   []byte `json:"componentsJson"`
	CheckinInterval  int    `json:"checkinInterval"`
}

// ReceiveFederatedPolicy handles POST /api/v1/federation/policies.
// Called by the global plane to push a global desired state policy to this regional plane.
func ReceiveFederatedPolicy(st federationPolicyStore, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload federationPolicyPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if payload.GroupID == "" {
			http.Error(w, "groupId required", http.StatusBadRequest)
			return
		}

		policy := store.GlobalPolicyCache{
			CacheID:          uuid.NewString(),
			GroupID:          payload.GroupID,
			GroupName:        payload.GroupName,
			SelectorJSON:     payload.SelectorJSON,
			ArtifactID:       payload.ArtifactID,
			DesiredVersion:   payload.DesiredVersion,
			DesiredConfigRev: payload.DesiredConfigRev,
			PolicyJSON:       payload.PolicyJSON,
			ComponentsJSON:   payload.ComponentsJSON,
			CheckinInterval:  payload.CheckinInterval,
		}
		if err := st.UpsertGlobalPolicyCache(policy); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[federation] upsert policy cache for group %s: %v", payload.GroupID, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"groupId": payload.GroupID})
	}
}

// ListFederatedPolicies handles GET /api/v1/federation/policies.
// Returns the cached global policies on this regional plane (for diagnostics).
func ListFederatedPolicies(st federationPolicyStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		policies, err := st.ListGlobalPolicyCaches()
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if policies == nil {
			policies = []store.GlobalPolicyCache{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(policies)
	}
}

// DeleteFederatedPolicy handles DELETE /api/v1/federation/policies/{groupId}.
// Called by the global plane when a global group or its desired state is deleted.
func DeleteFederatedPolicy(st federationPolicyStore, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		if err := st.DeleteGlobalPolicyCache(groupID); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[federation] delete policy cache for group %s: %v", groupID, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
