package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/globalplane"
	gpsync "github.com/parcel/control-plane/internal/globalplane/sync"
)

type federationStore interface {
	ListRegionalPlanes() ([]globalplane.RegionalPlane, error)
	CreateGlobalArtifact(a globalplane.GlobalArtifact) error
	GetGlobalArtifact(artifactID string) (globalplane.GlobalArtifact, bool, error)
	ListGlobalArtifacts(nameFilter string) ([]globalplane.GlobalArtifact, error)
	CreateReplicationStatusRows(artifactID string, planeIDs []string) error
	UpdateReplicationStatusMetadataPush(artifactID, planeID string, pushedAt *time.Time, pushErr string) error
	ListReplicationStatus(artifactID string) ([]globalplane.ArtifactReplicationStatus, error)
}

type objectPutter interface {
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (int64, error)
	PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error)
	EnsureBucket(ctx context.Context, bucket string) error
}

// UploadFederatedArtifact handles POST /api/v1/federation/artifacts/upload
// Accepts multipart/form-data: file + JSON metadata fields.
func UploadFederatedArtifact(
	st federationStore,
	objStore objectPutter,
	bucket string,
	presignExpires time.Duration,
	publicBaseURL string,
	encKey []byte,
	logger *log.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if objStore == nil {
			http.Error(w, "artifact storage not configured (GLOBAL_MINIO_ENDPOINT required)", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseMultipartForm(512 << 20); err != nil { // 512 MB
			http.Error(w, "parse multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}
		name := r.FormValue("name")
		version := r.FormValue("version")
		artifactType := r.FormValue("type")
		if name == "" || version == "" || artifactType == "" {
			http.Error(w, "name, version, and type are required", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file field required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Buffer and compute SHA-256 while uploading.
		pr, pw := io.Pipe()
		hash := sha256.New()
		var sizeBytes int64
		errCh := make(chan error, 1)
		artifactID := uuid.NewString()
		objectKey := fmt.Sprintf("artifacts/%s.tar.gz", artifactID)

		go func() {
			defer pw.Close()
			buf := make([]byte, 32*1024)
			for {
				n, readErr := file.Read(buf)
				if n > 0 {
					chunk := buf[:n]
					hash.Write(chunk)
					sizeBytes += int64(n)
					if _, werr := pw.Write(chunk); werr != nil {
						errCh <- werr
						return
					}
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					errCh <- readErr
					return
				}
			}
			errCh <- nil
		}()

		// Ensure bucket exists (idempotent).
		if err := objStore.EnsureBucket(r.Context(), bucket); err != nil {
			http.Error(w, "storage unavailable", http.StatusInternalServerError)
			logger.Printf("[global-plane] ensure bucket: %v", err)
			return
		}

		if _, err := objStore.PutObject(r.Context(), bucket, objectKey, pr, -1, "application/octet-stream"); err != nil {
			http.Error(w, "upload failed", http.StatusInternalServerError)
			logger.Printf("[global-plane] put object: %v", err)
			return
		}
		if err := <-errCh; err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		sha256hex := hex.EncodeToString(hash.Sum(nil))

		// Persist global artifact record.
		artifact := globalplane.GlobalArtifact{
			ArtifactID:   artifactID,
			Name:         name,
			Version:      version,
			ArtifactType: artifactType,
			Status:       "active",
			ObjectKey:    objectKey,
			SHA256:       sha256hex,
			SizeBytes:    sizeBytes,
		}
		if err := st.CreateGlobalArtifact(artifact); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			logger.Printf("[global-plane] create global artifact: %v", err)
			return
		}

		// Fan-out metadata push to all enabled regional planes.
		planes, err := st.ListRegionalPlanes()
		if err != nil {
			logger.Printf("[global-plane] list planes for replication: %v", err)
		}

		var planeIDs []string
		for _, p := range planes {
			if p.Enabled {
				planeIDs = append(planeIDs, p.PlaneID)
			}
		}
		if err := st.CreateReplicationStatusRows(artifactID, planeIDs); err != nil {
			logger.Printf("[global-plane] create replication rows: %v", err)
		}

		// Push metadata asynchronously per plane.
		for _, p := range planes {
			if !p.Enabled {
				continue
			}
			go pushMetadataToPlane(p, artifact, publicBaseURL, objectKey, encKey, st, logger)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"artifactId":    artifactID,
			"name":          name,
			"version":       version,
			"sha256":        sha256hex,
			"sizeBytes":     sizeBytes,
			"totalRegions":  len(planeIDs),
		})
	}
}

func pushMetadataToPlane(
	plane globalplane.RegionalPlane,
	artifact globalplane.GlobalArtifact,
	publicBaseURL, objectKey string,
	encKey []byte,
	st federationStore,
	logger *log.Logger,
) {
	token, err := globalplane.DecryptToken(plane.EncryptedToken, encKey)
	if err != nil {
		logger.Printf("[global-plane] push to %s: decrypt token: %v", plane.Name, err)
		errStr := err.Error()
		_ = st.UpdateReplicationStatusMetadataPush(artifact.ArtifactID, plane.PlaneID, nil, errStr)
		return
	}
	client, err := gpsync.NewRegionalClient(plane.BaseURL, token, plane.TLSCAPem)
	if err != nil {
		logger.Printf("[global-plane] push to %s: build client: %v", plane.Name, err)
		_ = st.UpdateReplicationStatusMetadataPush(artifact.ArtifactID, plane.PlaneID, nil, err.Error())
		return
	}
	payload := gpsync.FederationArtifactPayload{
		ArtifactID:           artifact.ArtifactID,
		Name:                 artifact.Name,
		Version:              artifact.Version,
		Type:                 artifact.ArtifactType,
		ObjectKey:            objectKey,
		SHA256:               artifact.SHA256,
		SizeBytes:            artifact.SizeBytes,
		GlobalPresignBaseURL: publicBaseURL,
		GlobalObjectKey:      objectKey,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.PushArtifactMetadata(ctx, payload); err != nil {
		logger.Printf("[global-plane] push to %s: %v", plane.Name, err)
		_ = st.UpdateReplicationStatusMetadataPush(artifact.ArtifactID, plane.PlaneID, nil, err.Error())
		return
	}
	now := time.Now()
	_ = st.UpdateReplicationStatusMetadataPush(artifact.ArtifactID, plane.PlaneID, &now, "")
	logger.Printf("[global-plane] pushed artifact %s metadata to plane %s", artifact.ArtifactID, plane.Name)
}

// ListFederatedArtifacts handles GET /api/v1/federation/artifacts
func ListFederatedArtifacts(st federationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nameFilter := r.URL.Query().Get("name")
		arts, err := st.ListGlobalArtifacts(nameFilter)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		// Enrich with replication summary counts.
		type artifactResponse struct {
			globalplane.GlobalArtifact
			ConfirmedRegions int `json:"confirmedRegions"`
			TotalRegions     int `json:"totalRegions"`
		}
		resp := make([]artifactResponse, len(arts))
		for i, a := range arts {
			rows, _ := st.ListReplicationStatus(a.ArtifactID)
			confirmed := 0
			for _, r := range rows {
				if r.BlobStatus == "confirmed" {
					confirmed++
				}
			}
			resp[i] = artifactResponse{GlobalArtifact: a, ConfirmedRegions: confirmed, TotalRegions: len(rows)}
		}
		if resp == nil {
			resp = []artifactResponse{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// GetFederatedArtifact handles GET /api/v1/federation/artifacts/{artifactId}
func GetFederatedArtifact(st federationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		a, ok, err := st.GetGlobalArtifact(artifactID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a)
	}
}

// GetReplicationStatus handles GET /api/v1/federation/artifacts/{artifactId}/replication-status
func GetReplicationStatus(st federationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		rows, err := st.ListReplicationStatus(artifactID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		confirmed := 0
		for _, row := range rows {
			if row.BlobStatus == "confirmed" {
				confirmed++
			}
		}
		if rows == nil {
			rows = []globalplane.ArtifactReplicationStatus{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"artifactId":       artifactID,
			"regions":          rows,
			"confirmedRegions": confirmed,
			"totalRegions":     len(rows),
		})
	}
}

// PresignFederatedArtifact handles GET /api/v1/federation/artifacts/{artifactId}/presign
// Returns a short-lived presigned URL for downloading the artifact from global MinIO.
func PresignFederatedArtifact(st federationStore, objStore objectPutter, bucket string, expires time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if objStore == nil {
			http.Error(w, "artifact storage not configured", http.StatusServiceUnavailable)
			return
		}
		artifactID := chi.URLParam(r, "artifactId")
		a, ok, err := st.GetGlobalArtifact(artifactID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		url, err := objStore.PresignGet(r.Context(), bucket, a.ObjectKey, expires)
		if err != nil {
			http.Error(w, "presign failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}
