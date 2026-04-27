package handlers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/parcel/control-plane/internal/store"
)

func GetDeviceLogs(logger *log.Logger, st store.Store, logDir string, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if logDir == "" {
			http.Error(w, "log dir not configured", http.StatusNotFound)
			return
		}
		deviceID := chi.URLParam(r, "deviceId")
		if deviceID == "" {
			http.Error(w, "deviceId required", http.StatusBadRequest)
			return
		}
		if strings.Contains(deviceID, "/") || strings.Contains(deviceID, "..") {
			http.Error(w, "invalid deviceId", http.StatusBadRequest)
			return
		}
		path := filepath.Join(logDir, "device-"+deviceID+".csv")
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			logger.Printf("open logs error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "device.logs.download", "device", deviceID)
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=\"device-"+deviceID+".csv\"")
		http.ServeContent(w, r, path, fileModTime(path), f)
	}
}

func fileModTime(path string) (mtimes time.Time) {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	return
}
