package vulnscan

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/store"
)

// NessusSyncJob periodically syncs Nessus scan results to HardwareOps devices.
type NessusSyncJob struct {
	client   *NessusClient
	store    store.Store
	hub      *events.Hub
	logger   *log.Logger
	interval time.Duration
	scanIDs  []string // optional filter; empty = all scans

	mu          sync.Mutex
	lastSyncAt  time.Time
	lastSyncErr error
	matchCount  int
}

// NessusSyncConfig configures the sync job.
type NessusSyncConfig struct {
	Client   *NessusClient
	Store    store.Store
	Hub      *events.Hub
	Logger   *log.Logger
	Interval time.Duration
	ScanIDs  []string
}

// NewNessusSyncJob creates a NessusSyncJob but does not start it.
func NewNessusSyncJob(cfg NessusSyncConfig) *NessusSyncJob {
	interval := cfg.Interval
	if interval <= 0 {
		interval = time.Hour
	}
	return &NessusSyncJob{
		client:   cfg.Client,
		store:    cfg.Store,
		hub:      cfg.Hub,
		logger:   cfg.Logger,
		interval: interval,
		scanIDs:  cfg.ScanIDs,
	}
}

// Start launches the background ticker. Call once from main.
func (j *NessusSyncJob) Start(ctx context.Context) {
	go func() {
		j.sync()
		ticker := time.NewTicker(j.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				j.sync()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Trigger runs an immediate sync (e.g. from an admin API call).
func (j *NessusSyncJob) Trigger() {
	go j.sync()
}

// Status returns the last sync time and any error.
func (j *NessusSyncJob) Status() (lastSync time.Time, matchCount int, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.lastSyncAt, j.matchCount, j.lastSyncErr
}

func (j *NessusSyncJob) sync() {
	j.logger.Printf("nessus sync: starting")
	scans, err := j.client.ListScans()
	if err != nil {
		j.setStatus(0, fmt.Errorf("list scans: %w", err))
		return
	}

	// Apply optional scan ID filter.
	if len(j.scanIDs) > 0 {
		allowed := make(map[string]struct{}, len(j.scanIDs))
		for _, id := range j.scanIDs {
			allowed[id] = struct{}{}
		}
		filtered := scans[:0]
		for _, s := range scans {
			if _, ok := allowed[strconv.Itoa(s.ID)]; ok {
				filtered = append(filtered, s)
			}
		}
		scans = filtered
	}

	devices, err := j.store.ListDevices(store.ListDevicesFilter{Limit: 10000})
	if err != nil {
		j.setStatus(0, fmt.Errorf("list devices: %w", err))
		return
	}

	// Build lookup: hostname → device, ip → device.
	byHostname := make(map[string]store.Device, len(devices))
	byIP := make(map[string]store.Device, len(devices))
	for _, d := range devices {
		var meta map[string]interface{}
		if len(d.MetadataJSON) > 0 {
			_ = json.Unmarshal(d.MetadataJSON, &meta)
		}
		if h, ok := meta["hwops.network.hostname"].(string); ok && h != "" {
			byHostname[strings.ToLower(h)] = d
		}
		if ip, ok := meta["hwops.network.ip"].(string); ok && ip != "" {
			byIP[ip] = d
		}
	}

	matchCount := 0
	for _, scan := range scans {
		scanIDStr := strconv.Itoa(scan.ID)
		hosts, err := j.client.GetScanHosts(scanIDStr)
		if err != nil {
			j.logger.Printf("nessus sync: get hosts for scan %d: %v", scan.ID, err)
			continue
		}
		for _, host := range hosts {
			device, matched := matchDevice(host, byHostname, byIP)
			hostIDStr := strconv.Itoa(host.HostID)

			if !matched {
				_ = j.store.UpsertDeviceVulnScan(store.DeviceVulnerabilityScan{
					DeviceID:       "",
					ScannerType:    "nessus",
					ExternalScanID: scanIDStr,
					ExternalHostID: hostIDStr,
					ScanStatus:     "no_match",
				})
				continue
			}

			vulns, err := j.client.GetHostFindings(scanIDStr, hostIDStr)
			if err != nil {
				j.logger.Printf("nessus sync: get findings for host %d: %v", host.HostID, err)
				_ = j.store.UpsertDeviceVulnScan(store.DeviceVulnerabilityScan{
					DeviceID:       device.DeviceID,
					ScannerType:    "nessus",
					ExternalScanID: scanIDStr,
					ExternalHostID: hostIDStr,
					ScanStatus:     "failed",
				})
				continue
			}

			findings := make([]VulnerabilityFinding, 0, len(vulns))
			for _, v := range vulns {
				findings = append(findings, nessusVulnToFinding(v))
			}
			counts := countSeverities(findings)
			findingsJSON, _ := json.Marshal(findings)
			countsJSON, _ := json.Marshal(counts)

			rec := store.DeviceVulnerabilityScan{
				DeviceID:           device.DeviceID,
				ScannerType:        "nessus",
				ExternalScanID:     scanIDStr,
				ExternalHostID:     hostIDStr,
				ScanStatus:         "synced",
				FindingsJSON:       findingsJSON,
				SeverityCountsJSON: countsJSON,
				ScannedAt:          time.Now().UTC(),
			}
			if err := j.store.UpsertDeviceVulnScan(rec); err != nil {
				j.logger.Printf("nessus sync: upsert device %s: %v", device.DeviceID, err)
				continue
			}
			matchCount++

			if j.hub != nil {
				payload, _ := json.Marshal(map[string]interface{}{
					"deviceId":       device.DeviceID,
					"externalScanId": scanIDStr,
					"severityCounts": counts,
				})
				j.hub.Publish(events.Event{
					Type:    "device.scan_synced",
					At:      time.Now().UTC(),
					Payload: payload,
				})
			}
		}
	}

	j.setStatus(matchCount, nil)
	j.logger.Printf("nessus sync: complete matched=%d", matchCount)
}

func (j *NessusSyncJob) setStatus(matchCount int, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.lastSyncAt = time.Now().UTC()
	j.lastSyncErr = err
	j.matchCount = matchCount
}

// matchDevice tries to find a HardwareOps device for a Nessus host.
func matchDevice(host NessusHost, byHostname, byIP map[string]store.Device) (store.Device, bool) {
	if host.Hostname != "" {
		if d, ok := byHostname[strings.ToLower(host.Hostname)]; ok {
			return d, true
		}
		if d, ok := byIP[host.Hostname]; ok {
			return d, true
		}
	}
	return store.Device{}, false
}
