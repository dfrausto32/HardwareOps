package webhooks

import (
	"encoding/json"
	"time"
)

// Payload is the JSON body delivered to a webhook endpoint.
type Payload struct {
	ID        string          `json:"id"`
	EventType string          `json:"eventType"`
	FiredAt   time.Time       `json:"firedAt"`
	Data      json.RawMessage `json:"data,omitempty"`
}
