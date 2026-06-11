package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
)

type Store struct {
	mu                    sync.Mutex
	devices               map[string]store.Device
	states                map[string]store.DeviceState
	tokens                map[string]store.EnrollmentToken
	enrollmentProfiles    map[string]store.EnrollmentProfile
	enrollmentProfileIdx  map[string]string
	pendingEnrollments    map[string]store.PendingEnrollment
	groups                map[string]store.Group
	desiredGroups         map[string]store.DesiredStateGroup
	desiredDevices        map[string]store.DesiredStateDevice
	artifacts             map[string]store.Artifact
	artifactPolicy        store.ArtifactLifecyclePolicy
	artifactTrustPolicy   *store.ArtifactTrustPolicy
	trustedSigningKeys    map[string]store.TrustedSigningKey
	releaseAuto           store.ReleaseAutoUpdateSettings
	applyResults          map[string]store.ApplyResult
	runtimeEvents         []store.RuntimeEvent
	runtimeRetention      store.RuntimeEventRetention
	auditEvents           []store.AuditEvent
	auditRetention        store.AuditRetention
	certRotation          *store.CertRotationState
	users                 map[string]store.User
	userEmailIndex        map[string]string
	vouchers              map[string]store.AuthVoucher
	voucherTokenIdx       map[string]string
	serviceTokens         map[string]store.ServiceToken
	serviceTokenIdx       map[string]string
	passwordResetTokens   map[string]store.PasswordResetToken
	passwordResetTokenIdx map[string]string
	revokedSerials        map[string]struct{}
	attestations          map[string]store.AttestationRecord
	artifactVulnScans     []store.ArtifactVulnerabilityScan
	deviceVulnScans       []store.DeviceVulnerabilityScan
	webhooks              map[string]store.Webhook
	webhookDeliveries     []store.WebhookDelivery
	deployTriggers        map[string]store.DeployTrigger
	federationIngests     map[string]store.FederationIngest
	globalPolicyCaches    map[string]store.GlobalPolicyCache // keyed by groupID
	changeRecords         map[string]store.ChangeRecord      // keyed by recordID
	changeRecordArtIdx    map[string]string                   // artifactID → recordID
	vexAssertions         map[string]store.VexAssertion       // keyed by assertionID
	vexAssertionIdx       map[string]string                   // "artifactID|cveID|componentName" → assertionID
}

func New() *Store {
	return &Store{
		devices:               map[string]store.Device{},
		states:                map[string]store.DeviceState{},
		tokens:                map[string]store.EnrollmentToken{},
		enrollmentProfiles:    map[string]store.EnrollmentProfile{},
		enrollmentProfileIdx:  map[string]string{},
		pendingEnrollments:    map[string]store.PendingEnrollment{},
		groups:                map[string]store.Group{},
		desiredGroups:         map[string]store.DesiredStateGroup{},
		desiredDevices:        map[string]store.DesiredStateDevice{},
		artifacts:             map[string]store.Artifact{},
		artifactPolicy:        store.ArtifactLifecyclePolicy{DeprecatedDeleteAfterDays: 30, UpdatedAt: time.Now().UTC()},
		artifactTrustPolicy:   nil,
		trustedSigningKeys:    map[string]store.TrustedSigningKey{},
		releaseAuto:           store.ReleaseAutoUpdateSettings{Enabled: false, AllowUnsigned: false, UpdatedAt: time.Now().UTC()},
		applyResults:          map[string]store.ApplyResult{},
		runtimeEvents:         []store.RuntimeEvent{},
		runtimeRetention:      store.RuntimeEventRetention{Days: 30, UpdatedAt: time.Now().UTC()},
		auditEvents:           []store.AuditEvent{},
		auditRetention:        store.AuditRetention{Days: 90, UpdatedAt: time.Now().UTC()},
		certRotation:          nil,
		users:                 map[string]store.User{},
		userEmailIndex:        map[string]string{},
		vouchers:              map[string]store.AuthVoucher{},
		voucherTokenIdx:       map[string]string{},
		serviceTokens:         map[string]store.ServiceToken{},
		serviceTokenIdx:       map[string]string{},
		passwordResetTokens:   map[string]store.PasswordResetToken{},
		passwordResetTokenIdx: map[string]string{},
		revokedSerials:        map[string]struct{}{},
		attestations:          map[string]store.AttestationRecord{},
		artifactVulnScans:     []store.ArtifactVulnerabilityScan{},
		deviceVulnScans:       []store.DeviceVulnerabilityScan{},
		webhooks:              map[string]store.Webhook{},
		webhookDeliveries:     []store.WebhookDelivery{},
		deployTriggers:        map[string]store.DeployTrigger{},
		federationIngests:     map[string]store.FederationIngest{},
		globalPolicyCaches:    map[string]store.GlobalPolicyCache{},
		changeRecords:         map[string]store.ChangeRecord{},
		changeRecordArtIdx:    map[string]string{},
		vexAssertions:         map[string]store.VexAssertion{},
		vexAssertionIdx:       map[string]string{},
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
		if device.CertSerial == "" {
			device.CertSerial = existing.CertSerial
		}
		if len(device.LabelsJSON) == 0 {
			device.LabelsJSON = existing.LabelsJSON
		}
		if len(device.MetadataJSON) == 0 {
			device.MetadataJSON = existing.MetadataJSON
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
		if len(state.ComponentsJSON) == 0 {
			state.ComponentsJSON = prev.ComponentsJSON
		}
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

func (s *Store) EnrollDeviceWithToken(tokenHash string, device store.Device, maxDevices int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tokenHash == "" {
		return errors.New("token_hash required")
	}
	if device.DeviceID == "" {
		return errors.New("device_id required")
	}
	tok, ok := s.tokens[tokenHash]
	if !ok {
		return store.ErrEnrollmentTokenInvalid
	}
	if time.Now().UTC().After(tok.ExpiresAt) {
		delete(s.tokens, tokenHash)
		return store.ErrEnrollmentTokenInvalid
	}
	if maxDevices > 0 && len(s.devices) >= maxDevices {
		return store.ErrDeviceLimitExceeded
	}
	if _, exists := s.devices[device.DeviceID]; exists {
		return errors.New("device already exists")
	}
	if device.CertFingerprint != "" {
		for _, d := range s.devices {
			if d.CertFingerprint == device.CertFingerprint {
				return errors.New("device cert already exists")
			}
		}
	}
	s.devices[device.DeviceID] = device
	delete(s.tokens, tokenHash)
	return nil
}

func (s *Store) CreateEnrollmentProfile(profile store.EnrollmentProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if profile.ProfileID == "" {
		return errors.New("profile_id required")
	}
	if profile.Name == "" {
		return errors.New("name required")
	}
	if profile.TokenHash == "" {
		return errors.New("token_hash required")
	}
	if _, exists := s.enrollmentProfiles[profile.ProfileID]; exists {
		return errors.New("enrollment profile already exists")
	}
	if existingID, exists := s.enrollmentProfileIdx[profile.TokenHash]; exists && existingID != "" {
		return errors.New("enrollment profile token already exists")
	}
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = time.Now().UTC()
	}
	profile.PreviousTokenHash = ""
	profile.PreviousTokenExpiresAt = time.Time{}
	profile.TokenRotatedAt = time.Time{}
	s.enrollmentProfiles[profile.ProfileID] = profile
	s.enrollmentProfileIdx[profile.TokenHash] = profile.ProfileID
	return nil
}

func (s *Store) ListEnrollmentProfiles() ([]store.EnrollmentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.EnrollmentProfile, 0, len(s.enrollmentProfiles))
	for _, profile := range s.enrollmentProfiles {
		out = append(out, profile)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) GetEnrollmentProfile(profileID string) (store.EnrollmentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.enrollmentProfiles[profileID]
	if !ok {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileNotFound
	}
	return profile, nil
}

func (s *Store) UpdateEnrollmentProfile(profileID string, update store.EnrollmentProfileUpdate) (store.EnrollmentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.enrollmentProfiles[profileID]
	if !ok {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileNotFound
	}
	profile.Name = update.Name
	profile.RequireApproval = update.RequireApproval
	profile.AllowUntrustedHW = update.AllowUntrustedHW
	profile.ChallengeHash = update.ChallengeHash
	profile.ChallengeHint = update.ChallengeHint
	profile.ApprovalDelaySec = update.ApprovalDelaySec
	profile.MaxUses = update.MaxUses
	profile.CertValidityDays = update.CertValidityDays
	profile.DefaultLabelsJSON = update.DefaultLabelsJSON
	s.enrollmentProfiles[profileID] = profile
	return profile, nil
}

func (s *Store) GetEnrollmentProfileByTokenHash(profileTokenHash string) (store.EnrollmentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resolveEnrollmentProfileByTokenHashLocked(profileTokenHash, time.Now().UTC())
}

func (s *Store) SetEnrollmentProfileDisabled(profileID string, disabled bool) (store.EnrollmentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.enrollmentProfiles[profileID]
	if !ok {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileNotFound
	}
	profile.Disabled = disabled
	s.enrollmentProfiles[profileID] = profile
	return profile, nil
}

func (s *Store) RotateEnrollmentProfileToken(profileID, tokenHash string, previousTokenValidUntil time.Time) (store.EnrollmentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.enrollmentProfiles[profileID]
	if !ok {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileNotFound
	}
	if tokenHash == "" {
		return store.EnrollmentProfile{}, errors.New("token_hash required")
	}
	if existingID, exists := s.enrollmentProfileIdx[tokenHash]; exists && existingID != "" && existingID != profileID {
		return store.EnrollmentProfile{}, errors.New("enrollment profile token already exists")
	}
	now := time.Now().UTC()
	delete(s.enrollmentProfileIdx, profile.PreviousTokenHash)
	currentTokenHash := profile.TokenHash
	if !previousTokenValidUntil.IsZero() && previousTokenValidUntil.After(now) {
		profile.PreviousTokenHash = currentTokenHash
		profile.PreviousTokenExpiresAt = previousTokenValidUntil
		s.enrollmentProfileIdx[currentTokenHash] = profileID
	} else {
		delete(s.enrollmentProfileIdx, currentTokenHash)
		profile.PreviousTokenHash = ""
		profile.PreviousTokenExpiresAt = time.Time{}
	}
	profile.TokenHash = tokenHash
	profile.TokenRotatedAt = now
	s.enrollmentProfiles[profileID] = profile
	s.enrollmentProfileIdx[tokenHash] = profileID
	return profile, nil
}

func (s *Store) CreatePendingEnrollmentForProfileToken(profileTokenHash string, pending store.PendingEnrollment) (store.EnrollmentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if profileTokenHash == "" {
		return store.EnrollmentProfile{}, errors.New("profile_token_hash required")
	}
	if pending.RequestID == "" {
		return store.EnrollmentProfile{}, errors.New("request_id required")
	}
	if pending.ClaimTokenHash == "" {
		return store.EnrollmentProfile{}, errors.New("claim_token_hash required")
	}
	if pending.CSR == "" {
		return store.EnrollmentProfile{}, errors.New("csr required")
	}
	if pending.ExpiresAt.IsZero() {
		return store.EnrollmentProfile{}, errors.New("expires_at required")
	}
	now := time.Now().UTC()
	profile, err := s.resolveEnrollmentProfileByTokenHashLocked(profileTokenHash, now)
	if err != nil {
		return store.EnrollmentProfile{}, err
	}
	if _, exists := s.pendingEnrollments[pending.RequestID]; exists {
		return store.EnrollmentProfile{}, errors.New("pending enrollment already exists")
	}
	if pending.Status == "" {
		pending.Status = "pending"
	}
	if pending.CreatedAt.IsZero() {
		pending.CreatedAt = now
	}
	if pending.ApprovalAvailableAt.IsZero() {
		pending.ApprovalAvailableAt = pending.CreatedAt
	}
	pending.ProfileID = profile.ProfileID
	s.pendingEnrollments[pending.RequestID] = pending
	profile.Uses++
	s.enrollmentProfiles[profile.ProfileID] = profile
	return profile, nil
}

func (s *Store) resolveEnrollmentProfileByTokenHashLocked(profileTokenHash string, now time.Time) (store.EnrollmentProfile, error) {
	profileID, ok := s.enrollmentProfileIdx[profileTokenHash]
	if !ok {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	profile, ok := s.enrollmentProfiles[profileID]
	if !ok {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	if profile.PreviousTokenHash != "" && profileTokenHash == profile.PreviousTokenHash {
		if profile.PreviousTokenExpiresAt.IsZero() || now.After(profile.PreviousTokenExpiresAt) {
			delete(s.enrollmentProfileIdx, profile.PreviousTokenHash)
			profile.PreviousTokenHash = ""
			profile.PreviousTokenExpiresAt = time.Time{}
			s.enrollmentProfiles[profileID] = profile
			return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
		}
	}
	if profile.Disabled || (!profile.ExpiresAt.IsZero() && now.After(profile.ExpiresAt)) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	if profile.MaxUses > 0 && profile.Uses >= profile.MaxUses {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileExhausted
	}
	return profile, nil
}

func (s *Store) ListPendingEnrollments(filter store.PendingEnrollmentFilter) ([]store.PendingEnrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	out := make([]store.PendingEnrollment, 0, len(s.pendingEnrollments))
	for _, pending := range s.pendingEnrollments {
		if filter.Status != "" && pending.Status != filter.Status {
			continue
		}
		out = append(out, pending)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if offset >= len(out) {
		return []store.PendingEnrollment{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (s *Store) ApprovePendingEnrollment(requestID, approvedByUserID string, at time.Time) (store.PendingEnrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingEnrollments[requestID]
	if !ok {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	if pending.Status != "pending" {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
	}
	now := time.Now().UTC()
	if at.IsZero() {
		at = now
	}
	if now.After(pending.ExpiresAt) {
		pending.Status = "expired"
		s.pendingEnrollments[requestID] = pending
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
	}
	if !pending.ApprovalAvailableAt.IsZero() && pending.ApprovalAvailableAt.After(at) {
		return pending, store.ErrPendingEnrollmentThrottled
	}
	pending.Status = "approved"
	pending.ApprovedAt = at
	pending.ApprovedByUserID = approvedByUserID
	s.pendingEnrollments[requestID] = pending
	return pending, nil
}

func (s *Store) DenyPendingEnrollment(requestID, reason, deniedByUserID string, at time.Time) (store.PendingEnrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingEnrollments[requestID]
	if !ok {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	if pending.Status != "pending" && pending.Status != "approved" {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	pending.Status = "denied"
	pending.DeniedReason = reason
	pending.DeniedAt = at
	pending.DeniedByUserID = deniedByUserID
	s.pendingEnrollments[requestID] = pending
	return pending, nil
}

func (s *Store) ConflictPendingEnrollment(requestID, reason string, at time.Time) (store.PendingEnrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingEnrollments[requestID]
	if !ok {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	if pending.Status != "pending" && pending.Status != "approved" {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	pending.Status = "conflict"
	pending.DeniedReason = reason
	pending.DeniedAt = at
	s.pendingEnrollments[requestID] = pending
	return pending, nil
}

func (s *Store) ResetPendingEnrollment(requestID string, expiresAt time.Time) (store.PendingEnrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingEnrollments[requestID]
	if !ok {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	switch pending.Status {
	case "denied", "conflict", "expired":
	default:
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(15 * time.Minute)
	}
	pending.Status = "pending"
	pending.ExpiresAt = expiresAt
	pending.DeniedReason = ""
	pending.ApprovedAt = time.Time{}
	pending.ApprovedByUserID = ""
	pending.DeniedAt = time.Time{}
	pending.DeniedByUserID = ""
	pending.IssuedAt = time.Time{}
	pending.IssuedDeviceID = ""
	s.pendingEnrollments[requestID] = pending
	return pending, nil
}

func (s *Store) ExpirePendingEnrollments(before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for requestID, pending := range s.pendingEnrollments {
		if (pending.Status == "pending" || pending.Status == "approved") && !pending.ExpiresAt.IsZero() && !before.Before(pending.ExpiresAt) {
			pending.Status = "expired"
			s.pendingEnrollments[requestID] = pending
			count++
		}
	}
	return count, nil
}

func (s *Store) CountActivePendingEnrollments(profileID, sourceIP string, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, pending := range s.pendingEnrollments {
		if pending.Status != "pending" && pending.Status != "approved" {
			continue
		}
		if !pending.ExpiresAt.IsZero() && !pending.ExpiresAt.After(now) {
			continue
		}
		if profileID != "" && pending.ProfileID != profileID {
			continue
		}
		if sourceIP != "" && pending.SourceIP != sourceIP {
			continue
		}
		count++
	}
	return count, nil
}

func (s *Store) GetPendingEnrollmentForClaim(requestID, claimTokenHash string) (store.PendingEnrollment, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingEnrollments[requestID]
	if !ok {
		return store.PendingEnrollment{}, false, nil
	}
	if pending.ClaimTokenHash != claimTokenHash {
		return store.PendingEnrollment{}, false, nil
	}
	if (pending.Status == "pending" || pending.Status == "approved") && time.Now().UTC().After(pending.ExpiresAt) {
		pending.Status = "expired"
		s.pendingEnrollments[requestID] = pending
	}
	return pending, true, nil
}

func (s *Store) MarkPendingEnrollmentIssued(requestID, claimTokenHash string, device store.Device, maxDevices int, issuedAt time.Time) (store.PendingEnrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingEnrollments[requestID]
	if !ok {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	if pending.ClaimTokenHash != claimTokenHash {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentToken
	}
	if pending.Status != "approved" {
		if (pending.Status == "pending" || pending.Status == "approved") && time.Now().UTC().After(pending.ExpiresAt) {
			pending.Status = "expired"
			s.pendingEnrollments[requestID] = pending
		}
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
	}
	if maxDevices > 0 && len(s.devices) >= maxDevices {
		return store.PendingEnrollment{}, store.ErrDeviceLimitExceeded
	}
	if device.DeviceID == "" {
		return store.PendingEnrollment{}, errors.New("device_id required")
	}
	if _, exists := s.devices[device.DeviceID]; exists {
		return store.PendingEnrollment{}, errors.New("device already exists")
	}
	if device.CertFingerprint != "" {
		for _, d := range s.devices {
			if d.CertFingerprint == device.CertFingerprint {
				return store.PendingEnrollment{}, errors.New("device cert already exists")
			}
		}
	}
	s.devices[device.DeviceID] = device
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	pending.Status = "issued"
	pending.IssuedAt = issuedAt
	pending.IssuedDeviceID = device.DeviceID
	s.pendingEnrollments[requestID] = pending
	return pending, nil
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
	if device.CertFingerprint != "" {
		for _, d := range s.devices {
			if d.CertFingerprint == device.CertFingerprint {
				return errors.New("device cert already exists")
			}
		}
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

func (s *Store) GetDeviceByHardwareID(hardwareID string) (store.Device, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if hardwareID == "" {
		return store.Device{}, false, nil
	}
	for _, d := range s.devices {
		if deviceHardwareID(d.MetadataJSON) == hardwareID {
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

func (s *Store) CountDevicesByStatus(status string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, d := range s.devices {
		if d.Status == status {
			count++
		}
	}
	return count, nil
}

func (s *Store) LatestDeviceSeen() (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest time.Time
	for _, d := range s.devices {
		if d.LastSeen.After(latest) {
			latest = d.LastSeen
		}
	}
	return latest, nil
}

func (s *Store) DeleteDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if deviceID == "" {
		return errors.New("device_id required")
	}
	if d, ok := s.devices[deviceID]; ok && d.CertSerial != "" {
		s.revokedSerials[d.CertSerial] = struct{}{}
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

func (s *Store) RevokeDeviceCertSerial(serial, deviceID string, at time.Time) error {
	if serial == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokedSerials[serial] = struct{}{}
	return nil
}

func (s *Store) IsCertSerialRevoked(serial string) (bool, error) {
	if serial == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, revoked := s.revokedSerials[serial]
	return revoked, nil
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
	if componentsHaveErrors(st.ComponentsJSON) {
		return "degraded"
	}
	if st.LastApplyStatus == "error" || st.LastPreApplyStatus == "error" {
		return "degraded"
	}
	return "active"
}

func deviceHardwareID(metadata []byte) string {
	if len(metadata) == 0 {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal(metadata, &raw); err != nil {
		return ""
	}
	hwops, _ := raw["hwops"].(map[string]any)
	identity, _ := hwops["identity"].(map[string]any)
	id, _ := identity["hardwareId"].(string)
	return id
}

func componentsHaveErrors(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var comps map[string]map[string]any
	if err := json.Unmarshal(raw, &comps); err != nil {
		return false
	}
	for _, comp := range comps {
		if val, ok := comp["lastApplyStatus"].(string); ok && val == "error" {
			return true
		}
		if val, ok := comp["lastPreApplyStatus"].(string); ok && val == "error" {
			return true
		}
	}
	return false
}

func pruneComponentsJSON(raw []byte, artifactID string) []byte {
	if len(raw) == 0 || artifactID == "" {
		return nil
	}
	var comps map[string]map[string]any
	if err := json.Unmarshal(raw, &comps); err != nil {
		return nil
	}
	changed := false
	for key, comp := range comps {
		if val, ok := comp["artifactId"].(string); ok && val == artifactID {
			delete(comps, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if len(comps) == 0 {
		return []byte("{}")
	}
	updated, err := json.Marshal(comps)
	if err != nil {
		return nil
	}
	return updated
}

func countArtifactRefsInComponents(raw []byte, artifactID string) int {
	if len(raw) == 0 || artifactID == "" {
		return 0
	}
	var comps map[string]map[string]any
	if err := json.Unmarshal(raw, &comps); err != nil {
		return 0
	}
	count := 0
	for _, comp := range comps {
		if val, ok := comp["artifactId"].(string); ok && val == artifactID {
			count++
		}
	}
	return count
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

func (s *Store) GetDesiredStateGroup(groupID string) (store.DesiredStateGroup, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.desiredGroups[groupID]
	return d, ok, nil
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
	if artifact.Status == "" {
		artifact.Status = "active"
	}
	if artifact.VerificationStatus == "" {
		artifact.VerificationStatus = "legacy"
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

func (s *Store) SetArtifactSBOMObjectKey(artifactID, sbomObjectKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.artifacts[artifactID]
	if !ok {
		return errors.New("artifact not found")
	}
	a.SBOMObjectKey = sbomObjectKey
	s.artifacts[artifactID] = a
	return nil
}

func (s *Store) SetArtifactVexObjectKey(artifactID, vexObjectKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.artifacts[artifactID]
	if !ok {
		return errors.New("artifact not found")
	}
	a.VexObjectKey = vexObjectKey
	s.artifacts[artifactID] = a
	return nil
}

func (s *Store) UpsertVexAssertion(assertion store.VexAssertion) (store.VexAssertion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idxKey := assertion.ArtifactID + "|" + assertion.CVEID + "|" + assertion.ComponentName
	if existing, ok := s.vexAssertionIdx[idxKey]; ok {
		assertion.AssertionID = existing
	}
	if assertion.AssertionID == "" {
		return store.VexAssertion{}, errors.New("assertionID required")
	}
	assertion.CreatedAt = time.Now().UTC()
	s.vexAssertions[assertion.AssertionID] = assertion
	s.vexAssertionIdx[idxKey] = assertion.AssertionID
	return assertion, nil
}

func (s *Store) ListVexAssertions(artifactID string) ([]store.VexAssertion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.VexAssertion
	for _, a := range s.vexAssertions {
		if a.ArtifactID == artifactID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *Store) FindArtifactByNameTypeVersion(name, artifactType, version string) (store.Artifact, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best store.Artifact
	found := false
	for _, a := range s.artifacts {
		if !strings.EqualFold(strings.TrimSpace(a.Status), "active") {
			continue
		}
		if a.Name != name || a.Type != artifactType || a.Version != version {
			continue
		}
		if !found {
			best = a
			found = true
			continue
		}
		if a.CreatedAt.After(best.CreatedAt) || (a.CreatedAt.Equal(best.CreatedAt) && a.ArtifactID > best.ArtifactID) {
			best = a
		}
	}
	return best, found, nil
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
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
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

func (s *Store) CountArtifactsByVerificationStatus(status string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status == "" {
		return 0, errors.New("verification status required")
	}
	count := 0
	for _, a := range s.artifacts {
		if a.VerificationStatus == status {
			count++
		}
	}
	return count, nil
}

func (s *Store) DeprecateArtifact(artifactID string, deprecatedAt, deleteAfter time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if artifactID == "" {
		return errors.New("artifact_id required")
	}
	artifact, ok := s.artifacts[artifactID]
	if !ok {
		return nil
	}
	artifact.Status = "deprecated"
	artifact.DeprecatedAt = deprecatedAt
	artifact.DeleteAfter = deleteAfter
	s.artifacts[artifactID] = artifact
	return nil
}

func (s *Store) RestoreArtifact(artifactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if artifactID == "" {
		return errors.New("artifact_id required")
	}
	artifact, ok := s.artifacts[artifactID]
	if !ok {
		return nil
	}
	artifact.Status = "active"
	artifact.DeprecatedAt = time.Time{}
	artifact.DeleteAfter = time.Time{}
	s.artifacts[artifactID] = artifact
	return nil
}

func (s *Store) ListTrustedSigningKeys(includeRetired bool) ([]store.TrustedSigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.TrustedSigningKey, 0, len(s.trustedSigningKeys))
	for _, key := range s.trustedSigningKeys {
		if !includeRetired && key.State == "retired" {
			continue
		}
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) GetTrustedSigningKey(keyID string) (store.TrustedSigningKey, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.trustedSigningKeys[keyID]
	return key, ok, nil
}

func (s *Store) UpsertTrustedSigningKey(key store.TrustedSigningKey) (store.TrustedSigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key.KeyID == "" {
		return store.TrustedSigningKey{}, errors.New("key_id required")
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now().UTC()
	}
	if key.State == "" {
		key.State = "active"
	}
	if existing, ok := s.trustedSigningKeys[key.KeyID]; ok {
		if key.CreatedAt.IsZero() {
			key.CreatedAt = existing.CreatedAt
		}
		if key.State == "" {
			key.State = existing.State
		}
		if key.RetiredAt.IsZero() {
			key.RetiredAt = existing.RetiredAt
		}
	}
	s.trustedSigningKeys[key.KeyID] = key
	return key, nil
}

func (s *Store) RetireTrustedSigningKey(keyID string, retiredAt time.Time) (store.TrustedSigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.trustedSigningKeys[keyID]
	if !ok {
		return store.TrustedSigningKey{}, errors.New("trusted signing key not found")
	}
	key.State = "retired"
	key.RetiredAt = retiredAt
	s.trustedSigningKeys[keyID] = key
	return key, nil
}

func (s *Store) GetArtifactTrustPolicy() (store.ArtifactTrustPolicy, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.artifactTrustPolicy == nil {
		return store.ArtifactTrustPolicy{}, false, nil
	}
	return *s.artifactTrustPolicy, true, nil
}

func (s *Store) SetArtifactTrustPolicy(policy store.ArtifactTrustPolicy) (store.ArtifactTrustPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if policy.UpdatedAt.IsZero() {
		policy.UpdatedAt = time.Now().UTC()
	}
	copy := policy
	s.artifactTrustPolicy = &copy
	return copy, nil
}

func (s *Store) ListArtifactsForPrune(cutoff time.Time, limit int) ([]store.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	out := make([]store.Artifact, 0, limit)
	for _, a := range s.artifacts {
		if a.Status != "deprecated" || a.DeleteAfter.IsZero() || a.DeleteAfter.After(cutoff) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].DeleteAfter.Before(out[j].DeleteAfter)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) CountArtifactReferences(artifactID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if artifactID == "" {
		return 0, errors.New("artifact_id required")
	}
	count := 0
	for _, d := range s.desiredDevices {
		if d.ArtifactID == artifactID {
			count++
		}
		count += countArtifactRefsInComponents(d.ComponentsJSON, artifactID)
	}
	for _, g := range s.desiredGroups {
		if g.ArtifactID == artifactID {
			count++
		}
		count += countArtifactRefsInComponents(g.ComponentsJSON, artifactID)
	}
	return count, nil
}

func (s *Store) GetArtifactLifecyclePolicy() (store.ArtifactLifecyclePolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.artifactPolicy, nil
}

func (s *Store) SetArtifactLifecyclePolicy(days int) (store.ArtifactLifecyclePolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if days < 1 {
		return store.ArtifactLifecyclePolicy{}, errors.New("days must be >= 1")
	}
	s.artifactPolicy = store.ArtifactLifecyclePolicy{
		DeprecatedDeleteAfterDays: days,
		UpdatedAt:                 time.Now().UTC(),
	}
	return s.artifactPolicy, nil
}

func (s *Store) GetReleaseAutoUpdateSettings() (store.ReleaseAutoUpdateSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.releaseAuto, nil
}

func (s *Store) SetReleaseAutoUpdateSettings(settings store.ReleaseAutoUpdateSettings) (store.ReleaseAutoUpdateSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings.UpdatedAt = time.Now().UTC()
	s.releaseAuto = settings
	return s.releaseAuto, nil
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
			continue
		}
		if updated := pruneComponentsJSON(d.ComponentsJSON, artifactID); updated != nil {
			d.ComponentsJSON = updated
			s.desiredDevices[id] = d
		}
	}
	for id, g := range s.desiredGroups {
		if g.ArtifactID == artifactID {
			delete(s.desiredGroups, id)
			continue
		}
		if updated := pruneComponentsJSON(g.ComponentsJSON, artifactID); updated != nil {
			g.ComponentsJSON = updated
			s.desiredGroups[id] = g
		}
	}
	return nil
}

func (s *Store) GetArtifactStats() (store.ArtifactStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := store.ArtifactStats{Count: len(s.artifacts)}
	for _, a := range s.artifacts {
		stats.SizeBytes += a.SizeBytes
	}
	return stats, nil
}

func (s *Store) CreateApplyResult(result store.ApplyResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.ApplyID == "" || result.DeviceID == "" {
		return errors.New("apply_id and device_id required")
	}
	s.applyResults[result.ApplyID] = result
	return nil
}

func (s *Store) CreateRuntimeEvent(event store.RuntimeEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if event.Type == "" {
		return errors.New("event type required")
	}
	s.runtimeEvents = append(s.runtimeEvents, event)
	return nil
}

func (s *Store) ListRuntimeEvents(filter store.RuntimeEventFilter) ([]store.RuntimeEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	matches := make([]store.RuntimeEvent, 0, len(s.runtimeEvents))
	for _, ev := range s.runtimeEvents {
		if filter.Type != "" && ev.Type != filter.Type {
			continue
		}
		if filter.DeviceID != "" && ev.DeviceID != filter.DeviceID {
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
		return []store.RuntimeEvent{}, nil
	}
	end := filter.Offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	return append([]store.RuntimeEvent{}, matches[filter.Offset:end]...), nil
}

func (s *Store) DeleteRuntimeEventsBefore(cutoff time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cutoff.IsZero() {
		return 0, nil
	}
	kept := make([]store.RuntimeEvent, 0, len(s.runtimeEvents))
	deleted := 0
	for _, ev := range s.runtimeEvents {
		if ev.OccurredAt.Before(cutoff) {
			deleted++
			continue
		}
		kept = append(kept, ev)
	}
	s.runtimeEvents = kept
	return deleted, nil
}

func (s *Store) EnsureRuntimeEventRetentionDays(days int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if days <= 0 {
		days = 30
	}
	if s.runtimeRetention.Days != days {
		s.runtimeRetention = store.RuntimeEventRetention{Days: days, UpdatedAt: time.Now().UTC()}
	}
	return nil
}

func (s *Store) GetRuntimeEventRetentionDays() (store.RuntimeEventRetention, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtimeRetention, nil
}

func (s *Store) SetRuntimeEventRetentionDays(days int) (store.RuntimeEventRetention, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if days <= 0 {
		days = 30
	}
	s.runtimeRetention = store.RuntimeEventRetention{Days: days, UpdatedAt: time.Now().UTC()}
	return s.runtimeRetention, nil
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

func (s *Store) GetUserByExternalID(provider, externalID string) (store.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, user := range s.users {
		if user.AuthProvider == provider && user.ExternalID == externalID && !user.Disabled {
			return user, true, nil
		}
	}
	return store.User{}, false, nil
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

func (s *Store) SetUserRecoveryCodes(userID string, recoveryCodesJSON []byte, generatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	if len(recoveryCodesJSON) == 0 {
		recoveryCodesJSON = []byte(`[]`)
	}
	user.RecoveryCodesJSON = append([]byte(nil), recoveryCodesJSON...)
	user.RecoveryCodesGeneratedAt = generatedAt
	user.UpdatedAt = time.Now().UTC()
	s.users[user.UserID] = user
	return nil
}

func (s *Store) SetUserTOTP(userID, encryptedSecret string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	user.TOTPSecret = encryptedSecret
	user.TOTPEnabled = enabled
	user.UpdatedAt = time.Now().UTC()
	s.users[user.UserID] = user
	return nil
}

func (s *Store) ConsumeUserRecoveryCode(email, recoveryCodeHash, passwordHash string, at time.Time) (store.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.userEmailIndex[email]
	if !ok {
		return store.User{}, false, nil
	}
	user, ok := s.users[userID]
	if !ok {
		return store.User{}, false, nil
	}
	if user.Disabled {
		return store.User{}, false, nil
	}
	var hashes []string
	if len(user.RecoveryCodesJSON) > 0 {
		if err := json.Unmarshal(user.RecoveryCodesJSON, &hashes); err != nil {
			return store.User{}, false, err
		}
	}
	match := -1
	for i, hash := range hashes {
		if hash == recoveryCodeHash {
			match = i
			break
		}
	}
	if match < 0 {
		return store.User{}, false, nil
	}
	hashes = append(hashes[:match], hashes[match+1:]...)
	nextJSON, err := json.Marshal(hashes)
	if err != nil {
		return store.User{}, false, err
	}
	user.RecoveryCodesJSON = nextJSON
	user.PasswordHash = passwordHash
	user.UpdatedAt = at
	s.users[user.UserID] = user
	return user, true, nil
}

func (s *Store) CreatePasswordResetToken(token store.PasswordResetToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token.TokenID == "" {
		return errors.New("token_id required")
	}
	if token.UserID == "" {
		return errors.New("user_id required")
	}
	if token.TokenHash == "" {
		return errors.New("token_hash required")
	}
	if _, ok := s.users[token.UserID]; !ok {
		return errors.New("user not found")
	}
	if existingID, ok := s.passwordResetTokenIdx[token.TokenHash]; ok {
		if existing, ok := s.passwordResetTokens[existingID]; ok && existing.UsedAt.IsZero() {
			return errors.New("password reset token already exists")
		}
	}
	s.passwordResetTokens[token.TokenID] = token
	s.passwordResetTokenIdx[token.TokenHash] = token.TokenID
	return nil
}

func (s *Store) ConsumePasswordResetToken(email, tokenHash, passwordHash string, at time.Time) (store.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokenID, ok := s.passwordResetTokenIdx[tokenHash]
	if !ok {
		return store.User{}, false, nil
	}
	token, ok := s.passwordResetTokens[tokenID]
	if !ok {
		return store.User{}, false, nil
	}
	if !token.UsedAt.IsZero() || (!token.ExpiresAt.IsZero() && at.After(token.ExpiresAt)) {
		return store.User{}, false, nil
	}
	user, ok := s.users[token.UserID]
	if !ok || user.Disabled || user.Email != email {
		return store.User{}, false, nil
	}
	token.UsedAt = at
	s.passwordResetTokens[token.TokenID] = token
	user.PasswordHash = passwordHash
	user.UpdatedAt = at
	s.users[user.UserID] = user
	return user, true, nil
}

func (s *Store) UpdatePasswordResetTokenEmailSent(tokenID string, sentAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.passwordResetTokens[tokenID]
	if !ok {
		return errors.New("password reset token not found")
	}
	token.EmailSentAt = sentAt
	s.passwordResetTokens[tokenID] = token
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

func (s *Store) CreateServiceToken(token store.ServiceToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token.TokenID == "" {
		return errors.New("token_id required")
	}
	if token.Name == "" {
		return errors.New("name required")
	}
	if token.TokenHash == "" {
		return errors.New("token_hash required")
	}
	if token.ExpiresAt.IsZero() {
		return errors.New("expires_at required")
	}
	if _, ok := s.serviceTokenIdx[token.TokenHash]; ok {
		return errors.New("token already exists")
	}
	now := time.Now().UTC()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = now
	}
	s.serviceTokens[token.TokenID] = token
	s.serviceTokenIdx[token.TokenHash] = token.TokenID
	return nil
}

func (s *Store) GetServiceToken(tokenID string) (store.ServiceToken, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.serviceTokens[tokenID]
	if !ok {
		return store.ServiceToken{}, false, nil
	}
	return token, true, nil
}

func (s *Store) GetServiceTokenByTokenHash(tokenHash string) (store.ServiceToken, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokenID, ok := s.serviceTokenIdx[tokenHash]
	if !ok {
		return store.ServiceToken{}, false, nil
	}
	token, ok := s.serviceTokens[tokenID]
	if !ok {
		return store.ServiceToken{}, false, nil
	}
	return token, true, nil
}

func (s *Store) ListServiceTokens(limit, offset int) ([]store.ServiceToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	out := make([]store.ServiceToken, 0, len(s.serviceTokens))
	for _, token := range s.serviceTokens {
		out = append(out, token)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if offset >= len(out) {
		return []store.ServiceToken{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return append([]store.ServiceToken{}, out[offset:end]...), nil
}

func (s *Store) SetServiceTokenLastUsed(tokenID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.serviceTokens[tokenID]
	if !ok {
		return errors.New("service token not found")
	}
	token.LastUsedAt = at
	s.serviceTokens[tokenID] = token
	return nil
}

func (s *Store) RevokeServiceToken(tokenID, revokedBy string, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.serviceTokens[tokenID]
	if !ok {
		return false, nil
	}
	if !token.RevokedAt.IsZero() {
		return false, nil
	}
	token.RevokedAt = at
	token.RevokedBy = revokedBy
	s.serviceTokens[tokenID] = token
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
		if filter.PhiTouched != nil && ev.PhiTouched != *filter.PhiTouched {
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

func (s *Store) GetCertRotationState() (store.CertRotationState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.certRotation == nil {
		return store.CertRotationState{}, false, nil
	}
	return *s.certRotation, true, nil
}

func (s *Store) SetCertRotationState(state store.CertRotationState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := state
	s.certRotation = &copy
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

// --- Attestations ------------------------------------------------------------

func (s *Store) CreateAttestation(rec store.AttestationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec.AttestationID = uuid.NewString()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	s.attestations[rec.AttestationID] = rec
	return nil
}

func (s *Store) GetAttestation(attestationID string) (store.AttestationRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.attestations[attestationID]
	return rec, ok, nil
}

func (s *Store) ListAttestations(artifactID string) ([]store.AttestationRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.AttestationRecord
	for _, rec := range s.attestations {
		if rec.ArtifactID == artifactID {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) DeleteAttestationsForArtifact(artifactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rec := range s.attestations {
		if rec.ArtifactID == artifactID {
			delete(s.attestations, id)
		}
	}
	return nil
}

// --- Vulnerability scans (artifact) ------------------------------------------

func (s *Store) CreateArtifactVulnScan(scan store.ArtifactVulnerabilityScan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scan.ScanID = uuid.NewString()
	if scan.CreatedAt.IsZero() {
		scan.CreatedAt = time.Now().UTC()
	}
	s.artifactVulnScans = append(s.artifactVulnScans, scan)
	return nil
}

func (s *Store) UpdateArtifactVulnScan(scan store.ArtifactVulnerabilityScan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, rec := range s.artifactVulnScans {
		if rec.ScanID == scan.ScanID {
			s.artifactVulnScans[i] = scan
			return nil
		}
	}
	return nil
}

func (s *Store) GetLatestArtifactVulnScan(artifactID string) (store.ArtifactVulnerabilityScan, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest store.ArtifactVulnerabilityScan
	found := false
	for _, rec := range s.artifactVulnScans {
		if rec.ArtifactID == artifactID {
			if !found || rec.CreatedAt.After(latest.CreatedAt) {
				latest = rec
				found = true
			}
		}
	}
	return latest, found, nil
}

func (s *Store) ListArtifactVulnScans(artifactID string) ([]store.ArtifactVulnerabilityScan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.ArtifactVulnerabilityScan
	for _, rec := range s.artifactVulnScans {
		if rec.ArtifactID == artifactID {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// --- Vulnerability scans (device) --------------------------------------------

func (s *Store) UpsertDeviceVulnScan(scan store.DeviceVulnerabilityScan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scan.ScanID = uuid.NewString()
	if scan.CreatedAt.IsZero() {
		scan.CreatedAt = time.Now().UTC()
	}
	if scan.SyncedAt.IsZero() {
		scan.SyncedAt = time.Now().UTC()
	}
	s.deviceVulnScans = append(s.deviceVulnScans, scan)
	return nil
}

func (s *Store) GetLatestDeviceVulnScan(deviceID string) (store.DeviceVulnerabilityScan, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest store.DeviceVulnerabilityScan
	found := false
	for _, rec := range s.deviceVulnScans {
		if rec.DeviceID == deviceID {
			if !found || rec.CreatedAt.After(latest.CreatedAt) {
				latest = rec
				found = true
			}
		}
	}
	return latest, found, nil
}

func (s *Store) ListDeviceVulnScans(deviceID string) ([]store.DeviceVulnerabilityScan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.DeviceVulnerabilityScan
	for _, rec := range s.deviceVulnScans {
		if rec.DeviceID == deviceID {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// ── Group lookup ──────────────────────────────────────────────────────────────

func (s *Store) GetGroup(groupID string) (store.Group, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.groups[groupID]
	return g, ok, nil
}

func (s *Store) ListDevicesForGroup(groupID string, limit int) ([]store.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grp, ok := s.groups[groupID]
	if !ok {
		return nil, nil
	}
	selector := map[string]any{}
	if len(grp.SelectorJSON) > 0 {
		_ = json.Unmarshal(grp.SelectorJSON, &selector)
	}
	if limit <= 0 {
		limit = 1000
	}
	var out []store.Device
	for _, d := range s.devices {
		if len(out) >= limit {
			break
		}
		if labelsContain(d.LabelsJSON, selector) {
			out = append(out, d)
		}
	}
	return out, nil
}

// labelsContain returns true when all key-value pairs in selector are present
// and equal in the device labels JSON.
func labelsContain(labelsJSON []byte, selector map[string]any) bool {
	if len(selector) == 0 {
		return true
	}
	labels := map[string]any{}
	if len(labelsJSON) > 0 {
		_ = json.Unmarshal(labelsJSON, &labels)
	}
	for k, v := range selector {
		lv, ok := labels[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", lv) != fmt.Sprintf("%v", v) {
			return false
		}
	}
	return true
}

// ── Webhooks ──────────────────────────────────────────────────────────────────

func (s *Store) CreateWebhook(webhook store.Webhook) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if webhook.CreatedAt.IsZero() {
		webhook.CreatedAt = time.Now().UTC()
	}
	s.webhooks[webhook.ID] = webhook
	return nil
}

func (s *Store) GetWebhook(id string) (store.Webhook, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wh, ok := s.webhooks[id]
	return wh, ok, nil
}

func (s *Store) ListWebhooks() ([]store.Webhook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.Webhook, 0, len(s.webhooks))
	for _, wh := range s.webhooks {
		out = append(out, wh)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) UpdateWebhook(webhook store.Webhook) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.webhooks[webhook.ID]
	if !ok {
		return errors.New("webhook not found")
	}
	existing.Name = webhook.Name
	existing.URL = webhook.URL
	existing.EventTypes = webhook.EventTypes
	existing.Enabled = webhook.Enabled
	s.webhooks[webhook.ID] = existing
	return nil
}

func (s *Store) DeleteWebhook(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.webhooks, id)
	return nil
}

func (s *Store) UpdateWebhookLastFired(id string, at time.Time, status int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	wh, ok := s.webhooks[id]
	if !ok {
		return nil
	}
	wh.LastFiredAt = at
	wh.LastStatus = status
	s.webhooks[id] = wh
	return nil
}

func (s *Store) CreateWebhookDelivery(delivery store.WebhookDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = time.Now().UTC()
	}
	s.webhookDeliveries = append(s.webhookDeliveries, delivery)
	return nil
}

func (s *Store) GetWebhookDelivery(deliveryID string) (store.WebhookDelivery, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.webhookDeliveries {
		if d.ID == deliveryID {
			return d, true, nil
		}
	}
	return store.WebhookDelivery{}, false, nil
}

func (s *Store) UpdateWebhookDelivery(delivery store.WebhookDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range s.webhookDeliveries {
		if d.ID == delivery.ID {
			s.webhookDeliveries[i].Status = delivery.Status
			s.webhookDeliveries[i].Attempts = delivery.Attempts
			s.webhookDeliveries[i].LastAttemptAt = delivery.LastAttemptAt
			s.webhookDeliveries[i].ResponseStatus = delivery.ResponseStatus
			return nil
		}
	}
	return nil
}

func (s *Store) ListWebhookDeliveries(webhookID string, limit int) ([]store.WebhookDelivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	var out []store.WebhookDelivery
	for _, d := range s.webhookDeliveries {
		if d.WebhookID == webhookID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ── Deploy triggers ───────────────────────────────────────────────────────────

func (s *Store) UpsertDeployTrigger(trigger store.DeployTrigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if trigger.TriggeredAt.IsZero() {
		trigger.TriggeredAt = time.Now().UTC()
	}
	s.deployTriggers[trigger.DeviceID] = trigger
	return nil
}

func (s *Store) ConsumeDeployTrigger(deviceID string) (store.DeployTrigger, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.deployTriggers[deviceID]
	if ok {
		delete(s.deployTriggers, deviceID)
	}
	return t, ok, nil
}

// ── Deployment status ─────────────────────────────────────────────────────────

func (s *Store) GetGroupDeploymentStatus(groupID, artifactID string) ([]store.DeviceDeploymentStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grp, ok := s.groups[groupID]
	if !ok {
		return nil, nil
	}
	selector := map[string]any{}
	if len(grp.SelectorJSON) > 0 {
		_ = json.Unmarshal(grp.SelectorJSON, &selector)
	}
	var out []store.DeviceDeploymentStatus
	for _, d := range s.devices {
		if d.Status == "decommissioned" {
			continue
		}
		if !labelsContain(d.LabelsJSON, selector) {
			continue
		}
		ds := store.DeviceDeploymentStatus{
			DeviceID: d.DeviceID,
			Status:   "pending",
		}
		// Find the latest apply result for this device + artifact.
		var latest store.ApplyResult
		var found bool
		for _, ar := range s.applyResults {
			if ar.DeviceID != d.DeviceID || ar.ArtifactID != artifactID {
				continue
			}
			if !found || ar.CreatedAt.After(latest.CreatedAt) {
				latest = ar
				found = true
			}
		}
		if found {
			ds.Status = latest.Status
			ds.AppliedVersion = latest.AppliedVersion
			ds.Error = latest.Error
			ds.LastApplyAt = latest.CreatedAt
		}
		out = append(out, ds)
	}
	return out, nil
}

// ── Federation ingest ──────────────────────────────────────────────────────────

func (s *Store) UpsertFederationIngest(ingest store.FederationIngest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if existing, ok := s.federationIngests[ingest.ArtifactID]; ok {
		existing.GlobalObjectKey = ingest.GlobalObjectKey
		existing.GlobalPresignBaseURL = ingest.GlobalPresignBaseURL
		existing.UpdatedAt = now
		s.federationIngests[ingest.ArtifactID] = existing
	} else {
		ingest.ReceivedAt = now
		ingest.UpdatedAt = now
		s.federationIngests[ingest.ArtifactID] = ingest
	}
	return nil
}

func (s *Store) GetFederationIngest(artifactID string) (store.FederationIngest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fi, ok := s.federationIngests[artifactID]
	return fi, ok, nil
}

func (s *Store) ConfirmFederationBlobLocal(artifactID string, confirmedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fi, ok := s.federationIngests[artifactID]
	if !ok {
		return nil
	}
	fi.BlobConfirmed = true
	fi.BlobConfirmedAt = confirmedAt
	fi.UpdatedAt = confirmedAt
	s.federationIngests[artifactID] = fi
	return nil
}

// ── Global policy cache ───────────────────────────────────────────────────────

func (s *Store) UpsertGlobalPolicyCache(policy store.GlobalPolicyCache) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if existing, ok := s.globalPolicyCaches[policy.GroupID]; ok {
		policy.CacheID = existing.CacheID
		policy.ReceivedAt = existing.ReceivedAt
	} else {
		policy.ReceivedAt = now
	}
	policy.UpdatedAt = now
	s.globalPolicyCaches[policy.GroupID] = policy
	return nil
}

func (s *Store) ListGlobalPolicyCaches() ([]store.GlobalPolicyCache, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.GlobalPolicyCache, 0, len(s.globalPolicyCaches))
	for _, p := range s.globalPolicyCaches {
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) GetGlobalPolicyCacheForDevice(deviceID string) (store.GlobalPolicyCache, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dev, ok := s.devices[deviceID]
	if !ok {
		return store.GlobalPolicyCache{}, false, nil
	}
	var devLabels map[string]string
	if len(dev.LabelsJSON) > 0 {
		_ = json.Unmarshal(dev.LabelsJSON, &devLabels)
	}
	for _, p := range s.globalPolicyCaches {
		var sel map[string]any
		if len(p.SelectorJSON) > 0 {
			_ = json.Unmarshal(p.SelectorJSON, &sel)
		}
		if labelsContain(dev.LabelsJSON, sel) {
			return p, true, nil
		}
	}
	return store.GlobalPolicyCache{}, false, nil
}

func (s *Store) DeleteGlobalPolicyCache(groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.globalPolicyCaches, groupID)
	return nil
}

func (s *Store) CreateChangeRecord(record store.ChangeRecord) (store.ChangeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.changeRecordArtIdx[record.ArtifactID]; exists {
		return store.ChangeRecord{}, fmt.Errorf("change record already exists for artifact %s", record.ArtifactID)
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	s.changeRecords[record.RecordID] = record
	s.changeRecordArtIdx[record.ArtifactID] = record.RecordID
	return record, nil
}

func (s *Store) GetChangeRecord(recordID string) (store.ChangeRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.changeRecords[recordID]
	return r, ok, nil
}

func (s *Store) GetChangeRecordForArtifact(artifactID string) (store.ChangeRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rid, ok := s.changeRecordArtIdx[artifactID]
	if !ok {
		return store.ChangeRecord{}, false, nil
	}
	r := s.changeRecords[rid]
	return r, true, nil
}

func (s *Store) UpdateChangeRecord(record store.ChangeRecord) (store.ChangeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.changeRecords[record.RecordID]; !ok {
		return store.ChangeRecord{}, store.ErrChangeRecordNotFound
	}
	record.UpdatedAt = time.Now().UTC()
	s.changeRecords[record.RecordID] = record
	return record, nil
}
