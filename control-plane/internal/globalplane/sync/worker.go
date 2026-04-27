package sync

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/parcel/control-plane/internal/globalplane"
)

// planeWorker polls a single regional plane on a ticker and upserts cached data.
type planeWorker struct {
	plane  globalplane.RegionalPlane
	store  globalplane.Store
	encKey []byte
	logger *log.Logger
	cancel context.CancelFunc
	done   chan struct{}
}

func newPlaneWorker(plane globalplane.RegionalPlane, store globalplane.Store, encKey []byte, logger *log.Logger) *planeWorker {
	return &planeWorker{
		plane:  plane,
		store:  store,
		encKey: encKey,
		logger: logger,
		done:   make(chan struct{}),
	}
}

func (w *planeWorker) start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	go w.run(ctx)
}

func (w *planeWorker) run(ctx context.Context) {
	defer close(w.done)

	interval := time.Duration(w.plane.SyncIntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Sync immediately on start.
	w.syncOnce(ctx)

	backoffFactor := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := w.syncOnce(ctx)
			if err != nil {
				backoffFactor = min(backoffFactor+1, 6) // max ~64× base interval
			} else {
				backoffFactor = 0
			}
			if backoffFactor > 0 {
				// Exponential backoff: sleep extra time before next tick.
				extra := time.Duration(math.Pow(2, float64(backoffFactor))) * 10 * time.Second
				if extra > 5*time.Minute {
					extra = 5 * time.Minute
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(extra):
				}
			}
		}
	}
}

func (w *planeWorker) syncOnce(ctx context.Context) error {
	start := time.Now()
	token, err := globalplane.DecryptToken(w.plane.EncryptedToken, w.encKey)
	if err != nil {
		w.logger.Printf("[global-plane] plane %s (%s): decrypt token: %v", w.plane.Name, w.plane.PlaneID, err)
		_ = w.store.UpdateRegionalPlaneSyncStatus(w.plane.PlaneID, time.Now(), err.Error())
		return err
	}

	client, err := newRegionalClient(w.plane.BaseURL, token, w.plane.TLSCAPem)
	if err != nil {
		w.logger.Printf("[global-plane] plane %s: build client: %v", w.plane.Name, err)
		_ = w.store.UpdateRegionalPlaneSyncStatus(w.plane.PlaneID, time.Now(), err.Error())
		return err
	}

	// Fetch devices.
	devices, err := client.FetchDevices(ctx)
	if err != nil {
		w.logger.Printf("[global-plane] plane %s: fetch devices: %v", w.plane.Name, err)
		_ = w.store.UpdateRegionalPlaneSyncStatus(w.plane.PlaneID, time.Now(), err.Error())
		return err
	}

	// Fetch health.
	health, err := client.FetchHealth(ctx)
	if err != nil {
		w.logger.Printf("[global-plane] plane %s: fetch health: %v", w.plane.Name, err)
		_ = w.store.UpdateRegionalPlaneSyncStatus(w.plane.PlaneID, time.Now(), err.Error())
		return err
	}

	// Fetch artifacts.
	artifacts, err := client.FetchArtifacts(ctx)
	if err != nil {
		w.logger.Printf("[global-plane] plane %s: fetch artifacts: %v", w.plane.Name, err)
		_ = w.store.UpdateRegionalPlaneSyncStatus(w.plane.PlaneID, time.Now(), err.Error())
		return err
	}

	// Persist.
	if err := w.store.UpsertDeviceCacheEntries(w.plane.PlaneID, devices); err != nil {
		w.logger.Printf("[global-plane] plane %s: upsert devices: %v", w.plane.Name, err)
		_ = w.store.UpdateRegionalPlaneSyncStatus(w.plane.PlaneID, time.Now(), err.Error())
		return err
	}

	snap := globalplane.HealthSnapshot{
		PlaneID:         w.plane.PlaneID,
		TotalDevices:    health.TotalDevices,
		ActiveDevices:   health.ActiveDevices,
		StaleDevices:    health.StaleDevices,
		OfflineDevices:  health.OfflineDevices,
		DegradedDevices: health.DegradedDevices,
		LastDeviceSeen:  health.LastDeviceSeen,
	}
	if err := w.store.UpsertHealthCache(snap); err != nil {
		w.logger.Printf("[global-plane] plane %s: upsert health: %v", w.plane.Name, err)
		_ = w.store.UpdateRegionalPlaneSyncStatus(w.plane.PlaneID, time.Now(), err.Error())
		return err
	}

	if err := w.store.UpsertArtifactCacheEntries(w.plane.PlaneID, artifacts); err != nil {
		w.logger.Printf("[global-plane] plane %s: upsert artifacts: %v", w.plane.Name, err)
		_ = w.store.UpdateRegionalPlaneSyncStatus(w.plane.PlaneID, time.Now(), err.Error())
		return err
	}

	now := time.Now()
	_ = w.store.UpdateRegionalPlaneSyncStatus(w.plane.PlaneID, now, "")
	w.logger.Printf("[global-plane] plane %s: synced %d devices, %d artifacts in %s",
		w.plane.Name, len(devices), len(artifacts), time.Since(start).Round(time.Millisecond))
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
