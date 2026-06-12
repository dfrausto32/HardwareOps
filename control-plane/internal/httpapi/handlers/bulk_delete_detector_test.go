package handlers

import (
	"bytes"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/store"
)

type fakeRuntimeEventStore struct {
	mu     sync.Mutex
	events []store.RuntimeEvent
}

func (f *fakeRuntimeEventStore) CreateRuntimeEvent(ev store.RuntimeEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
	return nil
}

func (f *fakeRuntimeEventStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

func newTestDetector(st runtimeEventCreator) *BulkDeletionDetector {
	d := NewBulkDeletionDetector(log.New(&bytes.Buffer{}, "", 0), nil, st)
	return d
}

// R-03 acceptance: 11 deletions inside the 5-minute window emit the alert.
func TestBulkDeletionDetector_AlertsAboveThreshold(t *testing.T) {
	st := &fakeRuntimeEventStore{}
	d := newTestDetector(st)

	for i := 0; i < 11; i++ {
		d.Record("actor-1")
	}

	if st.count() != 1 {
		t.Fatalf("expected exactly 1 alert event after 11 deletions, got %d", st.count())
	}
	ev := st.events[0]
	if ev.Type != "security.anomaly.bulk_artifact_deletion" {
		t.Fatalf("unexpected event type %q", ev.Type)
	}
	if !bytes.Contains(ev.PayloadJSON, []byte("actor-1")) {
		t.Fatalf("payload missing actor id: %s", ev.PayloadJSON)
	}
}

// R-03 acceptance: 9 deletions do not alert.
func TestBulkDeletionDetector_NoAlertBelowThreshold(t *testing.T) {
	st := &fakeRuntimeEventStore{}
	d := newTestDetector(st)

	for i := 0; i < 9; i++ {
		d.Record("actor-1")
	}

	if st.count() != 0 {
		t.Fatalf("expected no alert after 9 deletions, got %d", st.count())
	}
}

// Exactly the threshold (10) must not alert; the policy says "more than 10".
func TestBulkDeletionDetector_ThresholdIsExclusive(t *testing.T) {
	st := &fakeRuntimeEventStore{}
	d := newTestDetector(st)

	for i := 0; i < 10; i++ {
		d.Record("actor-1")
	}
	if st.count() != 0 {
		t.Fatalf("expected no alert at exactly 10 deletions, got %d", st.count())
	}
	d.Record("actor-1")
	if st.count() != 1 {
		t.Fatalf("expected alert on the 11th deletion, got %d", st.count())
	}
}

// Deletions outside the sliding window do not count toward the threshold.
func TestBulkDeletionDetector_WindowSlides(t *testing.T) {
	st := &fakeRuntimeEventStore{}
	d := newTestDetector(st)
	current := time.Now().UTC()
	d.now = func() time.Time { return current }

	for i := 0; i < 10; i++ {
		d.Record("actor-1")
	}
	// Move past the window; the next deletion starts a fresh count.
	current = current.Add(6 * time.Minute)
	d.Record("actor-1")

	if st.count() != 0 {
		t.Fatalf("expected no alert after window slid, got %d", st.count())
	}
}

// Counts are per actor: two actors below threshold do not combine into an alert.
func TestBulkDeletionDetector_PerActor(t *testing.T) {
	st := &fakeRuntimeEventStore{}
	d := newTestDetector(st)

	for i := 0; i < 8; i++ {
		d.Record("actor-1")
		d.Record("actor-2")
	}
	if st.count() != 0 {
		t.Fatalf("expected no alert for two below-threshold actors, got %d", st.count())
	}
}

// An ongoing spree re-alerts at most once per window, not per deletion.
func TestBulkDeletionDetector_NoAlertFlood(t *testing.T) {
	st := &fakeRuntimeEventStore{}
	d := newTestDetector(st)

	for i := 0; i < 50; i++ {
		d.Record("actor-1")
	}
	if st.count() != 1 {
		t.Fatalf("expected a single alert for a continuous spree, got %d", st.count())
	}
}

// A nil detector is a no-op so handlers can be wired without one.
func TestBulkDeletionDetector_NilSafe(t *testing.T) {
	var d *BulkDeletionDetector
	d.Record("actor-1") // must not panic
}
