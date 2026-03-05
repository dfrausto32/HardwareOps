package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/license"
	"github.com/hardwareops/control-plane/internal/metrics"
	"github.com/hardwareops/control-plane/internal/store"
)

const (
	defaultEnrollmentProfileTTL = 30 * 24 * time.Hour
	maxEnrollmentProfileTTL     = 365 * 24 * time.Hour
	defaultPendingEnrollTTL     = 15 * time.Minute
	maxPendingListLimit         = 500
	maxProfileRotateGrace       = 24 * time.Hour
	pendingMetricsBatchSize     = 200
)

const (
	ageBucketLT1M = iota
	ageBucketM1To5M
	ageBucketM5To15M
	ageBucketM15To1H
	ageBucketGTE1H
)

type CreateEnrollmentProfilePayload struct {
	Name             string          `json:"name"`
	ExpiresInSec     int64           `json:"expiresInSec"`
	MaxUses          int             `json:"maxUses"`
	RequireApproval  *bool           `json:"requireApproval"`
	AllowUntrustedHW bool            `json:"allowUnsignedHardwareIdentity"`
	ChallengeSecret  string          `json:"challengeSecret,omitempty"`
	ChallengeHint    string          `json:"challengeHint,omitempty"`
	ApprovalDelaySec int             `json:"approvalDelaySec,omitempty"`
	DefaultLabels    json.RawMessage `json:"defaultLabels,omitempty"`
}

type UpdateEnrollmentProfilePayload struct {
	Name               *string          `json:"name,omitempty"`
	MaxUses            *int             `json:"maxUses,omitempty"`
	RequireApproval    *bool            `json:"requireApproval,omitempty"`
	AllowUntrustedHW   *bool            `json:"allowUnsignedHardwareIdentity,omitempty"`
	ChallengeSecret    *string          `json:"challengeSecret,omitempty"`
	ChallengeHint      *string          `json:"challengeHint,omitempty"`
	ClearChallenge     *bool            `json:"clearChallenge,omitempty"`
	ApprovalDelaySec   *int             `json:"approvalDelaySec,omitempty"`
	DefaultLabels      *json.RawMessage `json:"defaultLabels,omitempty"`
	ClearDefaultLabels *bool            `json:"clearDefaultLabels,omitempty"`
}

type RotateEnrollmentProfileTokenPayload struct {
	Reason         string `json:"reason,omitempty"`
	GracePeriodSec int    `json:"gracePeriodSec,omitempty"`
}

type CreateEnrollmentProfileResponse struct {
	ProfileID               string    `json:"profileId"`
	Name                    string    `json:"name"`
	BootstrapToken          string    `json:"bootstrapToken"`
	ExpiresAt               time.Time `json:"expiresAt"`
	MaxUses                 int       `json:"maxUses"`
	Uses                    int       `json:"uses"`
	RequireApproval         bool      `json:"requireApproval"`
	ChallengeEnabled        bool      `json:"challengeEnabled"`
	ChallengeHint           string    `json:"challengeHint,omitempty"`
	ApprovalDelaySec        int       `json:"approvalDelaySec,omitempty"`
	TokenRotatedAt          time.Time `json:"tokenRotatedAt,omitempty"`
	PreviousTokenGraceUntil time.Time `json:"previousTokenGraceUntil,omitempty"`
}

type EnrollmentProfileTokenResponse = CreateEnrollmentProfileResponse

type EnrollmentProfileListResponse struct {
	Items []EnrollmentProfileListItem `json:"items"`
}

type EnrollmentProfileListItem struct {
	ProfileID               string          `json:"profileId"`
	Name                    string          `json:"name"`
	RequireApproval         bool            `json:"requireApproval"`
	AllowUntrustedHW        bool            `json:"allowUnsignedHardwareIdentity"`
	ChallengeEnabled        bool            `json:"challengeEnabled"`
	ChallengeHint           string          `json:"challengeHint,omitempty"`
	ApprovalDelaySec        int             `json:"approvalDelaySec,omitempty"`
	MaxUses                 int             `json:"maxUses"`
	Uses                    int             `json:"uses"`
	ExpiresAt               time.Time       `json:"expiresAt"`
	CreatedAt               time.Time       `json:"createdAt"`
	CreatedBy               string          `json:"createdBy,omitempty"`
	TokenRotatedAt          time.Time       `json:"tokenRotatedAt,omitempty"`
	PreviousTokenGraceUntil time.Time       `json:"previousTokenGraceUntil,omitempty"`
	Disabled                bool            `json:"disabled"`
	DefaultLabelsJSON       json.RawMessage `json:"defaultLabels,omitempty"`
}

type PendingEnrollmentRequestPayload struct {
	ProfileToken      string          `json:"profileToken"`
	ChallengeResponse string          `json:"challengeResponse,omitempty"`
	CSR               string          `json:"csr"`
	AgentVersion      string          `json:"agentVersion,omitempty"`
	Capabilities      json.RawMessage `json:"capabilities,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
}

type PendingEnrollmentRequestResponse struct {
	RequestID    string    `json:"requestId"`
	Status       string    `json:"status"`
	ClaimToken   string    `json:"claimToken"`
	PollAfterSec int       `json:"pollAfterSec"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type PendingEnrollmentListResponse struct {
	Items []PendingEnrollmentListItem `json:"items"`
}

type PendingEnrollmentListItem struct {
	RequestID           string          `json:"requestId"`
	Status              string          `json:"status"`
	ProfileID           string          `json:"profileId"`
	SourceIP            string          `json:"sourceIp,omitempty"`
	AgentVersion        string          `json:"agentVersion,omitempty"`
	HardwareID          string          `json:"hardwareId,omitempty"`
	DeniedReason        string          `json:"deniedReason,omitempty"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	Capabilities        json.RawMessage `json:"capabilities,omitempty"`
	CreatedAt           time.Time       `json:"createdAt"`
	ExpiresAt           time.Time       `json:"expiresAt"`
	ApprovalAvailableAt time.Time       `json:"approvalAvailableAt,omitempty"`
}

type ApprovePendingEnrollmentResponse struct {
	RequestID string    `json:"requestId"`
	Status    string    `json:"status"`
	At        time.Time `json:"at"`
}

type DenyPendingEnrollmentPayload struct {
	Reason string `json:"reason"`
}

type DenyPendingEnrollmentResponse struct {
	RequestID string    `json:"requestId"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	At        time.Time `json:"at"`
}

type ResetPendingEnrollmentResponse struct {
	RequestID string    `json:"requestId"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type ClaimPendingEnrollmentPayload struct {
	RequestID  string `json:"requestId"`
	ClaimToken string `json:"claimToken"`
}

type ClaimPendingEnrollmentResponse struct {
	Status       string    `json:"status"`
	PollAfterSec int       `json:"pollAfterSec,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	DeviceID     string    `json:"deviceId,omitempty"`
	CertPEM      string    `json:"certPem,omitempty"`
	CACertPEM    string    `json:"caCertPem,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
}

func CreateEnrollmentProfile(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			http.Error(w, "storage unavailable", http.StatusInternalServerError)
			return
		}
		var req CreateEnrollmentProfilePayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if req.MaxUses < 0 {
			http.Error(w, "maxUses must be >= 0", http.StatusBadRequest)
			return
		}
		if req.ApprovalDelaySec < 0 {
			http.Error(w, "approvalDelaySec must be >= 0", http.StatusBadRequest)
			return
		}
		if req.ApprovalDelaySec >= int(defaultPendingEnrollTTL.Seconds()) {
			http.Error(w, "approvalDelaySec must be shorter than pending enrollment ttl", http.StatusBadRequest)
			return
		}
		req.ChallengeSecret = strings.TrimSpace(req.ChallengeSecret)
		req.ChallengeHint = strings.TrimSpace(req.ChallengeHint)
		if len(req.DefaultLabels) > 0 && !isJSONObject(req.DefaultLabels) {
			http.Error(w, "defaultLabels must be an object", http.StatusBadRequest)
			return
		}

		ttl := defaultEnrollmentProfileTTL
		if req.ExpiresInSec > 0 {
			if req.ExpiresInSec > int64(maxEnrollmentProfileTTL.Seconds()) {
				http.Error(w, "expiresInSec too large", http.StatusBadRequest)
				return
			}
			ttl = time.Duration(req.ExpiresInSec) * time.Second
		}
		if req.ExpiresInSec < 0 {
			http.Error(w, "expiresInSec must be positive", http.StatusBadRequest)
			return
		}
		requireApproval := true
		if req.RequireApproval != nil {
			requireApproval = *req.RequireApproval
		}

		token, tokenHash, err := generateToken()
		if err != nil {
			if logger != nil {
				logger.Printf("generate enrollment profile token: %v", err)
			}
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}
		profile := store.EnrollmentProfile{
			ProfileID:         uuid.NewString(),
			Name:              req.Name,
			TokenHash:         tokenHash,
			RequireApproval:   requireApproval,
			AllowUntrustedHW:  req.AllowUntrustedHW,
			ChallengeHash:     hashOptionalSecret(req.ChallengeSecret),
			ChallengeHint:     req.ChallengeHint,
			ApprovalDelaySec:  req.ApprovalDelaySec,
			MaxUses:           req.MaxUses,
			Uses:              0,
			ExpiresAt:         time.Now().UTC().Add(ttl),
			CreatedAt:         time.Now().UTC(),
			DefaultLabelsJSON: req.DefaultLabels,
		}
		if err := st.CreateEnrollmentProfile(profile); err != nil {
			if logger != nil {
				logger.Printf("create enrollment profile: %v", err)
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("local"), "enrollment_profile.create", "enrollment_profile", profile.ProfileID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("local"), "enrollment_profile.create", "enrollment_profile", profile.ProfileID)
		event.MetadataJSON = auditJSON(map[string]any{
			"name":            profile.Name,
			"expiresAt":       profile.ExpiresAt,
			"maxUses":         profile.MaxUses,
			"requireApproval": profile.RequireApproval,
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(enrollmentProfileTokenResponse(profile, token))
	}
}

func UpdateEnrollmentProfile(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			http.Error(w, "storage unavailable", http.StatusInternalServerError)
			return
		}
		profileID := strings.TrimSpace(chi.URLParam(r, "profileId"))
		if profileID == "" {
			http.Error(w, "profileId required", http.StatusBadRequest)
			return
		}
		current, err := st.GetEnrollmentProfile(profileID)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrEnrollmentProfileNotFound):
				http.Error(w, "enrollment profile not found", http.StatusNotFound)
			default:
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("local"), "enrollment_profile.update", "enrollment_profile", profileID), err)
			return
		}

		var req UpdateEnrollmentProfilePayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		next := current
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				http.Error(w, "name required", http.StatusBadRequest)
				return
			}
			next.Name = name
		}
		if req.MaxUses != nil {
			if *req.MaxUses < 0 {
				http.Error(w, "maxUses must be >= 0", http.StatusBadRequest)
				return
			}
			if *req.MaxUses > 0 && *req.MaxUses < current.Uses {
				http.Error(w, "maxUses cannot be less than current uses", http.StatusBadRequest)
				return
			}
			next.MaxUses = *req.MaxUses
		}
		if req.RequireApproval != nil {
			next.RequireApproval = *req.RequireApproval
		}
		if req.AllowUntrustedHW != nil {
			next.AllowUntrustedHW = *req.AllowUntrustedHW
		}
		if req.ApprovalDelaySec != nil {
			if *req.ApprovalDelaySec < 0 {
				http.Error(w, "approvalDelaySec must be >= 0", http.StatusBadRequest)
				return
			}
			if *req.ApprovalDelaySec >= int(defaultPendingEnrollTTL.Seconds()) {
				http.Error(w, "approvalDelaySec must be shorter than pending enrollment ttl", http.StatusBadRequest)
				return
			}
			next.ApprovalDelaySec = *req.ApprovalDelaySec
		}
		if req.ClearDefaultLabels != nil && *req.ClearDefaultLabels {
			next.DefaultLabelsJSON = nil
		}
		if req.DefaultLabels != nil {
			raw := json.RawMessage(*req.DefaultLabels)
			if len(raw) > 0 && !isJSONObject(raw) {
				http.Error(w, "defaultLabels must be an object", http.StatusBadRequest)
				return
			}
			next.DefaultLabelsJSON = raw
		}

		clearChallenge := req.ClearChallenge != nil && *req.ClearChallenge
		if clearChallenge {
			next.ChallengeHash = ""
			next.ChallengeHint = ""
		}
		if req.ChallengeSecret != nil {
			secret := strings.TrimSpace(*req.ChallengeSecret)
			next.ChallengeHash = hashOptionalSecret(secret)
			if secret == "" {
				next.ChallengeHint = ""
			}
		}
		if req.ChallengeHint != nil {
			next.ChallengeHint = strings.TrimSpace(*req.ChallengeHint)
		}
		if next.ChallengeHash == "" {
			next.ChallengeHint = ""
		}

		updated, err := st.UpdateEnrollmentProfile(profileID, store.EnrollmentProfileUpdate{
			Name:              next.Name,
			RequireApproval:   next.RequireApproval,
			AllowUntrustedHW:  next.AllowUntrustedHW,
			ChallengeHash:     next.ChallengeHash,
			ChallengeHint:     next.ChallengeHint,
			ApprovalDelaySec:  next.ApprovalDelaySec,
			MaxUses:           next.MaxUses,
			DefaultLabelsJSON: next.DefaultLabelsJSON,
		})
		if err != nil {
			switch {
			case errors.Is(err, store.ErrEnrollmentProfileNotFound):
				http.Error(w, "enrollment profile not found", http.StatusNotFound)
			default:
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("local"), "enrollment_profile.update", "enrollment_profile", profileID), err)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("local"), "enrollment_profile.update", "enrollment_profile", profileID)
		event.BeforeJSON = auditJSON(map[string]any{
			"name":           current.Name,
			"maxUses":        current.MaxUses,
			"challengeSet":   current.ChallengeHash != "",
			"challengeHint":  current.ChallengeHint,
			"approvalDelay":  current.ApprovalDelaySec,
			"defaultLabels":  json.RawMessage(current.DefaultLabelsJSON),
			"allowUnsigned":  current.AllowUntrustedHW,
			"requireApprove": current.RequireApproval,
		})
		event.AfterJSON = auditJSON(map[string]any{
			"name":           updated.Name,
			"maxUses":        updated.MaxUses,
			"challengeSet":   updated.ChallengeHash != "",
			"challengeHint":  updated.ChallengeHint,
			"approvalDelay":  updated.ApprovalDelaySec,
			"defaultLabels":  json.RawMessage(updated.DefaultLabelsJSON),
			"allowUnsigned":  updated.AllowUntrustedHW,
			"requireApprove": updated.RequireApproval,
		})
		writeAudit(logger, st, event, nil)
		writeJSON(w, enrollmentProfileListItem(updated))
	}
}

func ListEnrollmentProfiles(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			http.Error(w, "storage unavailable", http.StatusInternalServerError)
			return
		}
		rows, err := st.ListEnrollmentProfiles()
		if err != nil {
			if logger != nil {
				logger.Printf("list enrollment profiles: %v", err)
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("local"), "enrollment_profile.list", "enrollment_profile", ""), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		items := make([]EnrollmentProfileListItem, 0, len(rows))
		for _, row := range rows {
			items = append(items, enrollmentProfileListItem(row))
		}
		event := buildAuditEvent(r, trustProxy, actorUser("local"), "enrollment_profile.list", "enrollment_profile", "")
		event.MetadataJSON = auditJSON(map[string]any{"count": len(items)})
		writeAudit(logger, st, event, nil)
		writeJSON(w, EnrollmentProfileListResponse{Items: items})
	}
}

func SetEnrollmentProfileDisabled(logger *log.Logger, st store.Store, trustProxy bool, disabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			http.Error(w, "storage unavailable", http.StatusInternalServerError)
			return
		}
		profileID := strings.TrimSpace(chi.URLParam(r, "profileId"))
		if profileID == "" {
			http.Error(w, "profileId required", http.StatusBadRequest)
			return
		}
		profile, err := st.SetEnrollmentProfileDisabled(profileID, disabled)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrEnrollmentProfileNotFound):
				http.Error(w, "enrollment profile not found", http.StatusNotFound)
			default:
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			action := "enrollment_profile.enable"
			if disabled {
				action = "enrollment_profile.disable"
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("local"), action, "enrollment_profile", profileID), err)
			return
		}
		action := "enrollment_profile.enable"
		if disabled {
			action = "enrollment_profile.disable"
		}
		event := buildAuditEvent(r, trustProxy, actorUser("local"), action, "enrollment_profile", profileID)
		event.MetadataJSON = auditJSON(map[string]any{
			"name":     profile.Name,
			"disabled": profile.Disabled,
		})
		writeAudit(logger, st, event, nil)
		writeJSON(w, enrollmentProfileListItem(profile))
	}
}

func RotateEnrollmentProfileToken(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			http.Error(w, "storage unavailable", http.StatusInternalServerError)
			return
		}
		profileID := strings.TrimSpace(chi.URLParam(r, "profileId"))
		if profileID == "" {
			http.Error(w, "profileId required", http.StatusBadRequest)
			return
		}
		var req RotateEnrollmentProfileTokenPayload
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
		}
		req.Reason = strings.TrimSpace(req.Reason)
		if req.GracePeriodSec < 0 {
			http.Error(w, "gracePeriodSec must be >= 0", http.StatusBadRequest)
			return
		}
		gracePeriod := time.Duration(req.GracePeriodSec) * time.Second
		if gracePeriod > maxProfileRotateGrace {
			http.Error(w, "gracePeriodSec exceeds max allowed (86400)", http.StatusBadRequest)
			return
		}
		var previousTokenValidUntil time.Time
		if gracePeriod > 0 {
			previousTokenValidUntil = time.Now().UTC().Add(gracePeriod)
		}
		token, tokenHash, err := generateToken()
		if err != nil {
			if logger != nil {
				logger.Printf("generate enrollment profile token: %v", err)
			}
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}
		profile, err := st.RotateEnrollmentProfileToken(profileID, tokenHash, previousTokenValidUntil)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrEnrollmentProfileNotFound):
				http.Error(w, "enrollment profile not found", http.StatusNotFound)
			default:
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("local"), "enrollment_profile.rotate_token", "enrollment_profile", profileID), err)
			return
		}
		if req.Reason == "" {
			req.Reason = "operator_requested"
		}
		event := buildAuditEvent(r, trustProxy, actorUser("local"), "enrollment_profile.rotate_token", "enrollment_profile", profileID)
		metadata := map[string]any{
			"name":           profile.Name,
			"reason":         req.Reason,
			"gracePeriodSec": req.GracePeriodSec,
		}
		if !profile.PreviousTokenExpiresAt.IsZero() {
			metadata["previousTokenGraceUntil"] = profile.PreviousTokenExpiresAt
		}
		event.MetadataJSON = auditJSON(metadata)
		writeAudit(logger, st, event, nil)
		writeJSON(w, enrollmentProfileTokenResponse(profile, token))
	}
}

func RequestPendingEnrollment(logger *log.Logger, st store.Store, identityPolicy DeviceIdentityPolicy, trustProxy bool, metricsCollector *metrics.Metrics, hub *events.Hub, guard *PendingEnrollmentGuard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status, reason string) {
			if metricsCollector != nil {
				metricsCollector.IncEnroll(status, reason)
			}
		}
		policy := identityPolicy.normalized()
		if st == nil {
			record("error", "storage_unavailable")
			http.Error(w, "storage unavailable", http.StatusInternalServerError)
			return
		}
		var req PendingEnrollmentRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			record("error", "bad_request")
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.ProfileToken = strings.TrimSpace(req.ProfileToken)
		req.ChallengeResponse = strings.TrimSpace(req.ChallengeResponse)
		req.CSR = strings.TrimSpace(req.CSR)
		if req.ProfileToken == "" || req.CSR == "" {
			record("error", "bad_request")
			http.Error(w, "profileToken and csr required", http.StatusBadRequest)
			return
		}
		if len(req.CSR) > maxCSRSize {
			record("error", "csr_too_large")
			http.Error(w, "csr too large", http.StatusBadRequest)
			return
		}
		if err := validateCSR([]byte(req.CSR)); err != nil {
			record("error", "csr_invalid")
			http.Error(w, "csr invalid", http.StatusBadRequest)
			return
		}
		if len(req.Metadata) > 0 && !isJSONObject(req.Metadata) {
			record("error", "metadata_invalid")
			http.Error(w, "metadata must be an object", http.StatusBadRequest)
			return
		}
		if len(req.Capabilities) > 0 && !isJSONObject(req.Capabilities) {
			record("error", "capabilities_invalid")
			http.Error(w, "capabilities must be an object", http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		if _, err := st.ExpirePendingEnrollments(now); err != nil && logger != nil {
			logger.Printf("expire pending enrollments: %v", err)
		}
		refreshPendingEnrollmentMetrics(st, metricsCollector, now)
		profileTokenHash := hashToken(req.ProfileToken)
		profile, err := st.GetEnrollmentProfileByTokenHash(profileTokenHash)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrEnrollmentProfileInvalid):
				record("error", "profile_invalid")
				http.Error(w, "invalid or expired profile token", http.StatusUnauthorized)
			case errors.Is(err, store.ErrEnrollmentProfileExhausted):
				record("error", "profile_exhausted")
				http.Error(w, "enrollment profile max uses reached", http.StatusForbidden)
			default:
				record("error", "storage_error")
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			return
		}
		if profile.ChallengeHash != "" && hashToken(req.ChallengeResponse) != profile.ChallengeHash {
			record("error", "challenge_invalid")
			http.Error(w, "invalid enrollment challenge", http.StatusUnauthorized)
			return
		}
		sourceIP := clientIP(r, trustProxy)
		if guard != nil {
			if allowed, retryAfter, reason := guard.AllowRequest(sourceIP, profile.ProfileID); !allowed {
				record("error", reason)
				writeRetryAfterHeader(w, retryAfter)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			if allowed, reason, err := guard.CheckQueueCaps(st, profile.ProfileID, sourceIP, now); err != nil {
				record("error", "storage_error")
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			} else if !allowed {
				record("error", reason)
				w.Header().Set("Retry-After", "60")
				http.Error(w, "pending enrollment queue is full", http.StatusTooManyRequests)
				return
			}
		}
		hardware := parseHardwareIdentity(req.Capabilities)
		if policy.Mode == deviceIdentityModeEnforce && policy.RequireOnEnroll && hardware.ID == "" {
			record("error", "hardware_identity_required")
			http.Error(w, "hardware identity required", http.StatusBadRequest)
			return
		}
		if hardware.ID != "" {
			_, ok, err := st.GetDeviceByHardwareID(hardware.ID)
			if err != nil {
				record("error", "storage_error")
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			}
			if ok {
				record("error", "hardware_identity_conflict")
				http.Error(w, "hardware identity already enrolled", http.StatusConflict)
				return
			}
		}
		claimToken, claimTokenHash, err := generateToken()
		if err != nil {
			record("error", "token_error")
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}
		approvalAvailableAt := now
		if profile.ApprovalDelaySec > 0 {
			approvalAvailableAt = now.Add(time.Duration(profile.ApprovalDelaySec) * time.Second)
		}
		pending := store.PendingEnrollment{
			RequestID:           uuid.NewString(),
			Status:              "pending",
			CSR:                 req.CSR,
			ClaimTokenHash:      claimTokenHash,
			CapabilitiesJSON:    req.Capabilities,
			MetadataJSON:        req.Metadata,
			SourceIP:            sourceIP,
			UserAgent:           r.UserAgent(),
			AgentVersion:        strings.TrimSpace(req.AgentVersion),
			HardwareID:          hardware.ID,
			ExpiresAt:           now.Add(defaultPendingEnrollTTL),
			ApprovalAvailableAt: approvalAvailableAt,
			CreatedAt:           now,
		}
		profile, err = st.CreatePendingEnrollmentForProfileToken(profileTokenHash, pending)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrEnrollmentProfileInvalid):
				record("error", "profile_invalid")
				http.Error(w, "invalid or expired profile token", http.StatusUnauthorized)
			case errors.Is(err, store.ErrEnrollmentProfileExhausted):
				record("error", "profile_exhausted")
				http.Error(w, "enrollment profile max uses reached", http.StatusForbidden)
			default:
				record("error", "storage_error")
				if logger != nil {
					logger.Printf("create pending enrollment: %v", err)
				}
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			return
		}
		refreshPendingEnrollmentMetrics(st, metricsCollector, now)

		event := buildAuditEvent(r, trustProxy, AuditActor{Type: "bootstrap_token", ID: profile.ProfileID, AuthMethod: "enrollment_profile"}, "pending_enrollment.request", "pending_enrollment", pending.RequestID)
		event.MetadataJSON = auditJSON(map[string]any{
			"profileId":    profile.ProfileID,
			"agentVersion": pending.AgentVersion,
			"hardwareId":   pending.HardwareID,
		})
		writeAudit(logger, st, event, nil)
		emitRuntimeEvent(logger, st, hub, events.Event{
			Type: events.TypeDeviceEnrollPending,
			At:   time.Now().UTC(),
			Payload: auditJSON(map[string]any{
				"requestId":    pending.RequestID,
				"profileId":    profile.ProfileID,
				"agentVersion": pending.AgentVersion,
				"hardwareId":   pending.HardwareID,
			}),
		})

		record("success", "pending_request")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(PendingEnrollmentRequestResponse{
			RequestID:    pending.RequestID,
			Status:       pending.Status,
			ClaimToken:   claimToken,
			PollAfterSec: 5,
			ExpiresAt:    pending.ExpiresAt,
		})
	}
}

func ApprovePendingEnrollment(logger *log.Logger, st store.Store, trustProxy bool, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(chi.URLParam(r, "requestId"))
		if requestID == "" {
			http.Error(w, "requestId required", http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		if _, err := st.ExpirePendingEnrollments(now); err != nil && logger != nil {
			logger.Printf("expire pending enrollments: %v", err)
		}
		refreshPendingEnrollmentMetrics(st, metricsCollector, now)
		approvedByUserID := ""
		if user, ok := auth.UserFromContext(r.Context()); ok {
			approvedByUserID = user.UserID
		}
		actor := actorUser("local")
		pending, err := st.ApprovePendingEnrollment(requestID, approvedByUserID, now)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrPendingEnrollmentNotFound):
				http.Error(w, "pending enrollment not found", http.StatusNotFound)
			case errors.Is(err, store.ErrPendingEnrollmentThrottled):
				if metricsCollector != nil {
					metricsCollector.IncPendingEnrollThrottle("approval_delay")
				}
				if !pending.ApprovalAvailableAt.IsZero() && pending.ApprovalAvailableAt.After(now) {
					writeRetryAfterHeader(w, pending.ApprovalAvailableAt.Sub(now))
				}
				http.Error(w, "pending enrollment approval is throttled", http.StatusTooManyRequests)
			case errors.Is(err, store.ErrPendingEnrollmentState):
				http.Error(w, "pending enrollment not in approvable state", http.StatusConflict)
			default:
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actor, "pending_enrollment.approve", "pending_enrollment", requestID), err)
			return
		}
		event := buildAuditEvent(r, trustProxy, actor, "pending_enrollment.approve", "pending_enrollment", requestID)
		event.MetadataJSON = auditJSON(map[string]any{
			"profileId":  pending.ProfileID,
			"hardwareId": pending.HardwareID,
		})
		writeAudit(logger, st, event, nil)
		refreshPendingEnrollmentMetrics(st, metricsCollector, now)
		writeJSON(w, ApprovePendingEnrollmentResponse{
			RequestID: pending.RequestID,
			Status:    pending.Status,
			At:        pending.ApprovedAt,
		})
	}
}

func DenyPendingEnrollment(logger *log.Logger, st store.Store, trustProxy bool, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(chi.URLParam(r, "requestId"))
		if requestID == "" {
			http.Error(w, "requestId required", http.StatusBadRequest)
			return
		}
		var req DenyPendingEnrollmentPayload
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = "denied_by_operator"
		}
		deniedByUserID := ""
		if user, ok := auth.UserFromContext(r.Context()); ok {
			deniedByUserID = user.UserID
		}
		actor := actorUser("local")
		pending, err := st.DenyPendingEnrollment(requestID, reason, deniedByUserID, time.Now().UTC())
		if err != nil {
			switch {
			case errors.Is(err, store.ErrPendingEnrollmentNotFound):
				http.Error(w, "pending enrollment not found", http.StatusNotFound)
			case errors.Is(err, store.ErrPendingEnrollmentState):
				http.Error(w, "pending enrollment not in denyable state", http.StatusConflict)
			default:
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actor, "pending_enrollment.deny", "pending_enrollment", requestID), err)
			return
		}
		event := buildAuditEvent(r, trustProxy, actor, "pending_enrollment.deny", "pending_enrollment", requestID)
		event.MetadataJSON = auditJSON(map[string]any{
			"reason":     pending.DeniedReason,
			"profileId":  pending.ProfileID,
			"hardwareId": pending.HardwareID,
		})
		writeAudit(logger, st, event, nil)
		refreshPendingEnrollmentMetrics(st, metricsCollector, time.Now().UTC())
		writeJSON(w, DenyPendingEnrollmentResponse{
			RequestID: pending.RequestID,
			Status:    pending.Status,
			Reason:    pending.DeniedReason,
			At:        pending.DeniedAt,
		})
	}
}

func ResetPendingEnrollment(logger *log.Logger, st store.Store, trustProxy bool, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(chi.URLParam(r, "requestId"))
		if requestID == "" {
			http.Error(w, "requestId required", http.StatusBadRequest)
			return
		}
		pending, err := st.ResetPendingEnrollment(requestID, time.Now().UTC().Add(defaultPendingEnrollTTL))
		if err != nil {
			switch {
			case errors.Is(err, store.ErrPendingEnrollmentNotFound):
				http.Error(w, "pending enrollment not found", http.StatusNotFound)
			case errors.Is(err, store.ErrPendingEnrollmentState):
				http.Error(w, "pending enrollment not resettable", http.StatusConflict)
			default:
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("local"), "pending_enrollment.reset", "pending_enrollment", requestID), err)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("local"), "pending_enrollment.reset", "pending_enrollment", requestID)
		event.MetadataJSON = auditJSON(map[string]any{
			"profileId":  pending.ProfileID,
			"hardwareId": pending.HardwareID,
			"expiresAt":  pending.ExpiresAt,
		})
		writeAudit(logger, st, event, nil)
		refreshPendingEnrollmentMetrics(st, metricsCollector, time.Now().UTC())
		writeJSON(w, ResetPendingEnrollmentResponse{
			RequestID: pending.RequestID,
			Status:    pending.Status,
			ExpiresAt: pending.ExpiresAt,
		})
	}
}

func ClaimPendingEnrollment(logger *log.Logger, st store.Store, lic *license.Manager, signer interface {
	SignDeviceCert(csrPEM []byte, deviceID string, validity time.Duration) ([]byte, string, error)
	CACertPEM() []byte
}, identityPolicy DeviceIdentityPolicy, trustProxy bool, metricsCollector *metrics.Metrics, hub *events.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status, reason string) {
			if metricsCollector != nil {
				metricsCollector.IncEnroll(status, reason)
			}
		}
		if st == nil {
			record("error", "storage_unavailable")
			http.Error(w, "storage unavailable", http.StatusInternalServerError)
			return
		}
		if signer == nil {
			record("error", "signer_missing")
			http.Error(w, "signer not configured", http.StatusInternalServerError)
			return
		}
		var req ClaimPendingEnrollmentPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			record("error", "bad_request")
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.RequestID = strings.TrimSpace(req.RequestID)
		req.ClaimToken = strings.TrimSpace(req.ClaimToken)
		if req.RequestID == "" || req.ClaimToken == "" {
			record("error", "bad_request")
			http.Error(w, "requestId and claimToken required", http.StatusBadRequest)
			return
		}
		claimHash := hashToken(req.ClaimToken)
		pending, ok, err := st.GetPendingEnrollmentForClaim(req.RequestID, claimHash)
		if err != nil {
			record("error", "storage_error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			record("error", "claim_invalid")
			http.Error(w, "invalid claim token", http.StatusUnauthorized)
			return
		}
		switch pending.Status {
		case "pending":
			record("success", "claim_pending")
			w.WriteHeader(http.StatusAccepted)
			writeJSON(w, ClaimPendingEnrollmentResponse{
				Status:       "pending",
				PollAfterSec: 5,
				ExpiresAt:    pending.ExpiresAt,
			})
			return
		case "denied":
			record("error", "claim_denied")
			w.WriteHeader(http.StatusGone)
			writeJSON(w, ClaimPendingEnrollmentResponse{
				Status: "denied",
				Reason: pending.DeniedReason,
			})
			return
		case "expired":
			record("error", "claim_expired")
			w.WriteHeader(http.StatusGone)
			writeJSON(w, ClaimPendingEnrollmentResponse{
				Status: "expired",
			})
			return
		case "conflict":
			record("error", "claim_conflict")
			w.WriteHeader(http.StatusConflict)
			writeJSON(w, ClaimPendingEnrollmentResponse{
				Status: "conflict",
				Reason: pending.DeniedReason,
			})
			return
		case "issued":
			record("error", "claim_already_issued")
			http.Error(w, "enrollment already claimed", http.StatusConflict)
			return
		case "approved":
			// Continue to issue.
		default:
			record("error", "claim_invalid_state")
			http.Error(w, "pending enrollment in invalid state", http.StatusConflict)
			return
		}

		maxDevices, err := enrollmentMaxDevices(lic)
		if err != nil {
			record("error", "license_invalid")
			http.Error(w, "license invalid", http.StatusForbidden)
			return
		}
		policy := identityPolicy.normalized()
		if policy.Mode == deviceIdentityModeEnforce && policy.RequireOnEnroll && pending.HardwareID == "" {
			record("error", "hardware_identity_required")
			http.Error(w, "hardware identity required", http.StatusBadRequest)
			return
		}
		if pending.HardwareID != "" {
			existing, exists, err := st.GetDeviceByHardwareID(pending.HardwareID)
			if err != nil {
				record("error", "storage_error")
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			}
			if exists {
				if _, err := st.ConflictPendingEnrollment(req.RequestID, "hardware_identity_already_enrolled", time.Now().UTC()); err != nil && logger != nil {
					logger.Printf("mark pending enrollment conflict: %v", err)
				}
				record("error", "hardware_identity_conflict")
				http.Error(w, "hardware identity already enrolled", http.StatusConflict)
				event := buildAuditEvent(r, trustProxy, AuditActor{Type: "bootstrap_token", ID: pending.ProfileID, AuthMethod: "pending_claim"}, "pending_enrollment.issue_rejected", "pending_enrollment", pending.RequestID)
				event.Status = "denied"
				event.Error = "hardware_identity_already_enrolled"
				event.MetadataJSON = auditJSON(map[string]any{
					"hardwareId":       pending.HardwareID,
					"existingDeviceId": existing.DeviceID,
				})
				writeAudit(logger, st, event, nil)
				refreshPendingEnrollmentMetrics(st, metricsCollector, time.Now().UTC())
				return
			}
		}
		if err := validateCSR([]byte(pending.CSR)); err != nil {
			record("error", "csr_invalid")
			http.Error(w, "csr invalid", http.StatusBadRequest)
			return
		}
		profile, err := st.GetEnrollmentProfile(pending.ProfileID)
		if err != nil {
			record("error", "storage_error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		deviceID := uuid.NewString()
		certPEM, fingerprint, err := signer.SignDeviceCert([]byte(pending.CSR), deviceID, 365*24*time.Hour)
		if err != nil {
			record("error", "sign_error")
			http.Error(w, "csr invalid", http.StatusBadRequest)
			return
		}
		meta := []byte(nil)
		if caFingerprint, _, err := caFingerprintFromPEM(signer.CACertPEM()); err == nil {
			meta = updateCertMeta(nil, map[string]any{
				"active":        true,
				"caFingerprint": caFingerprint,
				"checkedAt":     time.Now().UTC().Format(time.RFC3339),
			})
		}
		if hw := parseHardwareIdentity(pending.CapabilitiesJSON); hw.ID != "" {
			if next, changed := upsertHardwareIdentityMeta(meta, hw, time.Now().UTC()); changed {
				meta = next
			}
		}
		issuedAt := time.Now().UTC()
		updated, err := st.MarkPendingEnrollmentIssued(req.RequestID, claimHash, store.Device{
			DeviceID:        deviceID,
			CertFingerprint: fingerprint,
			Status:          "active",
			LastSeen:        issuedAt,
			LabelsJSON:      profile.DefaultLabelsJSON,
			MetadataJSON:    meta,
		}, maxDevices, issuedAt)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrDeviceLimitExceeded):
				record("error", "license_limit")
				http.Error(w, "device limit reached", http.StatusForbidden)
			case errors.Is(err, store.ErrPendingEnrollmentToken):
				record("error", "claim_invalid")
				http.Error(w, "invalid claim token", http.StatusUnauthorized)
			case errors.Is(err, store.ErrPendingEnrollmentNotFound):
				record("error", "claim_not_found")
				http.Error(w, "pending enrollment not found", http.StatusNotFound)
			case errors.Is(err, store.ErrPendingEnrollmentState):
				record("error", "claim_state_invalid")
				http.Error(w, "pending enrollment not ready for claim", http.StatusConflict)
			default:
				record("error", "storage_error")
				http.Error(w, "storage error", http.StatusInternalServerError)
			}
			return
		}

		event := buildAuditEvent(r, trustProxy, AuditActor{Type: "bootstrap_token", ID: pending.ProfileID, AuthMethod: "pending_claim"}, "pending_enrollment.issue", "pending_enrollment", pending.RequestID)
		event.MetadataJSON = auditJSON(map[string]any{
			"deviceId":    updated.IssuedDeviceID,
			"fingerprint": fingerprint,
			"hardwareId":  pending.HardwareID,
		})
		writeAudit(logger, st, event, nil)
		refreshPendingEnrollmentMetrics(st, metricsCollector, time.Now().UTC())
		emitRuntimeEvent(logger, st, hub, events.Event{
			Type:     events.TypeDeviceEnroll,
			DeviceID: updated.IssuedDeviceID,
			At:       time.Now().UTC(),
			Payload: auditJSON(map[string]any{
				"requestId":   pending.RequestID,
				"profileId":   pending.ProfileID,
				"fingerprint": fingerprint,
				"hardwareId":  pending.HardwareID,
			}),
		})

		record("success", "claim_issued")
		writeJSON(w, ClaimPendingEnrollmentResponse{
			Status:    "issued",
			DeviceID:  deviceID,
			CertPEM:   string(certPEM),
			CACertPEM: string(signer.CACertPEM()),
		})
	}
}

func ListPendingEnrollments(logger *log.Logger, st store.Store, trustProxy bool, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			http.Error(w, "storage unavailable", http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC()
		if _, err := st.ExpirePendingEnrollments(now); err != nil && logger != nil {
			logger.Printf("expire pending enrollments: %v", err)
		}
		refreshPendingEnrollmentMetrics(st, metricsCollector, now)
		q := r.URL.Query()
		limit := parseIntParam(q.Get("limit"), 100)
		if limit > maxPendingListLimit {
			limit = maxPendingListLimit
		}
		offset := parseIntParam(q.Get("offset"), 0)
		if offset < 0 {
			offset = 0
		}
		filter := store.PendingEnrollmentFilter{
			Status: strings.TrimSpace(q.Get("status")),
			Limit:  limit,
			Offset: offset,
		}
		rows, err := st.ListPendingEnrollments(filter)
		if err != nil {
			if logger != nil {
				logger.Printf("list pending enrollments: %v", err)
			}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("local"), "pending_enrollment.list", "pending_enrollment", ""), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		items := make([]PendingEnrollmentListItem, 0, len(rows))
		for _, row := range rows {
			items = append(items, PendingEnrollmentListItem{
				RequestID:           row.RequestID,
				Status:              row.Status,
				ProfileID:           row.ProfileID,
				SourceIP:            row.SourceIP,
				AgentVersion:        row.AgentVersion,
				HardwareID:          row.HardwareID,
				DeniedReason:        row.DeniedReason,
				Metadata:            row.MetadataJSON,
				Capabilities:        row.CapabilitiesJSON,
				CreatedAt:           row.CreatedAt,
				ExpiresAt:           row.ExpiresAt,
				ApprovalAvailableAt: row.ApprovalAvailableAt,
			})
		}
		event := buildAuditEvent(r, trustProxy, actorUser("local"), "pending_enrollment.list", "pending_enrollment", "")
		event.MetadataJSON = auditJSON(map[string]any{
			"status": filter.Status,
			"limit":  filter.Limit,
			"offset": filter.Offset,
			"count":  len(items),
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PendingEnrollmentListResponse{Items: items})
	}
}

func enrollmentProfileListItem(profile store.EnrollmentProfile) EnrollmentProfileListItem {
	return EnrollmentProfileListItem{
		ProfileID:               profile.ProfileID,
		Name:                    profile.Name,
		RequireApproval:         profile.RequireApproval,
		AllowUntrustedHW:        profile.AllowUntrustedHW,
		ChallengeEnabled:        profile.ChallengeHash != "",
		ChallengeHint:           profile.ChallengeHint,
		ApprovalDelaySec:        profile.ApprovalDelaySec,
		MaxUses:                 profile.MaxUses,
		Uses:                    profile.Uses,
		ExpiresAt:               profile.ExpiresAt,
		CreatedAt:               profile.CreatedAt,
		CreatedBy:               profile.CreatedBy,
		TokenRotatedAt:          profile.TokenRotatedAt,
		PreviousTokenGraceUntil: profile.PreviousTokenExpiresAt,
		Disabled:                profile.Disabled,
		DefaultLabelsJSON:       profile.DefaultLabelsJSON,
	}
}

func hashOptionalSecret(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	return hashToken(v)
}

func enrollmentProfileTokenResponse(profile store.EnrollmentProfile, token string) CreateEnrollmentProfileResponse {
	return CreateEnrollmentProfileResponse{
		ProfileID:               profile.ProfileID,
		Name:                    profile.Name,
		BootstrapToken:          token,
		ExpiresAt:               profile.ExpiresAt,
		MaxUses:                 profile.MaxUses,
		Uses:                    profile.Uses,
		RequireApproval:         profile.RequireApproval,
		ChallengeEnabled:        profile.ChallengeHash != "",
		ChallengeHint:           profile.ChallengeHint,
		ApprovalDelaySec:        profile.ApprovalDelaySec,
		TokenRotatedAt:          profile.TokenRotatedAt,
		PreviousTokenGraceUntil: profile.PreviousTokenExpiresAt,
	}
}

func refreshPendingEnrollmentMetrics(st store.Store, metricsCollector *metrics.Metrics, now time.Time) {
	if st == nil || metricsCollector == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	count, err := st.CountActivePendingEnrollments("", "", now)
	if err != nil {
		return
	}
	metricsCollector.SetPendingEnrollActive(count)
	if count <= 0 {
		metricsCollector.SetPendingEnrollQueueAgeBuckets(0, 0, 0, 0, 0)
		metricsCollector.SetPendingEnrollOldestAgeSeconds(0)
		return
	}
	buckets := [5]int{}
	oldestAgeSeconds := 0.0
	remaining := count
	offset := 0
	for remaining > 0 {
		limit := pendingMetricsBatchSize
		if remaining < limit {
			limit = remaining
		}
		rows, err := st.ListPendingEnrollments(store.PendingEnrollmentFilter{
			Status: "pending",
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			return
		}
		if len(rows) == 0 {
			break
		}
		offset += len(rows)
		for _, row := range rows {
			if row.Status != "pending" {
				continue
			}
			if !row.ExpiresAt.IsZero() && !row.ExpiresAt.After(now) {
				continue
			}
			age := now.Sub(row.CreatedAt)
			if age < 0 {
				age = 0
			}
			buckets[pendingQueueAgeBucket(age)]++
			remaining--
			if age.Seconds() > oldestAgeSeconds {
				oldestAgeSeconds = age.Seconds()
			}
			if remaining == 0 {
				break
			}
		}
		if len(rows) < limit {
			break
		}
	}
	metricsCollector.SetPendingEnrollQueueAgeBuckets(
		buckets[ageBucketLT1M],
		buckets[ageBucketM1To5M],
		buckets[ageBucketM5To15M],
		buckets[ageBucketM15To1H],
		buckets[ageBucketGTE1H],
	)
	metricsCollector.SetPendingEnrollOldestAgeSeconds(oldestAgeSeconds)
}

func pendingQueueAgeBucket(age time.Duration) int {
	switch {
	case age < time.Minute:
		return ageBucketLT1M
	case age < 5*time.Minute:
		return ageBucketM1To5M
	case age < 15*time.Minute:
		return ageBucketM5To15M
	case age < time.Hour:
		return ageBucketM15To1H
	default:
		return ageBucketGTE1H
	}
}
