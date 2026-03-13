package releaseautoupdate

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/artifacttrust"
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

func TestRunNow_ExplicitTrackingResolvesWithoutCurrentArtifact(t *testing.T) {
	st := memory.New()
	_, err := st.SetReleaseAutoUpdateSettings(store.ReleaseAutoUpdateSettings{
		Enabled:       true,
		AllowUnsigned: false,
	})
	if err != nil {
		t.Fatalf("set settings: %v", err)
	}

	artifactA := store.Artifact{
		ArtifactID:         uuid.NewString(),
		Name:               "customer-app",
		Type:               "app_bundle",
		Version:            "3.1.0",
		Status:             "active",
		Signature:          "sig",
		VerificationStatus: artifacttrust.VerificationStatusVerified,
		SignatureType:      artifacttrust.SignatureTypeEd25519,
		SignatureKeyID:     "sha256:test-key",
		CreatedAt:          time.Now().UTC(),
	}
	artifactB := artifactA
	artifactB.ArtifactID = uuid.NewString()
	artifactB.Version = "3.2.0"
	if err := st.CreateArtifact(artifactA); err != nil {
		t.Fatalf("create artifactA: %v", err)
	}
	if err := st.CreateArtifact(artifactB); err != nil {
		t.Fatalf("create artifactB: %v", err)
	}

	if err := st.UpsertDesiredStateGroup(store.DesiredStateGroup{
		GroupID:        uuid.NewString(),
		ComponentsJSON: []byte(`{"customer":{"artifactType":"app_bundle","desiredVersion":"","policy":{"hwops":{"tracking":{"mode":"enabled","name":"customer-app","artifactType":"app_bundle"}}}}}`),
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
	comp := decodeDesiredComponents(groups[0].ComponentsJSON)["customer"]
	if comp.ArtifactID != artifactB.ArtifactID {
		t.Fatalf("expected latest artifact %s, got %s", artifactB.ArtifactID, comp.ArtifactID)
	}
	if comp.DesiredVersion != artifactB.Version {
		t.Fatalf("expected desiredVersion %s, got %s", artifactB.Version, comp.DesiredVersion)
	}
}

func TestRunNow_ExplicitTrackingRequiresCompleteTarget(t *testing.T) {
	st := memory.New()
	_, err := st.SetReleaseAutoUpdateSettings(store.ReleaseAutoUpdateSettings{
		Enabled:       true,
		AllowUnsigned: false,
	})
	if err != nil {
		t.Fatalf("set settings: %v", err)
	}

	artifact := store.Artifact{
		ArtifactID: uuid.NewString(),
		Name:       "customer-app",
		Type:       "app_bundle",
		Version:    "1.0.0",
		Status:     "active",
		Signature:  "sig",
		CreatedAt:  time.Now().UTC(),
	}
	if err := st.CreateArtifact(artifact); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := st.UpsertDesiredStateGroup(store.DesiredStateGroup{
		GroupID:        uuid.NewString(),
		ComponentsJSON: []byte(`{"customer":{"policy":{"hwops":{"tracking":{"mode":"enabled","name":"customer-app"}}}}}`),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert desired group: %v", err)
	}

	mgr := New(Config{Store: st, Interval: time.Second})
	summary, err := mgr.RunNow("test")
	if err != nil {
		t.Fatalf("run now: %v", err)
	}
	if summary.SkippedMisconfig != 1 {
		t.Fatalf("expected one misconfigured component, got %+v", summary)
	}
	groups, err := st.ListDesiredStateGroups()
	if err != nil {
		t.Fatalf("list desired groups: %v", err)
	}
	comp := decodeDesiredComponents(groups[0].ComponentsJSON)["customer"]
	if comp.ArtifactID != "" {
		t.Fatalf("expected unresolved artifactId, got %s", comp.ArtifactID)
	}
}

func TestRunNow_ExplicitTrackingHonorsTrustPolicy(t *testing.T) {
	st := memory.New()
	_, err := st.SetReleaseAutoUpdateSettings(store.ReleaseAutoUpdateSettings{
		Enabled:       true,
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatalf("set settings: %v", err)
	}
	_, err = st.SetArtifactTrustPolicy(store.ArtifactTrustPolicy{
		VerificationMode:          artifacttrust.VerificationModeRequire,
		AllowedSigningKeyIDsJSON:  []byte(`["sha256:trusted"]`),
		AllowedSignatureTypesJSON: []byte(`["ed25519"]`),
		UpdatedAt:                 time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("set trust policy: %v", err)
	}

	legacyNew := store.Artifact{
		ArtifactID:         uuid.NewString(),
		Name:               "secure-app",
		Type:               "app_bundle",
		Version:            "2.0.0",
		Status:             "active",
		Signature:          "sig",
		SignatureType:      artifacttrust.SignatureTypeEd25519,
		SignatureKeyID:     "sha256:trusted",
		VerificationStatus: artifacttrust.VerificationStatusLegacy,
		CreatedAt:          time.Now().UTC(),
	}
	verifiedNew := legacyNew
	verifiedNew.ArtifactID = uuid.NewString()
	verifiedNew.Version = "2.1.0"
	verifiedNew.VerificationStatus = artifacttrust.VerificationStatusVerified
	if err := st.CreateArtifact(legacyNew); err != nil {
		t.Fatalf("create legacyNew: %v", err)
	}
	if err := st.CreateArtifact(verifiedNew); err != nil {
		t.Fatalf("create verifiedNew: %v", err)
	}

	if err := st.UpsertDesiredStateGroup(store.DesiredStateGroup{
		GroupID:        uuid.NewString(),
		ComponentsJSON: []byte(`{"secure":{"policy":{"hwops":{"tracking":{"mode":"enabled","name":"secure-app","artifactType":"app_bundle"}}}}}`),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert desired group: %v", err)
	}

	mgr := New(Config{Store: st, Interval: time.Second})
	summary, err := mgr.RunNow("test")
	if err != nil {
		t.Fatalf("run now: %v", err)
	}
	if summary.SkippedTrustBlocked != 0 {
		t.Fatalf("expected verified candidate to be selected, got summary %+v", summary)
	}

	groups, err := st.ListDesiredStateGroups()
	if err != nil {
		t.Fatalf("list desired groups: %v", err)
	}
	comp := decodeDesiredComponents(groups[0].ComponentsJSON)["secure"]
	if comp.ArtifactID != verifiedNew.ArtifactID {
		t.Fatalf("expected verified artifact %s, got %s", verifiedNew.ArtifactID, comp.ArtifactID)
	}
}

func TestRunNow_ExplicitTrackingPrefersNewestDuplicateVersion(t *testing.T) {
	st := memory.New()
	_, err := st.SetReleaseAutoUpdateSettings(store.ReleaseAutoUpdateSettings{
		Enabled:       true,
		AllowUnsigned: false,
	})
	if err != nil {
		t.Fatalf("set settings: %v", err)
	}

	older := store.Artifact{
		ArtifactID:         "11111111-1111-1111-1111-111111111111",
		Name:               "trackingdemo",
		Type:               "app_bundle",
		Version:            "1.2.0",
		Status:             "active",
		Signature:          "sig",
		VerificationStatus: artifacttrust.VerificationStatusVerified,
		SignatureType:      artifacttrust.SignatureTypeEd25519,
		SignatureKeyID:     "sha256:test-key",
		CreatedAt:          time.Now().UTC().Add(-time.Hour),
	}
	newer := older
	newer.ArtifactID = "22222222-2222-2222-2222-222222222222"
	newer.CreatedAt = time.Now().UTC()
	if err := st.CreateArtifact(older); err != nil {
		t.Fatalf("create older artifact: %v", err)
	}
	if err := st.CreateArtifact(newer); err != nil {
		t.Fatalf("create newer artifact: %v", err)
	}

	if err := st.UpsertDesiredStateGroup(store.DesiredStateGroup{
		GroupID:        uuid.NewString(),
		ComponentsJSON: []byte(`{"trackingdemo":{"artifactType":"app_bundle","desiredVersion":"","policy":{"hwops":{"tracking":{"mode":"enabled","name":"trackingdemo","artifactType":"app_bundle"}}}}}`),
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
	comp := decodeDesiredComponents(groups[0].ComponentsJSON)["trackingdemo"]
	if comp.ArtifactID != newer.ArtifactID {
		t.Fatalf("expected newest duplicate artifact %s, got %s", newer.ArtifactID, comp.ArtifactID)
	}
}

func TestPreferArtifactUsesArtifactIDAsStableTieBreaker(t *testing.T) {
	now := time.Now().UTC()
	a := versionedArtifact{
		artifact: store.Artifact{ArtifactID: "aaa", CreatedAt: now},
		version:  semver4{major: 1, minor: 2, patch: 3},
	}
	b := versionedArtifact{
		artifact: store.Artifact{ArtifactID: "bbb", CreatedAt: now},
		version:  semver4{major: 1, minor: 2, patch: 3},
	}
	if !preferArtifact(b, a) {
		t.Fatalf("expected lexically larger artifact id to win as final tie-breaker")
	}
	if preferArtifact(a, b) {
		t.Fatalf("expected lexically smaller artifact id to lose as final tie-breaker")
	}
}
