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

type federationIngestStore interface {
	UpsertFederationIngest(ingest store.FederationIngest) error
	GetFederationIngest(artifactID string) (store.FederationIngest, bool, error)
	ConfirmFederationBlobLocal(artifactID string, confirmedAt time.Time) error
}

type federationObjectStore interface {
	StatObject(ctx context.Context, bucket, key string) (int64, error)
}

// federationPayload is the shape pushed by the global plane.
type federationPayload struct {
	ArtifactID           string `json:"artifactId"`
	GlobalObjectKey      string `json:"globalObjectKey"`
	GlobalPresignBaseURL string `json:"globalPresignBaseUrl"`
}

// ReceiveFederatedArtifact handles POST /api/v1/federation/artifacts.
// Called by the global plane to push artifact metadata to this regional plane.
func ReceiveFederatedArtifact(st federationIngestStore, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload federationPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if payload.ArtifactID == "" {
			http.Error(w, "artifactId required", http.StatusBadRequest)
			return
		}

		ingest := store.FederationIngest{
			IngestID:             uuid.NewString(),
			ArtifactID:           payload.ArtifactID,
			GlobalObjectKey:      payload.GlobalObjectKey,
			GlobalPresignBaseURL: payload.GlobalPresignBaseURL,
		}
		if err := st.UpsertFederationIngest(ingest); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[federation] upsert ingest for %s: %v", payload.ArtifactID, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"artifactId": payload.ArtifactID})
	}
}

// GetFederatedBlobStatus handles GET /api/v1/federation/artifacts/{artifactId}/blob-status.
// Called by the global plane's replication reconciler to check local blob presence.
func GetFederatedBlobStatus(st federationIngestStore, objStore federationObjectStore, bucket string, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		fi, ok, err := st.GetFederationIngest(artifactID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		confirmed := fi.BlobConfirmed
		if !confirmed && fi.GlobalObjectKey != "" && objStore != nil {
			// Check if the blob has been replicated to local MinIO.
			_, statErr := objStore.StatObject(r.Context(), bucket, fi.GlobalObjectKey)
			if statErr == nil {
				confirmed = true
				_ = st.ConfirmFederationBlobLocal(artifactID, time.Now().UTC())
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"confirmed": confirmed})
	}
}
