package memory

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
)

func TestGetGroupDeploymentStatus(t *testing.T) {
	mem := New()
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
		{DeviceID: uuid.NewString(), Status: "active", LastSeen: now, LabelsJSON: []byte(`{"role":"core"}`)},
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
		AppliedVersion: "1.0.0",
		CreatedAt:      now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed success result: %v", err)
	}
	if err := mem.CreateApplyResult(store.ApplyResult{
		ApplyID:    uuid.NewString(),
		DeviceID:   errorID,
		ArtifactID: artifactID,
		Status:     "error",
		Error:      "first failure",
		CreatedAt:  now.Add(-3 * time.Minute),
	}); err != nil {
		t.Fatalf("seed first error result: %v", err)
	}
	if err := mem.CreateApplyResult(store.ApplyResult{
		ApplyID:        uuid.NewString(),
		DeviceID:       errorID,
		ArtifactID:     artifactID,
		Status:         "success",
		AppliedVersion: "2.0.0",
		CreatedAt:      now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed latest result: %v", err)
	}

	got, err := mem.GetGroupDeploymentStatus(groupID, artifactID)
	if err != nil {
		t.Fatalf("get deployment status: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 matching active devices, got %d", len(got))
	}
	if got[0].DeviceID != successID && got[1].DeviceID != successID && got[2].DeviceID != successID {
		t.Fatalf("expected matching devices in result set: %#v", got)
	}

	statuses := map[string]store.DeviceDeploymentStatus{}
	for _, item := range got {
		statuses[item.DeviceID] = item
	}
	if statuses[successID].Status != "success" || statuses[successID].AppliedVersion != "1.0.0" {
		t.Fatalf("unexpected success status: %#v", statuses[successID])
	}
	if statuses[errorID].Status != "success" || statuses[errorID].AppliedVersion != "2.0.0" {
		t.Fatalf("expected latest result to win: %#v", statuses[errorID])
	}
	if statuses[pendingID].Status != "pending" || !statuses[pendingID].LastApplyAt.IsZero() {
		t.Fatalf("expected pending device with zero lastApplyAt: %#v", statuses[pendingID])
	}
}
