package releaseautoupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hardwareops/control-plane/internal/artifacttrust"
	"github.com/hardwareops/control-plane/internal/store"
)

const (
	autoModeInherit  = "inherit"
	autoModeEnabled  = "enabled"
	autoModeDisabled = "disabled"
)

type Config struct {
	Store    store.Store
	Interval time.Duration
	Logger   func(string, ...interface{})
}

type RunSummary struct {
	Trigger             string    `json:"trigger"`
	StartedAt           time.Time `json:"startedAt,omitempty"`
	FinishedAt          time.Time `json:"finishedAt,omitempty"`
	GroupsUpdated       int       `json:"groupsUpdated"`
	DevicesUpdated      int       `json:"devicesUpdated"`
	ComponentsUpdated   int       `json:"componentsUpdated"`
	SkippedNonSemver    int       `json:"skippedNonSemver"`
	SkippedUnavailable  int       `json:"skippedUnavailable"`
	SkippedMisconfig    int       `json:"skippedMisconfig"`
	SkippedTrustBlocked int       `json:"skippedTrustBlocked"`
	Error               string    `json:"error,omitempty"`
}

type Status struct {
	Enabled         bool                            `json:"enabled"`
	AllowUnsigned   bool                            `json:"allowUnsigned"`
	IntervalSeconds int64                           `json:"intervalSeconds"`
	Running         bool                            `json:"running"`
	LastRun         *RunSummary                     `json:"lastRun,omitempty"`
	UpdatedAt       *time.Time                      `json:"updatedAt,omitempty"`
	UpdatedByUserID string                          `json:"updatedByUserId,omitempty"`
	Settings        store.ReleaseAutoUpdateSettings `json:"-"`
}

type Manager struct {
	mu       sync.RWMutex
	store    store.Store
	interval time.Duration
	logger   func(string, ...interface{})
	running  bool
	lastRun  *RunSummary
	trigger  chan string
}

type semver4 struct {
	major int
	minor int
	patch int
	build int
}

type componentState struct {
	ArtifactID       string          `json:"artifactId,omitempty"`
	ArtifactType     string          `json:"artifactType,omitempty"`
	DesiredVersion   string          `json:"desiredVersion,omitempty"`
	DesiredConfigRev string          `json:"desiredConfigRev,omitempty"`
	Policy           json.RawMessage `json:"policy,omitempty"`
	Source           string          `json:"source,omitempty"`
	Locked           bool            `json:"locked,omitempty"`
}

const (
	skipReasonNone         = ""
	skipReasonUnavailable  = "unavailable"
	skipReasonNonSemver    = "non_semver"
	skipReasonMisconfig    = "misconfigured"
	skipReasonTrustBlocked = "trust_blocked"
)

type componentResolution struct {
	Updated    bool
	SkipReason string
}

func New(cfg Config) *Manager {
	if cfg.Store == nil {
		return nil
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	return &Manager{
		store:    cfg.Store,
		interval: interval,
		logger:   cfg.Logger,
		trigger:  make(chan string, 1),
	}
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case reason := <-m.trigger:
				if _, err := m.run(reason); err != nil && m.logger != nil {
					m.logger("release auto-update run error: %v", err)
				}
			case <-ticker.C:
				if _, err := m.run("scheduled"); err != nil && m.logger != nil {
					m.logger("release auto-update scheduled run error: %v", err)
				}
			}
		}
	}()
}

func (m *Manager) Trigger(reason string) {
	if m == nil {
		return
	}
	if strings.TrimSpace(reason) == "" {
		reason = "artifact_ingest"
	}
	select {
	case m.trigger <- reason:
	default:
	}
}

func (m *Manager) RunNow(reason string) (RunSummary, error) {
	if m == nil {
		return RunSummary{}, errors.New("release auto-update manager not configured")
	}
	if strings.TrimSpace(reason) == "" {
		reason = "manual"
	}
	return m.run(reason)
}

func (m *Manager) Status() Status {
	if m == nil {
		return Status{}
	}
	settings, _ := m.store.GetReleaseAutoUpdateSettings()
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := Status{
		Enabled:         settings.Enabled,
		AllowUnsigned:   settings.AllowUnsigned,
		IntervalSeconds: int64(m.interval.Seconds()),
		Running:         m.running,
		UpdatedByUserID: settings.UpdatedByUserID,
		Settings:        settings,
	}
	if !settings.UpdatedAt.IsZero() {
		t := settings.UpdatedAt
		out.UpdatedAt = &t
	}
	if m.lastRun != nil {
		cp := *m.lastRun
		out.LastRun = &cp
	}
	return out
}

func (m *Manager) run(trigger string) (RunSummary, error) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return RunSummary{}, errors.New("release auto-update already running")
	}
	m.running = true
	run := RunSummary{
		Trigger:   trigger,
		StartedAt: time.Now().UTC(),
	}
	m.mu.Unlock()

	finish := func(run RunSummary, err error) (RunSummary, error) {
		run.FinishedAt = time.Now().UTC()
		if err != nil {
			run.Error = err.Error()
		}
		m.recordRunAudit(run, err)
		m.mu.Lock()
		m.running = false
		m.lastRun = &run
		m.mu.Unlock()
		return run, err
	}

	settings, err := m.store.GetReleaseAutoUpdateSettings()
	if err != nil {
		return finish(run, fmt.Errorf("get release auto-update settings: %w", err))
	}
	globalTrustPolicy, _, err := m.store.GetArtifactTrustPolicy()
	if err != nil {
		return finish(run, fmt.Errorf("get artifact trust policy: %w", err))
	}
	if globalTrustPolicy.VerificationMode == "" {
		globalTrustPolicy.VerificationMode = artifacttrust.VerificationModeWarnUnsigned
	}

	artifacts, err := listAllArtifacts(m.store)
	if err != nil {
		return finish(run, fmt.Errorf("list artifacts: %w", err))
	}
	byID := make(map[string]store.Artifact, len(artifacts))
	index := map[string][]versionedArtifact{}
	for _, artifact := range artifacts {
		byID[artifact.ArtifactID] = artifact
		if !strings.EqualFold(strings.TrimSpace(artifact.Status), "active") {
			continue
		}
		v, ok := parseSemver4(artifact.Version)
		if !ok {
			continue
		}
		key := artifactKey(artifact.Name, artifact.Type)
		index[key] = append(index[key], versionedArtifact{artifact: artifact, version: v})
	}
	for key := range index {
		sort.Slice(index[key], func(i, j int) bool {
			return preferArtifact(index[key][i], index[key][j])
		})
	}

	if err := m.updateGroups(settings.Enabled, settings.AllowUnsigned, globalTrustPolicy, byID, index, &run); err != nil {
		return finish(run, fmt.Errorf("update groups: %w", err))
	}
	if err := m.updateDevices(settings.Enabled, settings.AllowUnsigned, globalTrustPolicy, byID, index, &run); err != nil {
		return finish(run, fmt.Errorf("update devices: %w", err))
	}

	return finish(run, nil)
}

func (m *Manager) updateGroups(defaultEnabled, allowUnsigned bool, globalTrustPolicy store.ArtifactTrustPolicy, byID map[string]store.Artifact, index map[string][]versionedArtifact, run *RunSummary) error {
	groups, err := m.store.ListDesiredStateGroups()
	if err != nil {
		return err
	}
	for _, group := range groups {
		components := mergeLegacyDesiredComponents(decodeDesiredComponents(group.ComponentsJSON), group.ArtifactID, group.DesiredVersion, group.DesiredConfigRev, group.PolicyJSON, "")
		if len(components) == 0 {
			continue
		}
		changed := false
		for key, comp := range components {
			next, result := maybeAdvanceComponent(comp, defaultEnabled, allowUnsigned, globalTrustPolicy, byID, index)
			switch result.SkipReason {
			case skipReasonNonSemver:
				run.SkippedNonSemver++
			case skipReasonUnavailable:
				run.SkippedUnavailable++
			case skipReasonMisconfig:
				run.SkippedMisconfig++
			case skipReasonTrustBlocked:
				run.SkippedTrustBlocked++
			}
			if result.Updated {
				components[key] = next
				changed = true
				run.ComponentsUpdated++
			}
		}
		if !changed {
			continue
		}
		legacy := legacyFromComponents(components)
		if err := m.store.UpsertDesiredStateGroup(store.DesiredStateGroup{
			GroupID:          group.GroupID,
			ArtifactID:       legacy.ArtifactID,
			DesiredVersion:   legacy.DesiredVersion,
			DesiredConfigRev: legacy.DesiredConfigRev,
			PolicyJSON:       legacy.Policy,
			ComponentsJSON:   encodeDesiredComponents(components),
			CheckinInterval:  group.CheckinInterval,
			UpdatedAt:        time.Now().UTC(),
		}); err != nil {
			return err
		}
		run.GroupsUpdated++
		m.writeEntityAudit("group", group.GroupID, components)
	}
	return nil
}

func (m *Manager) updateDevices(defaultEnabled, allowUnsigned bool, globalTrustPolicy store.ArtifactTrustPolicy, byID map[string]store.Artifact, index map[string][]versionedArtifact, run *RunSummary) error {
	devices, err := m.store.ListDesiredStateDevices()
	if err != nil {
		return err
	}
	for _, desired := range devices {
		if strings.TrimSpace(strings.ToLower(desired.Source)) != "manual" {
			continue
		}
		components := mergeLegacyDesiredComponents(decodeDesiredComponents(desired.ComponentsJSON), desired.ArtifactID, desired.DesiredVersion, desired.DesiredConfigRev, desired.PolicyJSON, desired.Source)
		if len(components) == 0 {
			continue
		}
		changed := false
		for key, comp := range components {
			next, result := maybeAdvanceComponent(comp, defaultEnabled, allowUnsigned, globalTrustPolicy, byID, index)
			switch result.SkipReason {
			case skipReasonNonSemver:
				run.SkippedNonSemver++
			case skipReasonUnavailable:
				run.SkippedUnavailable++
			case skipReasonMisconfig:
				run.SkippedMisconfig++
			case skipReasonTrustBlocked:
				run.SkippedTrustBlocked++
			}
			if result.Updated {
				components[key] = next
				changed = true
				run.ComponentsUpdated++
			}
		}
		if !changed {
			continue
		}
		legacy := legacyFromComponents(components)
		if err := m.store.UpsertDesiredStateDevice(store.DesiredStateDevice{
			DeviceID:         desired.DeviceID,
			ArtifactID:       legacy.ArtifactID,
			DesiredVersion:   legacy.DesiredVersion,
			DesiredConfigRev: legacy.DesiredConfigRev,
			PolicyJSON:       legacy.Policy,
			ComponentsJSON:   encodeDesiredComponents(components),
			CheckinInterval:  desired.CheckinInterval,
			Source:           desired.Source,
			UpdatedAt:        time.Now().UTC(),
		}); err != nil {
			return err
		}
		run.DevicesUpdated++
		m.writeEntityAudit("device", desired.DeviceID, components)
	}
	return nil
}

func (m *Manager) writeEntityAudit(targetType, targetID string, components map[string]componentState) {
	if m.store == nil {
		return
	}
	meta := map[string]any{
		"components": components,
	}
	metaJSON, _ := json.Marshal(meta)
	_ = m.store.CreateAuditEvent(store.AuditEvent{
		OccurredAt:   time.Now().UTC(),
		ActorType:    "system",
		ActorID:      "release-auto-update",
		AuthMethod:   "system",
		Action:       "desired_state.autoversion.update",
		TargetType:   targetType,
		TargetID:     targetID,
		Status:       "success",
		MetadataJSON: metaJSON,
	})
}

func (m *Manager) recordRunAudit(run RunSummary, runErr error) {
	if m.store == nil {
		return
	}
	metaJSON, _ := json.Marshal(run)
	event := store.AuditEvent{
		OccurredAt:   time.Now().UTC(),
		ActorType:    "system",
		ActorID:      "release-auto-update",
		AuthMethod:   "system",
		Action:       "release_auto_update.run",
		TargetType:   "release_auto_update",
		TargetID:     "global",
		Status:       "success",
		MetadataJSON: metaJSON,
	}
	if runErr != nil {
		event.Status = "error"
		event.Error = runErr.Error()
	}
	_ = m.store.CreateAuditEvent(event)
}

type versionedArtifact struct {
	artifact store.Artifact
	version  semver4
}

func preferArtifact(a, b versionedArtifact) bool {
	if cmp := compareSemver(a.version, b.version); cmp != 0 {
		return cmp > 0
	}
	if !a.artifact.CreatedAt.Equal(b.artifact.CreatedAt) {
		return a.artifact.CreatedAt.After(b.artifact.CreatedAt)
	}
	return a.artifact.ArtifactID > b.artifact.ArtifactID
}

func maybeAdvanceComponent(comp componentState, defaultEnabled, allowUnsigned bool, globalTrustPolicy store.ArtifactTrustPolicy, byID map[string]store.Artifact, index map[string][]versionedArtifact) (componentState, componentResolution) {
	tracking := trackingFromPolicy(comp.Policy)
	mode := tracking.Mode
	if mode == autoModeInherit {
		mode = autoModeFromPolicy(comp.Policy)
	}
	enabled := defaultEnabled
	switch mode {
	case autoModeEnabled:
		enabled = true
	case autoModeDisabled:
		enabled = false
	}
	if !enabled {
		return comp, componentResolution{}
	}
	trustPolicy, err := artifacttrust.ResolvePolicy(globalTrustPolicy, comp.Policy)
	if err != nil {
		return comp, componentResolution{SkipReason: skipReasonMisconfig}
	}
	targetName := tracking.Name
	targetType := tracking.ArtifactType
	currentArtifact, currentArtifactFound := byID[comp.ArtifactID]
	if tracking.Explicit {
		if targetName == "" || targetType == "" {
			return comp, componentResolution{SkipReason: skipReasonMisconfig}
		}
	} else {
		if strings.TrimSpace(comp.ArtifactID) == "" {
			return comp, componentResolution{SkipReason: skipReasonUnavailable}
		}
		if !currentArtifactFound {
			return comp, componentResolution{SkipReason: skipReasonUnavailable}
		}
		targetName = currentArtifact.Name
		targetType = currentArtifact.Type
	}
	currentVersion, currentVersionKnown := semver4{}, false
	switch {
	case currentArtifactFound:
		var ok bool
		currentVersion, ok = parseSemver4(currentArtifact.Version)
		if !ok {
			if tracking.Explicit {
				if parsed, ok := parseSemver4(comp.DesiredVersion); ok {
					currentVersion = parsed
					currentVersionKnown = true
				}
			} else {
				return comp, componentResolution{SkipReason: skipReasonNonSemver}
			}
		} else {
			currentVersionKnown = true
		}
	case tracking.Explicit && strings.TrimSpace(comp.DesiredVersion) != "":
		if parsed, ok := parseSemver4(comp.DesiredVersion); ok {
			currentVersion = parsed
			currentVersionKnown = true
		}
	}
	candidates := index[artifactKey(targetName, targetType)]
	if len(candidates) == 0 {
		return comp, componentResolution{SkipReason: skipReasonUnavailable}
	}
	blockedByPolicy := false
	for _, candidate := range candidates {
		if currentArtifactFound && candidate.artifact.ArtifactID == currentArtifact.ArtifactID {
			continue
		}
		if currentVersionKnown && compareSemver(candidate.version, currentVersion) <= 0 {
			continue
		}
		if !allowUnsigned && strings.TrimSpace(candidate.artifact.Signature) == "" {
			blockedByPolicy = true
			continue
		}
		if err := artifacttrust.ArtifactAllowedByPolicy(candidate.artifact, trustPolicy); err != nil {
			blockedByPolicy = true
			continue
		}
		prevDesiredVersion := comp.DesiredVersion
		comp.ArtifactID = candidate.artifact.ArtifactID
		comp.ArtifactType = candidate.artifact.Type
		comp.DesiredVersion = candidate.artifact.Version
		if comp.DesiredConfigRev != "" && (comp.DesiredConfigRev == prevDesiredVersion || (currentArtifactFound && comp.DesiredConfigRev == currentArtifact.Version)) {
			comp.DesiredConfigRev = candidate.artifact.Version
		}
		return comp, componentResolution{Updated: true}
	}
	if blockedByPolicy {
		return comp, componentResolution{SkipReason: skipReasonTrustBlocked}
	}
	return comp, componentResolution{}
}

func listAllArtifacts(st store.Store) ([]store.Artifact, error) {
	const pageSize = 500
	offset := 0
	out := make([]store.Artifact, 0, pageSize)
	for {
		page, err := st.ListArtifacts("", "", pageSize, offset)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < pageSize {
			break
		}
		offset += len(page)
	}
	return out, nil
}

func artifactKey(name, artifactType string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "|" + strings.ToLower(strings.TrimSpace(artifactType))
}

func parseSemver4(raw string) (semver4, bool) {
	val := strings.TrimSpace(raw)
	val = strings.TrimPrefix(val, "v")
	parts := strings.Split(val, ".")
	if len(parts) != 3 && len(parts) != 4 {
		return semver4{}, false
	}
	nums := [4]int{}
	for idx := 0; idx < len(parts); idx++ {
		part := strings.TrimSpace(parts[idx])
		if part == "" {
			return semver4{}, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return semver4{}, false
		}
		nums[idx] = n
	}
	return semver4{major: nums[0], minor: nums[1], patch: nums[2], build: nums[3]}, true
}

func compareSemver(a, b semver4) int {
	if a.major != b.major {
		if a.major > b.major {
			return 1
		}
		return -1
	}
	if a.minor != b.minor {
		if a.minor > b.minor {
			return 1
		}
		return -1
	}
	if a.patch != b.patch {
		if a.patch > b.patch {
			return 1
		}
		return -1
	}
	if a.build != b.build {
		if a.build > b.build {
			return 1
		}
		return -1
	}
	return 0
}

type autoPolicyEnvelope struct {
	Hwops *autoPolicyRoot `json:"hwops,omitempty"`
}

type autoPolicyRoot struct {
	AutoVersion *autoVersionPolicy `json:"autoVersion,omitempty"`
	Tracking    *trackingPolicy    `json:"tracking,omitempty"`
}

type autoVersionPolicy struct {
	Mode string `json:"mode,omitempty"`
}

type trackingPolicy struct {
	Mode         string `json:"mode,omitempty"`
	Name         string `json:"name,omitempty"`
	ArtifactType string `json:"artifactType,omitempty"`
}

type trackingSelection struct {
	Mode         string
	Name         string
	ArtifactType string
	Explicit     bool
}

func autoModeFromPolicy(raw json.RawMessage) string {
	if len(raw) == 0 {
		return autoModeInherit
	}
	var env autoPolicyEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return autoModeInherit
	}
	if env.Hwops == nil || env.Hwops.AutoVersion == nil {
		return autoModeInherit
	}
	mode := strings.ToLower(strings.TrimSpace(env.Hwops.AutoVersion.Mode))
	switch mode {
	case autoModeEnabled, autoModeDisabled:
		return mode
	default:
		return autoModeInherit
	}
}

func trackingFromPolicy(raw json.RawMessage) trackingSelection {
	if len(raw) == 0 {
		return trackingSelection{Mode: autoModeInherit}
	}
	var env autoPolicyEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return trackingSelection{Mode: autoModeInherit}
	}
	if env.Hwops == nil || env.Hwops.Tracking == nil {
		return trackingSelection{Mode: autoModeInherit}
	}
	mode := strings.ToLower(strings.TrimSpace(env.Hwops.Tracking.Mode))
	switch mode {
	case autoModeEnabled, autoModeDisabled:
	default:
		mode = autoModeInherit
	}
	return trackingSelection{
		Mode:         mode,
		Name:         strings.TrimSpace(env.Hwops.Tracking.Name),
		ArtifactType: strings.TrimSpace(env.Hwops.Tracking.ArtifactType),
		Explicit:     true,
	}
}

type legacyDesired struct {
	ArtifactID       string
	DesiredVersion   string
	DesiredConfigRev string
	Policy           []byte
}

func decodeDesiredComponents(raw []byte) map[string]componentState {
	if len(raw) == 0 {
		return map[string]componentState{}
	}
	var comps map[string]componentState
	if err := json.Unmarshal(raw, &comps); err != nil {
		return map[string]componentState{}
	}
	return comps
}

func encodeDesiredComponents(components map[string]componentState) []byte {
	if len(components) == 0 {
		return nil
	}
	out, err := json.Marshal(components)
	if err != nil {
		return nil
	}
	return out
}

func mergeLegacyDesiredComponents(components map[string]componentState, artifactID, desiredVersion, desiredConfigRev string, policy []byte, source string) map[string]componentState {
	if components == nil {
		components = map[string]componentState{}
	}
	if artifactID != "" || desiredVersion != "" || desiredConfigRev != "" || len(policy) != 0 {
		if _, ok := components["app_bundle"]; !ok {
			components["app_bundle"] = componentState{
				ArtifactID:       artifactID,
				ArtifactType:     "app_bundle",
				DesiredVersion:   desiredVersion,
				DesiredConfigRev: desiredConfigRev,
				Policy:           policy,
				Source:           source,
			}
		}
	}
	if comp, ok := components["app_bundle"]; ok && comp.Source == "" && source != "" {
		comp.Source = source
		components["app_bundle"] = comp
	}
	return components
}

func legacyFromComponents(components map[string]componentState) legacyDesired {
	if comp, ok := components["app_bundle"]; ok {
		return legacyDesired{
			ArtifactID:       comp.ArtifactID,
			DesiredVersion:   comp.DesiredVersion,
			DesiredConfigRev: comp.DesiredConfigRev,
			Policy:           comp.Policy,
		}
	}
	return legacyDesired{}
}
