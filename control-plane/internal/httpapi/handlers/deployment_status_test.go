package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestGetGroupDeploymentStatus(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	groupID := uuid.NewString()
	artifactID := uuid.NewString()
	now := time.Now().UTC()

	if err := mem.UpsertGroup(store.Group{
		GroupID:      groupID,
		Name:         "edge",
		SelectorJSON: []byte(`{"role":"edge"}`),
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	successID := uuid.NewString()
	errorID := uuid.NewString()
	pendingID := uuid.NewString()
	decommissionedID := uuid.NewString()
	for _, d := range []store.Device{
		{DeviceID: successID, Status: "active", LastSeen: now, LabelsJSON: []byte(`{"role":"edge"}`)},
		{DeviceID: errorID, Status: "active", LastSeen: now, LabelsJSON: []byte(`{"role":"edge"}`)},
		{DeviceID: pendingID, Status: "active", LastSeen: now, LabelsJSON: []byte(`{"role":"edge"}`)},
		{DeviceID: decommissionedID, Status: "decommissioned", LastSeen: now, LabelsJSON: []byte(`{"role":"edge"}`)},
	} {
		if err := mem.UpsertDevice(d); err != nil {
			t.Fatalf("seed device %s: %v", d.DeviceID, err)
		}
	}

	if err := mem.CreateApplyResult(store.ApplyResult{
		ApplyID:        uuid.NewString(),
		DeviceID:       successID,
		ArtifactID:     artifactID,
		Status:         "success",
		AppliedVersion: "2.0.0",
		CreatedAt:      now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed success result: %v", err)
	}
	if err := mem.CreateApplyResult(store.ApplyResult{
		ApplyID:    uuid.NewString(),
		DeviceID:   errorID,
		ArtifactID: artifactID,
		Status:     "error",
		Error:      "boom",
		CreatedAt:  now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed error result: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID+"/deployment-status?artifactId="+artifactID, nil)
	req = withURLParam(req, "groupId", groupID)
	w := httptest.NewRecorder()

	GetGroupDeploymentStatus(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp groupDeploymentStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total != 3 || resp.Pending != 1 || resp.Applied != 1 || resp.Failed != 1 || resp.Complete {
		t.Fatalf("unexpected aggregate counts: %#v", resp)
	}
	if len(resp.Devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(resp.Devices))
	}
	for _, d := range resp.Devices {
		switch d.DeviceID {
		case successID:
			if d.AppliedVersion != "2.0.0" || d.LastApplyAt == nil {
				t.Fatalf("expected populated success device, got %#v", d)
			}
		case errorID:
			if d.Error != "boom" || d.LastApplyAt == nil {
				t.Fatalf("expected populated error device, got %#v", d)
			}
		case pendingID:
			if d.Status != "pending" || d.LastApplyAt != nil || d.AppliedVersion != "" || d.Error != "" {
				t.Fatalf("expected pending device with omitted optional fields, got %#v", d)
			}
		}
	}
}

func TestGetGroupDeploymentStatus_ValidationAndEmptyGroup(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	groupID := uuid.NewString()
	if err := mem.UpsertGroup(store.Group{
		GroupID:      groupID,
		Name:         "edge",
		SelectorJSON: []byte(`{"role":"edge"}`),
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	t.Run("missing artifactId", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID+"/deployment-status", nil)
		req = withURLParam(req, "groupId", groupID)
		w := httptest.NewRecorder()
		GetGroupDeploymentStatus(logger, mem, false).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("unknown group", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/missing/deployment-status?artifactId=x", nil)
		req = withURLParam(req, "groupId", "missing")
		w := httptest.NewRecorder()
		GetGroupDeploymentStatus(logger, mem, false).ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("empty group", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID+"/deployment-status?artifactId=x", nil)
		req = withURLParam(req, "groupId", groupID)
		w := httptest.NewRecorder()
		GetGroupDeploymentStatus(logger, mem, false).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp groupDeploymentStatusResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Total != 0 || resp.Complete {
			t.Fatalf("unexpected empty-group response: %#v", resp)
		}
	})
}
