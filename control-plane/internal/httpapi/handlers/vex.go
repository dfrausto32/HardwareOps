package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
)

// vexHandlerStore is the narrow interface VEX handlers need.
type vexHandlerStore interface {
	GetArtifact(artifactID string) (store.Artifact, bool, error)
	UpsertVexAssertion(assertion store.VexAssertion) (store.VexAssertion, error)
	ListVexAssertions(artifactID string) ([]store.VexAssertion, error)
	CreateAuditEvent(event store.AuditEvent) error
}

// vexPresignStore extends vexHandlerStore with object-store presigning.
type vexPresignStore interface {
	vexHandlerStore
}

type vexObjectStore interface {
	PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error)
}

// VexAssertionRequest is the upsert payload for a manual exploitability assertion.
type VexAssertionRequest struct {
	CVEID         string `json:"cveId"`
	ComponentName string `json:"componentName"`
	// Assertion is one of: affected | not_affected | under_investigation | fixed
	Assertion     string `json:"assertion"`
	Justification string `json:"justification"`
}

// VexAssertionResponse is the API representation of a VexAssertion.
type VexAssertionResponse struct {
	AssertionID   string    `json:"assertionId"`
	ArtifactID    string    `json:"artifactId"`
	CVEID         string    `json:"cveId"`
	ComponentName string    `json:"componentName"`
	Assertion     string    `json:"assertion"`
	Justification string    `json:"justification"`
	ActorUserID   string    `json:"actorUserId"`
	CreatedAt     time.Time `json:"createdAt"`
}

var validAssertions = map[string]struct{}{
	"affected":            {},
	"not_affected":        {},
	"under_investigation": {},
	"fixed":               {},
}

// PostVexPresign handles POST /api/v1/medical/artifacts/{artifactId}/sbom/vex/presign.
// Returns a presigned download URL for the artifact's VEX document.
func PostVexPresign(logger *log.Logger, st vexPresignStore, objStore vexObjectStore, bucket string, presignExpires time.Duration, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		artifact, ok, err := st.GetArtifact(artifactID)
		if err != nil {
			logger.Printf("vex presign get artifact %s: %v", artifactID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}
		if artifact.VexObjectKey == "" {
			http.Error(w, "VEX document not yet generated", http.StatusNotFound)
			return
		}

		url, err := objStore.PresignGet(r.Context(), bucket, artifact.VexObjectKey, presignExpires)
		if err != nil {
			logger.Printf("vex presign sign %s: %v", artifact.VexObjectKey, err)
			http.Error(w, "presign error", http.StatusInternalServerError)
			return
		}

		actorID := changeRecordActorID(r)
		event := buildAuditEvent(r, trustProxy, actorUser(actorID), "vex.presign", "artifact", artifactID)
		event.AfterJSON = auditJSON(map[string]any{"vexObjectKey": artifact.VexObjectKey})
		writeChangeRecordAudit(logger, st, event)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

// UpsertVexAssertion handles POST /api/v1/medical/artifacts/{artifactId}/sbom/vex/assertions.
// Manually sets or overrides an exploitability assertion for a specific CVE/component pair.
func UpsertVexAssertion(logger *log.Logger, st vexHandlerStore, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		if _, ok, err := st.GetArtifact(artifactID); err != nil {
			logger.Printf("vex assertion get artifact %s: %v", artifactID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}

		var req VexAssertionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.CVEID == "" {
			http.Error(w, "cveId required", http.StatusBadRequest)
			return
		}
		if _, ok := validAssertions[req.Assertion]; !ok {
			http.Error(w, "assertion must be one of: affected, not_affected, under_investigation, fixed", http.StatusBadRequest)
			return
		}

		actorID := changeRecordActorID(r)
		result, err := st.UpsertVexAssertion(store.VexAssertion{
			AssertionID:   uuid.NewString(),
			ArtifactID:    artifactID,
			CVEID:         req.CVEID,
			ComponentName: req.ComponentName,
			Assertion:     req.Assertion,
			Justification: req.Justification,
			ActorUserID:   actorID,
		})
		if err != nil {
			logger.Printf("vex assertion upsert %s: %v", artifactID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser(actorID), "vex.assertion.upserted", "artifact", artifactID)
		event.AfterJSON = auditJSON(map[string]any{
			"cveId":         req.CVEID,
			"componentName": req.ComponentName,
			"assertion":     req.Assertion,
			"justification": req.Justification,
		})
		writeChangeRecordAudit(logger, st, event)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(VexAssertionResponse{
			AssertionID:   result.AssertionID,
			ArtifactID:    result.ArtifactID,
			CVEID:         result.CVEID,
			ComponentName: result.ComponentName,
			Assertion:     result.Assertion,
			Justification: result.Justification,
			ActorUserID:   result.ActorUserID,
			CreatedAt:     result.CreatedAt,
		})
	}
}
