package sync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// pendingItemFixture returns a minimal pending enrollment list item JSON.
func pendingItemFixture(requestID string) map[string]interface{} {
	return map[string]interface{}{
		"requestId":    requestID,
		"profileId":    "prof-1",
		"status":       "pending",
		"sourceIp":     "10.0.0.1",
		"agentVersion": "1.2.3",
		"hardwareId":   "hw-abc",
		"createdAt":    time.Now().UTC().Format(time.RFC3339),
		"expiresAt":    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
}

func TestFetchPendingEnrollments_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/pending-enrollments" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []interface{}{}})
		}
	}))
	defer srv.Close()

	client, err := newRegionalClient(srv.URL, "token", "")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	items, err := client.FetchPendingEnrollments(t.Context())
	if err != nil {
		t.Fatalf("FetchPendingEnrollments: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestFetchPendingEnrollments_ParsesItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]interface{}{
			pendingItemFixture("req-aaa"),
			pendingItemFixture("req-bbb"),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}))
	defer srv.Close()

	client, _ := newRegionalClient(srv.URL, "token", "")
	items, err := client.FetchPendingEnrollments(t.Context())
	if err != nil {
		t.Fatalf("FetchPendingEnrollments: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].RequestID != "req-aaa" || items[1].RequestID != "req-bbb" {
		t.Fatalf("unexpected requestIds: %v %v", items[0].RequestID, items[1].RequestID)
	}
	if items[0].Status != "pending" {
		t.Fatalf("expected status 'pending', got %q", items[0].Status)
	}
}

func TestFetchPendingEnrollments_Paginated(t *testing.T) {
	// First page is full (500 items), second page is partial (10 items).
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var items []map[string]interface{}
		offset := r.URL.Query().Get("offset")
		if offset == "0" || offset == "" {
			for i := 0; i < 500; i++ {
				items = append(items, pendingItemFixture(fmt.Sprintf("req-%d", i)))
			}
		} else {
			// Second page.
			for i := 0; i < 10; i++ {
				items = append(items, pendingItemFixture(fmt.Sprintf("req-page2-%d", i)))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}))
	defer srv.Close()

	client, _ := newRegionalClient(srv.URL, "token", "")
	items, err := client.FetchPendingEnrollments(t.Context())
	if err != nil {
		t.Fatalf("FetchPendingEnrollments: %v", err)
	}
	if len(items) != 510 {
		t.Fatalf("expected 510 items across pages, got %d", len(items))
	}
	if callCount != 2 {
		t.Fatalf("expected 2 HTTP calls (pagination), got %d", callCount)
	}
}

func TestFetchPendingEnrollments_BearerTokenSent(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []interface{}{}})
	}))
	defer srv.Close()

	client, _ := newRegionalClient(srv.URL, "my-secret-token", "")
	_, _ = client.FetchPendingEnrollments(t.Context())

	if receivedAuth != "Bearer my-secret-token" {
		t.Fatalf("expected Bearer token header, got %q", receivedAuth)
	}
}

func TestApprovePendingEnrollment_Success(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, _ := newRegionalClient(srv.URL, "tok", "")
	if err := client.ApprovePendingEnrollment(t.Context(), "req-xyz"); err != nil {
		t.Fatalf("ApprovePendingEnrollment: %v", err)
	}
	expected := "/api/v1/pending-enrollments/req-xyz/approve"
	if receivedPath != expected {
		t.Fatalf("expected path %q, got %q", expected, receivedPath)
	}
}

func TestApprovePendingEnrollment_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client, _ := newRegionalClient(srv.URL, "tok", "")
	err := client.ApprovePendingEnrollment(t.Context(), "req-missing")
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}

func TestDenyPendingEnrollment_SendsReason(t *testing.T) {
	type payload struct {
		Reason string `json:"reason"`
	}
	var received payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, _ := newRegionalClient(srv.URL, "tok", "")
	if err := client.DenyPendingEnrollment(t.Context(), "req-deny", "bad device"); err != nil {
		t.Fatalf("DenyPendingEnrollment: %v", err)
	}
	if received.Reason != "bad device" {
		t.Fatalf("expected reason 'bad device', got %q", received.Reason)
	}
}

func TestDenyPendingEnrollment_EmptyReasonAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, _ := newRegionalClient(srv.URL, "tok", "")
	if err := client.DenyPendingEnrollment(t.Context(), "req-1", ""); err != nil {
		t.Fatalf("expected no error for empty reason, got: %v", err)
	}
}
