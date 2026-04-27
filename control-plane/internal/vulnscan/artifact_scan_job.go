package vulnscan

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/parcel/control-plane/internal/events"
	"github.com/parcel/control-plane/internal/store"
)

// ArtifactScanJob wraps a Scanner and persists results to the store.
type ArtifactScanJob struct {
	scanner Scanner
	store   store.Store
	hub     *events.Hub
	logger  *log.Logger
}

// NewArtifactScanJob creates an ArtifactScanJob.
func NewArtifactScanJob(scanner Scanner, st store.Store, hub *events.Hub, logger *log.Logger) *ArtifactScanJob {
	return &ArtifactScanJob{
		scanner: scanner,
		store:   st,
		hub:     hub,
		logger:  logger,
	}
}

// Trigger starts a background scan for the given artifact.
func (j *ArtifactScanJob) Trigger(artifactID, objectKey, sha256 string) {
	go j.run(artifactID, objectKey, sha256)
}

func (j *ArtifactScanJob) run(artifactID, objectKey, sha256 string) {
	pending := store.ArtifactVulnerabilityScan{
		ArtifactID:  artifactID,
		ScannerType: j.scanner.Type(),
		ScanStatus:  "pending",
	}
	if err := j.store.CreateArtifactVulnScan(pending); err != nil {
		j.logger.Printf("vulnscan: create pending record for %s: %v", artifactID, err)
		return
	}

	// Re-fetch to get the assigned ScanID.
	rec, ok, err := j.store.GetLatestArtifactVulnScan(artifactID)
	if err != nil || !ok {
		j.logger.Printf("vulnscan: fetch pending scan for %s: %v", artifactID, err)
		return
	}

	rec.ScanStatus = "running"
	if err := j.store.UpdateArtifactVulnScan(rec); err != nil {
		j.logger.Printf("vulnscan: set running for %s: %v", artifactID, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, scanErr := j.scanner.Scan(ctx, ScanRequest{
		ArtifactID: artifactID,
		ObjectKey:  objectKey,
		SHA256:     sha256,
	})

	now := time.Now().UTC()
	if scanErr != nil {
		j.logger.Printf("vulnscan: scan %s failed: %v", artifactID, scanErr)
		rec.ScanStatus = "failed"
		rec.ErrorMessage = scanErr.Error()
		rec.ScannedAt = now
		_ = j.store.UpdateArtifactVulnScan(rec)
		j.emitEvent(artifactID, "failed", nil)
		return
	}

	findingsJSON, _ := json.Marshal(result.Findings)
	countsJSON, _ := json.Marshal(result.SeverityCounts)

	rec.ScanStatus = "completed"
	rec.ScannerVersion = result.ScannerVersion
	rec.FindingsJSON = findingsJSON
	rec.SeverityCountsJSON = countsJSON
	rec.ScannedAt = now
	if err := j.store.UpdateArtifactVulnScan(rec); err != nil {
		j.logger.Printf("vulnscan: save results for %s: %v", artifactID, err)
	}
	j.emitEvent(artifactID, "completed", result.SeverityCounts)
}

func (j *ArtifactScanJob) emitEvent(artifactID, status string, counts interface{}) {
	if j.hub == nil {
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"artifactId":     artifactID,
		"scanStatus":     status,
		"severityCounts": counts,
	})
	j.hub.Publish(events.Event{
		Type:    "artifact.scan_completed",
		At:      time.Now().UTC(),
		Payload: payload,
	})
}
