package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/globalplane"
)

// testEncKey is a fixed 32-byte AES key for tests.
var testEncKey = []byte("test-encryption-key-32-bytes-xxx")

// withURLParam injects a chi URL parameter into the request context.
func withURLParam(req *http.Request, key, val string) *http.Request {
	rctx, _ := req.Context().Value(chi.RouteCtxKey).(*chi.Context)
	if rctx == nil {
		rctx = chi.NewRouteContext()
	}
	rctx.URLParams.Add(key, val)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// noopSync is a syncRefresher that does nothing.
type noopSync struct{}

func (noopSync) Refresh(_ interface{ Done() <-chan struct{} }) error { return nil }

// ── fakePlanesStore ───────────────────────────────────────────────────────────

type fakePlanesStore struct {
	planes   map[string]globalplane.RegionalPlane
	statuses []globalplane.PolicySyncStatus
}

func newFakePlanesStore() *fakePlanesStore {
	return &fakePlanesStore{planes: make(map[string]globalplane.RegionalPlane)}
}

func (f *fakePlanesStore) CreateRegionalPlane(p globalplane.RegionalPlane) error {
	if p.PlaneID == "" {
		p.PlaneID = uuid.NewString()
	}
	f.planes[p.PlaneID] = p
	return nil
}
func (f *fakePlanesStore) GetRegionalPlane(planeID string) (globalplane.RegionalPlane, bool, error) {
	p, ok := f.planes[planeID]
	return p, ok, nil
}
func (f *fakePlanesStore) ListRegionalPlanes() ([]globalplane.RegionalPlane, error) {
	out := make([]globalplane.RegionalPlane, 0, len(f.planes))
	for _, p := range f.planes {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakePlanesStore) UpdateRegionalPlane(planeID, name, baseURL string, enc []byte, tlsCAPem string, syncInterval int) error {
	p, ok := f.planes[planeID]
	if !ok {
		return errors.New("not found")
	}
	p.Name = name
	p.BaseURL = baseURL
	p.EncryptedToken = enc
	p.TLSCAPem = tlsCAPem
	p.SyncIntervalSeconds = syncInterval
	f.planes[planeID] = p
	return nil
}
func (f *fakePlanesStore) DeleteRegionalPlane(planeID string) error {
	delete(f.planes, planeID)
	return nil
}
func (f *fakePlanesStore) ListAllPolicySyncStatus() ([]globalplane.PolicySyncStatus, error) {
	return f.statuses, nil
}

// ── fakeGroupStore ────────────────────────────────────────────────────────────

type fakeGroupStore struct {
	groups        map[string]globalplane.GlobalGroup
	desiredStates map[string]globalplane.GlobalDesiredState
	planes        []globalplane.RegionalPlane
	syncStatuses  []globalplane.PolicySyncStatus
}

func newFakeGroupStore() *fakeGroupStore {
	return &fakeGroupStore{
		groups:        make(map[string]globalplane.GlobalGroup),
		desiredStates: make(map[string]globalplane.GlobalDesiredState),
	}
}

func (f *fakeGroupStore) CreateGlobalGroup(g globalplane.GlobalGroup) (globalplane.GlobalGroup, error) {
	if g.GroupID == "" {
		g.GroupID = uuid.NewString()
	}
	g.CreatedAt = time.Now().UTC()
	g.UpdatedAt = g.CreatedAt
	f.groups[g.GroupID] = g
	return g, nil
}
func (f *fakeGroupStore) GetGlobalGroup(groupID string) (globalplane.GlobalGroup, bool, error) {
	g, ok := f.groups[groupID]
	return g, ok, nil
}
func (f *fakeGroupStore) ListGlobalGroups() ([]globalplane.GlobalGroup, error) {
	out := make([]globalplane.GlobalGroup, 0, len(f.groups))
	for _, g := range f.groups {
		out = append(out, g)
	}
	return out, nil
}
func (f *fakeGroupStore) DeleteGlobalGroup(groupID string) error {
	delete(f.groups, groupID)
	delete(f.desiredStates, groupID)
	return nil
}
func (f *fakeGroupStore) UpsertGlobalDesiredState(state globalplane.GlobalDesiredState) error {
	state.UpdatedAt = time.Now().UTC()
	f.desiredStates[state.GroupID] = state
	return nil
}
func (f *fakeGroupStore) GetGlobalDesiredState(groupID string) (globalplane.GlobalDesiredState, bool, error) {
	d, ok := f.desiredStates[groupID]
	return d, ok, nil
}
func (f *fakeGroupStore) ListGlobalDesiredStatesWithGroups() ([]globalplane.GlobalDesiredStateWithGroup, error) {
	var out []globalplane.GlobalDesiredStateWithGroup
	for id, g := range f.groups {
		row := globalplane.GlobalDesiredStateWithGroup{GlobalGroup: g}
		if d, ok := f.desiredStates[id]; ok {
			row.GlobalDesiredState = d
		}
		row.GlobalDesiredState.GroupID = id
		out = append(out, row)
	}
	return out, nil
}
func (f *fakeGroupStore) DeleteGlobalDesiredState(groupID string) error {
	delete(f.desiredStates, groupID)
	return nil
}
func (f *fakeGroupStore) ListRegionalPlanes() ([]globalplane.RegionalPlane, error) {
	return f.planes, nil
}
func (f *fakeGroupStore) UpsertPolicySyncStatus(groupID, planeID string, pushedAt *time.Time, pushErr string, retryCount int) error {
	f.syncStatuses = append(f.syncStatuses, globalplane.PolicySyncStatus{
		GroupID:    groupID,
		PlaneID:    planeID,
		PushedAt:   pushedAt,
		PushError:  pushErr,
		RetryCount: retryCount,
	})
	return nil
}

// ── fakeEnrollmentStore ───────────────────────────────────────────────────────

type fakeEnrollmentStore struct {
	planes            map[string]globalplane.RegionalPlane
	profiles          map[string]globalplane.GlobalEnrollmentProfile
	pendingEnrollments []globalplane.GlobalPendingEnrollment
}

func newFakeEnrollmentStore() *fakeEnrollmentStore {
	return &fakeEnrollmentStore{
		planes:   make(map[string]globalplane.RegionalPlane),
		profiles: make(map[string]globalplane.GlobalEnrollmentProfile),
	}
}

func (f *fakeEnrollmentStore) GetRegionalPlane(planeID string) (globalplane.RegionalPlane, bool, error) {
	p, ok := f.planes[planeID]
	return p, ok, nil
}
func (f *fakeEnrollmentStore) ListRegionalPlanes() ([]globalplane.RegionalPlane, error) {
	out := make([]globalplane.RegionalPlane, 0, len(f.planes))
	for _, p := range f.planes {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeEnrollmentStore) CreateGlobalEnrollmentProfile(p globalplane.GlobalEnrollmentProfile) (globalplane.GlobalEnrollmentProfile, error) {
	if p.ProfileID == "" {
		p.ProfileID = uuid.NewString()
	}
	p.CreatedAt = time.Now().UTC()
	p.UpdatedAt = p.CreatedAt
	if len(p.DefaultLabelsJSON) == 0 {
		p.DefaultLabelsJSON = []byte("{}")
	}
	f.profiles[p.ProfileID] = p
	return p, nil
}
func (f *fakeEnrollmentStore) GetGlobalEnrollmentProfile(profileID string) (globalplane.GlobalEnrollmentProfile, bool, error) {
	p, ok := f.profiles[profileID]
	return p, ok, nil
}
func (f *fakeEnrollmentStore) ListGlobalEnrollmentProfiles() ([]globalplane.GlobalEnrollmentProfile, error) {
	out := make([]globalplane.GlobalEnrollmentProfile, 0, len(f.profiles))
	for _, p := range f.profiles {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeEnrollmentStore) UpdateGlobalEnrollmentProfile(profileID string, u globalplane.GlobalEnrollmentProfileUpdate) (globalplane.GlobalEnrollmentProfile, error) {
	p, ok := f.profiles[profileID]
	if !ok {
		return globalplane.GlobalEnrollmentProfile{}, errors.New("not found")
	}
	p.Name = u.Name
	p.RequireApproval = u.RequireApproval
	p.AllowUntrustedHW = u.AllowUntrustedHW
	p.ChallengeHint = u.ChallengeHint
	p.ApprovalDelaySec = u.ApprovalDelaySec
	p.MaxUses = u.MaxUses
	p.CertValidityDays = u.CertValidityDays
	p.DefaultLabelsJSON = u.DefaultLabelsJSON
	p.UpdatedAt = time.Now().UTC()
	f.profiles[profileID] = p
	return p, nil
}
func (f *fakeEnrollmentStore) DeleteGlobalEnrollmentProfile(profileID string) error {
	delete(f.profiles, profileID)
	return nil
}
func (f *fakeEnrollmentStore) ListGlobalPendingEnrollments(filter globalplane.GlobalPendingEnrollmentFilter) ([]globalplane.GlobalPendingEnrollment, error) {
	var out []globalplane.GlobalPendingEnrollment
	for _, e := range f.pendingEnrollments {
		if filter.Status != "" && e.Status != filter.Status {
			continue
		}
		if filter.PlaneID != "" && e.PlaneID != filter.PlaneID {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
