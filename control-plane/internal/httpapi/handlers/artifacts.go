package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
)

type CreateArtifactRequest struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	ObjectKey string `json:"objectKey"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
	SizeBytes int64  `json:"sizeBytes"`
}

type ArtifactResponse struct {
	ArtifactID string    `json:"artifactId"`
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	ObjectKey  string    `json:"objectKey"`
	SHA256     string    `json:"sha256"`
	Signature  string    `json:"signature,omitempty"`
	SizeBytes  int64     `json:"sizeBytes"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ArtifactListResponse struct {
	Items  []ArtifactResponse `json:"items"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

type PresignResponse struct {
	DownloadURL string    `json:"downloadUrl"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type ObjectStore interface {
	PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error)
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (int64, error)
	EnsureBucket(ctx context.Context, bucket string) error
}

type UploadArtifactResponse struct {
	ArtifactID string `json:"artifactId"`
	ObjectKey  string `json:"objectKey"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"sizeBytes"`
	Name       string `json:"name"`
	Version    string `json:"version"`
}

func CreateArtifact(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateArtifactRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Version == "" || req.ObjectKey == "" || req.SHA256 == "" || req.SizeBytes <= 0 {
			http.Error(w, "name, version, objectKey, sha256, sizeBytes required", http.StatusBadRequest)
			return
		}

		artifact := store.Artifact{
			ArtifactID: uuid.NewString(),
			Name:       req.Name,
			Version:    req.Version,
			ObjectKey:  req.ObjectKey,
			SHA256:     req.SHA256,
			Signature:  req.Signature,
			SizeBytes:  req.SizeBytes,
			CreatedAt:  time.Now().UTC(),
		}
		if err := st.CreateArtifact(artifact); err != nil {
			logger.Printf("create artifact error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		resp := ArtifactResponse{
			ArtifactID: artifact.ArtifactID,
			Name:       artifact.Name,
			Version:    artifact.Version,
			ObjectKey:  artifact.ObjectKey,
			SHA256:     artifact.SHA256,
			Signature:  artifact.Signature,
			SizeBytes:  artifact.SizeBytes,
			CreatedAt:  artifact.CreatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func UploadArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if objStore == nil || bucket == "" {
			http.Error(w, "object store not configured", http.StatusInternalServerError)
			return
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			http.Error(w, "invalid multipart", http.StatusBadRequest)
			return
		}
		name := r.FormValue("name")
		version := r.FormValue("version")
		if name == "" || version == "" {
			http.Error(w, "name and version required", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		artifactID := uuid.NewString()
		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".tar.gz"
		}
		objectKey := "artifacts/" + artifactID + ext

		if err := objStore.EnsureBucket(r.Context(), bucket); err != nil {
			logger.Printf("ensure bucket error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		h := sha256.New()
		tee := io.TeeReader(file, h)
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/gzip"
		}

		size, err := objStore.PutObject(r.Context(), bucket, objectKey, tee, header.Size, contentType)
		if err != nil {
			logger.Printf("put object error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		sha := hex.EncodeToString(h.Sum(nil))
		artifact := store.Artifact{
			ArtifactID: artifactID,
			Name:       name,
			Version:    version,
			ObjectKey:  objectKey,
			SHA256:     sha,
			SizeBytes:  size,
			CreatedAt:  time.Now().UTC(),
		}
		if err := st.CreateArtifact(artifact); err != nil {
			logger.Printf("create artifact error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		resp := UploadArtifactResponse{
			ArtifactID: artifactID,
			ObjectKey:  objectKey,
			SHA256:     sha,
			SizeBytes:  size,
			Name:       name,
			Version:    version,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func ListArtifacts(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		version := r.URL.Query().Get("version")
		limit := parseInt(r.URL.Query().Get("limit"), 100)
		offset := parseInt(r.URL.Query().Get("offset"), 0)

		items, err := st.ListArtifacts(name, version, limit, offset)
		if err != nil {
			logger.Printf("list artifacts error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		resp := ArtifactListResponse{Items: make([]ArtifactResponse, 0, len(items)), Limit: limit, Offset: offset}
		for _, a := range items {
			resp.Items = append(resp.Items, ArtifactResponse{
				ArtifactID: a.ArtifactID,
				Name:       a.Name,
				Version:    a.Version,
				ObjectKey:  a.ObjectKey,
				SHA256:     a.SHA256,
				Signature:  a.Signature,
				SizeBytes:  a.SizeBytes,
				CreatedAt:  a.CreatedAt,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func GetArtifact(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		if artifactID == "" {
			http.Error(w, "artifactId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(artifactID); err != nil {
			http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
			return
		}

		artifact, ok, err := st.GetArtifact(artifactID)
		if err != nil {
			logger.Printf("get artifact error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		resp := ArtifactResponse{
			ArtifactID: artifact.ArtifactID,
			Name:       artifact.Name,
			Version:    artifact.Version,
			ObjectKey:  artifact.ObjectKey,
			SHA256:     artifact.SHA256,
			Signature:  artifact.Signature,
			SizeBytes:  artifact.SizeBytes,
			CreatedAt:  artifact.CreatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func PresignArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, expires time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		if artifactID == "" {
			http.Error(w, "artifactId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(artifactID); err != nil {
			http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
			return
		}
		if objStore == nil || bucket == "" {
			http.Error(w, "object store not configured", http.StatusInternalServerError)
			return
		}

		artifact, ok, err := st.GetArtifact(artifactID)
		if err != nil {
			logger.Printf("get artifact error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		exp := expires
		if exp <= 0 {
			exp = 15 * time.Minute
		}
		url, err := objStore.PresignGet(r.Context(), bucket, artifact.ObjectKey, exp)
		if err != nil {
			logger.Printf("presign error: %v", err)
			http.Error(w, "presign error", http.StatusInternalServerError)
			return
		}

		resp := PresignResponse{DownloadURL: url, ExpiresAt: time.Now().UTC().Add(exp)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// parseInt lives in devices.go
