package sync

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/globalplane"
)

// syncOKStore wraps stubStore with no-op implementations of the persistence
// methods a successful syncOnce calls, so the worker can complete a sync cycle
// in tests without the stub's "not called" panics.
type syncOKStore struct {
	*stubStore
}

func (syncOKStore) UpdateRegionalPlaneSyncStatus(string, time.Time, string) error    { return nil }
func (syncOKStore) UpsertDeviceCacheEntries(string, []globalplane.CachedDevice) error { return nil }
func (syncOKStore) UpsertHealthCache(globalplane.HealthSnapshot) error                { return nil }
func (syncOKStore) UpsertArtifactCacheEntries(string, []globalplane.CachedArtifact) error {
	return nil
}

// TestRefreshUsesManagerContextNotCallerContext guards against a regression
// where Manager.Refresh started plane workers with the context passed by its
// caller. When Refresh is invoked from the RegisterPlane HTTP handler, that
// context is the request context — cancelled the moment the response is
// written — so the worker's first sync aborted with "context canceled" and no
// data was ever pulled from the regional plane until the global-plane
// restarted. Workers must run on the manager's long-lived context instead.
func TestRefreshUsesManagerContextNotCallerContext(t *testing.T) {
	fetched := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/devices") {
			select {
			case fetched <- struct{}{}:
			default:
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/health"):
			_, _ = w.Write([]byte("{}"))
		default: // devices, artifacts
			_, _ = w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	encKey := bytes.Repeat([]byte{0x2a}, 32)
	tok, err := globalplane.EncryptToken("svc-token", encKey)
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}

	st := newStubStore()
	st.planes = []globalplane.RegionalPlane{{
		PlaneID:             "p1",
		Name:                "p1",
		BaseURL:             srv.URL,
		EncryptedToken:      tok,
		SyncIntervalSeconds: 1,
		Enabled:             true,
	}}

	m := NewManager(syncOKStore{st}, encKey, log.New(&bytes.Buffer{}, "", 0))
	// Simulate that Start() has captured the long-lived manager context.
	mgrCtx, cancelMgr := context.WithCancel(context.Background())
	defer cancelMgr()
	m.baseCtx = mgrCtx
	defer m.stopAll()

	// Caller context that is already done, mimicking an HTTP request whose
	// context is cancelled as soon as the handler returns.
	reqCtx, cancelReq := context.WithCancel(context.Background())
	cancelReq()

	if err := m.Refresh(reqCtx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	select {
	case <-fetched:
		// Worker ran despite the cancelled caller context — correct behaviour.
	case <-time.After(3 * time.Second):
		t.Fatal("plane worker never fetched devices; it was bound to the cancelled caller context instead of the manager context")
	}
}
