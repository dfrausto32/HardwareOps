package lifecycle

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestManagerRunNowDeletesEligibleArtifacts(t *testing.T) {
	mem := memory.New()
	now := time.Now().UTC()
	artifactID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID:   artifactID,
		Name:         "bundle",
		Version:      "1.0.0",
		Type:         "app_bundle",
		Status:       "deprecated",
		ObjectKey:    "artifacts/a.tar.gz",
		SHA256:       "abc",
		SizeBytes:    10,
		CreatedAt:    now.Add(-24 * time.Hour),
		DeprecatedAt: now.Add(-12 * time.Hour),
		DeleteAfter:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	mgr := NewManager(ManagerConfig{
		Enabled:    true,
		Interval:   time.Hour,
		BatchLimit: 100,
		Store:      mem,
		Bucket:     "artifacts",
	})
	if mgr == nil {
		t.Fatalf("expected manager")
	}
	res, err := mgr.RunNow(0)
	if err != nil {
		t.Fatalf("run now error: %v", err)
	}
	if res.DeletedNum != 1 {
		t.Fatalf("expected deleted=1 got=%d", res.DeletedNum)
	}
	if _, ok, err := mem.GetArtifact(artifactID); err != nil {
		t.Fatalf("get artifact error: %v", err)
	} else if ok {
		t.Fatalf("artifact should be deleted")
	}
}

func TestManagerRunNowSkipsReferencedArtifacts(t *testing.T) {
	mem := memory.New()
	now := time.Now().UTC()
	artifactID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID:   artifactID,
		Name:         "bundle",
		Version:      "1.0.0",
		Type:         "app_bundle",
		Status:       "deprecated",
		ObjectKey:    "artifacts/a.tar.gz",
		SHA256:       "abc",
		SizeBytes:    10,
		CreatedAt:    now.Add(-24 * time.Hour),
		DeprecatedAt: now.Add(-12 * time.Hour),
		DeleteAfter:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := mem.UpsertDesiredStateDevice(store.DesiredStateDevice{
		DeviceID:   "dev-1",
		ArtifactID: artifactID,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("upsert desired device: %v", err)
	}
	mgr := NewManager(ManagerConfig{
		Enabled:                 true,
		Interval:                time.Hour,
		BatchLimit:              100,
		AlertReferenceThreshold: 1,
		Store:                   mem,
	})
	res, err := mgr.RunNow(0)
	if err != nil {
		t.Fatalf("run now error: %v", err)
	}
	if res.DeletedNum != 0 {
		t.Fatalf("expected deleted=0 got=%d", res.DeletedNum)
	}
	if res.SkippedReferenced != 1 {
		t.Fatalf("expected skipped referenced=1 got=%d", res.SkippedReferenced)
	}
	status := mgr.Status()
	if len(status.Alerts) == 0 {
		t.Fatalf("expected alerts for referenced backlog")
	}
}
