package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/globalplane"
)

// ── Stub store ────────────────────────────────────────────────────────────────

// stubStore implements globalplane.Store with only the methods used by
// the policy and pending-enrollment reconcilers. All other methods panic.
type stubStore struct {
	mu            sync.Mutex
	planes        []globalplane.RegionalPlane
	desiredStates []globalplane.GlobalDesiredStateWithGroup
	syncStatuses  []globalplane.PolicySyncStatus
	upsertedSync  []globalplane.PolicySyncStatus

	purgedPlanes   []string
	upsertedItems  map[string][]globalplane.GlobalPendingEnrollment
}

func newStubStore() *stubStore {
	return &stubStore{upsertedItems: make(map[string][]globalplane.GlobalPendingEnrollment)}
}

func (s *stubStore) ListRegionalPlanes() ([]globalplane.RegionalPlane, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.planes, nil
}
func (s *stubStore) ListGlobalDesiredStatesWithGroups() ([]globalplane.GlobalDesiredStateWithGroup, error) {
	return s.desiredStates, nil
}
func (s *stubStore) ListAllPolicySyncStatus() ([]globalplane.PolicySyncStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncStatuses, nil
}
func (s *stubStore) UpsertPolicySyncStatus(groupID, planeID string, pushedAt *time.Time, pushErr string, retryCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upsertedSync = append(s.upsertedSync, globalplane.PolicySyncStatus{
		GroupID:    groupID,
		PlaneID:    planeID,
		PushedAt:   pushedAt,
		PushError:  pushErr,
		RetryCount: retryCount,
	})
	return nil
}
func (s *stubStore) PurgeGlobalPendingEnrollmentsForPlane(planeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgedPlanes = append(s.purgedPlanes, planeID)
	delete(s.upsertedItems, planeID)
	return nil
}
func (s *stubStore) UpsertGlobalPendingEnrollments(planeID string, items []globalplane.GlobalPendingEnrollment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upsertedItems[planeID] = items
	return nil
}

// Unimplemented — these methods should not be called by the reconcilers.
func (s *stubStore) CreateRegionalPlane(p globalplane.RegionalPlane) error       { panic("not called") }
func (s *stubStore) GetRegionalPlane(id string) (globalplane.RegionalPlane, bool, error) {
	panic("not called")
}
func (s *stubStore) UpdateRegionalPlane(_, _, _ string, _ []byte, _ string, _ int) error {
	panic("not called")
}
func (s *stubStore) UpdateRegionalPlaneSyncStatus(_ string, _ time.Time, _ string) error {
	panic("not called")
}
func (s *stubStore) DeleteRegionalPlane(_ string) error { panic("not called") }
func (s *stubStore) UpsertDeviceCacheEntries(_ string, _ []globalplane.CachedDevice) error {
	panic("not called")
}
func (s *stubStore) ListDeviceCache(_ globalplane.DeviceCacheFilter) ([]globalplane.CachedDevice, error) {
	panic("not called")
}
func (s *stubStore) PurgeDeviceCacheForPlane(_ string) error { panic("not called") }
func (s *stubStore) UpsertArtifactCacheEntries(_ string, _ []globalplane.CachedArtifact) error {
	panic("not called")
}
func (s *stubStore) ListArtifactCache(_ globalplane.ArtifactCacheFilter) ([]globalplane.CachedArtifact, error) {
	panic("not called")
}
func (s *stubStore) UpsertHealthCache(_ globalplane.HealthSnapshot) error { panic("not called") }
func (s *stubStore) ListHealthCache() ([]globalplane.HealthSnapshot, error) {
	panic("not called")
}
func (s *stubStore) CreateGlobalArtifact(_ globalplane.GlobalArtifact) error { panic("not called") }
func (s *stubStore) GetGlobalArtifact(_ string) (globalplane.GlobalArtifact, bool, error) {
	panic("not called")
}
func (s *stubStore) ListGlobalArtifacts(_ string) ([]globalplane.GlobalArtifact, error) {
	panic("not called")
}
func (s *stubStore) CreateReplicationStatusRows(_ string, _ []string) error { panic("not called") }
func (s *stubStore) UpdateReplicationStatusMetadataPush(_ string, _ string, _ *time.Time, _ string) error {
	panic("not called")
}
func (s *stubStore) UpdateReplicationStatusBlobConfirmed(_ string, _ string, _ time.Time) error {
	panic("not called")
}
func (s *stubStore) UpdateReplicationStatusBlobCheckError(_, _, _ string) error {
	panic("not called")
}
func (s *stubStore) ListReplicationStatus(_ string) ([]globalplane.ArtifactReplicationStatus, error) {
	panic("not called")
}
func (s *stubStore) ListPendingReplicationRows() ([]globalplane.ArtifactReplicationStatus, error) {
	panic("not called")
}
func (s *stubStore) CreateGlobalGroup(_ globalplane.GlobalGroup) (globalplane.GlobalGroup, error) {
	panic("not called")
}
func (s *stubStore) GetGlobalGroup(_ string) (globalplane.GlobalGroup, bool, error) {
	panic("not called")
}
func (s *stubStore) ListGlobalGroups() ([]globalplane.GlobalGroup, error) { panic("not called") }
func (s *stubStore) DeleteGlobalGroup(_ string) error                     { panic("not called") }
func (s *stubStore) UpsertGlobalDesiredState(_ globalplane.GlobalDesiredState) error {
	panic("not called")
}
func (s *stubStore) GetGlobalDesiredState(_ string) (globalplane.GlobalDesiredState, bool, error) {
	panic("not called")
}
func (s *stubStore) DeleteGlobalDesiredState(_ string) error { panic("not called") }
func (s *stubStore) GetPolicySyncStatus(_, _ string) (globalplane.PolicySyncStatus, bool, error) {
	panic("not called")
}
func (s *stubStore) CreateGlobalEnrollmentProfile(_ globalplane.GlobalEnrollmentProfile) (globalplane.GlobalEnrollmentProfile, error) {
	panic("not called")
}
func (s *stubStore) GetGlobalEnrollmentProfile(_ string) (globalplane.GlobalEnrollmentProfile, bool, error) {
	panic("not called")
}
func (s *stubStore) ListGlobalEnrollmentProfiles() ([]globalplane.GlobalEnrollmentProfile, error) {
	panic("not called")
}
func (s *stubStore) UpdateGlobalEnrollmentProfile(_ string, _ globalplane.GlobalEnrollmentProfileUpdate) (globalplane.GlobalEnrollmentProfile, error) {
	panic("not called")
}
func (s *stubStore) DeleteGlobalEnrollmentProfile(_ string) error { panic("not called") }
func (s *stubStore) ListGlobalPendingEnrollments(_ globalplane.GlobalPendingEnrollmentFilter) ([]globalplane.GlobalPendingEnrollment, error) {
	panic("not called")
}

// testEncKey is a fixed 32-byte AES key for tests.
var testEncKey = []byte("test-encryption-key-32-bytes-xxx")

func silentLogger() *log.Logger { return log.New(&bytes.Buffer{}, "", 0) }

// ── policyRetryDelay tests ────────────────────────────────────────────────────

func TestPolicyRetryDelay_Sequence(t *testing.T) {
	cases := []struct {
		retryCount int
		want       time.Duration
	}{
		{0, 0},
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 2 * time.Minute},
		{4, 4 * time.Minute},
		{5, 8 * time.Minute},
		{6, 16 * time.Minute},
		{7, 16 * time.Minute}, // capped: shift is clamped at 5 → 2^5 * 30s = 16m
		{99, 16 * time.Minute},
	}
	for _, tc := range cases {
		got := policyRetryDelay(tc.retryCount)
		if got != tc.want {
			t.Errorf("policyRetryDelay(%d) = %v, want %v", tc.retryCount, got, tc.want)
		}
	}
}

// ── policyReconciler tests ────────────────────────────────────────────────────

func TestPolicyReconciler_SkipsRecentSuccess(t *testing.T) {
	// A regional plane whose policy was pushed successfully 1 minute ago (well within staleAfter).
	pushedAt := time.Now().Add(-1 * time.Minute)
	planeID := uuid.NewString()
	groupID := uuid.NewString()

	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{{PlaneID: planeID, BaseURL: "http://unused", Enabled: true}}
	st.desiredStates = []globalplane.GlobalDesiredStateWithGroup{{
		GlobalGroup:        globalplane.GlobalGroup{GroupID: groupID},
		GlobalDesiredState: globalplane.GlobalDesiredState{GroupID: groupID, ArtifactID: "a1"},
	}}
	st.syncStatuses = []globalplane.PolicySyncStatus{{
		GroupID:   groupID,
		PlaneID:   planeID,
		PushedAt:  &pushedAt,
		PushError: "",
	}}

	pr := newPolicyReconciler(st, testEncKey, silentLogger(), time.Hour)
	pr.reconcile(context.Background())

	// Nothing should have been pushed.
	if len(st.upsertedSync) != 0 {
		t.Fatalf("expected no pushes for recently-synced plane, got %d", len(st.upsertedSync))
	}
}

func TestPolicyReconciler_PushesStalePolicy(t *testing.T) {
	// Policy was last pushed 15 minutes ago — beyond the 10-minute staleAfter threshold.
	pushedAt := time.Now().Add(-15 * time.Minute)
	planeID := uuid.NewString()
	groupID := uuid.NewString()

	pushed := make(chan struct{}, 1)
	fakeRegional := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/federation/policies" {
			pushed <- struct{}{}
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer fakeRegional.Close()

	enc, _ := globalplane.EncryptToken("tok", testEncKey)
	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{{
		PlaneID:        planeID,
		BaseURL:        fakeRegional.URL,
		EncryptedToken: enc,
		Enabled:        true,
	}}
	st.desiredStates = []globalplane.GlobalDesiredStateWithGroup{{
		GlobalGroup:        globalplane.GlobalGroup{GroupID: groupID, Name: "g1"},
		GlobalDesiredState: globalplane.GlobalDesiredState{GroupID: groupID, ArtifactID: "a1", DesiredVersion: "1.0.0"},
	}}
	st.syncStatuses = []globalplane.PolicySyncStatus{{
		GroupID:   groupID,
		PlaneID:   planeID,
		PushedAt:  &pushedAt,
		PushError: "",
	}}

	pr := newPolicyReconciler(st, testEncKey, silentLogger(), time.Hour)
	pr.reconcile(context.Background())

	select {
	case <-pushed:
		// Pass.
	case <-time.After(3 * time.Second):
		t.Fatal("reconciler did not push stale policy within 3 seconds")
	}
}

func TestPolicyReconciler_PushesNeverSyncedPolicy(t *testing.T) {
	// No sync status exists for this (group, plane) pair.
	groupID := uuid.NewString()
	planeID := uuid.NewString()

	pushed := make(chan struct{}, 1)
	fakeRegional := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/federation/policies" {
			pushed <- struct{}{}
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer fakeRegional.Close()

	enc, _ := globalplane.EncryptToken("tok", testEncKey)
	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{{
		PlaneID:        planeID,
		BaseURL:        fakeRegional.URL,
		EncryptedToken: enc,
		Enabled:        true,
	}}
	st.desiredStates = []globalplane.GlobalDesiredStateWithGroup{{
		GlobalGroup:        globalplane.GlobalGroup{GroupID: groupID, Name: "new-group"},
		GlobalDesiredState: globalplane.GlobalDesiredState{GroupID: groupID, ArtifactID: "a1", DesiredVersion: "2.0.0"},
	}}
	// syncStatuses is empty — no prior push recorded.

	pr := newPolicyReconciler(st, testEncKey, silentLogger(), time.Hour)
	pr.reconcile(context.Background())

	select {
	case <-pushed:
		// Pass.
	case <-time.After(3 * time.Second):
		t.Fatal("reconciler did not push never-synced policy within 3 seconds")
	}
}

func TestPolicyReconciler_RespectsBackoffAfterFailure(t *testing.T) {
	// Last attempt was 5 seconds ago with retryCount=1 — backoff is 30s, so skip.
	lastAttempt := time.Now().Add(-5 * time.Second)
	groupID := uuid.NewString()
	planeID := uuid.NewString()

	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{{PlaneID: planeID, BaseURL: "http://unreachable", Enabled: true}}
	st.desiredStates = []globalplane.GlobalDesiredStateWithGroup{{
		GlobalGroup:        globalplane.GlobalGroup{GroupID: groupID},
		GlobalDesiredState: globalplane.GlobalDesiredState{GroupID: groupID, ArtifactID: "a1"},
	}}
	st.syncStatuses = []globalplane.PolicySyncStatus{{
		GroupID:       groupID,
		PlaneID:       planeID,
		PushError:     "timeout",
		RetryCount:    1,
		LastAttemptAt: &lastAttempt,
	}}

	pr := newPolicyReconciler(st, testEncKey, silentLogger(), time.Hour)
	pr.reconcile(context.Background())

	// 5s < 30s backoff → nothing should be attempted.
	if len(st.upsertedSync) != 0 {
		t.Fatalf("expected no retry within backoff window, got %d attempts", len(st.upsertedSync))
	}
}

func TestPolicyReconciler_SkipsDisabledPlanes(t *testing.T) {
	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{
		{PlaneID: uuid.NewString(), BaseURL: "http://unreachable", Enabled: false},
	}
	st.desiredStates = []globalplane.GlobalDesiredStateWithGroup{{
		GlobalGroup:        globalplane.GlobalGroup{GroupID: uuid.NewString()},
		GlobalDesiredState: globalplane.GlobalDesiredState{ArtifactID: "a1"},
	}}

	pr := newPolicyReconciler(st, testEncKey, silentLogger(), time.Hour)
	pr.reconcile(context.Background())

	if len(st.upsertedSync) != 0 {
		t.Fatalf("should not push to disabled plane, got %d attempts", len(st.upsertedSync))
	}
}

func TestPolicyReconciler_SkipsEmptyDesiredState(t *testing.T) {
	// A group with no artifactId or desiredVersion set should be skipped.
	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{
		{PlaneID: uuid.NewString(), BaseURL: "http://unused", Enabled: true},
	}
	st.desiredStates = []globalplane.GlobalDesiredStateWithGroup{{
		GlobalGroup:        globalplane.GlobalGroup{GroupID: uuid.NewString(), Name: "empty"},
		GlobalDesiredState: globalplane.GlobalDesiredState{ArtifactID: "", DesiredVersion: ""},
	}}

	pr := newPolicyReconciler(st, testEncKey, silentLogger(), time.Hour)
	pr.reconcile(context.Background())

	if len(st.upsertedSync) != 0 {
		t.Fatalf("should skip groups with no desired state content, got %d", len(st.upsertedSync))
	}
}

// ── pendingEnrollmentReconciler tests ────────────────────────────────────────

func TestPendingEnrollmentReconciler_PollsAndCaches(t *testing.T) {
	items := []map[string]interface{}{
		{"requestId": "req-1", "profileId": "p1", "status": "pending", "createdAt": time.Now().UTC().Format(time.RFC3339)},
		{"requestId": "req-2", "profileId": "p1", "status": "pending", "createdAt": time.Now().UTC().Format(time.RFC3339)},
	}
	fakeRegional := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}))
	defer fakeRegional.Close()

	enc, _ := globalplane.EncryptToken("tok", testEncKey)
	planeID := uuid.NewString()
	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{{
		PlaneID:        planeID,
		BaseURL:        fakeRegional.URL,
		EncryptedToken: enc,
		Enabled:        true,
	}}

	rec := newPendingEnrollmentReconciler(st, testEncKey, silentLogger(), time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rec.reconcile(ctx)

	// Give goroutines a moment to complete (pollPlane is async).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st.mu.Lock()
		cached := len(st.upsertedItems[planeID])
		st.mu.Unlock()
		if cached == 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.upsertedItems[planeID]) != 2 {
		t.Fatalf("expected 2 cached pending enrollments, got %d", len(st.upsertedItems[planeID]))
	}
}

func TestPendingEnrollmentReconciler_PurgesBeforeUpsert(t *testing.T) {
	// Verify purge is called before upsert so stale entries are cleared.
	callOrder := make([]string, 0, 2)
	var callMu sync.Mutex

	// We can't override the store methods at runtime in Go, but we can verify
	// the side effects: purgedPlanes should contain the planeID, and then
	// upsertedItems should have the fresh results.
	fakeRegional := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callMu.Lock()
		callOrder = append(callOrder, "http")
		callMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				{"requestId": "fresh-req", "status": "pending", "createdAt": time.Now().UTC().Format(time.RFC3339)},
			},
		})
	}))
	defer fakeRegional.Close()

	enc, _ := globalplane.EncryptToken("tok", testEncKey)
	planeID := uuid.NewString()
	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{{
		PlaneID:        planeID,
		BaseURL:        fakeRegional.URL,
		EncryptedToken: enc,
		Enabled:        true,
	}}

	rec := newPendingEnrollmentReconciler(st, testEncKey, silentLogger(), time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rec.reconcile(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st.mu.Lock()
		purged := len(st.purgedPlanes)
		cached := len(st.upsertedItems[planeID])
		st.mu.Unlock()
		if purged >= 1 && cached >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.purgedPlanes) == 0 || st.purgedPlanes[0] != planeID {
		t.Fatal("expected plane to be purged before new items upserted")
	}
	if _, ok := st.upsertedItems[planeID]; !ok {
		t.Fatal("expected fresh items to be upserted after purge")
	}
}

func TestPendingEnrollmentReconciler_SkipsDisabledPlanes(t *testing.T) {
	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{
		{PlaneID: uuid.NewString(), BaseURL: "http://unreachable", Enabled: false},
	}

	rec := newPendingEnrollmentReconciler(st, testEncKey, silentLogger(), time.Hour)
	rec.reconcile(context.Background())

	// No goroutines launched for disabled planes.
	time.Sleep(100 * time.Millisecond)
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.purgedPlanes) != 0 || len(st.upsertedItems) != 0 {
		t.Fatal("disabled plane should produce no store activity")
	}
}
