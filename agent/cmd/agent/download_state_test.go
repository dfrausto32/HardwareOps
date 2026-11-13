package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/parcel/agent/internal/artifacts"
	"github.com/parcel/agent/internal/client"
	"github.com/parcel/agent/internal/logging"
	"github.com/parcel/agent/internal/state"
)

// rangeStub is a Transport that also supports ranged reads, with injectable
// link failure, so the resumable path (not the single-shot fallback) is used.
type rangeStub struct {
	stubTransport
	dead     bool // every request fails (link down)
	requests int
}

func (r *rangeStub) DownloadArtifactRange(url string, offset int64, validator string) (int, string, int64, io.ReadCloser, error) {
	r.requests++
	if r.dead {
		return 0, "", 0, nil, errors.New("connection refused")
	}
	total := int64(len(r.content))
	if offset >= total {
		return 416, "", total, nil, nil
	}
	status := 200
	if offset > 0 {
		status = 206
	}
	return status, `"v1"`, total, io.NopCloser(bytes.NewReader(r.content[offset:])), nil
}

func newRangeStub(t *testing.T) (*rangeStub, map[string]client.DesiredComponent) {
	t.Helper()
	bundle := stubArtifactBundle(t)
	sum := sha256.Sum256(bundle)
	rs := &rangeStub{stubTransport: stubTransport{
		artifact: client.ArtifactResponse{
			ArtifactID: "a1", Name: "stub-app", Version: "2.0.0", Type: "app_bundle",
			SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(bundle)),
		},
		content:   bundle,
		presigned: "stub://a1",
	}}
	return rs, map[string]client.DesiredComponent{"app_bundle": {ArtifactID: "a1", SoftwareVersion: "2.0.0"}}
}

func quietLogger() *logging.Logger { return logging.New(logging.Error, "agent", "d", nil) }

func TestDeadLinkIsDeferredNotFailed(t *testing.T) {
	defer artifacts.ConfigureDownloadRetry(2, time.Millisecond, time.Millisecond)()
	rs, desired := newRangeStub(t)
	rs.dead = true
	st := state.State{DeviceID: "d"}

	// Repeated check-ins on a dead link must never accumulate failures.
	for i := 0; i < 8; i++ {
		if err := applyDesiredComponents(t.TempDir(), rs, desired, &st, quietLogger(),
			artifacts.ApplyOptions{VerificationMode: "allow_unsigned"}, 3); err != nil {
			t.Fatalf("check-in %d: dead link must not be an apply failure: %v", i, err)
		}
	}
	cs := st.Components["app_bundle"]
	if cs.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures=%d; safe-halt must not trigger on a flaky link", cs.ConsecutiveFailures)
	}
	if len(rs.applyResults) != 0 {
		t.Fatalf("no apply result should be posted for a deferred download, got %+v", rs.applyResults)
	}
	d := cs.Download
	if d == nil || d.State != state.DownloadStateWaiting || d.Reason == "" {
		t.Fatalf("want waiting download with reason, got %+v", d)
	}
	if strings.Contains(d.Reason, "stub://") || strings.Contains(d.Reason, "refused") {
		t.Fatalf("reason must not leak URLs or raw errors: %q", d.Reason)
	}
}

func TestByteBudgetSpreadsDownloadAcrossCheckins(t *testing.T) {
	rs, desired := newRangeStub(t)
	root := t.TempDir() // same root: the partial persists between check-ins
	st := state.State{DeviceID: "d"}
	budget := int64(len(rs.content)/3 + 1)
	opts := artifacts.ApplyOptions{VerificationMode: "allow_unsigned", ByteBudget: budget}

	var lastDone int64 = -1
	for i := 0; i < 2; i++ {
		if err := applyDesiredComponents(root, rs, desired, &st, quietLogger(), opts, 3); err != nil {
			t.Fatalf("check-in %d: %v", i, err)
		}
		d := st.Components["app_bundle"].Download
		if d == nil || d.State != state.DownloadStateWaiting || d.Reason != "byte budget reached" {
			t.Fatalf("check-in %d: want waiting/byte budget, got %+v", i, d)
		}
		if d.BytesDone <= lastDone {
			t.Fatalf("check-in %d: bytes did not advance (%d <= %d)", i, d.BytesDone, lastDone)
		}
		if d.BytesTotal != int64(len(rs.content)) {
			t.Fatalf("total %d, want %d", d.BytesTotal, len(rs.content))
		}
		lastDone = d.BytesDone
		if len(rs.applyResults) != 0 {
			t.Fatalf("no apply result mid-download, got %+v", rs.applyResults)
		}
	}
	if err := applyDesiredComponents(root, rs, desired, &st, quietLogger(), opts, 3); err != nil {
		t.Fatalf("final check-in: %v", err)
	}
	cs := st.Components["app_bundle"]
	if cs.Download != nil || cs.CurrentVersion != "2.0.0" {
		t.Fatalf("want applied and Download cleared, got %+v", cs)
	}
	if len(rs.applyResults) != 1 || rs.applyResults[0].Status != "success" {
		t.Fatalf("want exactly one success result, got %+v", rs.applyResults)
	}
}

func TestTransferWindowClosedDefersWithoutTouchingNetwork(t *testing.T) {
	rs, desired := newRangeStub(t)
	st := state.State{DeviceID: "d"}
	now := time.Now()
	w, err := artifacts.ParseTransferWindow(now.Add(time.Hour).Format("15:04") + "-" + now.Add(2*time.Hour).Format("15:04"))
	if err != nil {
		t.Fatal(err)
	}
	opts := artifacts.ApplyOptions{VerificationMode: "allow_unsigned", TransferWindow: w}
	if err := applyDesiredComponents(t.TempDir(), rs, desired, &st, quietLogger(), opts, 3); err != nil {
		t.Fatalf("closed window must not fail: %v", err)
	}
	if rs.requests != 0 {
		t.Fatalf("no download requests outside the window, got %d", rs.requests)
	}
	d := st.Components["app_bundle"].Download
	if d == nil || d.State != state.DownloadStateWaiting || d.Reason != "outside transfer window" {
		t.Fatalf("want waiting/outside window, got %+v", d)
	}
}

func TestProgressHeartbeatObservesDownloading(t *testing.T) {
	rs, desired := newRangeStub(t)
	st := state.State{DeviceID: "d"}
	var seen []state.DownloadState
	heartbeatHook = func(s *state.State) {
		if d := s.Components["app_bundle"].Download; d != nil {
			seen = append(seen, *d)
		}
	}
	defer func() { heartbeatHook = nil }()

	if err := applyDesiredComponents(t.TempDir(), rs, desired, &st, quietLogger(),
		artifacts.ApplyOptions{VerificationMode: "allow_unsigned"}, 3); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("heartbeat hook never saw progress")
	}
	last := seen[len(seen)-1]
	if last.State != state.DownloadStateDownloading || last.BytesDone != int64(len(rs.content)) {
		t.Fatalf("last heartbeat should show full download in progress, got %+v", last)
	}
}

func TestIntegrityFailureIsStillARealFailure(t *testing.T) {
	rs, desired := newRangeStub(t)
	rs.artifact.SHA256 = strings.Repeat("0", 64) // corrupt/wrong hash: must fail closed
	st := state.State{DeviceID: "d"}
	err := applyDesiredComponents(t.TempDir(), rs, desired, &st, quietLogger(),
		artifacts.ApplyOptions{VerificationMode: "allow_unsigned"}, 3)
	if err == nil {
		t.Fatal("sha mismatch must fail the apply")
	}
	cs := st.Components["app_bundle"]
	if cs.ConsecutiveFailures != 1 || cs.Download != nil {
		t.Fatalf("want 1 failure and cleared Download, got %+v", cs)
	}
	if len(rs.applyResults) != 1 || rs.applyResults[0].Status == "success" {
		t.Fatalf("want one failed apply result, got %+v", rs.applyResults)
	}
}
