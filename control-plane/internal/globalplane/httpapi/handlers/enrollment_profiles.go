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

type enrollmentStore interface {
	GetRegionalPlane(planeID string) (globalplane.RegionalPlane, bool, error)
	ListRegionalPlanes() ([]globalplane.RegionalPlane, error)
	CreateGlobalEnrollmentProfile(p globalplane.GlobalEnrollmentProfile) (globalplane.GlobalEnrollmentProfile, error)
	GetGlobalEnrollmentProfile(profileID string) (globalplane.GlobalEnrollmentProfile, bool, error)
	ListGlobalEnrollmentProfiles() ([]globalplane.GlobalEnrollmentProfile, error)
	UpdateGlobalEnrollmentProfile(profileID string, u globalplane.GlobalEnrollmentProfileUpdate) (globalplane.GlobalEnrollmentProfile, error)
	DeleteGlobalEnrollmentProfile(profileID string) error
	ListGlobalPendingEnrollments(filter globalplane.GlobalPendingEnrollmentFilter) ([]globalplane.GlobalPendingEnrollment, error)
}

// ListGlobalEnrollmentProfiles handles GET /api/v1/enrollment-profiles.
func ListGlobalEnrollmentProfiles(st enrollmentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles, err := st.ListGlobalEnrollmentProfiles()
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if profiles == nil {
			profiles = []globalplane.GlobalEnrollmentProfile{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(profiles)
	}
}

// CreateGlobalEnrollmentProfile handles POST /api/v1/enrollment-profiles.
func CreateGlobalEnrollmentProfile(st enrollmentStore, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name              string          `json:"name"`
			RequireApproval   *bool           `json:"requireApproval"`
			AllowUntrustedHW  bool            `json:"allowUntrustedHw"`
			ChallengeHint     string          `json:"challengeHint"`
			ApprovalDelaySec  int             `json:"approvalDelaySec"`
			MaxUses           int             `json:"maxUses"`
			CertValidityDays  int             `json:"certValidityDays"`
			DefaultLabelsJSON json.RawMessage `json:"defaultLabels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		requireApproval := true
		if req.RequireApproval != nil {
			requireApproval = *req.RequireApproval
		}
		certDays := req.CertValidityDays
		if certDays == 0 {
			certDays = 365
		}
		labels := []byte("{}")
		if len(req.DefaultLabelsJSON) > 0 {
			labels = req.DefaultLabelsJSON
		}
		p, err := st.CreateGlobalEnrollmentProfile(globalplane.GlobalEnrollmentProfile{
			Name:              req.Name,
			RequireApproval:   requireApproval,
			AllowUntrustedHW:  req.AllowUntrustedHW,
			ChallengeHint:     req.ChallengeHint,
			ApprovalDelaySec:  req.ApprovalDelaySec,
			MaxUses:           req.MaxUses,
			CertValidityDays:  certDays,
			DefaultLabelsJSON: labels,
		})
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[global-plane] create enrollment profile: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)
	}
}

// UpdateGlobalEnrollmentProfile handles PATCH /api/v1/enrollment-profiles/{profileId}.
func UpdateGlobalEnrollmentProfile(st enrollmentStore, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profileID := chi.URLParam(r, "profileId")
		if _, ok, err := st.GetGlobalEnrollmentProfile(profileID); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req struct {
			Name              string          `json:"name"`
			RequireApproval   bool            `json:"requireApproval"`
			AllowUntrustedHW  bool            `json:"allowUntrustedHw"`
			ChallengeHint     string          `json:"challengeHint"`
			ApprovalDelaySec  int             `json:"approvalDelaySec"`
			MaxUses           int             `json:"maxUses"`
			CertValidityDays  int             `json:"certValidityDays"`
			DefaultLabelsJSON json.RawMessage `json:"defaultLabels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		labels := []byte("{}")
		if len(req.DefaultLabelsJSON) > 0 {
			labels = req.DefaultLabelsJSON
		}
		p, err := st.UpdateGlobalEnrollmentProfile(profileID, globalplane.GlobalEnrollmentProfileUpdate{
			Name:              req.Name,
			RequireApproval:   req.RequireApproval,
			AllowUntrustedHW:  req.AllowUntrustedHW,
			ChallengeHint:     req.ChallengeHint,
			ApprovalDelaySec:  req.ApprovalDelaySec,
			MaxUses:           req.MaxUses,
			CertValidityDays:  req.CertValidityDays,
			DefaultLabelsJSON: labels,
		})
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[global-plane] update enrollment profile %s: %v", profileID, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p)
	}
}

// DeleteGlobalEnrollmentProfile handles DELETE /api/v1/enrollment-profiles/{profileId}.
func DeleteGlobalEnrollmentProfile(st enrollmentStore, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profileID := chi.URLParam(r, "profileId")
		if _, ok, err := st.GetGlobalEnrollmentProfile(profileID); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := st.DeleteGlobalEnrollmentProfile(profileID); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[global-plane] delete enrollment profile %s: %v", profileID, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ListGlobalPendingEnrollments handles GET /api/v1/pending-enrollments.
// Returns the cached pending enrollments from all regional planes.
func ListGlobalPendingEnrollments(st enrollmentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := globalplane.GlobalPendingEnrollmentFilter{
			Status:  r.URL.Query().Get("status"),
			PlaneID: r.URL.Query().Get("planeId"),
		}
		items, err := st.ListGlobalPendingEnrollments(filter)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []globalplane.GlobalPendingEnrollment{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	}
}

// ApproveGlobalPendingEnrollment handles POST /api/v1/pending-enrollments/{requestId}/approve.
// Looks up which regional plane owns the request and proxies the approve call.
func ApproveGlobalPendingEnrollment(st enrollmentStore, encKey []byte, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := chi.URLParam(r, "requestId")
		items, err := st.ListGlobalPendingEnrollments(globalplane.GlobalPendingEnrollmentFilter{})
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		var planeID string
		for _, item := range items {
			if item.RequestID == requestID {
				planeID = item.PlaneID
				break
			}
		}
		if planeID == "" {
			http.Error(w, "request not found in cache", http.StatusNotFound)
			return
		}
		plane, ok, err := st.GetRegionalPlane(planeID)
		if err != nil || !ok {
			http.Error(w, "plane not found", http.StatusNotFound)
			return
		}
		if err := proxyApprove(r.Context(), plane, requestID, encKey, logger); err != nil {
			http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"requestId": requestID, "status": "approved"})
	}
}

// DenyGlobalPendingEnrollment handles POST /api/v1/pending-enrollments/{requestId}/deny.
func DenyGlobalPendingEnrollment(st enrollmentStore, encKey []byte, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := chi.URLParam(r, "requestId")
		var req struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		items, err := st.ListGlobalPendingEnrollments(globalplane.GlobalPendingEnrollmentFilter{})
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		var planeID string
		for _, item := range items {
			if item.RequestID == requestID {
				planeID = item.PlaneID
				break
			}
		}
		if planeID == "" {
			http.Error(w, "request not found in cache", http.StatusNotFound)
			return
		}
		plane, ok, err := st.GetRegionalPlane(planeID)
		if err != nil || !ok {
			http.Error(w, "plane not found", http.StatusNotFound)
			return
		}
		if err := proxyDeny(r.Context(), plane, requestID, req.Reason, encKey, logger); err != nil {
			http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"requestId": requestID, "status": "denied"})
	}
}

// ── proxy helpers ─────────────────────────────────────────────────────────────

func proxyApprove(ctx context.Context, plane globalplane.RegionalPlane, requestID string, encKey []byte, logger *log.Logger) error {
	token, err := globalplane.DecryptToken(plane.EncryptedToken, encKey)
	if err != nil {
		logger.Printf("[global-plane] approve proxy: decrypt token for %s: %v", plane.Name, err)
		return err
	}
	client, err := gpsync.NewRegionalClient(plane.BaseURL, token, plane.TLSCAPem)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return client.ApprovePendingEnrollment(callCtx, requestID)
}

func proxyDeny(ctx context.Context, plane globalplane.RegionalPlane, requestID, reason string, encKey []byte, logger *log.Logger) error {
	token, err := globalplane.DecryptToken(plane.EncryptedToken, encKey)
	if err != nil {
		logger.Printf("[global-plane] deny proxy: decrypt token for %s: %v", plane.Name, err)
		return err
	}
	client, err := gpsync.NewRegionalClient(plane.BaseURL, token, plane.TLSCAPem)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return client.DenyPendingEnrollment(callCtx, requestID, reason)
}
