package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/store"
	"nhooyr.io/websocket"
)

const (
	defaultEventLimit = 200
	maxEventLimit     = 5000
)

type RuntimeEventResponse struct {
	EventID    string          `json:"eventId,omitempty"`
	OccurredAt time.Time       `json:"occurredAt"`
	Type       string          `json:"type"`
	DeviceID   string          `json:"deviceId,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

type RuntimeEventListResponse struct {
	Items  []RuntimeEventResponse `json:"items"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

type RuntimeEventRetentionResponse struct {
	Days      int       `json:"days"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func StreamEvents(logger *log.Logger, hub *events.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if hub == nil {
			http.Error(w, "events not configured", http.StatusServiceUnavailable)
			return
		}

		origin := r.Header.Get("Origin")
		ua := r.Header.Get("User-Agent")
		remote := r.RemoteAddr
		if logger != nil {
			logger.Printf("events connect start remote=%s origin=%s ua=%s", remote, origin, ua)
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			if logger != nil {
				logger.Printf("events accept error remote=%s origin=%s err=%v", remote, origin, err)
			}
			return
		}
		if logger != nil {
			logger.Printf("events connected remote=%s origin=%s", remote, origin)
		}
		defer conn.Close(websocket.StatusNormalClosure, "closing")
		defer func() {
			if logger != nil {
				logger.Printf("events disconnected remote=%s origin=%s", remote, origin)
			}
		}()

		eventsCh, unsubscribe := hub.Subscribe()
		defer unsubscribe()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-eventsCh:
				if !ok {
					return
				}
				payload, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err = conn.Write(writeCtx, websocket.MessageText, payload)
				cancel()
				if err != nil {
					if logger != nil {
						if isNormalClose(err) {
							logger.Printf("events closed remote=%s origin=%s", remote, origin)
						} else {
							logger.Printf("events write error remote=%s origin=%s err=%v", remote, origin, err)
						}
					}
					return
				}
			}
		}
	}
}

func ListRuntimeEvents(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseRuntimeEventFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rows, err := st.ListRuntimeEvents(filter)
		if err != nil {
			if logger != nil {
				logger.Printf("list runtime events error: %v", err)
			}
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		items := make([]RuntimeEventResponse, 0, len(rows))
		for _, row := range rows {
			items = append(items, RuntimeEventResponse{
				EventID:    row.EventID,
				OccurredAt: row.OccurredAt,
				Type:       row.Type,
				DeviceID:   row.DeviceID,
				Payload:    row.PayloadJSON,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuntimeEventListResponse{
			Items:  items,
			Limit:  filter.Limit,
			Offset: filter.Offset,
		})

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "events.list", "events", "")
		event.MetadataJSON = auditJSON(map[string]any{
			"count":  len(items),
			"filter": filter,
		})
		writeAudit(logger, st, event, nil)
	}
}

func GetRuntimeEventRetention(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retention, err := st.GetRuntimeEventRetentionDays()
		if err != nil {
			if logger != nil {
				logger.Printf("get runtime event retention error: %v", err)
			}
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuntimeEventRetentionResponse{
			Days:      retention.Days,
			UpdatedAt: retention.UpdatedAt,
		})
	}
}

func SetRuntimeEventRetention(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Days int `json:"days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Days <= 0 || req.Days > 3650 {
			http.Error(w, "days must be between 1 and 3650", http.StatusBadRequest)
			return
		}
		before, _ := st.GetRuntimeEventRetentionDays()
		retention, err := st.SetRuntimeEventRetentionDays(req.Days)
		if err != nil {
			if logger != nil {
				logger.Printf("set runtime event retention error: %v", err)
			}
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		cutoff := time.Now().UTC().Add(-time.Duration(retention.Days) * 24 * time.Hour)
		if _, err := st.DeleteRuntimeEventsBefore(cutoff); err != nil && logger != nil {
			logger.Printf("prune runtime events error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuntimeEventRetentionResponse{
			Days:      retention.Days,
			UpdatedAt: retention.UpdatedAt,
		})

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "events.retention.update", "events", "")
		event.BeforeJSON = auditJSON(map[string]any{"days": before.Days, "updatedAt": before.UpdatedAt})
		event.AfterJSON = auditJSON(map[string]any{"days": retention.Days, "updatedAt": retention.UpdatedAt})
		writeAudit(logger, st, event, nil)
	}
}

func parseRuntimeEventFilter(r *http.Request) (store.RuntimeEventFilter, error) {
	q := r.URL.Query()
	since, err := parseTimeParam(q.Get("since"))
	if err != nil {
		return store.RuntimeEventFilter{}, err
	}
	until, err := parseTimeParam(q.Get("until"))
	if err != nil {
		return store.RuntimeEventFilter{}, err
	}
	if !since.IsZero() && !until.IsZero() && until.Before(since) {
		return store.RuntimeEventFilter{}, fmt.Errorf("until must be after since")
	}
	limit := parseIntParam(q.Get("limit"), defaultEventLimit)
	if limit > maxEventLimit {
		limit = maxEventLimit
	}
	offset := parseIntParam(q.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}
	return store.RuntimeEventFilter{
		Type:     q.Get("type"),
		DeviceID: q.Get("deviceId"),
		Since:    since,
		Until:    until,
		Limit:    limit,
		Offset:   offset,
	}, nil
}

func emitRuntimeEvent(logger *log.Logger, st store.Store, hub *events.Hub, ev events.Event) {
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	if hub != nil {
		hub.Publish(ev)
	}
	if st == nil {
		return
	}
	row := store.RuntimeEvent{
		Type:        ev.Type,
		DeviceID:    ev.DeviceID,
		OccurredAt:  ev.At,
		PayloadJSON: ev.Payload,
	}
	if err := st.CreateRuntimeEvent(row); err != nil && logger != nil {
		logger.Printf("runtime event write error: %v", err)
	}
}

func isNormalClose(err error) bool {
	if websocket.CloseStatus(err) != -1 {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return true
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	return false
}
