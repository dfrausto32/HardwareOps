package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/parcel/control-plane/internal/store"
)

// --- Response types ----------------------------------------------------------

// VulnFindingResponse is one CVE finding serialized to the API.
type VulnFindingResponse struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Package     string `json:"package"`
	Version     string `json:"version,omitempty"`
	FixedIn     string `json:"fixedIn,omitempty"`
	Description string `json:"description,omitempty"`
}

// SeverityCountsResponse is the rolled-up count per severity.
type SeverityCountsResponse struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
}

// ArtifactVulnScanResponse is the API representation of one artifact scan.
type ArtifactVulnScanResponse struct {
	ScanID         string                 `json:"scanId"`
	ArtifactID     string                 `json:"artifactId"`
	ScannerType    string                 `json:"scannerType"`
	ScannerVersion string                 `json:"scannerVersion,omitempty"`
	ScanStatus     string                 `json:"scanStatus"`
	Findings       []VulnFindingResponse  `json:"findings,omitempty"`
	SeverityCounts *SeverityCountsResponse `json:"severityCounts,omitempty"`
	ErrorMessage   string                 `json:"errorMessage,omitempty"`
	ScannedAt      *time.Time             `json:"scannedAt,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
}

// DeviceVulnScanResponse is the API representation of one device scan.
type DeviceVulnScanResponse struct {
	ScanID         string                  `json:"scanId"`
	DeviceID       string                  `json:"deviceId"`
	ScannerType    string                  `json:"scannerType"`
	ExternalScanID string                  `json:"externalScanId,omitempty"`
	ExternalHostID string                  `json:"externalHostId,omitempty"`
	ScanStatus     string                  `json:"scanStatus"`
	Findings       []VulnFindingResponse   `json:"findings,omitempty"`
	SeverityCounts *SeverityCountsResponse  `json:"severityCounts,omitempty"`
	ScannedAt      *time.Time              `json:"scannedAt,omitempty"`
	SyncedAt       time.Time               `json:"syncedAt"`
	CreatedAt      time.Time               `json:"createdAt"`
}

// NessusSyncStatusResponse is returned by GET /vulnerability-scans/nessus/status.
type NessusSyncStatusResponse struct {
	LastSyncAt  *time.Time `json:"lastSyncAt,omitempty"`
	MatchCount  int        `json:"matchCount"`
	Error       string     `json:"error,omitempty"`
}

// --- Converters --------------------------------------------------------------

func artifactVulnScanToResponse(rec store.ArtifactVulnerabilityScan) ArtifactVulnScanResponse {
	resp := ArtifactVulnScanResponse{
		ScanID:         rec.ScanID,
		ArtifactID:     rec.ArtifactID,
		ScannerType:    rec.ScannerType,
		ScannerVersion: rec.ScannerVersion,
		ScanStatus:     rec.ScanStatus,
		ErrorMessage:   rec.ErrorMessage,
		CreatedAt:      rec.CreatedAt,
	}
	if !rec.ScannedAt.IsZero() {
		t := rec.ScannedAt
		resp.ScannedAt = &t
	}
	if len(rec.FindingsJSON) > 0 {
		_ = json.Unmarshal(rec.FindingsJSON, &resp.Findings)
	}
	if len(rec.SeverityCountsJSON) > 0 {
		var c SeverityCountsResponse
		if err := json.Unmarshal(rec.SeverityCountsJSON, &c); err == nil {
			resp.SeverityCounts = &c
		}
	}
	return resp
}

func deviceVulnScanToResponse(rec store.DeviceVulnerabilityScan) DeviceVulnScanResponse {
	resp := DeviceVulnScanResponse{
		ScanID:         rec.ScanID,
		DeviceID:       rec.DeviceID,
		ScannerType:    rec.ScannerType,
		ExternalScanID: rec.ExternalScanID,
		ExternalHostID: rec.ExternalHostID,
		ScanStatus:     rec.ScanStatus,
		SyncedAt:       rec.SyncedAt,
		CreatedAt:      rec.CreatedAt,
	}
	if !rec.ScannedAt.IsZero() {
		t := rec.ScannedAt
		resp.ScannedAt = &t
	}
	if len(rec.FindingsJSON) > 0 {
		_ = json.Unmarshal(rec.FindingsJSON, &resp.Findings)
	}
	if len(rec.SeverityCountsJSON) > 0 {
		var c SeverityCountsResponse
		if err := json.Unmarshal(rec.SeverityCountsJSON, &c); err == nil {
			resp.SeverityCounts = &c
		}
	}
	return resp
}

// --- Artifact scan handlers --------------------------------------------------

// ListArtifactVulnScans returns all vulnerability scans for an artifact.
func ListArtifactVulnScans(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		if _, ok, err := st.GetArtifact(artifactID); err != nil {
			logger.Printf("ListArtifactVulnScans: get artifact %s: %v", artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}
		scans, err := st.ListArtifactVulnScans(artifactID)
		if err != nil {
			logger.Printf("ListArtifactVulnScans: list %s: %v", artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		resp := make([]ArtifactVulnScanResponse, 0, len(scans))
		for _, s := range scans {
			resp = append(resp, artifactVulnScanToResponse(s))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// GetLatestArtifactVulnScan returns the most recent vulnerability scan for an artifact.
func GetLatestArtifactVulnScan(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		if _, ok, err := st.GetArtifact(artifactID); err != nil {
			logger.Printf("GetLatestArtifactVulnScan: get artifact %s: %v", artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}
		scan, ok, err := st.GetLatestArtifactVulnScan(artifactID)
		if err != nil {
			logger.Printf("GetLatestArtifactVulnScan: %s: %v", artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "no scan found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(artifactVulnScanToResponse(scan))
	}
}

// TriggerArtifactVulnScan triggers a manual rescan of an artifact.
func TriggerArtifactVulnScan(logger *log.Logger, st store.Store, scanJob ArtifactVulnScanTrigger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")
		artifact, ok, err := st.GetArtifact(artifactID)
		if err != nil {
			logger.Printf("TriggerArtifactVulnScan: get artifact %s: %v", artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}
		if scanJob == nil {
			http.Error(w, "artifact scanning is not configured", http.StatusServiceUnavailable)
			return
		}
		scanJob.Trigger(artifact.ArtifactID, artifact.ObjectKey, artifact.SHA256)
		w.WriteHeader(http.StatusAccepted)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}
}

// --- Device scan handlers ----------------------------------------------------

// ListDeviceVulnScans returns all vulnerability scans for a device.
func ListDeviceVulnScans(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")
		if _, ok, err := st.GetDevice(deviceID); err != nil {
			logger.Printf("ListDeviceVulnScans: get device %s: %v", deviceID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		scans, err := st.ListDeviceVulnScans(deviceID)
		if err != nil {
			logger.Printf("ListDeviceVulnScans: list %s: %v", deviceID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		resp := make([]DeviceVulnScanResponse, 0, len(scans))
		for _, s := range scans {
			resp = append(resp, deviceVulnScanToResponse(s))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// GetLatestDeviceVulnScan returns the most recent vulnerability scan for a device.
func GetLatestDeviceVulnScan(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")
		if _, ok, err := st.GetDevice(deviceID); err != nil {
			logger.Printf("GetLatestDeviceVulnScan: get device %s: %v", deviceID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		scan, ok, err := st.GetLatestDeviceVulnScan(deviceID)
		if err != nil {
			logger.Printf("GetLatestDeviceVulnScan: %s: %v", deviceID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "no scan found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deviceVulnScanToResponse(scan))
	}
}

// --- Nessus sync handlers ----------------------------------------------------

// NessusSyncJobTrigger is a narrow interface for the Nessus sync job.
type NessusSyncJobTrigger interface {
	Trigger()
	Status() (lastSync time.Time, matchCount int, err error)
}

// TriggerNessusSync triggers an immediate Nessus sync.
func TriggerNessusSync(logger *log.Logger, job NessusSyncJobTrigger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if job == nil {
			http.Error(w, "Nessus integration is not configured", http.StatusServiceUnavailable)
			return
		}
		job.Trigger()
		w.WriteHeader(http.StatusAccepted)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}
}

// GetNessusSyncStatus returns the Nessus sync job status.
func GetNessusSyncStatus(logger *log.Logger, job NessusSyncJobTrigger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if job == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(NessusSyncStatusResponse{})
			return
		}
		lastSync, matchCount, syncErr := job.Status()
		resp := NessusSyncStatusResponse{MatchCount: matchCount}
		if !lastSync.IsZero() {
			resp.LastSyncAt = &lastSync
		}
		if syncErr != nil {
			resp.Error = syncErr.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
