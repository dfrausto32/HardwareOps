package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
	whcrypto "github.com/parcel/control-plane/internal/webhooks"
)

func testWebhookKey() []byte {
	return []byte("01234567890123456789012345678901")
}

func seedWebhookWithSecret(t *testing.T, mem *memory.Store, url, secret string) string {
	t.Helper()

	encrypted, err := whcrypto.EncryptSecret(testWebhookKey(), secret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	id := uuid.NewString()
	if err := mem.CreateWebhook(store.Webhook{
		ID:              id,
		Name:            "ci",
		URL:             url,
		EncryptedSecret: encrypted,
		EventTypes:      []string{"ping", "deployment.triggered"},
		Enabled:         true,
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed webhook: %v", err)
	}
	return id
}

func TestTestWebhook_SignsPayload(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	received := make(chan struct {
		signature string
		body      []byte
	}, 1)
	secret := "shared-secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- struct {
			signature string
			body      []byte
		}{
			signature: r.Header.Get("X-Parcel-Signature"),
			body:      body,
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	webhookID := seedWebhookWithSecret(t, mem, srv.URL, secret)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID+"/test", nil)
	req = withURLParam(req, "webhookId", webhookID)
	w := httptest.NewRecorder()

	TestWebhook(logger, mem, testWebhookKey()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	got := <-received
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(got.body)
	wantSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got.signature != wantSig {
		t.Fatalf("expected signature %q, got %q", wantSig, got.signature)
	}
	var payload map[string]any
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["eventType"] != "ping" {
		t.Fatalf("expected ping payload, got %#v", payload["eventType"])
	}
}

func TestRedeliverWebhookDelivery(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	secret := "shared-secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	webhookID := seedWebhookWithSecret(t, mem, srv.URL, secret)
	originalPayload := []byte(`{"eventType":"deployment.triggered","deviceId":"dev-1"}`)
	original := store.WebhookDelivery{
		ID:          uuid.NewString(),
		WebhookID:   webhookID,
		EventType:   "deployment.triggered",
		PayloadJSON: originalPayload,
		Status:      "failed",
		Attempts:    1,
		CreatedAt:   time.Now().UTC().Add(-time.Minute),
	}
	if err := mem.CreateWebhookDelivery(original); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID+"/deliveries/"+original.ID+"/redeliver", nil)
	req = withURLParam(req, "webhookId", webhookID)
	req = withURLParam(req, "deliveryId", original.ID)
	w := httptest.NewRecorder()

	RedeliverWebhookDelivery(logger, mem, testWebhookKey()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	newID, _ := resp["deliveryId"].(string)
	if newID == "" || newID == original.ID {
		t.Fatalf("expected fresh delivery id, got %#v", resp["deliveryId"])
	}
	if resp["success"] != true {
		t.Fatalf("expected success=true, got %#v", resp["success"])
	}

	originalAfter, found, err := mem.GetWebhookDelivery(original.ID)
	if err != nil || !found {
		t.Fatalf("get original delivery: found=%v err=%v", found, err)
	}
	if originalAfter.Status != "failed" {
		t.Fatalf("expected original delivery preserved as failed, got %#v", originalAfter)
	}
	newDelivery, found, err := mem.GetWebhookDelivery(newID)
	if err != nil || !found {
		t.Fatalf("get new delivery: found=%v err=%v", found, err)
	}
	if newDelivery.Status != "delivered" || newDelivery.Attempts != 1 || string(newDelivery.PayloadJSON) != string(originalPayload) {
		t.Fatalf("unexpected redelivery record: %#v", newDelivery)
	}

	webhook, found, err := mem.GetWebhook(webhookID)
	if err != nil || !found {
		t.Fatalf("get webhook: found=%v err=%v", found, err)
	}
	if webhook.LastStatus != http.StatusNoContent || webhook.LastFiredAt.IsZero() {
		t.Fatalf("expected last fired metadata updated on success, got %#v", webhook)
	}
}

func TestRedeliverWebhookDelivery_ErrorCases(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	webhookID := seedWebhookWithSecret(t, mem, "://bad url", "shared-secret")
	deliveryID := uuid.NewString()
	if err := mem.CreateWebhookDelivery(store.WebhookDelivery{
		ID:          deliveryID,
		WebhookID:   webhookID,
		EventType:   "ping",
		PayloadJSON: []byte(`{"eventType":"ping"}`),
		Status:      "failed",
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}

	t.Run("missing encryption key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID+"/deliveries/"+deliveryID+"/redeliver", nil)
		req = withURLParam(req, "webhookId", webhookID)
		req = withURLParam(req, "deliveryId", deliveryID)
		w := httptest.NewRecorder()
		RedeliverWebhookDelivery(logger, mem, nil).ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})

	t.Run("unknown delivery", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID+"/deliveries/missing/redeliver", nil)
		req = withURLParam(req, "webhookId", webhookID)
		req = withURLParam(req, "deliveryId", "missing")
		w := httptest.NewRecorder()
		RedeliverWebhookDelivery(logger, mem, testWebhookKey()).ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("invalid target url records failed delivery", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID+"/deliveries/"+deliveryID+"/redeliver", nil)
		req = withURLParam(req, "webhookId", webhookID)
		req = withURLParam(req, "deliveryId", deliveryID)
		w := httptest.NewRecorder()
		RedeliverWebhookDelivery(logger, mem, testWebhookKey()).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["success"] != false {
			t.Fatalf("expected success=false, got %#v", resp["success"])
		}
		newID, _ := resp["deliveryId"].(string)
		delivery, found, err := mem.GetWebhookDelivery(newID)
		if err != nil || !found {
			t.Fatalf("expected failed redelivery record: found=%v err=%v", found, err)
		}
		if delivery.Status != "failed" {
			t.Fatalf("expected failed redelivery, got %#v", delivery)
		}
	})
}
