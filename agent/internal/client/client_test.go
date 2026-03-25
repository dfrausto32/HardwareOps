package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hardwareops/agent/internal/state"
)

func TestCheckIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/devices/checkin" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"serverTime": time.Now().UTC(),
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	st := state.State{DeviceID: "dev-1", AgentVersion: "0.1.0"}
	resp, err := c.CheckIn(st, nil)
	if err != nil {
		t.Fatalf("checkin failed: %v", err)
	}
	if resp.ImmediateRecheckin {
		t.Fatal("expected immediateRecheckin=false by default")
	}
}

func TestCheckIn_ImmediateRecheckin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/devices/checkin" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"serverTime":         time.Now().UTC(),
			"immediateRecheckin": true,
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	st := state.State{DeviceID: "dev-1", AgentVersion: "0.1.0"}
	resp, err := c.CheckIn(st, nil)
	if err != nil {
		t.Fatalf("checkin failed: %v", err)
	}
	if !resp.ImmediateRecheckin {
		t.Fatal("expected immediateRecheckin to be preserved from JSON")
	}
}

func TestRequestPendingEnrollment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pending-enrollments/request" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"requestId":    "req-1",
			"status":       "pending",
			"claimToken":   "claim-1",
			"pollAfterSec": 5,
			"expiresAt":    time.Now().UTC(),
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	resp, err := c.RequestPendingEnrollment(PendingEnrollmentRequest{
		ProfileToken: "profile",
		CSR:          "csr",
		AgentVersion: "0.1.0",
	})
	if err != nil {
		t.Fatalf("request pending enrollment: %v", err)
	}
	if resp.RequestID != "req-1" || resp.ClaimToken != "claim-1" || resp.Status != "pending" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestClaimPendingEnrollmentPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pending-enrollments/claim" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":       "pending",
			"pollAfterSec": 5,
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	resp, err := c.ClaimPendingEnrollment(ClaimPendingEnrollmentRequest{
		RequestID:  "req-1",
		ClaimToken: "claim-1",
	})
	if err != nil {
		t.Fatalf("claim pending enrollment: %v", err)
	}
	if resp.Status != "pending" || resp.PollAfterSec != 5 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
