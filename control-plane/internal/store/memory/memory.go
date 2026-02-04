package memory

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/hardwareops/control-plane/internal/store"
)

type Store struct {
	mu             sync.Mutex
	devices        map[string]store.Device
	states         map[string]store.DeviceState
	tokens         map[string]store.EnrollmentToken
	groups         map[string]store.Group
	desiredGroups  map[string]store.DesiredStateGroup
	desiredDevices map[string]store.DesiredStateDevice
	artifacts      map[string]store.Artifact
	applyResults   map[string]store.ApplyResult
}

func New() *Store {
	return &Store{
		devices:        map[string]store.Device{},
		states:         map[string]store.DeviceState{},
		tokens:         map[string]store.EnrollmentToken{},
		groups:         map[string]store.Group{},
		desiredGroups:  map[string]store.DesiredStateGroup{},
		desiredDevices: map[string]store.DesiredStateDevice{},
		artifacts:      map[string]store.Artifact{},
		applyResults:   map[string]store.ApplyResult{},
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

func (s *Store) ListDesiredStateDevices() ([]store.DesiredStateDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.DesiredStateDevice, 0, len(s.desiredDevices))
	for _, d := range s.desiredDevices {
		out = append(out, d)
	}
	return out, nil
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
