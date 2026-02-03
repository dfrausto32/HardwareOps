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
	if _, err := c.CheckIn(st); err != nil {
		t.Fatalf("checkin failed: %v", err)
	}
}
