package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/webhooks"
)

type CreateWebhookRequest struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	EventTypes []string `json:"eventTypes"`
	Enabled    bool     `json:"enabled"`
}

type WebhookResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	EventTypes  []string  `json:"eventTypes"`
	Enabled     bool      `json:"enabled"`
	CreatedBy   string    `json:"createdBy,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	LastFiredAt time.Time `json:"lastFiredAt,omitempty"`
	LastStatus  int       `json:"lastStatus,omitempty"`
	// Secret is only populated on creation; never returned again.
	Secret string `json:"secret,omitempty"`
}

type WebhookDeliveryResponse struct {
	ID             string    `json:"id"`
	WebhookID      string    `json:"webhookId"`
	EventType      string    `json:"eventType"`
	Status         string    `json:"status"`
	Attempts       int       `json:"attempts"`
	LastAttemptAt  time.Time `json:"lastAttemptAt,omitempty"`
	ResponseStatus int       `json:"responseStatus,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

func webhookView(wh store.Webhook) WebhookResponse {
	return WebhookResponse{
		ID:          wh.ID,
		Name:        wh.Name,
		URL:         wh.URL,
		EventTypes:  wh.EventTypes,
		Enabled:     wh.Enabled,
		CreatedBy:   wh.CreatedBy,
		CreatedAt:   wh.CreatedAt,
		LastFiredAt: wh.LastFiredAt,
		LastStatus:  wh.LastStatus,
	}
}

func CreateWebhook(logger *log.Logger, st store.Store, encryptionKey []byte, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateWebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.URL = strings.TrimSpace(req.URL)
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if req.URL == "" || (!strings.HasPrefix(req.URL, "https://") && !strings.HasPrefix(req.URL, "http://")) {
			http.Error(w, "url must be a valid http/https URL", http.StatusBadRequest)
			return
		}
		if len(req.EventTypes) == 0 {
			http.Error(w, "eventTypes must contain at least one entry", http.StatusBadRequest)
			return
		}
		if len(encryptionKey) == 0 {
			http.Error(w, "webhook secret encryption is not configured on this server", http.StatusServiceUnavailable)
			return
		}

		// Generate a random 32-byte signing secret returned once to the caller.
		rawSecret := make([]byte, 32)
		if _, err := rand.Read(rawSecret); err != nil {
			logger.Printf("webhook: generate secret: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		secretStr := base64.StdEncoding.EncodeToString(rawSecret)
		encrypted, err := webhooks.EncryptSecret(encryptionKey, secretStr)
		if err != nil {
			logger.Printf("webhook: encrypt secret: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		wh := store.Webhook{
			ID:              uuid.NewString(),
			Name:            req.Name,
			URL:             req.URL,
			EncryptedSecret: encrypted,
			EventTypes:      req.EventTypes,
			Enabled:         req.Enabled,
			CreatedAt:       time.Now().UTC(),
		}
		if u, ok := auth.UserFromContext(r.Context()); ok {
			wh.CreatedBy = u.UserID
		}
		if err := st.CreateWebhook(wh); err != nil {
			logger.Printf("create webhook error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "webhook.create", "webhook", wh.ID)
		event.AfterJSON = auditJSON(map[string]any{"name": wh.Name, "url": wh.URL, "eventTypes": wh.EventTypes})
		writeAudit(logger, st, event, nil)

		resp := webhookView(wh)
		resp.Secret = secretStr
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func ListWebhooks(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hooks, err := st.ListWebhooks()
		if err != nil {
			logger.Printf("list webhooks error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		out := make([]WebhookResponse, 0, len(hooks))
		for _, wh := range hooks {
			out = append(out, webhookView(wh))
		}
		writeJSON(w, map[string]any{"items": out})
	}
}

func GetWebhook(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "webhookId")
		wh, found, err := st.GetWebhook(id)
		if err != nil {
			logger.Printf("get webhook error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, webhookView(wh))
	}
}

func UpdateWebhook(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "webhookId")
		existing, found, err := st.GetWebhook(id)
		if err != nil {
			logger.Printf("get webhook error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req CreateWebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if n := strings.TrimSpace(req.Name); n != "" {
			existing.Name = n
		}
		if u := strings.TrimSpace(req.URL); u != "" {
			existing.URL = u
		}
		if len(req.EventTypes) > 0 {
			existing.EventTypes = req.EventTypes
		}
		existing.Enabled = req.Enabled
		if err := st.UpdateWebhook(existing); err != nil {
			logger.Printf("update webhook error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "webhook.update", "webhook", id)
		writeAudit(logger, st, event, nil)
		writeJSON(w, webhookView(existing))
	}
}

func DeleteWebhook(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "webhookId")
		if _, found, err := st.GetWebhook(id); err != nil {
			logger.Printf("get webhook error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		} else if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := st.DeleteWebhook(id); err != nil {
			logger.Printf("delete webhook error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "webhook.delete", "webhook", id)
		writeAudit(logger, st, event, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func ListWebhookDeliveries(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "webhookId")
		if _, found, err := st.GetWebhook(id); err != nil {
			logger.Printf("get webhook error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		} else if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		limit := parseInt(r.URL.Query().Get("limit"), 50)
		deliveries, err := st.ListWebhookDeliveries(id, limit)
		if err != nil {
			logger.Printf("list webhook deliveries error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		out := make([]WebhookDeliveryResponse, 0, len(deliveries))
		for _, d := range deliveries {
			out = append(out, WebhookDeliveryResponse{
				ID:             d.ID,
				WebhookID:      d.WebhookID,
				EventType:      d.EventType,
				Status:         d.Status,
				Attempts:       d.Attempts,
				LastAttemptAt:  d.LastAttemptAt,
				ResponseStatus: d.ResponseStatus,
				CreatedAt:      d.CreatedAt,
			})
		}
		writeJSON(w, map[string]any{"items": out})
	}
}

// TestWebhook sends a synthetic ping event so operators can verify connectivity.
func TestWebhook(logger *log.Logger, st store.Store, encryptionKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "webhookId")
		wh, found, err := st.GetWebhook(id)
		if err != nil {
			logger.Printf("get webhook error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if len(encryptionKey) == 0 {
			http.Error(w, "webhook encryption not configured", http.StatusServiceUnavailable)
			return
		}
		secret, err := webhooks.DecryptSecret(encryptionKey, wh.EncryptedSecret)
		if err != nil {
			logger.Printf("webhook test: decrypt secret: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"id":        uuid.NewString(),
			"eventType": "ping",
			"firedAt":   time.Now().UTC(),
			"data":      map[string]any{"webhookId": id, "test": true},
		})
		sig := hmacSHA256Hex(secret, payload)
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, wh.URL, strings.NewReader(string(payload)))
		if err != nil {
			http.Error(w, "invalid webhook url", http.StatusBadRequest)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-HardwareOps-Signature", "sha256="+sig)
		req.Header.Set("User-Agent", "HardwareOps-Webhook/1.0")
		resp, err := client.Do(req)
		if err != nil {
			writeJSON(w, map[string]any{"success": false, "error": err.Error()})
			return
		}
		_ = resp.Body.Close()
		writeJSON(w, map[string]any{"success": resp.StatusCode >= 200 && resp.StatusCode < 300, "statusCode": resp.StatusCode})
	}
}

func hmacSHA256Hex(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
