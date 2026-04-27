package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/parcel/control-plane/internal/globalplane"
	gpsync "github.com/parcel/control-plane/internal/globalplane/sync"
)

type groupStore interface {
	CreateGlobalGroup(g globalplane.GlobalGroup) (globalplane.GlobalGroup, error)
	GetGlobalGroup(groupID string) (globalplane.GlobalGroup, bool, error)
	ListGlobalGroups() ([]globalplane.GlobalGroup, error)
	DeleteGlobalGroup(groupID string) error
	UpsertGlobalDesiredState(state globalplane.GlobalDesiredState) error
	GetGlobalDesiredState(groupID string) (globalplane.GlobalDesiredState, bool, error)
	ListGlobalDesiredStatesWithGroups() ([]globalplane.GlobalDesiredStateWithGroup, error)
	DeleteGlobalDesiredState(groupID string) error
	ListRegionalPlanes() ([]globalplane.RegionalPlane, error)
}

// ListGlobalGroups handles GET /api/v1/groups.
func ListGlobalGroups(st groupStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groups, err := st.ListGlobalGroups()
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if groups == nil {
			groups = []globalplane.GlobalGroup{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(groups)
	}
}

// CreateGlobalGroup handles POST /api/v1/groups.
func CreateGlobalGroup(st groupStore, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name         string          `json:"name"`
			SelectorJSON json.RawMessage `json:"selectorJson"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		sel := []byte("{}")
		if len(req.SelectorJSON) > 0 {
			sel = req.SelectorJSON
		}
		g, err := st.CreateGlobalGroup(globalplane.GlobalGroup{
			Name:         req.Name,
			SelectorJSON: sel,
		})
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[global-plane] create group: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(g)
	}
}

// DeleteGlobalGroup handles DELETE /api/v1/groups/{groupId}.
// Also fans out policy deletion to all enabled regional planes.
func DeleteGlobalGroup(st groupStore, encKey []byte, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		if _, ok, err := st.GetGlobalGroup(groupID); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Fan-out deletion to regional planes before removing from DB.
		go fanOutDeletePolicy(st, encKey, groupID, logger)

		if err := st.DeleteGlobalGroup(groupID); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[global-plane] delete group %s: %v", groupID, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetGlobalDesiredState handles GET /api/v1/groups/{groupId}/desired-state.
func GetGlobalDesiredState(st groupStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		state, ok, err := st.GetGlobalDesiredState(groupID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "no desired state set for this group", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state)
	}
}

// PutGlobalDesiredState handles PUT /api/v1/groups/{groupId}/desired-state.
// Persists the state and fans out to all enabled regional planes.
func PutGlobalDesiredState(st groupStore, encKey []byte, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		group, ok, err := st.GetGlobalGroup(groupID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "group not found", http.StatusNotFound)
			return
		}

		var req struct {
			ArtifactID       string          `json:"artifactId"`
			DesiredVersion   string          `json:"desiredVersion"`
			DesiredConfigRev string          `json:"desiredConfigRev"`
			PolicyJSON       json.RawMessage `json:"policyJson"`
			ComponentsJSON   json.RawMessage `json:"componentsJson"`
			CheckinInterval  int             `json:"checkinInterval"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		state := globalplane.GlobalDesiredState{
			GroupID:          groupID,
			ArtifactID:       req.ArtifactID,
			DesiredVersion:   req.DesiredVersion,
			DesiredConfigRev: req.DesiredConfigRev,
			PolicyJSON:       req.PolicyJSON,
			ComponentsJSON:   req.ComponentsJSON,
			CheckinInterval:  req.CheckinInterval,
		}
		if err := st.UpsertGlobalDesiredState(state); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[global-plane] upsert desired state for group %s: %v", groupID, err)
			return
		}

		// Fan-out to all enabled regional planes asynchronously.
		go fanOutPushPolicy(st, encKey, group, state, logger)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state)
	}
}

// DeleteGlobalDesiredState handles DELETE /api/v1/groups/{groupId}/desired-state.
func DeleteGlobalDesiredState(st groupStore, encKey []byte, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		go fanOutDeletePolicy(st, encKey, groupID, logger)
		if err := st.DeleteGlobalDesiredState(groupID); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[global-plane] delete desired state for group %s: %v", groupID, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ListGlobalDesiredStates handles GET /api/v1/desired-state.
func ListGlobalDesiredStates(st groupStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := st.ListGlobalDesiredStatesWithGroups()
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if rows == nil {
			rows = []globalplane.GlobalDesiredStateWithGroup{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rows)
	}
}

// ── Fan-out helpers ───────────────────────────────────────────────────────────

func fanOutPushPolicy(st groupStore, encKey []byte, group globalplane.GlobalGroup, state globalplane.GlobalDesiredState, logger *log.Logger) {
	planes, err := st.ListRegionalPlanes()
	if err != nil {
		logger.Printf("[global-plane] fan-out policy push: list planes: %v", err)
		return
	}
	payload := gpsync.FederationPolicyPayload{
		GroupID:          group.GroupID,
		GroupName:        group.Name,
		SelectorJSON:     group.SelectorJSON,
		ArtifactID:       state.ArtifactID,
		DesiredVersion:   state.DesiredVersion,
		DesiredConfigRev: state.DesiredConfigRev,
		PolicyJSON:       state.PolicyJSON,
		ComponentsJSON:   state.ComponentsJSON,
		CheckinInterval:  state.CheckinInterval,
	}
	for _, p := range planes {
		if !p.Enabled {
			continue
		}
		go pushPolicyToPlane(p, payload, encKey, logger)
	}
}

func pushPolicyToPlane(plane globalplane.RegionalPlane, payload gpsync.FederationPolicyPayload, encKey []byte, logger *log.Logger) {
	token, err := globalplane.DecryptToken(plane.EncryptedToken, encKey)
	if err != nil {
		logger.Printf("[global-plane] push policy to %s: decrypt token: %v", plane.Name, err)
		return
	}
	client, err := gpsync.NewRegionalClient(plane.BaseURL, token, plane.TLSCAPem)
	if err != nil {
		logger.Printf("[global-plane] push policy to %s: build client: %v", plane.Name, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.PushPolicy(ctx, payload); err != nil {
		logger.Printf("[global-plane] push policy to %s: %v", plane.Name, err)
		return
	}
	logger.Printf("[global-plane] pushed policy group %s to plane %s", payload.GroupID, plane.Name)
}

func fanOutDeletePolicy(st groupStore, encKey []byte, groupID string, logger *log.Logger) {
	planes, err := st.ListRegionalPlanes()
	if err != nil {
		logger.Printf("[global-plane] fan-out policy delete: list planes: %v", err)
		return
	}
	for _, p := range planes {
		if !p.Enabled {
			continue
		}
		go deletePolicyFromPlane(p, groupID, encKey, logger)
	}
}

func deletePolicyFromPlane(plane globalplane.RegionalPlane, groupID string, encKey []byte, logger *log.Logger) {
	token, err := globalplane.DecryptToken(plane.EncryptedToken, encKey)
	if err != nil {
		logger.Printf("[global-plane] delete policy from %s: decrypt token: %v", plane.Name, err)
		return
	}
	client, err := gpsync.NewRegionalClient(plane.BaseURL, token, plane.TLSCAPem)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.DeletePolicy(ctx, groupID); err != nil {
		logger.Printf("[global-plane] delete policy from %s: %v", plane.Name, err)
	}
}
