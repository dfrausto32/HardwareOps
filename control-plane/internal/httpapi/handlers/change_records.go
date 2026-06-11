package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

// changeRecordStore is the narrow interface the change-record handlers need.
type changeRecordStore interface {
	GetArtifact(artifactID string) (store.Artifact, bool, error)
	CreateChangeRecord(record store.ChangeRecord) (store.ChangeRecord, error)
	GetChangeRecordForArtifact(artifactID string) (store.ChangeRecord, bool, error)
	GetChangeRecord(recordID string) (store.ChangeRecord, bool, error)
	UpdateChangeRecord(record store.ChangeRecord) (store.ChangeRecord, error)
	SetArtifactSafetyClass(artifactID, safetyClass string) error
	CreateAuditEvent(event store.AuditEvent) error
}

// ChangeRecordRequest is the create/update payload.
type ChangeRecordRequest struct {
	SafetyClass   string `json:"safetyClass"`
	ImpactSummary string `json:"impactSummary"`
	RiskControls  string `json:"riskControls"`
}

type ChangeRecordResponse struct {
	RecordID         string     `json:"recordId"`
	ArtifactID       string     `json:"artifactId"`
	SafetyClass      string     `json:"safetyClass"`
	ImpactSummary    string     `json:"impactSummary"`
	RiskControls     string     `json:"riskControls"`
	Status           string     `json:"status"`
	CreatedByUserID  string     `json:"createdByUserId"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	ApprovedByUserID string     `json:"approvedByUserId,omitempty"`
	ApprovedAt       *time.Time `json:"approvedAt,omitempty"`
	RejectedByUserID string     `json:"rejectedByUserId,omitempty"`
	RejectedAt       *time.Time `json:"rejectedAt,omitempty"`
	RejectedReason   string     `json:"rejectedReason,omitempty"`
}

func changeRecordToResponse(r store.ChangeRecord) ChangeRecordResponse {
	resp := ChangeRecordResponse{
		RecordID:         r.RecordID,
		ArtifactID:       r.ArtifactID,
		SafetyClass:      r.SafetyClass,
		ImpactSummary:    r.ImpactSummary,
		RiskControls:     r.RiskControls,
		Status:           r.Status,
		CreatedByUserID:  r.CreatedByUserID,
		UpdatedAt:        r.UpdatedAt,
		CreatedAt:        r.CreatedAt,
		ApprovedByUserID: r.ApprovedByUserID,
		RejectedByUserID: r.RejectedByUserID,
		RejectedReason:   r.RejectedReason,
	}
	if !r.ApprovedAt.IsZero() {
		t := r.ApprovedAt
		resp.ApprovedAt = &t
	}
	if !r.RejectedAt.IsZero() {
		t := r.RejectedAt
		resp.RejectedAt = &t
	}
	return resp
}

// UpsertChangeRecord handles POST /api/v1/medical/artifacts/{artifactId}/change-record.
// Creates a new record if none exists; updates if status is draft or rejected.
func UpsertChangeRecord(logger *log.Logger, st changeRecordStore, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		if artifactID == "" {
			http.Error(w, "artifactId required", http.StatusBadRequest)
			return
		}

		if _, ok, err := st.GetArtifact(artifactID); err != nil {
			logger.Printf("change_record upsert get artifact error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}

		var req ChangeRecordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.SafetyClass != "ClassA" && req.SafetyClass != "ClassB" && req.SafetyClass != "ClassC" {
			http.Error(w, "safetyClass must be ClassA, ClassB, or ClassC", http.StatusBadRequest)
			return
		}

		actorID := changeRecordActorID(r)

		existing, exists, err := st.GetChangeRecordForArtifact(artifactID)
		if err != nil {
			logger.Printf("change_record upsert lookup error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		var result store.ChangeRecord
		if !exists {
			result, err = st.CreateChangeRecord(store.ChangeRecord{
				RecordID:        uuid.NewString(),
				ArtifactID:      artifactID,
				SafetyClass:     req.SafetyClass,
				ImpactSummary:   req.ImpactSummary,
				RiskControls:    req.RiskControls,
				Status:          "draft",
				CreatedByUserID: actorID,
			})
		} else {
			if existing.Status != "draft" && existing.Status != "rejected" {
				http.Error(w, "change record can only be edited in draft or rejected status", http.StatusConflict)
				return
			}
			existing.SafetyClass = req.SafetyClass
			existing.ImpactSummary = req.ImpactSummary
			existing.RiskControls = req.RiskControls
			existing.Status = "draft" // re-open rejected back to draft on edit
			result, err = st.UpdateChangeRecord(existing)
		}
		if err != nil {
			logger.Printf("change_record upsert store error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		// The change record is the classification authority: the deployment gate
		// and QMS package read the class from the artifact row, so keep it in sync.
		if err := st.SetArtifactSafetyClass(artifactID, req.SafetyClass); err != nil {
			logger.Printf("change_record upsert set artifact safety class error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		action := "change_record.created"
		if exists {
			action = "change_record.updated"
		}
		event := buildAuditEvent(r, trustProxy, actorUser(actorID), action, "change_record", result.RecordID)
		event.AfterJSON = auditJSON(map[string]any{"artifactId": artifactID, "safetyClass": req.SafetyClass, "status": result.Status})
		writeChangeRecordAudit(logger, st, event)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changeRecordToResponse(result))
	}
}

// GetChangeRecord handles GET /api/v1/medical/artifacts/{artifactId}/change-record.
func GetChangeRecord(logger *log.Logger, st changeRecordStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		if artifactID == "" {
			http.Error(w, "artifactId required", http.StatusBadRequest)
			return
		}
		rec, ok, err := st.GetChangeRecordForArtifact(artifactID)
		if err != nil {
			logger.Printf("change_record get error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "change record not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changeRecordToResponse(rec))
	}
}

// SubmitChangeRecord handles POST /api/v1/medical/artifacts/{artifactId}/change-record/submit.
// Transitions: draft → pending_approval.
func SubmitChangeRecord(logger *log.Logger, st changeRecordStore, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		rec, ok, err := st.GetChangeRecordForArtifact(artifactID)
		if err != nil {
			logger.Printf("change_record submit lookup error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "change record not found", http.StatusNotFound)
			return
		}
		if rec.Status != "draft" {
			http.Error(w, "change record must be in draft status to submit", http.StatusConflict)
			return
		}
		if rec.ImpactSummary == "" || rec.RiskControls == "" {
			http.Error(w, "impactSummary and riskControls required before submission", http.StatusUnprocessableEntity)
			return
		}
		rec.Status = "pending_approval"
		result, err := st.UpdateChangeRecord(rec)
		if err != nil {
			logger.Printf("change_record submit update error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser(changeRecordActorID(r)), "change_record.submitted", "change_record", rec.RecordID)
		event.AfterJSON = auditJSON(map[string]any{"artifactId": artifactID, "status": result.Status})
		writeChangeRecordAudit(logger, st, event)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changeRecordToResponse(result))
	}
}

// ApproveChangeRecord handles POST /api/v1/medical/artifacts/{artifactId}/change-record/approve.
// Transitions: pending_approval → approved.
func ApproveChangeRecord(logger *log.Logger, st changeRecordStore, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		rec, ok, err := st.GetChangeRecordForArtifact(artifactID)
		if err != nil {
			logger.Printf("change_record approve lookup error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "change record not found", http.StatusNotFound)
			return
		}
		if rec.Status != "pending_approval" {
			http.Error(w, "change record must be pending_approval to approve", http.StatusConflict)
			return
		}
		approverID := changeRecordActorID(r)
		now := time.Now().UTC()
		rec.Status = "approved"
		rec.ApprovedByUserID = approverID
		rec.ApprovedAt = now
		result, err := st.UpdateChangeRecord(rec)
		if err != nil {
			logger.Printf("change_record approve update error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser(approverID), "change_record.approved", "change_record", rec.RecordID)
		event.AfterJSON = auditJSON(map[string]any{"artifactId": artifactID, "approvedBy": approverID})
		writeChangeRecordAudit(logger, st, event)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changeRecordToResponse(result))
	}
}

// RejectChangeRecordRequest carries the required rejection reason.
type RejectChangeRecordRequest struct {
	Reason string `json:"reason"`
}

// RejectChangeRecord handles POST /api/v1/medical/artifacts/{artifactId}/change-record/reject.
// Transitions: pending_approval → rejected.
func RejectChangeRecord(logger *log.Logger, st changeRecordStore, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		rec, ok, err := st.GetChangeRecordForArtifact(artifactID)
		if err != nil {
			logger.Printf("change_record reject lookup error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "change record not found", http.StatusNotFound)
			return
		}
		if rec.Status != "pending_approval" {
			http.Error(w, "change record must be pending_approval to reject", http.StatusConflict)
			return
		}
		var req RejectChangeRecordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Reason == "" {
			http.Error(w, "reason required", http.StatusUnprocessableEntity)
			return
		}
		rejecterID := changeRecordActorID(r)
		now := time.Now().UTC()
		rec.Status = "rejected"
		rec.RejectedByUserID = rejecterID
		rec.RejectedAt = now
		rec.RejectedReason = req.Reason
		result, err := st.UpdateChangeRecord(rec)
		if err != nil {
			logger.Printf("change_record reject update error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser(rejecterID), "change_record.rejected", "change_record", rec.RecordID)
		event.AfterJSON = auditJSON(map[string]any{"artifactId": artifactID, "rejectedBy": rejecterID, "reason": req.Reason})
		writeChangeRecordAudit(logger, st, event)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(changeRecordToResponse(result))
	}
}

// auditCreator is satisfied by any narrow store interface that has CreateAuditEvent.
type auditCreator interface {
	CreateAuditEvent(event store.AuditEvent) error
}

// writeChangeRecordAudit writes an audit event using any narrow store with CreateAuditEvent.
// writeAudit requires the full store.Store; this wrapper avoids widening the interface.
func writeChangeRecordAudit(logger *log.Logger, st auditCreator, event store.AuditEvent) {
	if err := st.CreateAuditEvent(event); err != nil && logger != nil {
		logger.Printf("audit write error: %v", err)
	}
}

// changeRecordActorID extracts the authenticated user or service token ID.
func changeRecordActorID(r *http.Request) string {
	if u, ok := auth.UserFromContext(r.Context()); ok {
		return u.UserID
	}
	if t, ok := auth.ServiceTokenFromContext(r.Context()); ok {
		return "token:" + t.TokenID
	}
	return "unknown"
}
