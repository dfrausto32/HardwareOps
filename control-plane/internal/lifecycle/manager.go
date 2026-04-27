package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/parcel/control-plane/internal/metrics"
	"github.com/parcel/control-plane/internal/store"
)

type ObjectStore interface {
	DeleteObject(ctx context.Context, bucket, key string) error
}

type ManagerConfig struct {
	Enabled                 bool
	Interval                time.Duration
	BatchLimit              int
	AlertReferenceThreshold int
	Store                   store.Store
	ObjectStore             ObjectStore
	Bucket                  string
	Metrics                 *metrics.Metrics
	Logger                  func(string, ...interface{})
}

type Alert struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type PruneSkip struct {
	ArtifactID string `json:"artifactId"`
	Reason     string `json:"reason"`
	Code       string `json:"code,omitempty"`
}

type PruneRun struct {
	Trigger            string      `json:"trigger"`
	StartedAt          time.Time   `json:"startedAt,omitempty"`
	FinishedAt         time.Time   `json:"finishedAt,omitempty"`
	CutoffUTC          time.Time   `json:"cutoffUtc,omitempty"`
	Limit              int         `json:"limit"`
	Deleted            []string    `json:"deleted,omitempty"`
	Skipped            []PruneSkip `json:"skipped,omitempty"`
	DeletedNum         int         `json:"deletedNum"`
	SkippedNum         int         `json:"skippedNum"`
	SkippedReferenced  int         `json:"skippedReferenced"`
	SkippedByReason    map[string]int
	Error              string `json:"error,omitempty"`
	ConsecutiveFailure int    `json:"consecutiveFailures"`
}

type Status struct {
	Enabled                 bool      `json:"enabled"`
	Running                 bool      `json:"running"`
	IntervalSeconds         int64     `json:"intervalSeconds"`
	BatchLimit              int       `json:"batchLimit"`
	AlertReferenceThreshold int       `json:"alertReferenceThreshold"`
	LastRun                 *PruneRun `json:"lastRun,omitempty"`
	Alerts                  []Alert   `json:"alerts,omitempty"`
}

type Manager struct {
	mu                      sync.RWMutex
	enabled                 bool
	interval                time.Duration
	batchLimit              int
	alertReferenceThreshold int
	store                   store.Store
	objStore                ObjectStore
	bucket                  string
	metrics                 *metrics.Metrics
	logger                  func(string, ...interface{})
	running                 bool
	lastRun                 *PruneRun
}

func NewManager(cfg ManagerConfig) *Manager {
	if cfg.Store == nil {
		return nil
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	batchLimit := cfg.BatchLimit
	if batchLimit <= 0 {
		batchLimit = 200
	}
	alertThreshold := cfg.AlertReferenceThreshold
	if alertThreshold <= 0 {
		alertThreshold = 10
	}
	return &Manager{
		enabled:                 cfg.Enabled,
		interval:                interval,
		batchLimit:              batchLimit,
		alertReferenceThreshold: alertThreshold,
		store:                   cfg.Store,
		objStore:                cfg.ObjectStore,
		bucket:                  strings.TrimSpace(cfg.Bucket),
		metrics:                 cfg.Metrics,
		logger:                  cfg.Logger,
	}
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil || !m.enabled {
		return
	}
	go func() {
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		// Wait for first interval; manual runs remain available immediately.
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := m.run("scheduled", 0); err != nil && m.logger != nil {
					m.logger("artifact prune scheduled run error: %v", err)
				}
			}
		}
	}()
}

func (m *Manager) RunNow(limit int) (PruneRun, error) {
	if m == nil {
		return PruneRun{}, errors.New("artifact lifecycle manager not configured")
	}
	return m.run("manual", limit)
}

func (m *Manager) Status() Status {
	if m == nil {
		return Status{Enabled: false}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := Status{
		Enabled:                 m.enabled,
		Running:                 m.running,
		IntervalSeconds:         int64(m.interval.Seconds()),
		BatchLimit:              m.batchLimit,
		AlertReferenceThreshold: m.alertReferenceThreshold,
	}
	if m.lastRun != nil {
		cp := *m.lastRun
		if cp.Deleted != nil {
			cp.Deleted = append([]string(nil), cp.Deleted...)
		}
		if cp.Skipped != nil {
			cp.Skipped = append([]PruneSkip(nil), cp.Skipped...)
		}
		out.LastRun = &cp
		out.Alerts = computeAlerts(m.enabled, m.interval, m.alertReferenceThreshold, &cp)
	}
	return out
}

func (m *Manager) run(trigger string, limit int) (PruneRun, error) {
	m.mu.Lock()
	if m.running {
		defer m.mu.Unlock()
		return PruneRun{}, errors.New("artifact prune already running")
	}
	m.running = true
	run := PruneRun{
		Trigger:         trigger,
		StartedAt:       time.Now().UTC(),
		Limit:           limit,
		Deleted:         []string{},
		Skipped:         []PruneSkip{},
		SkippedByReason: map[string]int{},
	}
	if run.Limit <= 0 {
		run.Limit = m.batchLimit
	}
	m.mu.Unlock()

	finish := func(run PruneRun, runErr error) (PruneRun, error) {
		run.FinishedAt = time.Now().UTC()
		run.DeletedNum = len(run.Deleted)
		run.SkippedNum = len(run.Skipped)
		run.ConsecutiveFailure = m.nextConsecutiveFailures(runErr != nil)
		if runErr != nil {
			run.Error = runErr.Error()
		}
		m.recordMetrics(run)
		m.writeAudit(run, runErr)
		m.mu.Lock()
		m.running = false
		m.lastRun = &run
		m.mu.Unlock()
		return run, runErr
	}

	cutoff := time.Now().UTC()
	run.CutoffUTC = cutoff
	candidates, err := m.store.ListArtifactsForPrune(cutoff, run.Limit)
	if err != nil {
		return finish(run, fmt.Errorf("list prune candidates: %w", err))
	}

	for _, artifact := range candidates {
		refs, err := m.store.CountArtifactReferences(artifact.ArtifactID)
		if err != nil {
			run.Skipped = append(run.Skipped, PruneSkip{
				ArtifactID: artifact.ArtifactID,
				Reason:     "failed to count references",
				Code:       "reference_count_error",
			})
			run.SkippedByReason["reference_count_error"]++
			continue
		}
		if refs > 0 {
			run.SkippedReferenced++
			run.Skipped = append(run.Skipped, PruneSkip{
				ArtifactID: artifact.ArtifactID,
				Reason:     fmt.Sprintf("still referenced (%d)", refs),
				Code:       "referenced",
			})
			run.SkippedByReason["referenced"]++
			continue
		}
		if m.objStore != nil && m.bucket != "" && artifact.ObjectKey != "" {
			if err := m.objStore.DeleteObject(context.Background(), m.bucket, artifact.ObjectKey); err != nil {
				run.Skipped = append(run.Skipped, PruneSkip{
					ArtifactID: artifact.ArtifactID,
					Reason:     "failed to delete object",
					Code:       "object_delete_error",
				})
				run.SkippedByReason["object_delete_error"]++
				continue
			}
		}
		if err := m.store.DeleteArtifact(artifact.ArtifactID); err != nil {
			run.Skipped = append(run.Skipped, PruneSkip{
				ArtifactID: artifact.ArtifactID,
				Reason:     "failed to delete record",
				Code:       "record_delete_error",
			})
			run.SkippedByReason["record_delete_error"]++
			continue
		}
		run.Deleted = append(run.Deleted, artifact.ArtifactID)
	}

	return finish(run, nil)
}

func (m *Manager) nextConsecutiveFailures(failed bool) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lastRun == nil {
		if failed {
			return 1
		}
		return 0
	}
	prev := m.lastRun.ConsecutiveFailure
	if failed {
		return prev + 1
	}
	return 0
}

func (m *Manager) recordMetrics(run PruneRun) {
	if m.metrics == nil {
		return
	}
	status := "success"
	if run.Error != "" {
		status = "error"
	}
	m.metrics.ObserveArtifactPrune(run.Trigger, status, run.DeletedNum, run.SkippedByReason, run.FinishedAt, run.ConsecutiveFailure)
}

func (m *Manager) writeAudit(run PruneRun, runErr error) {
	if m.store == nil {
		return
	}
	status := "success"
	errText := ""
	action := "artifact.prune"
	if run.Trigger == "scheduled" {
		action = "artifact.prune.auto"
	}
	if runErr != nil {
		status = "error"
		errText = runErr.Error()
	}
	meta := map[string]any{
		"trigger":            run.Trigger,
		"cutoffUtc":          run.CutoffUTC,
		"limit":              run.Limit,
		"deletedNum":         run.DeletedNum,
		"skippedNum":         run.SkippedNum,
		"skippedReferenced":  run.SkippedReferenced,
		"consecutiveFailure": run.ConsecutiveFailure,
	}
	if len(run.SkippedByReason) > 0 {
		meta["skippedByReason"] = run.SkippedByReason
	}
	metadataJSON, _ := json.Marshal(meta)
	_ = m.store.CreateAuditEvent(store.AuditEvent{
		OccurredAt:   time.Now().UTC(),
		ActorType:    "system",
		ActorID:      "artifact-lifecycle",
		AuthMethod:   "internal",
		Action:       action,
		TargetType:   "artifact",
		Status:       status,
		Error:        errText,
		MetadataJSON: metadataJSON,
	})
}

func computeAlerts(enabled bool, interval time.Duration, threshold int, run *PruneRun) []Alert {
	if run == nil {
		return nil
	}
	alerts := make([]Alert, 0, 3)
	if run.ConsecutiveFailure > 0 {
		alerts = append(alerts, Alert{
			Code:     "consecutive_failures",
			Severity: "error",
			Message:  fmt.Sprintf("Artifact prune failed %d run(s) in a row.", run.ConsecutiveFailure),
		})
	}
	if threshold > 0 && run.SkippedReferenced >= threshold {
		alerts = append(alerts, Alert{
			Code:     "referenced_backlog",
			Severity: "warning",
			Message:  fmt.Sprintf("%d artifact(s) skipped because they are still referenced.", run.SkippedReferenced),
		})
	}
	if enabled && interval > 0 && !run.FinishedAt.IsZero() && time.Since(run.FinishedAt) > (interval*2) {
		alerts = append(alerts, Alert{
			Code:     "stale_run",
			Severity: "warning",
			Message:  "Artifact prune has not completed in over two prune intervals.",
		})
	}
	return alerts
}
