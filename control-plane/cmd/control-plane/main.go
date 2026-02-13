package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/backup"
	"github.com/hardwareops/control-plane/internal/certs"
	"github.com/hardwareops/control-plane/internal/config"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/httpapi"
	"github.com/hardwareops/control-plane/internal/license"
	"github.com/hardwareops/control-plane/internal/logging"
	"github.com/hardwareops/control-plane/internal/metrics"
	"github.com/hardwareops/control-plane/internal/migrate"
	"github.com/hardwareops/control-plane/internal/objectstore"
	"github.com/hardwareops/control-plane/internal/store/postgres"
	"github.com/hardwareops/control-plane/internal/upgrade"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.FromEnv()
	logger := log.New(os.Stdout, "", log.LstdFlags)

	if cfg.DatabaseURL == "" {
		logger.Fatal("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		if err := migrate.Apply(context.Background(), pool, cfg.MigrationsDir); err != nil {
			logger.Fatalf("auto-migrate: %v", err)
		}
		logger.Printf("auto-migrate complete (%s)", cfg.MigrationsDir)
	}

	activeCertPath := cfg.ActiveCACertPath
	activeKeyPath := cfg.ActiveCAKeyPath
	if activeCertPath == "" {
		activeCertPath = cfg.CACertPath
	}
	if activeKeyPath == "" {
		activeKeyPath = cfg.CAKeyPath
	}

	clientCAPath := cfg.CABundlePath
	if clientCAPath == "" {
		clientCAPath = cfg.TLSClientCA
	}
	if clientCAPath == "" {
		clientCAPath = cfg.CACertPath
	}

	certManager := certs.NewManager()
	state, err := certManager.Load(activeCertPath, activeKeyPath, clientCAPath)
	if err != nil {
		logger.Fatalf("cert manager load: %v", err)
	}
	if state.ActiveCert != nil && state.Signer == nil {
		logger.Printf("active CA cert set but ACTIVE_CA_KEY_PATH missing; signer disabled")
	}

	var objStore httpapi.ObjectStore
	if cfg.S3Endpoint != "" && cfg.S3AccessKey != "" && cfg.S3SecretKey != "" {
		store, err := objectstore.NewMinIO(objectstore.MinIOConfig{
			Endpoint:  cfg.S3Endpoint,
			AccessKey: cfg.S3AccessKey,
			SecretKey: cfg.S3SecretKey,
			UseSSL:    cfg.S3UseSSL,
			Region:    cfg.S3Region,
		})
		if err != nil {
			logger.Fatalf("object store init: %v", err)
		}
		objStore = store
	}

	hub := events.NewHub(128)
	store := postgres.New(pool)
	var metricsCollector *metrics.Metrics
	if cfg.MetricsEnabled {
		metricsCollector = metrics.New()
	}
	var licenseManager *license.Manager
	if cfg.LicenseEnforce {
		keyMode := strings.ToLower(strings.TrimSpace(cfg.LicenseKeyMode))
		inlineKey := cfg.LicensePublicKey
		keyPath := cfg.LicensePublicKeyPath
		if keyMode == "embedded" || keyMode == "locked" {
			inlineKey = license.EmbeddedPublicKey
			keyPath = ""
			if strings.TrimSpace(inlineKey) == "" {
				logger.Fatal("license key mode is embedded but EmbeddedPublicKey is not set at build time")
			}
		}
		manager, err := license.NewManager(cfg.LicensePath, inlineKey, keyPath, cfg.LicenseEnforce, cfg.LicenseCacheTTL)
		if err != nil {
			logger.Fatalf("license init: %v", err)
		}
		licenseManager = manager
	}
	upgradeLogDir := cfg.UpgradeLogDir
	if upgradeLogDir == "" {
		upgradeLogDir = cfg.LogDir
	}
	var upgradeRunner *upgrade.Runner
	if strings.EqualFold(cfg.UpgradeRunnerMode, "docker") {
		upgradeRunner = upgrade.NewRunner(cfg.UpgradeApplyCmd, cfg.UpgradeWorkDir, upgradeLogDir, logger.Printf)
	} else if cfg.UpgradeRunnerMode != "" {
		logger.Printf("upgrade runner disabled: mode %q not supported (docker only)", cfg.UpgradeRunnerMode)
	}
	if upgradeRunner != nil {
		env := map[string]string{
			"STACK_DIR":           "/stack",
			"UPGRADE_UPDATES_DIR": "/stack/updates",
		}
		if baseURL := os.Getenv("PUBLIC_BASE_URL"); baseURL != "" {
			env["PUBLIC_BASE_URL"] = baseURL
		}
		if cfg.MaintenanceToken != "" {
			env["MAINTENANCE_TOKEN"] = cfg.MaintenanceToken
		}
		if pull := os.Getenv("PULL_IMAGES"); pull != "" {
			env["PULL_IMAGES"] = pull
		}
		if project := os.Getenv("PROJECT_NAME"); project != "" {
			env["PROJECT_NAME"] = project
		}
		if certsDir := os.Getenv("CERTS_DIR"); certsDir != "" {
			env["CERTS_DIR"] = "/certs"
			env["CA_CERT_PATH"] = "/certs/ca.crt"
			upgradeRunner.ConfigureDocker(cfg.UpgradeRunnerImage, certsDir, env)
		} else {
			upgradeRunner.ConfigureDocker(cfg.UpgradeRunnerImage, "", env)
		}
	}

	backupLogDir := cfg.BackupLogDir
	if backupLogDir == "" {
		backupLogDir = cfg.LogDir
	}
	var backupRunner *backup.Runner
	if strings.EqualFold(cfg.BackupRunnerMode, "docker") || strings.EqualFold(cfg.BackupRunnerMode, "local") {
		backupRunner = backup.NewRunner("backup", cfg.BackupCmd, cfg.BackupWorkDir, backupLogDir, logger.Printf)
	} else if cfg.BackupRunnerMode != "" && !strings.EqualFold(cfg.BackupRunnerMode, "disabled") {
		logger.Printf("backup runner disabled: mode %q not supported (docker/local only)", cfg.BackupRunnerMode)
	}
	if backupRunner != nil && strings.EqualFold(cfg.BackupRunnerMode, "docker") {
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
		backupRunner.ConfigureDocker(cfg.BackupRunnerImage, certsDir, env)
	}

	var restoreRunner *backup.Runner
	if strings.EqualFold(cfg.BackupRunnerMode, "docker") || strings.EqualFold(cfg.BackupRunnerMode, "local") {
		restoreRunner = backup.NewRunner("restore", cfg.RestoreCmd, cfg.BackupWorkDir, backupLogDir, logger.Printf)
	}
	if restoreRunner != nil && strings.EqualFold(cfg.BackupRunnerMode, "docker") {
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
		restoreRunner.ConfigureDocker(cfg.BackupRunnerImage, certsDir, env)
	}
	var authManager *auth.Manager
	if cfg.AuthMode != "" && cfg.AuthMode != "disabled" {
		manager, err := auth.NewManager(cfg.AuthMode, cfg.AuthJWTSecret, cfg.AuthTokenTTL, cfg.AuthIssuer, store)
		if err != nil {
			logger.Fatalf("auth init: %v", err)
		}
		authManager = manager
		if cfg.AuthMode == auth.ModeLocal {
			if created, err := auth.EnsureBootstrapAdmin(store, cfg.AuthBootstrapEmail, cfg.AuthBootstrapPassword); err != nil {
				logger.Printf("auth bootstrap error: %v", err)
			} else if created {
				logger.Printf("bootstrap admin created: %s", cfg.AuthBootstrapEmail)
			}
		}
	}
	deps := httpapi.Dependencies{
		Store:            store,
		Signer:           certManager,
		ObjectStore:      objStore,
		S3Bucket:         cfg.S3Bucket,
		PresignExpires:   cfg.PresignTTL,
		TrustProxy:       cfg.TrustProxy,
		ClientCertHeader: cfg.ClientCertHeader,
		RateLimits: httpapi.RateLimitConfig{
			EnrollmentTokenRPM: cfg.EnrollmentTokenRPM,
			EnrollRPM:          cfg.EnrollRPM,
			CheckinRPM:         cfg.CheckinRPM,
			ApplyResultRPM:     cfg.ApplyResultRPM,
		},
		LogDir:             cfg.LogDir,
		Events:             hub,
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		Maintenance:        httpapi.NewMaintenanceState(cfg.MaintenanceEnabled, cfg.MaintenanceMessage),
		MaintenanceToken:   cfg.MaintenanceToken,
		Upgrade:            upgradeRunner,
		UpgradeUpdatesDir:  cfg.UpgradeUpdatesDir,
		Backup:             backupRunner,
		Restore:            restoreRunner,
		BackupDir:          cfg.BackupDir,
		Metrics:            metricsCollector,
		MetricsPath:        cfg.MetricsPath,
		Auth:               authManager,
		License:            licenseManager,
		CertManager:        certManager,
		CertRotationGrace:  cfg.CertRotationGracePeriod,
	}

	updateMetricsCounts := func() {
		if metricsCollector == nil {
			return
		}
		total, err := store.CountDevices()
		if err != nil {
			logger.Printf("metrics devices total error: %v", err)
			return
		}
		counts := map[string]int{}
		for _, status := range []string{"active", "stale", "offline", "degraded"} {
			if count, err := store.CountDevicesByStatus(status); err == nil {
				counts[status] = count
			} else {
				logger.Printf("metrics devices status=%s error: %v", status, err)
			}
		}
		metricsCollector.SetDeviceStatusCounts(total, counts)
		if pool != nil {
			stats := pool.Stat()
			metricsCollector.SetDBStats(int(stats.TotalConns()), int(stats.AcquiredConns()), stats.EmptyAcquireCount())
		}
		if stats, err := store.GetArtifactStats(); err == nil {
			metricsCollector.SetStorageUsage(stats.Count, stats.SizeBytes)
		} else if err != nil {
			logger.Printf("metrics artifact stats error: %v", err)
		}
	}
	updateMetricsCounts()
	if metricsCollector != nil && cfg.MetricsRefreshInterval > 0 {
		interval := cfg.MetricsRefreshInterval
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				<-ticker.C
				updateMetricsCounts()
			}
		}()
	}

	if err := store.EnsureAuditRetentionDays(cfg.AuditRetentionDays); err != nil {
		logger.Printf("audit retention init error: %v", err)
	}

	if cfg.AuditRetentionCleanupInterval > 0 {
		interval := cfg.AuditRetentionCleanupInterval
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				retention, err := store.GetAuditRetentionDays()
				if err != nil {
					logger.Printf("audit retention fetch error: %v", err)
				} else {
					days := retention.Days
					if days <= 0 {
						days = cfg.AuditRetentionDays
					}
					if days <= 0 {
						days = 90
					}
					cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
					if count, err := store.DeleteAuditEventsBefore(cutoff); err != nil {
						logger.Printf("delete audit events error: %v", err)
					} else if count > 0 {
						logger.Printf("deleted audit events count=%d cutoff=%s", count, cutoff.Format(time.RFC3339))
					}
				}
				<-ticker.C
			}
		}()
	}

	if cfg.LogIngestAddr != "" {
		store := logging.NewStore(cfg.LogDir)
		if _, err := logging.StartIngest(context.Background(), cfg.LogIngestAddr, store, logger.Printf); err != nil {
			logger.Fatalf("log ingest: %v", err)
		}
	}

	if cfg.DeviceStaleTTL > 0 {
		interval := cfg.DeviceCleanupInterval
		if interval <= 0 {
			interval = 5 * time.Minute
		}
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				<-ticker.C
				cutoff := time.Now().UTC().Add(-cfg.DeviceStaleTTL)
				count, err := deps.Store.DeleteStaleDevices(cutoff)
				if err != nil {
					logger.Printf("delete stale devices error: %v", err)
					continue
				}
				if count > 0 {
					logger.Printf("deleted stale devices count=%d cutoff=%s", count, cutoff.Format(time.RFC3339))
				}
			}
		}()
	}

	if cfg.DeviceStatusStaleAfter > 0 && cfg.DeviceStatusOfflineAfter > 0 {
		interval := cfg.DeviceStatusRefreshInterval
		if interval <= 0 {
			interval = 30 * time.Second
		}
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				cutoffStale := time.Now().UTC().Add(-cfg.DeviceStatusStaleAfter)
				cutoffOffline := time.Now().UTC().Add(-cfg.DeviceStatusOfflineAfter)
				if count, err := deps.Store.UpdateDeviceStatuses(cutoffStale, cutoffOffline); err != nil {
					logger.Printf("update device status error: %v", err)
				} else if count > 0 {
					logger.Printf("updated device statuses count=%d", count)
				}
				<-ticker.C
			}
		}()
	}

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: httpapi.NewRouter(logger, deps),
	}

	if cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		certPair, err := tls.LoadX509KeyPair(cfg.TLSCertPath, cfg.TLSKeyPath)
		if err != nil {
			logger.Fatalf("load TLS cert/key error: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certPair}

		if clientCAPath == "" {
			logger.Fatal("TLS enabled but TLS_CLIENT_CA_PATH or CA_CERT_PATH not set")
		}
		pool := certManager.ClientPool()
		if pool == nil {
			logger.Fatal("invalid client CA cert")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		tlsConfig.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
			cfg := tlsConfig.Clone()
			cfg.GetConfigForClient = nil
			if nextPool := certManager.ClientPool(); nextPool != nil {
				cfg.ClientCAs = nextPool
			}
			return cfg, nil
		}

		srv.TLSConfig = tlsConfig
		if cfg.DisableHTTP2 {
			srv.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){}
		}
		logger.Printf("control-plane listening on https://%s", cfg.HTTPAddr)
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server error: %v", err)
		}
		return
	}

	logger.Printf("control-plane listening on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server error: %v", err)
	}
}
