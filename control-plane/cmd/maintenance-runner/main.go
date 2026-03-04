package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/backup"
	"github.com/hardwareops/control-plane/internal/config"
	"github.com/hardwareops/control-plane/internal/upgrade"
)

type startRequest struct {
	Env map[string]string `json:"env,omitempty"`
}

func main() {
	cfg := config.FromEnv()
	logger := log.New(os.Stdout, "", log.LstdFlags)

	addr := strings.TrimSpace(os.Getenv("MAINTENANCE_RUNNER_ADDR"))
	if addr == "" {
		addr = ":8090"
	}

	upgradeRunner := buildUpgradeRunner(cfg, logger)
	backupRunner, restoreRunner := buildBackupRunners(cfg, logger)

	upgradeToken := strings.TrimSpace(os.Getenv("UPGRADE_RUNNER_TOKEN"))
	backupToken := strings.TrimSpace(os.Getenv("BACKUP_RUNNER_TOKEN"))
	fallbackToken := strings.TrimSpace(os.Getenv("MAINTENANCE_RUNNER_TOKEN"))
	if upgradeToken == "" {
		upgradeToken = fallbackToken
	}
	if backupToken == "" {
		backupToken = fallbackToken
	}
	if backupToken == "" {
		backupToken = upgradeToken
	}
	if upgradeToken == "" && backupToken == "" {
		logger.Printf("warning: no maintenance runner token configured; API is unauthenticated")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	mux.HandleFunc("/v1/upgrade/status", auth(upgradeToken, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeStatus(w, upgradeRunner)
	}))
	mux.HandleFunc("/v1/upgrade/start", auth(upgradeToken, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if upgradeRunner == nil || !upgradeRunner.Enabled() {
			http.Error(w, "upgrade runner not configured", http.StatusNotFound)
			return
		}
		status, err := upgradeRunner.Start()
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}))
	mux.HandleFunc("/v1/backup/status", auth(backupToken, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeStatus(w, backupRunner)
	}))
	mux.HandleFunc("/v1/backup/start", auth(backupToken, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if backupRunner == nil || !backupRunner.Enabled() {
			http.Error(w, "backup runner not configured", http.StatusNotFound)
			return
		}
		req, err := decodeStartRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var status backup.Status
		if len(req.Env) == 0 {
			status, err = backupRunner.Start()
		} else {
			status, err = backupRunner.StartWithEnv(req.Env)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}))
	mux.HandleFunc("/v1/restore/status", auth(backupToken, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeStatus(w, restoreRunner)
	}))
	mux.HandleFunc("/v1/restore/start", auth(backupToken, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if restoreRunner == nil || !restoreRunner.Enabled() {
			http.Error(w, "restore runner not configured", http.StatusNotFound)
			return
		}
		req, err := decodeStartRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		status, err := restoreRunner.StartWithEnv(req.Env)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}))

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Printf("maintenance-runner listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("maintenance-runner failed: %v", err)
	}
}

func buildUpgradeRunner(cfg config.Config, logger *log.Logger) *upgrade.Runner {
	mode := strings.ToLower(strings.TrimSpace(cfg.UpgradeRunnerMode))
	if mode == "" || mode == "disabled" {
		mode = "docker"
	}
	if mode != "docker" && mode != "local" {
		logger.Printf("upgrade runner disabled in maintenance-runner: unsupported mode %q", mode)
		return nil
	}
	r := upgrade.NewRunner(cfg.UpgradeApplyCmd, cfg.UpgradeWorkDir, cfg.UpgradeLogDir, logger.Printf)
	if r == nil {
		logger.Printf("upgrade runner disabled in maintenance-runner: UPGRADE_APPLY_CMD not set")
		return nil
	}
	if mode == "docker" {
		env := map[string]string{
			"STACK_DIR":           "/stack",
			"UPGRADE_UPDATES_DIR": "/stack/updates",
		}
		if baseURL := os.Getenv("PUBLIC_BASE_URL"); baseURL != "" {
			env["PUBLIC_BASE_URL"] = baseURL
		}
		if token := os.Getenv("MAINTENANCE_TOKEN"); token != "" {
			env["MAINTENANCE_TOKEN"] = token
		}
		if pull := os.Getenv("PULL_IMAGES"); pull != "" {
			env["PULL_IMAGES"] = pull
		}
		if project := os.Getenv("PROJECT_NAME"); project != "" {
			env["PROJECT_NAME"] = project
		}
		certsDir := ""
		if caPath := strings.TrimSpace(cfg.CACertPath); caPath != "" {
			certsDir = filepath.Dir(caPath)
			env["CERTS_DIR"] = "/certs"
			env["CA_CERT_PATH"] = "/certs/ca.crt"
		}
		r.ConfigureDocker(cfg.UpgradeRunnerImage, certsDir, env)
	}
	return r
}

func buildBackupRunners(cfg config.Config, logger *log.Logger) (*backup.Runner, *backup.Runner) {
	mode := strings.ToLower(strings.TrimSpace(cfg.BackupRunnerMode))
	if mode == "" || mode == "disabled" {
		mode = "docker"
	}
	if mode != "docker" && mode != "local" {
		logger.Printf("backup/restore runners disabled in maintenance-runner: unsupported mode %q", mode)
		return nil, nil
	}
	backupRunner := backup.NewRunner("backup", cfg.BackupCmd, cfg.BackupWorkDir, cfg.BackupLogDir, logger.Printf)
	restoreRunner := backup.NewRunner("restore", cfg.RestoreCmd, cfg.BackupWorkDir, cfg.BackupLogDir, logger.Printf)
	if mode == "docker" {
		certsDir := ""
		if cfg.CACertPath != "" {
			certsDir = filepath.Dir(cfg.CACertPath)
		}
		env := map[string]string{
			"BACKUP_DIR":                cfg.BackupDir,
			"BACKUP_POSTGRES_CONTAINER": cfg.BackupPostgresContainer,
			"BACKUP_MINIO_CONTAINER":    cfg.BackupMinioContainer,
			"BACKUP_POSTGRES_USER":      cfg.BackupPostgresUser,
			"BACKUP_POSTGRES_DB":        cfg.BackupPostgresDB,
		}
		if backupRunner != nil {
			backupRunner.ConfigureDocker(cfg.BackupRunnerImage, certsDir, env)
		}
		if restoreRunner != nil {
			restoreRunner.ConfigureDocker(cfg.BackupRunnerImage, certsDir, env)
		}
	}
	return backupRunner, restoreRunner
}

func decodeStartRequest(r *http.Request) (startRequest, error) {
	var req startRequest
	if r.Body == nil {
		return req, nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		return startRequest{}, errors.New("invalid json")
	}
	if req.Env == nil {
		req.Env = map[string]string{}
	}
	return req, nil
}

func auth(expectedToken string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if strings.TrimSpace(expectedToken) != "" {
			authz := strings.TrimSpace(r.Header.Get("Authorization"))
			prefix := "Bearer "
			if !strings.HasPrefix(authz, prefix) || strings.TrimSpace(strings.TrimPrefix(authz, prefix)) != strings.TrimSpace(expectedToken) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}

func writeStatus(w http.ResponseWriter, runner interface{ Enabled() bool }) {
	w.Header().Set("Content-Type", "application/json")
	switch r := runner.(type) {
	case *upgrade.Runner:
		if r == nil || !r.Enabled() {
			_ = json.NewEncoder(w).Encode(upgrade.Status{Enabled: false, State: "disabled"})
			return
		}
		_ = json.NewEncoder(w).Encode(r.Status())
	case *backup.Runner:
		if r == nil || !r.Enabled() {
			_ = json.NewEncoder(w).Encode(backup.Status{Enabled: false, State: "disabled"})
			return
		}
		_ = json.NewEncoder(w).Encode(r.Status())
	default:
		http.Error(w, "runner not configured", http.StatusNotFound)
	}
}
