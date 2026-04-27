package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/parcel/control-plane/internal/store"
)

type BreakglassRequest struct {
	Reason string `json:"reason"`
}

func decodeBreakglassRequest(r *http.Request, req *BreakglassRequest) error {
	if r == nil || req == nil || r.Body == nil {
		return errors.New("reason required")
	}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("reason required")
		}
		return err
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		return errors.New("reason required")
	}
	return nil
}

func breakglassMetadata(reason string, extra map[string]any) []byte {
	meta := map[string]any{
		"breakGlass": true,
		"reason":     reason,
	}
	for key, value := range extra {
		meta[key] = value
	}
	return auditJSON(meta)
}

func auditBreakglassFailure(logger *log.Logger, st store.Store, event store.AuditEvent, status, message, reason string, extra map[string]any) {
	event.Status = status
	event.Error = message
	event.MetadataJSON = breakglassMetadata(reason, extra)
	writeAudit(logger, st, event, nil)
}
