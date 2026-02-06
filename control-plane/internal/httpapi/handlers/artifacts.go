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
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
)

type CreateArtifactRequest struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Type      string `json:"type"`
	ObjectKey string `json:"objectKey"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
	SignatureKeyID string `json:"signatureKeyId"`
	SizeBytes int64  `json:"sizeBytes"`
	Metadata  json.RawMessage `json:"metadata"`
}

type ArtifactResponse struct {
	ArtifactID string    `json:"artifactId"`
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	Type       string    `json:"type"`
	ObjectKey  string    `json:"objectKey"`
	SHA256     string    `json:"sha256"`
	Signature  string    `json:"signature,omitempty"`
	SizeBytes  int64     `json:"sizeBytes"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
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
	DeleteObject(ctx context.Context, bucket, key string) error
}

type UploadArtifactResponse struct {
	ArtifactID string `json:"artifactId"`
	ObjectKey  string `json:"objectKey"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"sizeBytes"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Type       string `json:"type"`
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
		atype, err := normalizeArtifactType(req.Type)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := normalizeMetadata(req.Metadata)
		if err != nil {
			http.Error(w, "metadata must be valid json", http.StatusBadRequest)
			return
		}
		meta, err = mergeSignatureKeyID(meta, req.SignatureKeyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		artifact := store.Artifact{
			ArtifactID: uuid.NewString(),
			Name:       req.Name,
			Version:    req.Version,
			Type:       atype,
			ObjectKey:  req.ObjectKey,
			SHA256:     req.SHA256,
			Signature:  req.Signature,
			SizeBytes:  req.SizeBytes,
			MetadataJSON: meta,
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
			Type:       artifact.Type,
			ObjectKey:  artifact.ObjectKey,
			SHA256:     artifact.SHA256,
			Signature:  artifact.Signature,
			SizeBytes:  artifact.SizeBytes,
			Metadata:   json.RawMessage(artifact.MetadataJSON),
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
		atypeRaw := r.FormValue("type")
		metaRaw := r.FormValue("metadata")
		if name == "" || version == "" {
			http.Error(w, "name and version required", http.StatusBadRequest)
			return
		}
		atype, err := normalizeArtifactType(atypeRaw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := parseMetadataString(metaRaw)
		if err != nil {
			http.Error(w, "metadata must be valid json", http.StatusBadRequest)
			return
		}
		signature := strings.TrimSpace(r.FormValue("signature"))
		signatureKeyID := strings.TrimSpace(r.FormValue("signatureKeyId"))
		meta, err = mergeSignatureKeyID(meta, signatureKeyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			Type:       atype,
			ObjectKey:  objectKey,
			SHA256:     sha,
			Signature:  signature,
			SizeBytes:  size,
			MetadataJSON: meta,
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
			Type:       atype,
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
				Type:       a.Type,
				ObjectKey:  a.ObjectKey,
				SHA256:     a.SHA256,
				Signature:  a.Signature,
				SizeBytes:  a.SizeBytes,
				Metadata:   json.RawMessage(a.MetadataJSON),
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
			Type:       artifact.Type,
			ObjectKey:  artifact.ObjectKey,
			SHA256:     artifact.SHA256,
			Signature:  artifact.Signature,
			SizeBytes:  artifact.SizeBytes,
			Metadata:   json.RawMessage(artifact.MetadataJSON),
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

func DeleteArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string) http.HandlerFunc {
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
		} else if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if objStore != nil && bucket != "" && artifact.ObjectKey != "" {
			if err := objStore.DeleteObject(r.Context(), bucket, artifact.ObjectKey); err != nil {
				logger.Printf("delete artifact object error: %v", err)
				http.Error(w, "object delete error", http.StatusInternalServerError)
				return
			}
		}

		if err := st.DeleteArtifact(artifactID); err != nil {
			logger.Printf("delete artifact error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// parseInt lives in devices.go

var allowedArtifactTypes = map[string]struct{}{
	"app_bundle":     {},
	"config_bundle":  {},
	"data_bundle":    {},
	"firmware":       {},
	"container_image": {},
}

func normalizeArtifactType(val string) (string, error) {
	atype := strings.TrimSpace(strings.ToLower(val))
	if atype == "" {
		return "app_bundle", nil
	}
	if _, ok := allowedArtifactTypes[atype]; !ok {
		return "", fmt.Errorf("invalid type: %s", atype)
	}
	return atype, nil
}

func normalizeMetadata(val json.RawMessage) ([]byte, error) {
	if len(val) == 0 {
		return nil, nil
	}
	if !json.Valid(val) {
		return nil, fmt.Errorf("invalid metadata")
	}
	return val, nil
}

func parseMetadataString(val string) ([]byte, error) {
	if strings.TrimSpace(val) == "" {
		return nil, nil
	}
	raw := []byte(val)
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid metadata")
	}
	return raw, nil
}

func mergeSignatureKeyID(meta []byte, keyID string) ([]byte, error) {
	if strings.TrimSpace(keyID) == "" {
		return meta, nil
	}
	if len(meta) == 0 || string(meta) == "null" {
		out, _ := json.Marshal(map[string]any{"signatureKeyId": keyID})
		return out, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(meta, &obj); err != nil {
		return nil, fmt.Errorf("metadata must be a JSON object when signatureKeyId is provided")
	}
	if obj == nil {
		obj = map[string]any{}
	}
	obj["signatureKeyId"] = keyID
	out, _ := json.Marshal(obj)
	return out, nil
}
