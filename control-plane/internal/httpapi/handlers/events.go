package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/hardwareops/control-plane/internal/events"
	"nhooyr.io/websocket"
)

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
