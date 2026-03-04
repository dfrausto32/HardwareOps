package releaseautoupdate

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestRunNow_UpdatesGroupDesiredToLatestVersion(t *testing.T) {
	st := memory.New()
	_, err := st.SetReleaseAutoUpdateSettings(store.ReleaseAutoUpdateSettings{
		Enabled:       true,
		AllowUnsigned: false,
	})
	if err != nil {
		t.Fatalf("set settings: %v", err)
	}

	oldArtifact := store.Artifact{
		ArtifactID: uuid.NewString(),
		Name:       "customer-app",
		Type:       "app_bundle",
		Version:    "1.2.3",
		Status:     "active",
		Signature:  "sig",
		CreatedAt:  time.Now().UTC(),
	}
	newArtifact := oldArtifact
	newArtifact.ArtifactID = uuid.NewString()
	newArtifact.Version = "1.2.4"
	if err := st.CreateArtifact(oldArtifact); err != nil {
		t.Fatalf("create old artifact: %v", err)
	}
	if err := st.CreateArtifact(newArtifact); err != nil {
		t.Fatalf("create new artifact: %v", err)
	}

	groupID := uuid.NewString()
	if err := st.UpsertDesiredStateGroup(store.DesiredStateGroup{
		GroupID:        groupID,
		ArtifactID:     oldArtifact.ArtifactID,
		DesiredVersion: oldArtifact.Version,
		ComponentsJSON: []byte(`{"app_bundle":{"artifactId":"` + oldArtifact.ArtifactID + `","artifactType":"app_bundle","desiredVersion":"1.2.3","policy":{"hwops":{"autoVersion":{"mode":"enabled"}}}}}`),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert desired group: %v", err)
	}

	mgr := New(Config{Store: st, Interval: time.Second})
	if _, err := mgr.RunNow("test"); err != nil {
		t.Fatalf("run now: %v", err)
	}

	groups, err := st.ListDesiredStateGroups()
	if err != nil {
		t.Fatalf("list desired groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected one group, got %d", len(groups))
	}
	components := decodeDesiredComponents(groups[0].ComponentsJSON)
	comp := components["app_bundle"]
	if comp.ArtifactID != newArtifact.ArtifactID {
		t.Fatalf("expected artifact %s, got %s", newArtifact.ArtifactID, comp.ArtifactID)
	}
	if comp.DesiredVersion != newArtifact.Version {
		t.Fatalf("expected desiredVersion %s, got %s", newArtifact.Version, comp.DesiredVersion)
	}
}

func TestRunNow_UnsignedArtifactBlockedByDefault(t *testing.T) {
	st := memory.New()
	_, err := st.SetReleaseAutoUpdateSettings(store.ReleaseAutoUpdateSettings{
		Enabled:       true,
		AllowUnsigned: false,
	})
	if err != nil {
		t.Fatalf("set settings: %v", err)
	}

	oldArtifact := store.Artifact{
		ArtifactID: uuid.NewString(),
		Name:       "integration",
		Type:       "container_image",
		Version:    "2.0.0",
		Status:     "active",
		Signature:  "sig",
		CreatedAt:  time.Now().UTC(),
	}
	unsignedNew := oldArtifact
	unsignedNew.ArtifactID = uuid.NewString()
	unsignedNew.Version = "2.0.1"
	unsignedNew.Signature = ""
	if err := st.CreateArtifact(oldArtifact); err != nil {
		t.Fatalf("create old artifact: %v", err)
	}
	if err := st.CreateArtifact(unsignedNew); err != nil {
		t.Fatalf("create unsigned artifact: %v", err)
	}
	if err := st.UpsertDesiredStateGroup(store.DesiredStateGroup{
		GroupID:        uuid.NewString(),
		ArtifactID:     oldArtifact.ArtifactID,
		DesiredVersion: oldArtifact.Version,
		ComponentsJSON: []byte(`{"integration":{"artifactId":"` + oldArtifact.ArtifactID + `","artifactType":"container_image","desiredVersion":"2.0.0","policy":{"hwops":{"autoVersion":{"mode":"enabled"}}}}}`),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert desired group: %v", err)
	}

	mgr := New(Config{Store: st, Interval: time.Second})
	if _, err := mgr.RunNow("test"); err != nil {
		t.Fatalf("run now: %v", err)
	}

	groups, err := st.ListDesiredStateGroups()
	if err != nil {
		t.Fatalf("list desired groups: %v", err)
	}
	components := decodeDesiredComponents(groups[0].ComponentsJSON)
	comp := components["integration"]
	if comp.ArtifactID != oldArtifact.ArtifactID {
		t.Fatalf("expected artifact to remain %s, got %s", oldArtifact.ArtifactID, comp.ArtifactID)
	}
}

func TestParseSemver4(t *testing.T) {
	cases := []struct {
		value string
		ok    bool
	}{
		{value: "1.2.3", ok: true},
		{value: "1.2.3.4", ok: true},
		{value: "v1.2.3.4", ok: true},
		{value: "1.2", ok: false},
		{value: "1.2.3-beta", ok: false},
	}
	for _, tc := range cases {
		_, ok := parseSemver4(tc.value)
		if ok != tc.ok {
			t.Fatalf("parseSemver4(%q) expected %v, got %v", tc.value, tc.ok, ok)
		}
	}
}
