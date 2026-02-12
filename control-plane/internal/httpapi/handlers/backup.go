package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/backup"
	"github.com/hardwareops/control-plane/internal/store"
)

type BackupItem struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt,omitempty"`
	PostgresDump string    `json:"postgresDump,omitempty"`
	MinioArchive string    `json:"minioArchive,omitempty"`
	SizeBytes    int64     `json:"sizeBytes,omitempty"`
}

type BackupListResponse struct {
	Dir   string       `json:"dir"`
	Items []BackupItem `json:"items"`
}

type RestoreRequest struct {
	ID   string `json:"id"`
	Wipe bool   `json:"wipe"`
}

func GetBackupStatus(runner *backup.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if runner == nil || !runner.Enabled() {
			_ = json.NewEncoder(w).Encode(backup.Status{Enabled: false, State: "disabled"})
			return
		}
		_ = json.NewEncoder(w).Encode(runner.Status())
	}
}

func GetRestoreStatus(runner *backup.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if runner == nil || !runner.Enabled() {
			_ = json.NewEncoder(w).Encode(backup.Status{Enabled: false, State: "disabled"})
			return
		}
		_ = json.NewEncoder(w).Encode(runner.Status())
	}
}

func ListBackups(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := BackupListResponse{Dir: dir}
		if dir == "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		items, err := collectBackups(dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp.Items = items
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func StartBackup(logger *log.Logger, st store.Store, runner *backup.Runner, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if runner == nil || !runner.Enabled() {
			http.Error(w, "backup runner not configured", http.StatusNotFound)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "backup.create", "backup", "stack")
		status, err := runner.Start()
		if err != nil {
			writeAudit(logger, st, event, err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		event.AfterJSON = auditJSON(status)
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func StartRestore(logger *log.Logger, st store.Store, runner *backup.Runner, maintenance MaintenanceStateView, backupDir string, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if runner == nil || !runner.Enabled() {
			http.Error(w, "restore runner not configured", http.StatusNotFound)
			return
		}
		if maintenance == nil {
			http.Error(w, "maintenance state not configured", http.StatusPreconditionFailed)
			return
		}
		enabled, _, _ := maintenance.Get()
		if !enabled {
			http.Error(w, "maintenance mode must be enabled to restore backups", http.StatusConflict)
			return
		}
		var req RestoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			http.Error(w, "backup id required", http.StatusBadRequest)
			return
		}
		if !req.Wipe {
			http.Error(w, "wipe must be true to restore", http.StatusBadRequest)
			return
		}
		if err := ensureBackupExists(backupDir, strings.TrimSpace(req.ID)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "backup.restore", "backup", req.ID)
		status, err := runner.StartWithEnv(map[string]string{
			"BACKUP_ID": strings.TrimSpace(req.ID),
			"WIPE":      "1",
		})
		if err != nil {
			writeAudit(logger, st, event, err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		event.AfterJSON = auditJSON(status)
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func collectBackups(dir string) ([]BackupItem, error) {
	if dir == "" {
		return nil, nil
	}
	if stat, err := os.Stat(dir); err != nil || !stat.IsDir() {
		return nil, nil
	}
	matches, err := filepath.Glob(filepath.Join(dir, "backup-*.pg.dump"))
	if err != nil {
		return nil, err
	}
	items := make([]BackupItem, 0, len(matches))
	for _, pgDump := range matches {
		base := filepath.Base(pgDump)
		id := strings.TrimSuffix(base, ".pg.dump")
		minio := filepath.Join(dir, id+".minio.tgz")
		info, _ := os.Stat(pgDump)
		size := int64(0)
		createdAt := time.Time{}
		if info != nil {
			size += info.Size()
			createdAt = info.ModTime().UTC()
		}
		if minioInfo, err := os.Stat(minio); err == nil {
			size += minioInfo.Size()
			if createdAt.IsZero() {
				createdAt = minioInfo.ModTime().UTC()
			}
		}
		items = append(items, BackupItem{
			ID:           id,
			CreatedAt:    createdAt,
			PostgresDump: pgDump,
			MinioArchive: minio,
			SizeBytes:    size,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func ensureBackupExists(dir, id string) error {
	if id == "" {
		return errors.New("backup id required")
	}
	if dir == "" {
		return errors.New("backup dir not configured")
	}
	pgDump := filepath.Join(dir, id+".pg.dump")
	if _, err := os.Stat(pgDump); err != nil {
		return fmt.Errorf("missing postgres dump: %s", pgDump)
	}
	return nil
}
