package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/artifactingest"
	"github.com/hardwareops/control-plane/internal/artifacttrust"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/lifecycle"
	"github.com/hardwareops/control-plane/internal/metrics"
	"github.com/hardwareops/control-plane/internal/store"
)

// ArtifactVulnScanTrigger is a narrow interface so this package does not
// directly import the vulnscan package.
type ArtifactVulnScanTrigger interface {
	Trigger(artifactID, objectKey, sha256 string)
}

// shouldScanArtifact returns true when the artifact type is not in skipTypes.
func shouldScanArtifact(artifactType string, skipTypes []string) bool {
	for _, t := range skipTypes {
		if strings.EqualFold(t, artifactType) {
			return false
		}
	}
	return true
}

type CreateArtifactRequest struct {
	ArtifactID     string          `json:"artifactId"`
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	Type           string          `json:"type"`
	ObjectKey      string          `json:"objectKey"`
	SHA256         string          `json:"sha256"`
	Signature      string          `json:"signature"`
	SignatureType  string          `json:"signatureType,omitempty"`
	SignatureKeyID string          `json:"signatureKeyId"`
	SizeBytes      int64           `json:"sizeBytes"`
	Metadata       json.RawMessage `json:"metadata"`
}

type ArtifactResponse struct {
	ArtifactID     string          `json:"artifactId"`
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	Type           string          `json:"type"`
	Status         string          `json:"status"`
	ObjectKey      string          `json:"objectKey"`
	SHA256         string          `json:"sha256"`
	Signature      string          `json:"signature,omitempty"`
	SignatureType  string          `json:"signatureType,omitempty"`
	SignatureKeyID string          `json:"signatureKeyId,omitempty"`
	VerificationStatus string      `json:"verificationStatus,omitempty"`
	VerificationError  string      `json:"verificationError,omitempty"`
	VerifiedAt     *time.Time      `json:"verifiedAt,omitempty"`
	SizeBytes      int64           `json:"sizeBytes"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	DeprecatedAt   *time.Time      `json:"deprecatedAt,omitempty"`
	DeleteAfter    *time.Time      `json:"deleteAfter,omitempty"`
	ReferenceCount int             `json:"referenceCount"`
	Duplicate      bool            `json:"duplicate,omitempty"`
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
	PresignPut(ctx context.Context, bucket, key string, expires time.Duration, contentType string) (string, error)
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (int64, error)
	EnsureBucket(ctx context.Context, bucket string) error
	DeleteObject(ctx context.Context, bucket, key string) error
	StatObject(ctx context.Context, bucket, key string) (int64, error)
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

type UploadArtifactResponse struct {
	ArtifactID string `json:"artifactId"`
	ObjectKey  string `json:"objectKey"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"sizeBytes"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Type       string `json:"type"`
	Duplicate  bool   `json:"duplicate,omitempty"`
}

type PresignUploadRequest struct {
	Filename       string `json:"filename"`
	ContentType    string `json:"contentType"`
	ExpiresSeconds int    `json:"expiresSeconds"`
}

type PresignUploadResponse struct {
	ArtifactID string    `json:"artifactId"`
	ObjectKey  string    `json:"objectKey"`
	UploadURL  string    `json:"uploadUrl"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type PullArtifactRequest struct {
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	Type           string          `json:"type"`
	SourceURL      string          `json:"sourceUrl"`
	Source         *PullSourceSpec `json:"source,omitempty"`
	SHA256         string          `json:"sha256"`
	Signature      string          `json:"signature"`
	SignatureType  string          `json:"signatureType,omitempty"`
	SignatureKeyID string          `json:"signatureKeyId"`
	SizeBytes      int64           `json:"sizeBytes"`
	Metadata       json.RawMessage `json:"metadata"`
}

type PullSourceSpec struct {
	Kind          string `json:"kind"`
	URI           string `json:"uri"`
	CredentialRef string `json:"credentialRef"`
}

type releaseAutoTrigger interface {
	Trigger(reason string)
}

type DeprecateArtifactRequest struct {
	DeleteAfterDays int `json:"deleteAfterDays"`
}

type ArtifactLifecyclePolicyRequest struct {
	DeprecatedDeleteAfterDays int `json:"deprecatedDeleteAfterDays"`
}

type ArtifactLifecyclePolicyResponse struct {
	DeprecatedDeleteAfterDays int       `json:"deprecatedDeleteAfterDays"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}

type PruneArtifactsResponse struct {
	Trigger    string                   `json:"trigger,omitempty"`
	StartedAt  time.Time                `json:"startedAt,omitempty"`
	FinishedAt time.Time                `json:"finishedAt,omitempty"`
	CutoffUTC  time.Time                `json:"cutoffUtc"`
	Limit      int                      `json:"limit"`
	Deleted    []string                 `json:"deleted"`
	Skipped    []PruneArtifactSkipEntry `json:"skipped"`
	DeletedNum int                      `json:"deletedNum"`
	SkippedNum int                      `json:"skippedNum"`
	Error      string                   `json:"error,omitempty"`
}

type PruneArtifactSkipEntry struct {
	ArtifactID string `json:"artifactId"`
	Reason     string `json:"reason"`
}

type ArtifactLifecycleStatusResponse struct {
	Enabled                 bool                    `json:"enabled"`
	Running                 bool                    `json:"running"`
	IntervalSeconds         int64                   `json:"intervalSeconds"`
	BatchLimit              int                     `json:"batchLimit"`
	AlertReferenceThreshold int                     `json:"alertReferenceThreshold"`
	LastRun                 *PruneArtifactsResponse `json:"lastRun,omitempty"`
	Alerts                  []lifecycle.Alert       `json:"alerts,omitempty"`
}

func CreateArtifact(logger *log.Logger, st store.Store, trustProxy bool, sigPolicy ArtifactSignaturePolicy) http.HandlerFunc {
	return createArtifact(logger, st, nil, "", trustProxy, sigPolicy, nil, nil, nil, nil)
}

func CreateArtifactWithReleaseAuto(logger *log.Logger, st store.Store, trustProxy bool, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger) http.HandlerFunc {
	return createArtifact(logger, st, nil, "", trustProxy, sigPolicy, releaseAuto, nil, nil, nil)
}

func CreateArtifactWithObjectStore(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, sigPolicy ArtifactSignaturePolicy) http.HandlerFunc {
	return createArtifact(logger, st, objStore, bucket, trustProxy, sigPolicy, nil, nil, nil, nil)
}

func CreateArtifactWithReleaseAutoObjectStore(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger) http.HandlerFunc {
	return createArtifact(logger, st, objStore, bucket, trustProxy, sigPolicy, releaseAuto, nil, nil, nil)
}

func CreateArtifactWithRealtime(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger, hub *events.Hub, vulnScan ArtifactVulnScanTrigger, vulnSkipTypes []string) http.HandlerFunc {
	return createArtifact(logger, st, objStore, bucket, trustProxy, sigPolicy, releaseAuto, hub, vulnScan, vulnSkipTypes)
}

func createArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger, hub *events.Hub, vulnScan ArtifactVulnScanTrigger, vulnSkipTypes []string) http.HandlerFunc {
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
		signatureType, err := normalizeSignatureTypeForRequest(req.Signature, req.SignatureType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err = mergeSignatureFields(meta, req.SignatureKeyID, signatureType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		artifactID := strings.TrimSpace(req.ArtifactID)
		if artifactID == "" {
			artifactID = uuid.NewString()
		} else if _, err := uuid.Parse(artifactID); err != nil {
			http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
			return
		}

		verification := artifacttrust.VerificationResult{}
		actualSHA := req.SHA256
		sizeBytes := req.SizeBytes
		if objStore != nil && bucket != "" {
			verification, actualSHA, sizeBytes, err = verifyStoredArtifact(r.Context(), st, objStore, bucket, req.ObjectKey, req.SHA256, req.Signature, signatureType, req.SignatureKeyID, sigPolicy)
			if err != nil {
				logger.Printf("verify artifact error: %v", err)
				http.Error(w, "artifact verification failed", http.StatusBadRequest)
				return
			}
		} else {
			verification, err = fallbackCreateArtifactVerification(req.Signature, signatureType, req.SignatureKeyID, sigPolicy)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		artifact := store.Artifact{
			ArtifactID:          artifactID,
			Name:                req.Name,
			Version:             req.Version,
			Type:                atype,
			Status:              "active",
			ObjectKey:           req.ObjectKey,
			SHA256:              actualSHA,
			Signature:           req.Signature,
			SignatureType:       signatureType,
			SignatureKeyID:      strings.TrimSpace(req.SignatureKeyID),
			VerificationStatus:  verification.Status,
			VerificationError:   verification.Error,
			VerifiedAt:          verification.VerifiedAt,
			SizeBytes:           sizeBytes,
			MetadataJSON:        meta,
			CreatedAt:           time.Now().UTC(),
		}
		if existing, isDupe, isConflict := artifactDuplicateCheck(r, st, artifact.Name, artifact.Type, artifact.Version, artifact.SHA256); isConflict {
			http.Error(w, "artifact_version_conflict: an artifact with this name/type/version exists with different content; use ?supersede=true to register as a distinct artifact", http.StatusConflict)
			return
		} else if isDupe {
			resp := artifactToResponse(existing, 0)
			resp.Duplicate = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if err := st.CreateArtifact(artifact); err != nil {
			logger.Printf("create artifact error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.create", "artifact", artifact.ArtifactID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.create", "artifact", artifact.ArtifactID)
		event.AfterJSON = auditJSON(map[string]any{
			"artifactId":          artifact.ArtifactID,
			"name":                artifact.Name,
			"version":             artifact.Version,
			"type":                artifact.Type,
			"sha256":              artifact.SHA256,
			"sizeBytes":           artifact.SizeBytes,
			"verificationStatus":  artifact.VerificationStatus,
			"verificationError":   artifact.VerificationError,
			"signatureType":       artifact.SignatureType,
			"signatureKeyId":      artifact.SignatureKeyID,
		})
		writeAudit(logger, st, event, nil)
		emitArtifactRegisteredEvent(logger, st, hub, artifact, "create")
		if releaseAuto != nil {
			releaseAuto.Trigger("artifact_create")
		}
		if vulnScan != nil && shouldScanArtifact(artifact.Type, vulnSkipTypes) {
			vulnScan.Trigger(artifact.ArtifactID, artifact.ObjectKey, artifact.SHA256)
		}

		resp := artifactToResponse(artifact, 0)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func emitArtifactRegisteredEvent(logger *log.Logger, st store.Store, hub *events.Hub, artifact store.Artifact, source string) {
	emitRuntimeEvent(logger, st, hub, events.Event{
		Type: events.TypeArtifactRegistered,
		Payload: auditJSON(map[string]any{
			"artifactId": artifact.ArtifactID,
			"name":       artifact.Name,
			"version":    artifact.Version,
			"type":       artifact.Type,
			"status":     artifact.Status,
			"source":     source,
			"sizeBytes":  artifact.SizeBytes,
		}),
	})
}

func UploadArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy) http.HandlerFunc {
	return uploadArtifact(logger, st, objStore, bucket, trustProxy, metricsCollector, sigPolicy, nil, nil, nil, nil)
}

func UploadArtifactWithReleaseAuto(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger) http.HandlerFunc {
	return uploadArtifact(logger, st, objStore, bucket, trustProxy, metricsCollector, sigPolicy, releaseAuto, nil, nil, nil)
}

func UploadArtifactWithRealtime(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger, hub *events.Hub, vulnScan ArtifactVulnScanTrigger, vulnSkipTypes []string) http.HandlerFunc {
	return uploadArtifact(logger, st, objStore, bucket, trustProxy, metricsCollector, sigPolicy, releaseAuto, hub, vulnScan, vulnSkipTypes)
}

func uploadArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger, hub *events.Hub, vulnScan ArtifactVulnScanTrigger, vulnSkipTypes []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status string) {
			if metricsCollector != nil {
				metricsCollector.IncArtifactUpload(status)
			}
		}
		if objStore == nil || bucket == "" {
			record("error")
			http.Error(w, "object store not configured", http.StatusInternalServerError)
			return
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			record("error")
			http.Error(w, "invalid multipart", http.StatusBadRequest)
			return
		}
		name := r.FormValue("name")
		version := r.FormValue("version")
		atypeRaw := r.FormValue("type")
		metaRaw := r.FormValue("metadata")
		if name == "" || version == "" {
			record("error")
			http.Error(w, "name and version required", http.StatusBadRequest)
			return
		}
		atype, err := normalizeArtifactType(atypeRaw)
		if err != nil {
			record("error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := parseMetadataString(metaRaw)
		if err != nil {
			record("error")
			http.Error(w, "metadata must be valid json", http.StatusBadRequest)
			return
		}
		signature := strings.TrimSpace(r.FormValue("signature"))
		signatureTypeRaw := strings.TrimSpace(r.FormValue("signatureType"))
		signatureKeyID := strings.TrimSpace(r.FormValue("signatureKeyId"))
		signatureType, err := normalizeSignatureTypeForRequest(signature, signatureTypeRaw)
		if err != nil {
			record("error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err = mergeSignatureFields(meta, signatureKeyID, signatureType)
		if err != nil {
			record("error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			record("error")
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
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		h := sha256.New()
		tee := io.TeeReader(file, h)
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/gzip"
		}

		_, err = objStore.PutObject(r.Context(), bucket, objectKey, tee, header.Size, contentType)
		if err != nil {
			logger.Printf("put object error: %v", err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		sha := hex.EncodeToString(h.Sum(nil))
		verification, actualSHA, sizeBytes, err := verifyStoredArtifact(r.Context(), st, objStore, bucket, objectKey, sha, signature, signatureType, signatureKeyID, sigPolicy)
		recordArtifactVerificationMetrics(metricsCollector, "upload", verification)
		if err != nil {
			record("error")
			_ = objStore.DeleteObject(r.Context(), bucket, objectKey)
			logger.Printf("verify artifact error: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		artifact := store.Artifact{
			ArtifactID:          artifactID,
			Name:                name,
			Version:             version,
			Type:                atype,
			Status:              "active",
			ObjectKey:           objectKey,
			SHA256:              actualSHA,
			Signature:           signature,
			SignatureType:       signatureType,
			SignatureKeyID:      signatureKeyID,
			VerificationStatus:  verification.Status,
			VerificationError:   verification.Error,
			VerifiedAt:          verification.VerifiedAt,
			SizeBytes:           sizeBytes,
			MetadataJSON:        meta,
			CreatedAt:           time.Now().UTC(),
		}
		if existing, isDupe, isConflict := artifactDuplicateCheck(r, st, artifact.Name, artifact.Type, artifact.Version, artifact.SHA256); isConflict {
			record("error")
			_ = objStore.DeleteObject(r.Context(), bucket, objectKey)
			http.Error(w, "artifact_version_conflict: an artifact with this name/type/version exists with different content; use ?supersede=true to register as a distinct artifact", http.StatusConflict)
			return
		} else if isDupe {
			record("success")
			_ = objStore.DeleteObject(r.Context(), bucket, objectKey)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(UploadArtifactResponse{
				ArtifactID: existing.ArtifactID,
				ObjectKey:  existing.ObjectKey,
				SHA256:     existing.SHA256,
				SizeBytes:  existing.SizeBytes,
				Name:       existing.Name,
				Version:    existing.Version,
				Type:       existing.Type,
				Duplicate:  true,
			})
			return
		}

		if err := st.CreateArtifact(artifact); err != nil {
			logger.Printf("create artifact error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.upload", "artifact", artifactID), err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		record("success")
		refreshArtifactVerificationMetrics(st, metricsCollector)

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.upload", "artifact", artifactID)
		event.AfterJSON = auditJSON(map[string]any{
			"artifactId":         artifactID,
			"name":               name,
			"version":            version,
			"type":               atype,
			"sha256":             actualSHA,
			"sizeBytes":          sizeBytes,
			"verificationStatus": verification.Status,
			"verificationError":  verification.Error,
			"signatureType":      signatureType,
			"signatureKeyId":     signatureKeyID,
		})
		writeAudit(logger, st, event, nil)
		emitArtifactRegisteredEvent(logger, st, hub, artifact, "upload")
		if releaseAuto != nil {
			releaseAuto.Trigger("artifact_upload")
		}
		if vulnScan != nil && shouldScanArtifact(atype, vulnSkipTypes) {
			vulnScan.Trigger(artifactID, objectKey, actualSHA)
		}

		resp := UploadArtifactResponse{
			ArtifactID: artifactID,
			ObjectKey:  objectKey,
			SHA256:     actualSHA,
			SizeBytes:  sizeBytes,
			Name:       name,
			Version:    version,
			Type:       atype,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func PullArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, allowedHosts []string, maxBytes int64, timeout time.Duration, allowInsecureHTTP bool, credentialResolver artifactingest.CredentialResolver, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy) http.HandlerFunc {
	return pullArtifact(logger, st, objStore, bucket, allowedHosts, maxBytes, timeout, allowInsecureHTTP, credentialResolver, trustProxy, metricsCollector, sigPolicy, nil, nil, nil, nil)
}

func PullArtifactWithReleaseAuto(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, allowedHosts []string, maxBytes int64, timeout time.Duration, allowInsecureHTTP bool, credentialResolver artifactingest.CredentialResolver, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger) http.HandlerFunc {
	return pullArtifact(logger, st, objStore, bucket, allowedHosts, maxBytes, timeout, allowInsecureHTTP, credentialResolver, trustProxy, metricsCollector, sigPolicy, releaseAuto, nil, nil, nil)
}

func PullArtifactWithRealtime(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, allowedHosts []string, maxBytes int64, timeout time.Duration, allowInsecureHTTP bool, credentialResolver artifactingest.CredentialResolver, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger, hub *events.Hub, vulnScan ArtifactVulnScanTrigger, vulnSkipTypes []string) http.HandlerFunc {
	return pullArtifact(logger, st, objStore, bucket, allowedHosts, maxBytes, timeout, allowInsecureHTTP, credentialResolver, trustProxy, metricsCollector, sigPolicy, releaseAuto, hub, vulnScan, vulnSkipTypes)
}

func pullArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, allowedHosts []string, maxBytes int64, timeout time.Duration, allowInsecureHTTP bool, credentialResolver artifactingest.CredentialResolver, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger, hub *events.Hub, vulnScan ArtifactVulnScanTrigger, vulnSkipTypes []string) http.HandlerFunc {
	adapterRegistry := artifactingest.NewPullAdapterRegistry(
		artifactingest.NewHTTPPullAdapter(allowedHosts, timeout, allowInsecureHTTP),
		artifactingest.NewArtifactoryPullAdapter(allowedHosts, timeout, allowInsecureHTTP),
		artifactingest.NewS3PullAdapter(timeout),
		artifactingest.NewGCSPullAdapter(timeout),
	)
	if credentialResolver == nil {
		credentialResolver = artifactingest.NoopCredentialResolver{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status string) {
			if metricsCollector != nil {
				metricsCollector.IncArtifactUpload(status)
			}
		}
		if objStore == nil || bucket == "" {
			record("error")
			http.Error(w, "object store not configured", http.StatusInternalServerError)
			return
		}
		if maxBytes <= 0 {
			maxBytes = 1024 * 1024 * 1024 // 1 GiB default safety limit
		}

		var req PullArtifactRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			record("error")
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Version = strings.TrimSpace(req.Version)
		req.SourceURL = strings.TrimSpace(req.SourceURL)
		req.SHA256 = strings.TrimSpace(strings.ToLower(req.SHA256))
		if req.Name == "" || req.Version == "" || req.SHA256 == "" {
			record("error")
			http.Error(w, "name, version, sha256 required", http.StatusBadRequest)
			return
		}
		var sourceInput *artifactingest.PullSource
		if req.Source != nil {
			sourceInput = &artifactingest.PullSource{
				Kind:          req.Source.Kind,
				URI:           req.Source.URI,
				CredentialRef: req.Source.CredentialRef,
			}
		}
		sourceSpec, err := artifactingest.ResolvePullSource(sourceInput, req.SourceURL)
		if err != nil {
			record("error")
			http.Error(w, "invalid source specification", http.StatusBadRequest)
			return
		}
		adapter, ok := adapterRegistry.Get(sourceSpec.Kind)
		if !ok {
			record("error")
			http.Error(w, fmt.Sprintf("source kind %q not supported", sourceSpec.Kind), http.StatusBadRequest)
			return
		}
		parsedSourceURL, _ := url.Parse(sourceSpec.URI)

		atype, err := normalizeArtifactType(req.Type)
		if err != nil {
			record("error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := normalizeMetadata(req.Metadata)
		if err != nil {
			record("error")
			http.Error(w, "metadata must be valid json", http.StatusBadRequest)
			return
		}
		signatureType, err := normalizeSignatureTypeForRequest(req.Signature, req.SignatureType)
		if err != nil {
			record("error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err = mergeSignatureFields(meta, req.SignatureKeyID, signatureType)
		if err != nil {
			record("error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		credentials := artifactingest.Credentials{}
		if sourceSpec.CredentialRef != "" {
			resolved, err := credentialResolver.Resolve(r.Context(), sourceSpec.CredentialRef)
			if err != nil {
				record("error")
				if errors.Is(err, artifactingest.ErrCredentialNotFound) {
					http.Error(w, "credentialRef not found", http.StatusBadRequest)
					return
				}
				if errors.Is(err, artifactingest.ErrCredentialResolverUnavailable) {
					http.Error(w, "credentialRef unsupported", http.StatusBadRequest)
					return
				}
				logger.Printf("resolve pull credentials error: %v", err)
				http.Error(w, "credential resolution error", http.StatusInternalServerError)
				return
			}
			credentials = resolved
		}

		pulled, err := adapter.Pull(r.Context(), artifactingest.PullRequest{
			URI:           sourceSpec.URI,
			CredentialRef: sourceSpec.CredentialRef,
			Credentials:   credentials,
		})
		if err != nil {
			record("error")
			if errors.Is(err, artifactingest.ErrSourceNotAllowed) {
				http.Error(w, "sourceUrl host not allowed", http.StatusForbidden)
				return
			}
			if errors.Is(err, artifactingest.ErrInvalidSource) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			logger.Printf("pull artifact request error: %v", err)
			http.Error(w, "pull source error", http.StatusBadGateway)
			return
		}
		defer pulled.Body.Close()
		if pulled.ContentLength > 0 && pulled.ContentLength > maxBytes {
			record("error")
			http.Error(w, "artifact exceeds pull size limit", http.StatusBadRequest)
			return
		}

		tmpFile, err := os.CreateTemp("", "hardwareops-artifact-pull-*.tar.gz")
		if err != nil {
			logger.Printf("create temp artifact file error: %v", err)
			record("error")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)
		defer tmpFile.Close()

		h := sha256.New()
		limited := io.LimitReader(pulled.Body, maxBytes+1)
		written, err := io.Copy(io.MultiWriter(tmpFile, h), limited)
		if err != nil {
			logger.Printf("pull artifact copy error: %v", err)
			record("error")
			http.Error(w, "pull source read error", http.StatusBadGateway)
			return
		}
		if written > maxBytes {
			record("error")
			http.Error(w, "artifact exceeds pull size limit", http.StatusBadRequest)
			return
		}
		computedSHA := hex.EncodeToString(h.Sum(nil))
		if computedSHA != req.SHA256 {
			record("error")
			http.Error(w, "sha256 mismatch", http.StatusBadRequest)
			return
		}
		if req.SizeBytes > 0 && req.SizeBytes != written {
			record("error")
			http.Error(w, "size mismatch", http.StatusBadRequest)
			return
		}

		artifactID := uuid.NewString()
		ext := ""
		if parsedSourceURL != nil {
			ext = filepath.Ext(parsedSourceURL.Path)
		}
		if ext == "" {
			ext = ".tar.gz"
		}
		objectKey := "artifacts/" + artifactID + ext

		if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
			logger.Printf("seek temp artifact file error: %v", err)
			record("error")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := objStore.EnsureBucket(r.Context(), bucket); err != nil {
			logger.Printf("ensure bucket error: %v", err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		contentType := pulled.ContentType
		if contentType == "" {
			contentType = "application/gzip"
		}
		_, err = objStore.PutObject(r.Context(), bucket, objectKey, tmpFile, written, contentType)
		if err != nil {
			logger.Printf("put pulled object error: %v", err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		verification, actualSHA, actualSize, err := verifyStoredArtifact(r.Context(), st, objStore, bucket, objectKey, req.SHA256, req.Signature, signatureType, req.SignatureKeyID, sigPolicy)
		recordArtifactVerificationMetrics(metricsCollector, "pull", verification)
		if err != nil {
			record("error")
			_ = objStore.DeleteObject(r.Context(), bucket, objectKey)
			logger.Printf("verify artifact error: %v", err)
			http.Error(w, "artifact verification failed", http.StatusBadRequest)
			return
		}

		artifact := store.Artifact{
			ArtifactID:          artifactID,
			Name:                req.Name,
			Version:             req.Version,
			Type:                atype,
			Status:              "active",
			ObjectKey:           objectKey,
			SHA256:              actualSHA,
			Signature:           strings.TrimSpace(req.Signature),
			SignatureType:       signatureType,
			SignatureKeyID:      strings.TrimSpace(req.SignatureKeyID),
			VerificationStatus:  verification.Status,
			VerificationError:   verification.Error,
			VerifiedAt:          verification.VerifiedAt,
			SizeBytes:           actualSize,
			MetadataJSON:        meta,
			CreatedAt:           time.Now().UTC(),
		}
		if existing, isDupe, isConflict := artifactDuplicateCheck(r, st, artifact.Name, artifact.Type, artifact.Version, artifact.SHA256); isConflict {
			record("error")
			_ = objStore.DeleteObject(r.Context(), bucket, objectKey)
			http.Error(w, "artifact_version_conflict: an artifact with this name/type/version exists with different content; use ?supersede=true to register as a distinct artifact", http.StatusConflict)
			return
		} else if isDupe {
			record("success")
			_ = objStore.DeleteObject(r.Context(), bucket, objectKey)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(UploadArtifactResponse{
				ArtifactID: existing.ArtifactID,
				ObjectKey:  existing.ObjectKey,
				SHA256:     existing.SHA256,
				SizeBytes:  existing.SizeBytes,
				Name:       existing.Name,
				Version:    existing.Version,
				Type:       existing.Type,
				Duplicate:  true,
			})
			return
		}

		if err := st.CreateArtifact(artifact); err != nil {
			logger.Printf("create pulled artifact error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.pull", "artifact", artifactID), err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		record("success")
		refreshArtifactVerificationMetrics(st, metricsCollector)

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.pull", "artifact", artifactID)
		event.AfterJSON = auditJSON(map[string]any{
			"artifactId":         artifactID,
			"name":               req.Name,
			"version":            req.Version,
			"type":               atype,
			"objectKey":          objectKey,
			"sha256":             actualSHA,
			"sizeBytes":          actualSize,
			"sourceKind":         sourceSpec.Kind,
			"source":             scrubSourceURL(sourceSpec.URI),
			"credentialRef":      sourceSpec.CredentialRef,
			"verificationStatus": verification.Status,
			"verificationError":  verification.Error,
			"signatureType":      signatureType,
			"signatureKeyId":     req.SignatureKeyID,
		})
		writeAudit(logger, st, event, nil)
		emitArtifactRegisteredEvent(logger, st, hub, artifact, "pull")
		if releaseAuto != nil {
			releaseAuto.Trigger("artifact_pull")
		}
		if vulnScan != nil && shouldScanArtifact(atype, vulnSkipTypes) {
			vulnScan.Trigger(artifactID, objectKey, actualSHA)
		}

		out := UploadArtifactResponse{
			ArtifactID: artifactID,
			ObjectKey:  objectKey,
			SHA256:     actualSHA,
			SizeBytes:  actualSize,
			Name:       req.Name,
			Version:    req.Version,
			Type:       atype,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

func PresignArtifactUpload(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, defaultExpires time.Duration, trustProxy bool, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status string) {
			if metricsCollector != nil {
				metricsCollector.IncArtifactPresign(status)
			}
		}
		if objStore == nil || bucket == "" {
			record("error")
			http.Error(w, "object store not configured", http.StatusInternalServerError)
			return
		}
		var req PresignUploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			record("error")
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		artifactID := uuid.NewString()
		ext := filepath.Ext(strings.TrimSpace(req.Filename))
		if ext == "" {
			ext = ".tar.gz"
		}
		objectKey := "artifacts/" + artifactID + ext
		contentType := strings.TrimSpace(req.ContentType)
		if contentType == "" {
			contentType = "application/gzip"
		}
		expires := defaultExpires
		if expires <= 0 {
			expires = 15 * time.Minute
		}
		if req.ExpiresSeconds > 0 {
			expires = time.Duration(req.ExpiresSeconds) * time.Second
		}
		if expires < 60*time.Second {
			expires = 60 * time.Second
		}
		if expires > time.Hour {
			expires = time.Hour
		}

		if err := objStore.EnsureBucket(r.Context(), bucket); err != nil {
			logger.Printf("ensure bucket error: %v", err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		uploadURL, err := objStore.PresignPut(r.Context(), bucket, objectKey, expires, contentType)
		if err != nil {
			logger.Printf("presign upload error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("system"), "artifact.upload.presign", "artifact", artifactID), err)
			record("error")
			http.Error(w, "presign error", http.StatusInternalServerError)
			return
		}
		record("success")

		event := buildAuditEvent(r, trustProxy, actorUser("system"), "artifact.upload.presign", "artifact", artifactID)
		event.MetadataJSON = auditJSON(map[string]any{
			"objectKey": objectKey,
			"expiresAt": time.Now().UTC().Add(expires),
		})
		writeAudit(logger, st, event, nil)

		resp := PresignUploadResponse{
			ArtifactID: artifactID,
			ObjectKey:  objectKey,
			UploadURL:  uploadURL,
			ExpiresAt:  time.Now().UTC().Add(expires),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func CompleteArtifactUpload(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy) http.HandlerFunc {
	return completeArtifactUpload(logger, st, objStore, bucket, trustProxy, metricsCollector, sigPolicy, nil, nil, nil, nil)
}

func CompleteArtifactUploadWithReleaseAuto(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger) http.HandlerFunc {
	return completeArtifactUpload(logger, st, objStore, bucket, trustProxy, metricsCollector, sigPolicy, releaseAuto, nil, nil, nil)
}

func CompleteArtifactUploadWithRealtime(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger, hub *events.Hub, vulnScan ArtifactVulnScanTrigger, vulnSkipTypes []string) http.HandlerFunc {
	return completeArtifactUpload(logger, st, objStore, bucket, trustProxy, metricsCollector, sigPolicy, releaseAuto, hub, vulnScan, vulnSkipTypes)
}

func completeArtifactUpload(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, metricsCollector *metrics.Metrics, sigPolicy ArtifactSignaturePolicy, releaseAuto releaseAutoTrigger, hub *events.Hub, vulnScan ArtifactVulnScanTrigger, vulnSkipTypes []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status string) {
			if metricsCollector != nil {
				metricsCollector.IncArtifactUpload(status)
			}
		}
		if objStore == nil || bucket == "" {
			record("error")
			http.Error(w, "object store not configured", http.StatusInternalServerError)
			return
		}
		var req CreateArtifactRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			record("error")
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.ArtifactID = strings.TrimSpace(req.ArtifactID)
		req.Name = strings.TrimSpace(req.Name)
		req.Version = strings.TrimSpace(req.Version)
		req.ObjectKey = strings.TrimSpace(req.ObjectKey)
		req.SHA256 = strings.TrimSpace(strings.ToLower(req.SHA256))
		if req.ArtifactID == "" || req.Name == "" || req.Version == "" || req.ObjectKey == "" || req.SHA256 == "" {
			record("error")
			http.Error(w, "artifactId, name, version, objectKey, sha256 required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(req.ArtifactID); err != nil {
			record("error")
			http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(req.ObjectKey, "artifacts/"+req.ArtifactID) {
			record("error")
			http.Error(w, "objectKey does not match artifactId", http.StatusBadRequest)
			return
		}
		atype, err := normalizeArtifactType(req.Type)
		if err != nil {
			record("error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err := normalizeMetadata(req.Metadata)
		if err != nil {
			record("error")
			http.Error(w, "metadata must be valid json", http.StatusBadRequest)
			return
		}
		signatureType, err := normalizeSignatureTypeForRequest(req.Signature, req.SignatureType)
		if err != nil {
			record("error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		meta, err = mergeSignatureFields(meta, req.SignatureKeyID, signatureType)
		if err != nil {
			record("error")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := objStore.EnsureBucket(r.Context(), bucket); err != nil {
			logger.Printf("ensure bucket error: %v", err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		size, err := objStore.StatObject(r.Context(), bucket, req.ObjectKey)
		if err != nil {
			logger.Printf("stat object error: %v", err)
			record("error")
			http.Error(w, "object not found", http.StatusBadRequest)
			return
		}
		if req.SizeBytes > 0 && req.SizeBytes != size {
			record("error")
			http.Error(w, "size mismatch", http.StatusBadRequest)
			return
		}
		reader, err := objStore.GetObject(r.Context(), bucket, req.ObjectKey)
		if err != nil {
			logger.Printf("get object error: %v", err)
			record("error")
			http.Error(w, "object read error", http.StatusBadRequest)
			return
		}
		defer reader.Close()
		h := sha256.New()
		if _, err := io.Copy(h, reader); err != nil {
			logger.Printf("hash object error: %v", err)
			record("error")
			http.Error(w, "object read error", http.StatusBadRequest)
			return
		}
		computedSHA := hex.EncodeToString(h.Sum(nil))
		if computedSHA != req.SHA256 {
			record("error")
			http.Error(w, "sha256 mismatch", http.StatusBadRequest)
			return
		}

		verification, actualSHA, actualSize, err := verifyStoredArtifact(r.Context(), st, objStore, bucket, req.ObjectKey, req.SHA256, req.Signature, signatureType, req.SignatureKeyID, sigPolicy)
		recordArtifactVerificationMetrics(metricsCollector, "complete", verification)
		if err != nil {
			record("error")
			logger.Printf("verify artifact error: %v", err)
			http.Error(w, "artifact verification failed", http.StatusBadRequest)
			return
		}

		artifact := store.Artifact{
			ArtifactID:          req.ArtifactID,
			Name:                req.Name,
			Version:             req.Version,
			Type:                atype,
			Status:              "active",
			ObjectKey:           req.ObjectKey,
			SHA256:              actualSHA,
			Signature:           strings.TrimSpace(req.Signature),
			SignatureType:       signatureType,
			SignatureKeyID:      strings.TrimSpace(req.SignatureKeyID),
			VerificationStatus:  verification.Status,
			VerificationError:   verification.Error,
			VerifiedAt:          verification.VerifiedAt,
			SizeBytes:           actualSize,
			MetadataJSON:        meta,
			CreatedAt:           time.Now().UTC(),
		}
		if existing, isDupe, isConflict := artifactDuplicateCheck(r, st, artifact.Name, artifact.Type, artifact.Version, artifact.SHA256); isConflict {
			record("error")
			_ = objStore.DeleteObject(r.Context(), bucket, req.ObjectKey)
			http.Error(w, "artifact_version_conflict: an artifact with this name/type/version exists with different content; use ?supersede=true to register as a distinct artifact", http.StatusConflict)
			return
		} else if isDupe {
			record("success")
			_ = objStore.DeleteObject(r.Context(), bucket, req.ObjectKey)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(UploadArtifactResponse{
				ArtifactID: existing.ArtifactID,
				ObjectKey:  existing.ObjectKey,
				SHA256:     existing.SHA256,
				SizeBytes:  existing.SizeBytes,
				Name:       existing.Name,
				Version:    existing.Version,
				Type:       existing.Type,
				Duplicate:  true,
			})
			return
		}

		if err := st.CreateArtifact(artifact); err != nil {
			logger.Printf("create artifact error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.upload.complete", "artifact", req.ArtifactID), err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		record("success")
		refreshArtifactVerificationMetrics(st, metricsCollector)

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.upload.complete", "artifact", req.ArtifactID)
		event.AfterJSON = auditJSON(map[string]any{
			"artifactId":         req.ArtifactID,
			"name":               req.Name,
			"version":            req.Version,
			"type":               atype,
			"objectKey":          req.ObjectKey,
			"sha256":             actualSHA,
			"sizeBytes":          actualSize,
			"verificationStatus": verification.Status,
			"verificationError":  verification.Error,
			"signatureType":      signatureType,
			"signatureKeyId":     req.SignatureKeyID,
		})
		writeAudit(logger, st, event, nil)
		emitArtifactRegisteredEvent(logger, st, hub, artifact, "complete")
		if releaseAuto != nil {
			releaseAuto.Trigger("artifact_complete_upload")
		}
		if vulnScan != nil && shouldScanArtifact(atype, vulnSkipTypes) {
			vulnScan.Trigger(req.ArtifactID, req.ObjectKey, actualSHA)
		}

		resp := UploadArtifactResponse{
			ArtifactID: req.ArtifactID,
			ObjectKey:  req.ObjectKey,
			SHA256:     actualSHA,
			SizeBytes:  actualSize,
			Name:       req.Name,
			Version:    req.Version,
			Type:       atype,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func ListArtifacts(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
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
			refs, err := st.CountArtifactReferences(a.ArtifactID)
			if err != nil {
				logger.Printf("count artifact refs error: %v", err)
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			}
			resp.Items = append(resp.Items, artifactToResponse(a, refs))
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.list", "artifact", "")
		event.MetadataJSON = auditJSON(map[string]any{
			"name":    name,
			"version": version,
			"limit":   limit,
			"offset":  offset,
			"count":   len(resp.Items),
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func GetArtifact(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
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

		refs, err := st.CountArtifactReferences(artifactID)
		if err != nil {
			logger.Printf("count artifact refs error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		resp := artifactToResponse(artifact, refs)
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.read", "artifact", artifactID)
		event.MetadataJSON = auditJSON(map[string]any{
			"name":    artifact.Name,
			"version": artifact.Version,
			"type":    artifact.Type,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func PresignArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, expires time.Duration, trustProxy bool, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status string) {
			if metricsCollector != nil {
				metricsCollector.IncArtifactPresign(status)
			}
		}
		artifactID := chi.URLParam(r, "artifactId")
		if artifactID == "" {
			record("error")
			http.Error(w, "artifactId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(artifactID); err != nil {
			record("error")
			http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
			return
		}
		if objStore == nil || bucket == "" {
			record("error")
			http.Error(w, "object store not configured", http.StatusInternalServerError)
			return
		}

		artifact, ok, err := st.GetArtifact(artifactID)
		if err != nil {
			logger.Printf("get artifact error: %v", err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			record("error")
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
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("system"), "artifact.presign", "artifact", artifactID), err)
			record("error")
			http.Error(w, "presign error", http.StatusInternalServerError)
			return
		}
		record("success")

		event := buildAuditEvent(r, trustProxy, actorUser("system"), "artifact.presign", "artifact", artifactID)
		event.MetadataJSON = auditJSON(map[string]any{"expiresAt": time.Now().UTC().Add(exp)})
		writeAudit(logger, st, event, nil)

		resp := PresignResponse{DownloadURL: url, ExpiresAt: time.Now().UTC().Add(exp)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func DeleteArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool) http.HandlerFunc {
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
		force := parseBool(r.URL.Query().Get("force"))
		refs, err := st.CountArtifactReferences(artifactID)
		if err != nil {
			logger.Printf("count artifact refs error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if refs > 0 && !force {
			http.Error(w, fmt.Sprintf("artifact is still referenced by desired state (%d references)", refs), http.StatusConflict)
			return
		}
		if artifact.Status != "deprecated" && !force {
			http.Error(w, "artifact must be deprecated before delete", http.StatusConflict)
			return
		}

		if objStore != nil && bucket != "" && artifact.ObjectKey != "" {
			if err := objStore.DeleteObject(r.Context(), bucket, artifact.ObjectKey); err != nil {
				logger.Printf("delete artifact object error: %v", err)
				writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.delete", "artifact", artifactID), err)
				http.Error(w, "object delete error", http.StatusInternalServerError)
				return
			}
		}

		if err := st.DeleteArtifact(artifactID); err != nil {
			logger.Printf("delete artifact error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.delete", "artifact", artifactID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.delete", "artifact", artifactID)
		event.BeforeJSON = auditJSON(map[string]any{
			"artifactId": artifact.ArtifactID,
			"name":       artifact.Name,
			"version":    artifact.Version,
			"type":       artifact.Type,
			"status":     artifact.Status,
			"force":      force,
			"references": refs,
		})
		writeAudit(logger, st, event, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func GetArtifactLifecyclePolicy(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		policy, err := st.GetArtifactLifecyclePolicy()
		if err != nil {
			logger.Printf("get artifact lifecycle policy error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		resp := ArtifactLifecyclePolicyResponse{
			DeprecatedDeleteAfterDays: policy.DeprecatedDeleteAfterDays,
			UpdatedAt:                 policy.UpdatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func GetArtifactLifecycleStatus(manager *lifecycle.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := ArtifactLifecycleStatusResponse{
			Enabled: false,
		}
		if manager != nil {
			st := manager.Status()
			status.Enabled = st.Enabled
			status.Running = st.Running
			status.IntervalSeconds = st.IntervalSeconds
			status.BatchLimit = st.BatchLimit
			status.AlertReferenceThreshold = st.AlertReferenceThreshold
			status.Alerts = st.Alerts
			if st.LastRun != nil {
				status.LastRun = &PruneArtifactsResponse{
					Trigger:    st.LastRun.Trigger,
					StartedAt:  st.LastRun.StartedAt,
					FinishedAt: st.LastRun.FinishedAt,
					CutoffUTC:  st.LastRun.CutoffUTC,
					Limit:      st.LastRun.Limit,
					Deleted:    append([]string(nil), st.LastRun.Deleted...),
					Skipped:    convertLifecycleSkips(st.LastRun.Skipped),
					DeletedNum: st.LastRun.DeletedNum,
					SkippedNum: st.LastRun.SkippedNum,
					Error:      st.LastRun.Error,
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func SetArtifactLifecyclePolicy(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ArtifactLifecyclePolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.DeprecatedDeleteAfterDays < 1 {
			http.Error(w, "deprecatedDeleteAfterDays must be >= 1", http.StatusBadRequest)
			return
		}
		policy, err := st.SetArtifactLifecyclePolicy(req.DeprecatedDeleteAfterDays)
		if err != nil {
			logger.Printf("set artifact lifecycle policy error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.lifecycle.policy.set", "artifact", ""), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.lifecycle.policy.set", "artifact", "")
		event.AfterJSON = auditJSON(map[string]any{
			"deprecatedDeleteAfterDays": policy.DeprecatedDeleteAfterDays,
		})
		writeAudit(logger, st, event, nil)

		resp := ArtifactLifecyclePolicyResponse{
			DeprecatedDeleteAfterDays: policy.DeprecatedDeleteAfterDays,
			UpdatedAt:                 policy.UpdatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func DeprecateArtifact(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
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

		var req DeprecateArtifactRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
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

		policy, err := st.GetArtifactLifecyclePolicy()
		if err != nil {
			logger.Printf("get artifact lifecycle policy error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		deleteAfterDays := req.DeleteAfterDays
		if deleteAfterDays < 1 {
			deleteAfterDays = policy.DeprecatedDeleteAfterDays
		}
		now := time.Now().UTC()
		deleteAfter := now.Add(time.Duration(deleteAfterDays) * 24 * time.Hour)
		if err := st.DeprecateArtifact(artifactID, now, deleteAfter); err != nil {
			logger.Printf("deprecate artifact error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.deprecate", "artifact", artifactID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		updated, _, _ := st.GetArtifact(artifactID)
		refs, _ := st.CountArtifactReferences(artifactID)

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.deprecate", "artifact", artifactID)
		event.BeforeJSON = auditJSON(map[string]any{
			"artifactId": artifact.ArtifactID,
			"status":     artifact.Status,
		})
		event.AfterJSON = auditJSON(map[string]any{
			"artifactId":      updated.ArtifactID,
			"status":          updated.Status,
			"deleteAfterDays": deleteAfterDays,
			"deleteAfter":     deleteAfter,
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(artifactToResponse(updated, refs))
	}
}

func RestoreArtifact(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
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
		if err := st.RestoreArtifact(artifactID); err != nil {
			logger.Printf("restore artifact error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.restore", "artifact", artifactID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		updated, _, _ := st.GetArtifact(artifactID)
		refs, _ := st.CountArtifactReferences(artifactID)

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.restore", "artifact", artifactID)
		event.BeforeJSON = auditJSON(map[string]any{
			"artifactId": artifact.ArtifactID,
			"status":     artifact.Status,
		})
		event.AfterJSON = auditJSON(map[string]any{
			"artifactId": updated.ArtifactID,
			"status":     updated.Status,
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(artifactToResponse(updated, refs))
	}
}

func PruneArtifacts(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, manager *lifecycle.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := parseInt(r.URL.Query().Get("limit"), 100)
		if manager != nil {
			run, err := manager.RunNow(limit)
			if err != nil {
				if logger != nil {
					logger.Printf("manual artifact prune error: %v", err)
				}
				writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("system"), "artifact.prune", "artifact", ""), err)
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			resp := PruneArtifactsResponse{
				Trigger:    run.Trigger,
				StartedAt:  run.StartedAt,
				FinishedAt: run.FinishedAt,
				CutoffUTC:  run.CutoffUTC,
				Limit:      run.Limit,
				Deleted:    run.Deleted,
				Skipped:    convertLifecycleSkips(run.Skipped),
				DeletedNum: run.DeletedNum,
				SkippedNum: run.SkippedNum,
				Error:      run.Error,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		cutoff := time.Now().UTC()
		resp, err := pruneArtifactsOnce(r.Context(), st, objStore, bucket, cutoff, limit)
		if err != nil {
			logger.Printf("list artifacts for prune error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("system"), "artifact.prune", "artifact", ""), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("system"), "artifact.prune", "artifact", "")
		event.MetadataJSON = auditJSON(map[string]any{
			"cutoffUtc": resp.CutoffUTC,
			"limit":     resp.Limit,
			"deleted":   resp.Deleted,
			"skipped":   resp.Skipped,
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// parseInt lives in devices.go

func parseBool(val string) bool {
	switch strings.TrimSpace(strings.ToLower(val)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func scrubSourceURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return ""
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.Redacted()
}

func pruneArtifactsOnce(ctx context.Context, st store.Store, objStore ObjectStore, bucket string, cutoff time.Time, limit int) (PruneArtifactsResponse, error) {
	candidates, err := st.ListArtifactsForPrune(cutoff, limit)
	if err != nil {
		return PruneArtifactsResponse{}, err
	}
	resp := PruneArtifactsResponse{
		Trigger:   "manual",
		StartedAt: time.Now().UTC(),
		CutoffUTC: cutoff,
		Limit:     limit,
		Deleted:   make([]string, 0, len(candidates)),
		Skipped:   []PruneArtifactSkipEntry{},
	}
	for _, artifact := range candidates {
		refs, err := st.CountArtifactReferences(artifact.ArtifactID)
		if err != nil {
			resp.Skipped = append(resp.Skipped, PruneArtifactSkipEntry{ArtifactID: artifact.ArtifactID, Reason: "failed to count references"})
			continue
		}
		if refs > 0 {
			resp.Skipped = append(resp.Skipped, PruneArtifactSkipEntry{ArtifactID: artifact.ArtifactID, Reason: fmt.Sprintf("still referenced (%d)", refs)})
			continue
		}
		if objStore != nil && bucket != "" && artifact.ObjectKey != "" {
			if err := objStore.DeleteObject(ctx, bucket, artifact.ObjectKey); err != nil {
				resp.Skipped = append(resp.Skipped, PruneArtifactSkipEntry{ArtifactID: artifact.ArtifactID, Reason: "failed to delete object"})
				continue
			}
		}
		if err := st.DeleteArtifact(artifact.ArtifactID); err != nil {
			resp.Skipped = append(resp.Skipped, PruneArtifactSkipEntry{ArtifactID: artifact.ArtifactID, Reason: "failed to delete record"})
			continue
		}
		resp.Deleted = append(resp.Deleted, artifact.ArtifactID)
	}
	resp.DeletedNum = len(resp.Deleted)
	resp.SkippedNum = len(resp.Skipped)
	resp.FinishedAt = time.Now().UTC()
	return resp, nil
}

func convertLifecycleSkips(skips []lifecycle.PruneSkip) []PruneArtifactSkipEntry {
	if len(skips) == 0 {
		return []PruneArtifactSkipEntry{}
	}
	out := make([]PruneArtifactSkipEntry, 0, len(skips))
	for _, s := range skips {
		out = append(out, PruneArtifactSkipEntry{
			ArtifactID: s.ArtifactID,
			Reason:     s.Reason,
		})
	}
	return out
}

func artifactToResponse(a store.Artifact, refs int) ArtifactResponse {
	status := strings.TrimSpace(strings.ToLower(a.Status))
	if status == "" {
		status = "active"
	}
	return ArtifactResponse{
		ArtifactID:         a.ArtifactID,
		Name:               a.Name,
		Version:            a.Version,
		Type:               a.Type,
		Status:             status,
		ObjectKey:          a.ObjectKey,
		SHA256:             a.SHA256,
		Signature:          a.Signature,
		SignatureType:      a.SignatureType,
		SignatureKeyID:     a.SignatureKeyID,
		VerificationStatus: a.VerificationStatus,
		VerificationError:  a.VerificationError,
		VerifiedAt:         timePtr(a.VerifiedAt),
		SizeBytes:          a.SizeBytes,
		Metadata:           json.RawMessage(a.MetadataJSON),
		CreatedAt:          a.CreatedAt,
		DeprecatedAt:       timePtr(a.DeprecatedAt),
		DeleteAfter:        timePtr(a.DeleteAfter),
		ReferenceCount:     refs,
	}
}

var allowedArtifactTypes = map[string]struct{}{
	"app_bundle":      {},
	"config_bundle":   {},
	"data_bundle":     {},
	"firmware":        {},
	"container_image": {},
	"agent_bundle":    {},
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

// artifactDuplicateCheck looks up an active artifact by name+type+version.
// Returns (existing, isDuplicate, isConflict):
//   - isDuplicate=true means same SHA256 → caller should return the existing artifact (idempotent).
//   - isConflict=true means different SHA256 → caller should return 409.
//   - Both false means no existing artifact found, or ?supersede=true was set.
func artifactDuplicateCheck(r *http.Request, st store.Store, name, artifactType, version, sha256 string) (store.Artifact, bool, bool) {
	if r.URL.Query().Get("supersede") == "true" {
		return store.Artifact{}, false, false
	}
	existing, found, err := st.FindArtifactByNameTypeVersion(name, artifactType, version)
	if err != nil || !found {
		return store.Artifact{}, false, false
	}
	if strings.EqualFold(existing.SHA256, sha256) {
		return existing, true, false
	}
	return store.Artifact{}, false, true
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

func mergeSignatureFields(meta []byte, keyID, signatureType string) ([]byte, error) {
	keyID = strings.TrimSpace(keyID)
	signatureType = strings.TrimSpace(signatureType)
	if keyID == "" && signatureType == "" {
		return meta, nil
	}
	if len(meta) == 0 || string(meta) == "null" {
		obj := map[string]any{}
		if keyID != "" {
			obj["signatureKeyId"] = keyID
		}
		if signatureType != "" {
			obj["signatureType"] = signatureType
		}
		out, _ := json.Marshal(obj)
		return out, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(meta, &obj); err != nil {
		return nil, fmt.Errorf("metadata must be a JSON object when signature fields are provided")
	}
	if obj == nil {
		obj = map[string]any{}
	}
	if keyID != "" {
		obj["signatureKeyId"] = keyID
	}
	if signatureType != "" {
		obj["signatureType"] = signatureType
	}
	out, _ := json.Marshal(obj)
	return out, nil
}

func normalizeSignatureTypeForRequest(signature, signatureType string) (string, error) {
	if strings.TrimSpace(signature) == "" {
		return "", nil
	}
	return artifacttrust.NormalizeSignatureType(signatureType)
}

func verifyStoredArtifact(ctx context.Context, st store.Store, objStore ObjectStore, bucket, objectKey, expectedSHA, signature, signatureType, signatureKeyID string, sigPolicy ArtifactSignaturePolicy) (artifacttrust.VerificationResult, string, int64, error) {
	if objStore == nil || bucket == "" {
		return artifacttrust.VerificationResult{}, "", 0, fmt.Errorf("object store not configured")
	}
	reader, err := objStore.GetObject(ctx, bucket, objectKey)
	if err != nil {
		return artifacttrust.VerificationResult{}, "", 0, fmt.Errorf("object read error")
	}
	defer reader.Close()
	body, actualSHA, err := artifacttrust.ReadAllAndSHA256(reader)
	if err != nil {
		return artifacttrust.VerificationResult{}, "", 0, fmt.Errorf("object read error")
	}
	if expectedSHA != "" && !strings.EqualFold(actualSHA, strings.TrimSpace(expectedSHA)) {
		return artifacttrust.VerificationResult{}, "", 0, fmt.Errorf("sha256 mismatch")
	}
	globalPolicy, err := sigPolicy.ResolvePolicy(nil)
	if err != nil {
		return artifacttrust.VerificationResult{}, "", 0, err
	}
	signature = strings.TrimSpace(signature)
	signatureKeyID = strings.TrimSpace(signatureKeyID)
	if signature == "" {
		result, decisionErr := artifacttrust.VerificationOutcomeForIngest(globalPolicy, "", "", "", nil, nil)
		return result, actualSHA, int64(len(body)), decisionErr
	}
	// Keyless cosign path: signature field contains the JSON-encoded KeylessCosignBundle.
	if strings.ToLower(strings.TrimSpace(signatureType)) == artifacttrust.SignatureTypeKeyless {
		_, _, verifyErr := artifacttrust.VerifyArtifactKeyless(signature, actualSHA, sigPolicy.KeylessOpts)
		result, decisionErr := artifacttrust.VerificationOutcomeForIngest(globalPolicy, signature, signatureType, "", nil, verifyErr)
		if verifyErr == nil {
			result.Status = artifacttrust.VerificationStatusVerified
		}
		return result, actualSHA, int64(len(body)), decisionErr
	}
	if signatureKeyID == "" {
		result := artifacttrust.VerificationResult{
			Status:         artifacttrust.VerificationStatusUntrusted,
			Error:          "signatureKeyId required when signature is provided",
			SignatureType:  signatureType,
			SignatureKeyID: signatureKeyID,
		}
			if globalPolicy.VerificationMode == artifacttrust.VerificationModeRequire {
				return result, actualSHA, int64(len(body)), errors.New(result.Error)
			}
		return result, actualSHA, int64(len(body)), nil
	}
	key, ok, err := st.GetTrustedSigningKey(signatureKeyID)
	if err != nil {
		return artifacttrust.VerificationResult{}, "", 0, err
	}
	var keyRef *store.TrustedSigningKey
	if ok {
		keyRef = &key
		if key.State != artifacttrust.KeyStateActive {
			keyRef = nil
		}
	}
	verifyErr := error(nil)
	if keyRef != nil {
		verifyErr = artifacttrust.VerifyArtifact(signatureType, signature, keyRef.PublicKeyPEM, actualSHA, body)
	}
	result, decisionErr := artifacttrust.VerificationOutcomeForIngest(globalPolicy, signature, signatureType, signatureKeyID, keyRef, verifyErr)
	return result, actualSHA, int64(len(body)), decisionErr
}

func recordArtifactVerificationMetrics(metricsCollector *metrics.Metrics, operation string, result artifacttrust.VerificationResult) {
	if metricsCollector == nil {
		return
	}
	if result.Status == "" {
		return
	}
	metricsCollector.IncArtifactVerification(operation, result.Status, result.SignatureType)
}

func refreshArtifactVerificationMetrics(st store.Store, metricsCollector *metrics.Metrics) {
	if st == nil || metricsCollector == nil {
		return
	}
	for _, status := range []string{
		artifacttrust.VerificationStatusUnsigned,
		artifacttrust.VerificationStatusLegacy,
		artifacttrust.VerificationStatusVerified,
		artifacttrust.VerificationStatusFailed,
		artifacttrust.VerificationStatusUntrusted,
	} {
		count, err := st.CountArtifactsByVerificationStatus(status)
		if err != nil {
			continue
		}
		metricsCollector.SetArtifactVerificationState(status, count)
	}
}

func fallbackCreateArtifactVerification(signature, signatureType, signatureKeyID string, sigPolicy ArtifactSignaturePolicy) (artifacttrust.VerificationResult, error) {
	globalPolicy, err := sigPolicy.ResolvePolicy(nil)
	if err != nil {
		return artifacttrust.VerificationResult{}, err
	}
	signature = strings.TrimSpace(signature)
	signatureKeyID = strings.TrimSpace(signatureKeyID)
	if signature == "" {
		return artifacttrust.VerificationOutcomeForIngest(globalPolicy, "", "", "", nil, nil)
	}
	result := artifacttrust.VerificationResult{
		Status:         artifacttrust.VerificationStatusLegacy,
		Error:          "artifact registered without cryptographic verification",
		SignatureType:  signatureType,
		SignatureKeyID: signatureKeyID,
	}
	if globalPolicy.VerificationMode == artifacttrust.VerificationModeRequire {
		if signatureKeyID == "" {
			return result, fmt.Errorf("signatureKeyId required when signature is provided")
		}
		return result, fmt.Errorf("cryptographic verification requires configured object storage")
	}
	return result, nil
}
