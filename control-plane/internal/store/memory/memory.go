package memory

import (
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/hardwareops/control-plane/internal/store"
)

type Store struct {
	mu              sync.Mutex
	devices         map[string]store.Device
	states          map[string]store.DeviceState
	tokens          map[string]store.EnrollmentToken
	groups          map[string]store.Group
	desiredGroups   map[string]store.DesiredStateGroup
	desiredDevices  map[string]store.DesiredStateDevice
	artifacts       map[string]store.Artifact
	applyResults    map[string]store.ApplyResult
	auditEvents     []store.AuditEvent
	auditRetention  store.AuditRetention
	users           map[string]store.User
	userEmailIndex  map[string]string
	vouchers        map[string]store.AuthVoucher
	voucherTokenIdx map[string]string
}

func New() *Store {
	return &Store{
		devices:         map[string]store.Device{},
		states:          map[string]store.DeviceState{},
		tokens:          map[string]store.EnrollmentToken{},
		groups:          map[string]store.Group{},
		desiredGroups:   map[string]store.DesiredStateGroup{},
		desiredDevices:  map[string]store.DesiredStateDevice{},
		artifacts:       map[string]store.Artifact{},
		applyResults:    map[string]store.ApplyResult{},
		auditEvents:     []store.AuditEvent{},
		auditRetention:  store.AuditRetention{Days: 90, UpdatedAt: time.Now().UTC()},
		users:           map[string]store.User{},
		userEmailIndex:  map[string]string{},
		vouchers:        map[string]store.AuthVoucher{},
		voucherTokenIdx: map[string]string{},
	}
}

func (s *Store) UpsertDevice(device store.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if device.DeviceID == "" {
		return errors.New("device_id required")
	}
	if existing, ok := s.devices[device.DeviceID]; ok {
		if device.CertFingerprint == "" {
			device.CertFingerprint = existing.CertFingerprint
		}
	}
	s.devices[device.DeviceID] = device
	return nil
}

func (s *Store) UpsertDeviceState(state store.DeviceState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.DeviceID == "" {
		return errors.New("device_id required")
	}
	if prev, ok := s.states[state.DeviceID]; ok {
		if state.LastApplyStatus == "" {
			state.LastApplyStatus = prev.LastApplyStatus
		}
		if state.LastApplyError == "" {
			state.LastApplyError = prev.LastApplyError
		}
		if state.LastApplyAt.IsZero() {
			state.LastApplyAt = prev.LastApplyAt
		}
		if state.LastApplyArtifactID == "" {
			state.LastApplyArtifactID = prev.LastApplyArtifactID
		}
		if state.LastPreApplyStatus == "" {
			state.LastPreApplyStatus = prev.LastPreApplyStatus
		}
		if state.LastPreApplyError == "" {
			state.LastPreApplyError = prev.LastPreApplyError
		}
		if state.LastPreApplyAt.IsZero() {
			state.LastPreApplyAt = prev.LastPreApplyAt
		}
	}
	s.states[state.DeviceID] = state
	return nil
}

func (s *Store) CreateEnrollmentToken(tokenHash string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tokenHash == "" {
		return errors.New("token_hash required")
	}
	s.tokens[tokenHash] = store.EnrollmentToken{TokenHash: tokenHash, ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}
	return nil
}

func (s *Store) ConsumeEnrollmentToken(tokenHash string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tokenHash == "" {
		return false, errors.New("token_hash required")
	}
	tok, ok := s.tokens[tokenHash]
	if !ok {
		return false, nil
	}
	if time.Now().UTC().After(tok.ExpiresAt) {
		delete(s.tokens, tokenHash)
		return false, nil
	}
	delete(s.tokens, tokenHash)
	return true, nil
}

func (s *Store) CreateDevice(device store.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if device.DeviceID == "" {
		return errors.New("device_id required")
	}
	if _, exists := s.devices[device.DeviceID]; exists {
		return errors.New("device already exists")
	}
	s.devices[device.DeviceID] = device
	return nil
}

func (s *Store) GetDevice(deviceID string) (store.Device, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[deviceID]
	return d, ok, nil
}

func (s *Store) GetDeviceByFingerprint(fingerprint string) (store.Device, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.devices {
		if d.CertFingerprint == fingerprint {
			return d, true, nil
		}
	}
	return store.Device{}, false, nil
}

func (s *Store) GetDeviceState(deviceID string) (store.DeviceState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.states[deviceID]
	return d, ok, nil
}

func (s *Store) ListDevices(filter store.ListDevicesFilter) ([]store.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]store.Device, 0, len(s.devices))
	for _, d := range s.devices {
		if filter.Status != "" && d.Status != filter.Status {
			continue
		}
		out = append(out, d)
	}

	// naive pagination for in-memory store
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	if start > len(out) {
		return []store.Device{}, nil
	}
	end := len(out)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return out[start:end], nil
}

func (s *Store) CountDevices() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.devices), nil
}

func (s *Store) DeleteDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if deviceID == "" {
		return errors.New("device_id required")
	}
	delete(s.devices, deviceID)
	delete(s.states, deviceID)
	delete(s.desiredDevices, deviceID)
	for id, res := range s.applyResults {
		if res.DeviceID == deviceID {
			delete(s.applyResults, id)
		}
	}
	return nil
}

func (s *Store) DeleteStaleDevices(cutoff time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for id, d := range s.devices {
		if !d.LastSeen.IsZero() && d.LastSeen.Before(cutoff) {
			delete(s.devices, id)
			delete(s.states, id)
			delete(s.desiredDevices, id)
			for arID, res := range s.applyResults {
				if res.DeviceID == id {
					delete(s.applyResults, arID)
				}
			}
			count++
		}
	}
	return count, nil
}

func (s *Store) UpdateDeviceStatuses(staleCutoff, offlineCutoff time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated := 0
	for id, d := range s.devices {
		prev := d.Status
		status := computeDeviceStatus(d.LastSeen, staleCutoff, offlineCutoff, s.states[id])
		if status != prev {
			d.Status = status
			s.devices[id] = d
			updated++
		}
	}
	return updated, nil
}

func computeDeviceStatus(lastSeen time.Time, staleCutoff, offlineCutoff time.Time, st store.DeviceState) string {
	if lastSeen.IsZero() || (!offlineCutoff.IsZero() && lastSeen.Before(offlineCutoff)) {
		return "offline"
	}
	if !staleCutoff.IsZero() && lastSeen.Before(staleCutoff) {
		return "stale"
	}
	if st.LastApplyStatus == "error" || st.LastPreApplyStatus == "error" {
		return "degraded"
	}
	return "active"
}

func (s *Store) UpsertGroup(group store.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if group.GroupID == "" {
		return errors.New("group_id required")
	}
	if len(group.SelectorJSON) == 0 {
		group.SelectorJSON = []byte("{}")
	}
	s.groups[group.GroupID] = group
	return nil
}

func (s *Store) ListGroups() ([]store.Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.Group, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, g)
	}
	return out, nil
}

func (s *Store) DeleteGroup(groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if groupID == "" {
		return errors.New("group_id required")
	}
	delete(s.groups, groupID)
	delete(s.desiredGroups, groupID)
	return nil
}

func (s *Store) UpsertDesiredStateGroup(state store.DesiredStateGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.GroupID == "" {
		return errors.New("group_id required")
	}
	s.desiredGroups[state.GroupID] = state
	return nil
}

func (s *Store) UpsertDesiredStateDevice(state store.DesiredStateDevice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.DeviceID == "" {
		return errors.New("device_id required")
	}
	s.desiredDevices[state.DeviceID] = state
	return nil
}

func (s *Store) GetDesiredStateDevice(deviceID string) (store.DesiredStateDevice, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.desiredDevices[deviceID]
	return d, ok, nil
}

func (s *Store) GetDesiredStateGroupForDevice(deviceID string) (store.DesiredStateGroup, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, ok := s.devices[deviceID]
	if !ok {
		return store.DesiredStateGroup{}, false, nil
	}
	labels := parseJSONMap(device.LabelsJSON)

	var picked store.DesiredStateGroup
	var found bool
	for _, g := range s.groups {
		selector := parseJSONMap(g.SelectorJSON)
		if !selectorMatches(labels, selector) {
			continue
		}
		state, ok := s.desiredGroups[g.GroupID]
		if !ok {
			continue
		}
		if !found || state.UpdatedAt.After(picked.UpdatedAt) {
			picked = state
			found = true
		}
	}

	return picked, found, nil
}

func (s *Store) ListDesiredStateGroups() ([]store.DesiredStateGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.DesiredStateGroup, 0, len(s.desiredGroups))
	for _, d := range s.desiredGroups {
		out = append(out, d)
	}
	return out, nil
}

func (s *Store) DeleteDesiredStateGroup(groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if groupID == "" {
		return errors.New("group_id required")
	}
	delete(s.desiredGroups, groupID)
	return nil
}

func (s *Store) ListDesiredStateDevices() ([]store.DesiredStateDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.DesiredStateDevice, 0, len(s.desiredDevices))
	for _, d := range s.desiredDevices {
		out = append(out, d)
	}
	return out, nil
}

func (s *Store) DeleteDesiredStateDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if deviceID == "" {
		return errors.New("device_id required")
	}
	delete(s.desiredDevices, deviceID)
	return nil
}

func (s *Store) CreateArtifact(artifact store.Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if artifact.ArtifactID == "" {
		return errors.New("artifact_id required")
	}
	if artifact.Type == "" {
		artifact.Type = "app_bundle"
	}
	s.artifacts[artifact.ArtifactID] = artifact
	return nil
}

func (s *Store) GetArtifact(artifactID string) (store.Artifact, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.artifacts[artifactID]
	return a, ok, nil
}

func (s *Store) ListArtifacts(name, version string, limit, offset int) ([]store.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.Artifact, 0, len(s.artifacts))
	for _, a := range s.artifacts {
		if name != "" && a.Name != name {
			continue
		}
		if version != "" && a.Version != version {
			continue
		}
		out = append(out, a)
	}
	start := offset
	if start < 0 {
		start = 0
	}
	if start > len(out) {
		return []store.Artifact{}, nil
	}
	end := len(out)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return out[start:end], nil
}

func (s *Store) DeleteArtifact(artifactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if artifactID == "" {
		return errors.New("artifact_id required")
	}
	delete(s.artifacts, artifactID)
	for id, d := range s.desiredDevices {
		if d.ArtifactID == artifactID {
			delete(s.desiredDevices, id)
		}
	}
	for id, g := range s.desiredGroups {
		if g.ArtifactID == artifactID {
			delete(s.desiredGroups, id)
		}
	}
	return nil
}

func (s *Store) CreateApplyResult(result store.ApplyResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.ApplyID == "" || result.DeviceID == "" {
		return errors.New("apply_id and device_id required")
	}
	s.applyResults[result.ApplyID] = result
	// update device_state with last apply fields
	st, ok := s.states[result.DeviceID]
	if ok {
		if result.AppliedVersion != "" {
			st.CurrentVersion = result.AppliedVersion
		}
		if result.AppliedConfigRev != "" {
			st.CurrentConfigRev = result.AppliedConfigRev
		}
		st.LastApplyStatus = result.Status
		st.LastApplyError = result.Error
		st.LastApplyAt = result.CreatedAt
		if result.ArtifactID != "" {
			st.LastApplyArtifactID = result.ArtifactID
		}
		if result.PreApplyStatus != "" {
			st.LastPreApplyStatus = result.PreApplyStatus
			st.LastPreApplyError = result.PreApplyError
			st.LastPreApplyAt = result.CreatedAt
		}
		st.UpdatedAt = time.Now().UTC()
		s.states[result.DeviceID] = st
	}
	return nil
}

func (s *Store) CreateAuditEvent(event store.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	s.auditEvents = append(s.auditEvents, event)
	return nil
}

func (s *Store) CreateUser(user store.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user.UserID == "" {
		return errors.New("user_id required")
	}
	if user.Email == "" {
		return errors.New("email required")
	}
	if _, ok := s.userEmailIndex[user.Email]; ok {
		return errors.New("email already exists")
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now
	s.users[user.UserID] = user
	s.userEmailIndex[user.Email] = user.UserID
	return nil
}

func (s *Store) GetUser(userID string) (store.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return store.User{}, false, nil
	}
	return user, true, nil
}

func (s *Store) GetUserByEmail(email string) (store.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.userEmailIndex[email]
	if !ok {
		return store.User{}, false, nil
	}
	user, ok := s.users[id]
	if !ok {
		return store.User{}, false, nil
	}
	return user, true, nil
}

func (s *Store) ListUsers(limit, offset int) ([]store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	out := make([]store.User, 0, len(s.users))
	for _, user := range s.users {
		out = append(out, user)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if offset >= len(out) {
		return []store.User{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return append([]store.User{}, out[offset:end]...), nil
}

func (s *Store) UpdateUser(update store.UserUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[update.UserID]
	if !ok {
		return errors.New("user not found")
	}
	if update.DisplayName != nil {
		user.DisplayName = *update.DisplayName
	}
	if update.RolesJSON != nil {
		user.RolesJSON = update.RolesJSON
	}
	if update.Disabled != nil {
		user.Disabled = *update.Disabled
	}
	if update.PasswordHash != nil {
		user.PasswordHash = *update.PasswordHash
	}
	user.UpdatedAt = time.Now().UTC()
	s.users[user.UserID] = user
	return nil
}

func (s *Store) SetUserLastLogin(userID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	user.LastLoginAt = at
	user.UpdatedAt = time.Now().UTC()
	s.users[user.UserID] = user
	return nil
}

func (s *Store) CreateAuthVoucher(voucher store.AuthVoucher) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if voucher.VoucherID == "" {
		return errors.New("voucher_id required")
	}
	if voucher.TokenHash == "" {
		return errors.New("token_hash required")
	}
	if _, ok := s.voucherTokenIdx[voucher.TokenHash]; ok {
		return errors.New("token already exists")
	}
	if voucher.ExpiresAt.IsZero() {
		return errors.New("expires_at required")
	}
	now := time.Now().UTC()
	if voucher.CreatedAt.IsZero() {
		voucher.CreatedAt = now
	}
	s.vouchers[voucher.VoucherID] = voucher
	s.voucherTokenIdx[voucher.TokenHash] = voucher.VoucherID
	return nil
}

func (s *Store) GetAuthVoucherByTokenHash(tokenHash string) (store.AuthVoucher, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.voucherTokenIdx[tokenHash]
	if !ok {
		return store.AuthVoucher{}, false, nil
	}
	v, ok := s.vouchers[id]
	if !ok {
		return store.AuthVoucher{}, false, nil
	}
	return v, true, nil
}

func (s *Store) MarkAuthVoucherUsed(voucherID, usedBy string, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.vouchers[voucherID]
	if !ok {
		return false, errors.New("voucher not found")
	}
	if v.Revoked || !v.UsedAt.IsZero() {
		return false, nil
	}
	if !v.ExpiresAt.IsZero() && time.Now().UTC().After(v.ExpiresAt) {
		return false, nil
	}
	v.UsedAt = at
	v.UsedBy = usedBy
	s.vouchers[voucherID] = v
	return true, nil
}

func (s *Store) ListAuditEvents(filter store.AuditEventFilter) ([]store.AuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	matches := make([]store.AuditEvent, 0, len(s.auditEvents))
	for _, ev := range s.auditEvents {
		if filter.Action != "" && ev.Action != filter.Action {
			continue
		}
		if filter.ActorType != "" && ev.ActorType != filter.ActorType {
			continue
		}
		if filter.ActorID != "" && ev.ActorID != filter.ActorID {
			continue
		}
		if filter.ActorEmail != "" && ev.ActorEmail != filter.ActorEmail {
			continue
		}
		if filter.TargetType != "" && ev.TargetType != filter.TargetType {
			continue
		}
		if filter.TargetID != "" && ev.TargetID != filter.TargetID {
			continue
		}
		if filter.Status != "" && ev.Status != filter.Status {
			continue
		}
		if !filter.Since.IsZero() && ev.OccurredAt.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && ev.OccurredAt.After(filter.Until) {
			continue
		}
		matches = append(matches, ev)
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].OccurredAt.After(matches[j].OccurredAt)
	})
	if filter.Offset >= len(matches) {
		return []store.AuditEvent{}, nil
	}
	end := filter.Offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	return append([]store.AuditEvent{}, matches[filter.Offset:end]...), nil
}

func (s *Store) DeleteAuditEventsBefore(cutoff time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cutoff.IsZero() {
		return 0, nil
	}
	kept := make([]store.AuditEvent, 0, len(s.auditEvents))
	deleted := 0
	for _, ev := range s.auditEvents {
		if ev.OccurredAt.Before(cutoff) {
			deleted++
			continue
		}
		kept = append(kept, ev)
	}
	s.auditEvents = kept
	return deleted, nil
}

func (s *Store) EnsureAuditRetentionDays(days int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if days <= 0 {
		days = 90
	}
	if s.auditRetention.Days != days {
		s.auditRetention = store.AuditRetention{Days: days, UpdatedAt: time.Now().UTC()}
	}
	return nil
}

func (s *Store) GetAuditRetentionDays() (store.AuditRetention, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.auditRetention, nil
}

func (s *Store) SetAuditRetentionDays(days int) (store.AuditRetention, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if days <= 0 {
		days = 90
	}
	s.auditRetention = store.AuditRetention{Days: days, UpdatedAt: time.Now().UTC()}
	return s.auditRetention, nil
}

func parseJSONMap(data []byte) map[string]interface{} {
	if len(data) == 0 {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func selectorMatches(labels, selector map[string]interface{}) bool {
	if len(selector) == 0 {
		return true
	}
	for k, v := range selector {
		if lv, ok := labels[k]; !ok || lv != v {
			return false
		}
	}
	return true
}
