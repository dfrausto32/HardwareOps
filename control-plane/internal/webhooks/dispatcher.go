package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/store"
)

// Store is the narrow interface the Dispatcher needs from the data layer.
type Store interface {
	ListWebhooks() ([]store.Webhook, error)
	CreateWebhookDelivery(delivery store.WebhookDelivery) error
	UpdateWebhookDelivery(delivery store.WebhookDelivery) error
	UpdateWebhookLastFired(id string, at time.Time, status int) error
}

type deliveryJob struct {
	webhookID string
	url       string
	secret    string
	payload   []byte
	eventType string
	deliveryID string
}

// Dispatcher fans out events from the Hub to registered webhook endpoints.
type Dispatcher struct {
	store         Store
	encryptionKey []byte
	httpClient    *http.Client
	maxRetries    int
	jobs          chan deliveryJob
	logger        *log.Logger
}

type Config struct {
	Store          Store
	EncryptionKey  []byte
	Workers        int
	DeliveryTimeout time.Duration
	MaxRetries     int
}

func NewDispatcher(cfg Config, logger *log.Logger) *Dispatcher {
	workers := cfg.Workers
	if workers <= 0 {
		workers = 4
	}
	timeout := cfg.DeliveryTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &Dispatcher{
		store:         cfg.Store,
		encryptionKey: cfg.EncryptionKey,
		httpClient:    &http.Client{Timeout: timeout},
		maxRetries:    maxRetries,
		jobs:          make(chan deliveryJob, workers*64),
		logger:        logger,
	}
}

// Start launches the worker goroutines. Call before wiring the Dispatcher into
// a Hub via hub.SetDispatcher(d).
func (d *Dispatcher) Start(ctx context.Context) {
	workers := cap(d.jobs) / 64
	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		go d.worker(ctx)
	}
}

// Fanout implements events.WebhookDispatcher. It is called by the Hub on every
// published event and must return quickly.
func (d *Dispatcher) Fanout(event events.Event) {
	d.fanout(event)
}

func (d *Dispatcher) fanout(event events.Event) {
	if len(d.encryptionKey) == 0 {
		return // webhooks disabled when no encryption key is configured
	}
	hooks, err := d.store.ListWebhooks()
	if err != nil || len(hooks) == 0 {
		return
	}
	payload, _ := json.Marshal(Payload{
		ID:        uuid.NewString(),
		EventType: event.Type,
		DeviceID:  event.DeviceID,
		FiredAt:   event.At,
		Data:      event.Payload,
	})
	for _, wh := range hooks {
		if !wh.Enabled || !matchesEventType(wh.EventTypes, event.Type) {
			continue
		}
		secret, err := DecryptSecret(d.encryptionKey, wh.EncryptedSecret)
		if err != nil {
			d.logger.Printf("webhooks: decrypt secret for %s: %v", wh.ID, err)
			continue
		}
		deliveryID := uuid.NewString()
		_ = d.store.CreateWebhookDelivery(store.WebhookDelivery{
			ID:          deliveryID,
			WebhookID:   wh.ID,
			EventType:   event.Type,
			PayloadJSON: payload,
			Status:      "pending",
			CreatedAt:   time.Now().UTC(),
		})
		select {
		case d.jobs <- deliveryJob{
			webhookID:  wh.ID,
			url:        wh.URL,
			secret:     secret,
			payload:    payload,
			eventType:  event.Type,
			deliveryID: deliveryID,
		}:
		default:
			d.logger.Printf("webhooks: delivery queue full, dropping delivery for webhook %s", wh.ID)
		}
	}
}

func (d *Dispatcher) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-d.jobs:
			if !ok {
				return
			}
			d.deliver(ctx, job)
		}
	}
}

func (d *Dispatcher) deliver(ctx context.Context, job deliveryJob) {
	var lastStatus int
	var lastErr error

	backoff := time.Second
	for attempt := 1; attempt <= d.maxRetries; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff *= 5
			}
		}

		status, err := d.send(job.url, job.secret, job.payload)
		lastStatus = status
		lastErr = err

		now := time.Now().UTC()
		delivery := store.WebhookDelivery{
			ID:             job.deliveryID,
			WebhookID:      job.webhookID,
			EventType:      job.eventType,
			PayloadJSON:    job.payload,
			Attempts:       attempt,
			LastAttemptAt:  now,
			ResponseStatus: status,
		}

		if err == nil && status >= 200 && status < 300 {
			delivery.Status = "delivered"
			_ = d.store.UpdateWebhookDelivery(delivery)
			_ = d.store.UpdateWebhookLastFired(job.webhookID, now, status)
			return
		}

		delivery.Status = "pending"
		if attempt == d.maxRetries {
			delivery.Status = "failed"
		}
		_ = d.store.UpdateWebhookDelivery(delivery)
	}

	if lastErr != nil {
		d.logger.Printf("webhooks: delivery %s to %s failed after %d attempts: %v", job.deliveryID, job.url, d.maxRetries, lastErr)
	} else {
		d.logger.Printf("webhooks: delivery %s to %s failed after %d attempts: HTTP %d", job.deliveryID, job.url, d.maxRetries, lastStatus)
	}
}

func (d *Dispatcher) send(url, secret string, body []byte) (int, error) {
	sig := signPayload(secret, body)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HardwareOps-Signature", "sha256="+sig)
	req.Header.Set("User-Agent", "HardwareOps-Webhook/1.0")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http: %w", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func signPayload(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func matchesEventType(eventTypes []string, eventType string) bool {
	for _, t := range eventTypes {
		if t == "*" || t == eventType {
			return true
		}
	}
	return false
}
