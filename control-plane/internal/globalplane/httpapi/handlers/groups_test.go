package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/globalplane"
)

func silentLogger() *log.Logger { return log.New(&bytes.Buffer{}, "", 0) }

func TestListGlobalGroups_Empty(t *testing.T) {
	st := newFakeGroupStore()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	w := httptest.NewRecorder()

	ListGlobalGroups(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(body, "[") {
		t.Fatalf("expected JSON array, got: %s", body)
	}
}

func TestCreateGlobalGroup_MissingName(t *testing.T) {
	st := newFakeGroupStore()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(`{"selectorJson":{}}`))
	w := httptest.NewRecorder()

	CreateGlobalGroup(st, silentLogger()).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateGlobalGroup_Valid(t *testing.T) {
	st := newFakeGroupStore()
	body := `{"name":"fleet-prod","selectorJson":{"env":"prod"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(body))
	w := httptest.NewRecorder()

	CreateGlobalGroup(st, silentLogger()).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var g globalplane.GlobalGroup
	if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if g.GroupID == "" || g.Name != "fleet-prod" {
		t.Fatalf("unexpected group: %+v", g)
	}
}

func TestDeleteGlobalGroup_NotFound(t *testing.T) {
	st := newFakeGroupStore()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+uuid.NewString(), nil)
	req = withURLParam(req, "groupId", uuid.NewString())
	w := httptest.NewRecorder()

	DeleteGlobalGroup(st, testEncKey, silentLogger()).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteGlobalGroup_Valid(t *testing.T) {
	st := newFakeGroupStore()
	g, _ := st.CreateGlobalGroup(globalplane.GlobalGroup{Name: "to-delete"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+g.GroupID, nil)
	req = withURLParam(req, "groupId", g.GroupID)
	w := httptest.NewRecorder()

	DeleteGlobalGroup(st, testEncKey, silentLogger()).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if _, ok, _ := st.GetGlobalGroup(g.GroupID); ok {
		t.Fatal("group should have been removed from store")
	}
}

func TestPutGlobalDesiredState_GroupNotFound(t *testing.T) {
	st := newFakeGroupStore()
	body := `{"artifactId":"a1","desiredVersion":"1.0.0"}`
	gid := uuid.NewString()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/groups/"+gid+"/desired-state", strings.NewReader(body))
	req = withURLParam(req, "groupId", gid)
	w := httptest.NewRecorder()

	PutGlobalDesiredState(st, testEncKey, silentLogger()).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPutGlobalDesiredState_Valid(t *testing.T) {
	st := newFakeGroupStore()
	g, _ := st.CreateGlobalGroup(globalplane.GlobalGroup{Name: "fleet-canary"})

	body := `{"artifactId":"artifact-123","desiredVersion":"2.0.0","checkinInterval":60}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/groups/"+g.GroupID+"/desired-state", strings.NewReader(body))
	req = withURLParam(req, "groupId", g.GroupID)
	w := httptest.NewRecorder()

	PutGlobalDesiredState(st, testEncKey, silentLogger()).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var state globalplane.GlobalDesiredState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if state.ArtifactID != "artifact-123" || state.DesiredVersion != "2.0.0" {
		t.Fatalf("unexpected state: %+v", state)
	}
	stored, ok, _ := st.GetGlobalDesiredState(g.GroupID)
	if !ok {
		t.Fatal("desired state not persisted to store")
	}
	if stored.CheckinInterval != 60 {
		t.Fatalf("expected checkinInterval 60, got %d", stored.CheckinInterval)
	}
}

func TestGetGlobalDesiredState_NotFound(t *testing.T) {
	st := newFakeGroupStore()
	g, _ := st.CreateGlobalGroup(globalplane.GlobalGroup{Name: "g1"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+g.GroupID+"/desired-state", nil)
	req = withURLParam(req, "groupId", g.GroupID)
	w := httptest.NewRecorder()

	GetGlobalDesiredState(st).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetGlobalDesiredState_Found(t *testing.T) {
	st := newFakeGroupStore()
	g, _ := st.CreateGlobalGroup(globalplane.GlobalGroup{Name: "g1"})
	_ = st.UpsertGlobalDesiredState(globalplane.GlobalDesiredState{
		GroupID:        g.GroupID,
		ArtifactID:     "art-1",
		DesiredVersion: "1.5.0",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+g.GroupID+"/desired-state", nil)
	req = withURLParam(req, "groupId", g.GroupID)
	w := httptest.NewRecorder()

	GetGlobalDesiredState(st).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var state globalplane.GlobalDesiredState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if state.ArtifactID != "art-1" {
		t.Fatalf("expected artifactId art-1, got %q", state.ArtifactID)
	}
}

func TestListGlobalDesiredStates_IncludesGroupsWithoutState(t *testing.T) {
	st := newFakeGroupStore()
	g1, _ := st.CreateGlobalGroup(globalplane.GlobalGroup{Name: "with-state"})
	_, _ = st.CreateGlobalGroup(globalplane.GlobalGroup{Name: "without-state"})
	_ = st.UpsertGlobalDesiredState(globalplane.GlobalDesiredState{GroupID: g1.GroupID, ArtifactID: "a1"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/desired-state", nil)
	w := httptest.NewRecorder()

	ListGlobalDesiredStates(st).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var rows []globalplane.GlobalDesiredStateWithGroup
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (both groups), got %d", len(rows))
	}
}

func TestDeleteGlobalDesiredState_Valid(t *testing.T) {
	st := newFakeGroupStore()
	g, _ := st.CreateGlobalGroup(globalplane.GlobalGroup{Name: "g1"})
	_ = st.UpsertGlobalDesiredState(globalplane.GlobalDesiredState{GroupID: g.GroupID, ArtifactID: "a1"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+g.GroupID+"/desired-state", nil)
	req = withURLParam(req, "groupId", g.GroupID)
	w := httptest.NewRecorder()

	DeleteGlobalDesiredState(st, testEncKey, silentLogger()).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if _, ok, _ := st.GetGlobalDesiredState(g.GroupID); ok {
		t.Fatal("desired state should have been removed")
	}
}

func TestPutGlobalDesiredState_FansOutToPlanes(t *testing.T) {
	received := make(chan struct{}, 1)
	fakeRegional := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/federation/policies" && r.Method == http.MethodPost {
			received <- struct{}{}
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer fakeRegional.Close()

	st := newFakeGroupStore()
	enc, _ := globalplane.EncryptToken("test-token", testEncKey)
	st.planes = []globalplane.RegionalPlane{{
		PlaneID:        uuid.NewString(),
		BaseURL:        fakeRegional.URL,
		EncryptedToken: enc,
		Enabled:        true,
	}}

	g, _ := st.CreateGlobalGroup(globalplane.GlobalGroup{Name: "fan-out-test"})
	body := `{"artifactId":"a1","desiredVersion":"1.0.0"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/groups/"+g.GroupID+"/desired-state", strings.NewReader(body))
	req = withURLParam(req, "groupId", g.GroupID)
	w := httptest.NewRecorder()

	PutGlobalDesiredState(st, testEncKey, silentLogger()).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case <-received:
		// Pass.
	case <-time.After(3 * time.Second):
		t.Fatal("fan-out to regional plane did not arrive within 3 seconds")
	}
}
