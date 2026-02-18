package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestPutGroup(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"name":"canary","selector":{"region":"west"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/groups/"+id, bytes.NewReader(body))
	req = withURLParam(req, "groupId", id)
	w := httptest.NewRecorder()

	PutGroup(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp GroupResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.GroupID != id {
		t.Fatalf("unexpected groupId")
	}
}

func TestListGroups(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	_ = mem.UpsertGroup(store.Group{GroupID: id, Name: "g1", SelectorJSON: []byte(`{"role":"edge"}`)})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	w := httptest.NewRecorder()

	ListGroups(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp GroupListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if len(resp.Items) == 0 {
		t.Fatalf("expected groups")
	}
}

func TestBatchGroups(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	existingID := uuid.NewString()
	if err := mem.UpsertGroup(store.Group{
		GroupID:      existingID,
		Name:         "existing",
		SelectorJSON: []byte(`{"role":"edge"}`),
	}); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	createID := uuid.NewString()
	body := []byte(`{
		"actions":[
			{"action":"upsert","groupId":"` + createID + `","name":"canary","selector":{"region":"west"}},
			{"action":"delete","groupId":"` + existingID + `"},
			{"action":"delete","groupId":"not-a-uuid"}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()

	BatchGroups(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp GroupBatchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Applied != 2 || resp.Failed != 1 {
		t.Fatalf("unexpected batch counts: applied=%d failed=%d", resp.Applied, resp.Failed)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("unexpected result count: %d", len(resp.Results))
	}

	groups, err := mem.ListGroups()
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	gotCreate := false
	gotExisting := false
	for _, g := range groups {
		if g.GroupID == createID {
			gotCreate = true
		}
		if g.GroupID == existingID {
			gotExisting = true
		}
	}
	if !gotCreate {
		t.Fatalf("expected created group in store")
	}
	if gotExisting {
		t.Fatalf("expected deleted group to be removed")
	}
}
