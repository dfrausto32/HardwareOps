package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/artifactingest"
	"github.com/hardwareops/control-plane/internal/artifacttrust"
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/backup"
	"github.com/hardwareops/control-plane/internal/certs"
	"github.com/hardwareops/control-plane/internal/config"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/httpapi"
	"github.com/hardwareops/control-plane/internal/license"
	"github.com/hardwareops/control-plane/internal/lifecycle"
	"github.com/hardwareops/control-plane/internal/logging"
	"github.com/hardwareops/control-plane/internal/metrics"
	"github.com/hardwareops/control-plane/internal/migrate"
	"github.com/hardwareops/control-plane/internal/objectstore"
	"github.com/hardwareops/control-plane/internal/releaseautoupdate"
	storepkg "github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/postgres"
	"github.com/hardwareops/control-plane/internal/upgrade"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	if handled, err := tryRunBreakglassCommand(logger); handled {
		if err != nil {
			logger.Fatal(err)
		}
		return
	}
	cfg := config.FromEnv()

	cleanupPEMFiles, err := materializePEMFiles(&cfg)
	if err != nil {
		logger.Fatalf("materialize PEM env files: %v", err)
	}
	defer cleanupPEMFiles()

	if err := config.ValidateHardening(cfg); err != nil {
		logger.Fatalf("hardened profile validation failed: %v", err)
	}

	if cfg.DatabaseURL == "" {
		logger.Fatal("DATABASE_URL is required")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Fatalf("db config parse: %v", err)
	}
	if cfg.DBMaxConns > 0 {
		poolCfg.MaxConns = int32(cfg.DBMaxConns)
	}
	if cfg.DBMinConns > 0 {
		poolCfg.MinConns = int32(cfg.DBMinConns)
	}
	if cfg.DBMaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
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
	if cfg.S3Endpoint != "" && cfg.S3Bucket != "" {
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
		authMode := "iam"
		if cfg.S3AccessKey != "" || cfg.S3SecretKey != "" {
			authMode = "static"
		}
		logger.Printf("object store initialized endpoint=%s bucket=%s auth=%s", cfg.S3Endpoint, cfg.S3Bucket, authMode)
	}

	hub := events.NewHub(128)
	store := postgres.New(pool)
	var metricsCollector *metrics.Metrics
	if cfg.MetricsEnabled {
		metricsCollector = metrics.New()
	}
	seededTrustPolicy, seededTrustKeys, err := artifacttrust.SeedBootstrapState(context.Background(), store, artifacttrust.BootstrapConfig{
		StaticFile:  cfg.TrustedSigningKeysFile,
		StaticJSON:  cfg.TrustedSigningKeysJSON,
		AWSSecretID: cfg.TrustedSigningKeysAWSSecretID,
		AWSRegion:   cfg.TrustedSigningKeysAWSRegion,
		DefaultPolicy: artifacttrust.DefaultPolicyConfig{
			HardenedProfile:         cfg.HardenedProfile,
			VerificationMode:        cfg.ArtifactTrustVerificationMode,
			AllowedSigningKeyIDs:    cfg.ArtifactTrustAllowedSigningKeyIDs,
			AllowedSignatureTypes:   cfg.ArtifactTrustAllowedSignatureTypes,
			RequireSignatureDefault: cfg.ArtifactSignatureRequireDefault,
			EnforceIngest:           cfg.ArtifactSignatureEnforceIngest,
			RequiredKeyID:           cfg.ArtifactSignatureKeyID,
		},
	})
	if err != nil {
		logger.Fatalf("artifact trust bootstrap: %v", err)
	}
	logger.Printf(
		"artifact trust initialized mode=%s trusted_keys=%d static=%t aws_secret=%t",
		seededTrustPolicy.VerificationMode,
		len(seededTrustKeys),
		strings.TrimSpace(cfg.TrustedSigningKeysFile) != "" || strings.TrimSpace(cfg.TrustedSigningKeysJSON) != "",
		strings.TrimSpace(cfg.TrustedSigningKeysAWSSecretID) != "",
	)
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
	if strings.EqualFold(cfg.UpgradeRunnerMode, "docker") || strings.EqualFold(cfg.UpgradeRunnerMode, "remote") {
		upgradeRunner = upgrade.NewRunner(cfg.UpgradeApplyCmd, cfg.UpgradeWorkDir, upgradeLogDir, logger.Printf)
	} else if cfg.UpgradeRunnerMode != "" && !strings.EqualFold(cfg.UpgradeRunnerMode, "disabled") {
		logger.Printf("upgrade runner disabled: mode %q not supported (docker/remote only)", cfg.UpgradeRunnerMode)
	}
	if upgradeRunner != nil {
		if strings.EqualFold(cfg.UpgradeRunnerMode, "remote") {
			upgradeRunner.ConfigureRemote(cfg.UpgradeRunnerURL, cfg.UpgradeRunnerToken)
		} else {
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
	}

	backupLogDir := cfg.BackupLogDir
	if backupLogDir == "" {
		backupLogDir = cfg.LogDir
	}
	var backupRunner *backup.Runner
	if strings.EqualFold(cfg.BackupRunnerMode, "docker") || strings.EqualFold(cfg.BackupRunnerMode, "local") || strings.EqualFold(cfg.BackupRunnerMode, "remote") {
		backupRunner = backup.NewRunner("backup", cfg.BackupCmd, cfg.BackupWorkDir, backupLogDir, logger.Printf)
	} else if cfg.BackupRunnerMode != "" && !strings.EqualFold(cfg.BackupRunnerMode, "disabled") {
		logger.Printf("backup runner disabled: mode %q not supported (docker/local/remote only)", cfg.BackupRunnerMode)
	}
	if backupRunner != nil && strings.EqualFold(cfg.BackupRunnerMode, "remote") {
		backupRunner.ConfigureRemote(cfg.BackupRunnerURL, cfg.BackupRunnerToken)
	} else if backupRunner != nil && strings.EqualFold(cfg.BackupRunnerMode, "docker") {
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
	if strings.EqualFold(cfg.BackupRunnerMode, "docker") || strings.EqualFold(cfg.BackupRunnerMode, "local") || strings.EqualFold(cfg.BackupRunnerMode, "remote") {
		restoreRunner = backup.NewRunner("restore", cfg.RestoreCmd, cfg.BackupWorkDir, backupLogDir, logger.Printf)
	}
	if restoreRunner != nil && strings.EqualFold(cfg.BackupRunnerMode, "remote") {
		restoreRunner.ConfigureRemote(cfg.BackupRunnerURL, cfg.BackupRunnerToken)
	} else if restoreRunner != nil && strings.EqualFold(cfg.BackupRunnerMode, "docker") {
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
	var authLoginBackoff *auth.LoginBackoff
	var oidcProvider *auth.OIDCProvider
	var workloadIdentityManager *auth.WorkloadIdentityManager
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
		authLoginBackoff = auth.NewLoginBackoff(auth.LoginBackoffConfig{
			Enabled:   cfg.AuthLoginBackoffEnabled,
			Threshold: cfg.AuthLoginBackoffThreshold,
			BaseDelay: cfg.AuthLoginBackoffBase,
			MaxDelay:  cfg.AuthLoginBackoffMax,
			Window:    cfg.AuthLoginBackoffWindow,
		})
	}
	if cfg.OIDCEnabled() {
		if strings.TrimSpace(cfg.AuthOIDCClientSecret) == "" {
			logger.Fatal("AUTH_OIDC_CLIENT_SECRET is required when AUTH_OIDC_ISSUER is set")
		}
		if strings.TrimSpace(cfg.AuthOIDCRedirectURL) == "" {
			logger.Fatal("AUTH_OIDC_REDIRECT_URL is required when AUTH_OIDC_ISSUER is set")
		}
		p, err := auth.NewOIDCProvider(context.Background(), &cfg, store, authManager)
		if err != nil {
			logger.Fatalf("oidc provider init: %v", err)
		}
		oidcProvider = p
		logger.Printf("oidc provider initialized issuer=%s", cfg.AuthOIDCIssuer)
	}
	if cfg.WorkloadIdentityEnabled() {
		if authManager == nil || !authManager.Enabled() {
			logger.Fatal("workload identity requires AUTH_MODE to be enabled")
		}
		manager, err := auth.NewWorkloadIdentityManager(context.Background(), &cfg)
		if err != nil {
			logger.Fatalf("workload identity init: %v", err)
		}
		workloadIdentityManager = manager
		logger.Printf("workload identity initialized providers=%s", strings.Join(manager.ProviderNames(), ","))
	}
	artifactLifecycleManager := lifecycle.NewManager(lifecycle.ManagerConfig{
		Enabled:                 cfg.ArtifactPruneEnabled,
		Interval:                cfg.ArtifactPruneInterval,
		BatchLimit:              cfg.ArtifactPruneBatchLimit,
		AlertReferenceThreshold: cfg.ArtifactPruneAlertRefThreshold,
		Store:                   store,
		ObjectStore:             objStore,
		Bucket:                  cfg.S3Bucket,
		Metrics:                 metricsCollector,
		Logger:                  logger.Printf,
	})
	if artifactLifecycleManager != nil {
		artifactLifecycleManager.Start(context.Background())
	}
	releaseAutoUpdateManager := releaseautoupdate.New(releaseautoupdate.Config{
		Store:    store,
		Interval: cfg.ReleaseAutoUpdateInterval,
		Logger:   logger.Printf,
	})
	if releaseAutoUpdateManager != nil {
		releaseAutoUpdateManager.Start(context.Background())
	}
	pullCredentialManager, err := artifactingest.NewPullCredentialManager(
		cfg.ArtifactPullCredentialsFile,
		cfg.ArtifactPullCredentialsJSON,
		cfg.ArtifactPullCredentialsAWSSecretID,
		cfg.ArtifactPullCredentialsAWSRegion,
	)
	if err != nil {
		logger.Fatalf("artifact pull credentials init: %v", err)
	}
	pullCredentialStatus := pullCredentialManager.Status()
	if pullCredentialStatus.Configured {
		logger.Printf(
			"artifact pull credentials loaded refs=%d staticRefs=%d awsRefs=%d",
			pullCredentialStatus.CredentialRefCount,
			pullCredentialStatus.StaticCredentialRefs,
			pullCredentialStatus.AWSCredentialRefs,
		)
	}
	deps := httpapi.Dependencies{
		Store:             store,
		Signer:            certManager,
		ObjectStore:       objStore,
		S3Bucket:          cfg.S3Bucket,
		PresignExpires:    cfg.PresignTTL,
		TrustProxy:        cfg.TrustProxy,
		TrustedProxyCIDRs: cfg.TrustedProxyCIDRs,
		ClientCertHeader:  cfg.ClientCertHeader,
		RateLimits: httpapi.RateLimitConfig{
			EnrollmentTokenRPM: cfg.EnrollmentTokenRPM,
			EnrollRPM:          cfg.EnrollRPM,
			CheckinRPM:         cfg.CheckinRPM,
			ApplyResultRPM:     cfg.ApplyResultRPM,
			AuthLoginRPM:       cfg.AuthLoginRPM,
		},
		PendingEnrollmentGuardrails: httpapi.PendingEnrollmentGuardrailConfig{
			RequestRPMPerSource:  cfg.PendingEnrollRequestRPMPerSource,
			RequestRPMPerProfile: cfg.PendingEnrollRequestRPMPerProfile,
			MaxActive:            cfg.PendingEnrollMaxActive,
			MaxActivePerProfile:  cfg.PendingEnrollMaxActivePerProfile,
			MaxActivePerSource:   cfg.PendingEnrollMaxActivePerSource,
		},
		LogDir:                          cfg.LogDir,
		Events:                          hub,
		CORSAllowedOrigins:              cfg.CORSAllowedOrigins,
		Maintenance:                     httpapi.NewMaintenanceState(cfg.MaintenanceEnabled, cfg.MaintenanceMessage),
		Upgrade:                         upgradeRunner,
		UpgradeUpdatesDir:               cfg.UpgradeUpdatesDir,
		Backup:                          backupRunner,
		Restore:                         restoreRunner,
		BackupDir:                       cfg.BackupDir,
		Metrics:                         metricsCollector,
		MetricsPath:                     cfg.MetricsPath,
		Auth:                            authManager,
		AuthLoginBackoff:                authLoginBackoff,
		OIDCProvider:                    oidcProvider,
		WorkloadIdentity:                workloadIdentityManager,
		BootstrapToken:                  cfg.BootstrapToken,
		License:                         licenseManager,
		CertManager:                     certManager,
		CertRotationGrace:               cfg.CertRotationGracePeriod,
		ArtifactLifecycle:               artifactLifecycleManager,
		ArtifactPullHosts:               cfg.ArtifactPullAllowedHosts,
		ArtifactPullMaxBytes:            cfg.ArtifactPullMaxBytes,
		ArtifactPullTimeout:             cfg.ArtifactPullTimeout,
		ArtifactPullAllowInsecureHTTP:   cfg.ArtifactPullAllowInsecureHTTP,
		ArtifactTrustVerificationMode:   cfg.ArtifactTrustVerificationMode,
		ArtifactTrustAllowedKeyIDs:      cfg.ArtifactTrustAllowedSigningKeyIDs,
		ArtifactTrustAllowedSigTypes:    cfg.ArtifactTrustAllowedSignatureTypes,
		ArtifactSignatureRequireDefault: cfg.ArtifactSignatureRequireDefault,
		ArtifactSignatureEnforceIngest:  cfg.ArtifactSignatureEnforceIngest,
		ArtifactSignatureKeyID:          cfg.ArtifactSignatureKeyID,
		HardenedProfile:                 cfg.HardenedProfile,
		ArtifactPullCreds:               pullCredentialManager,
		ArtifactPullCredsManager:        pullCredentialManager,
		ReleaseAutoUpdate:               releaseAutoUpdateManager,
		DeviceIdentityMode:              cfg.DeviceIdentityMode,
		DeviceIdentityRequireOnEnroll:   cfg.DeviceIdentityRequireOnEnroll,
		DeviceIdentityRequireOnCheckin:  cfg.DeviceIdentityRequireOnCheckin,
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
		for _, status := range []string{
			artifacttrust.VerificationStatusUnsigned,
			artifacttrust.VerificationStatusLegacy,
			artifacttrust.VerificationStatusVerified,
			artifacttrust.VerificationStatusFailed,
			artifacttrust.VerificationStatusUntrusted,
		} {
			if count, err := store.CountArtifactsByVerificationStatus(status); err == nil {
				metricsCollector.SetArtifactVerificationState(status, count)
			} else {
				logger.Printf("metrics artifact verification status=%s error: %v", status, err)
			}
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

	if err := store.EnsureRuntimeEventRetentionDays(cfg.RuntimeEventRetentionDays); err != nil {
		logger.Printf("runtime event retention init error: %v", err)
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

	if cfg.RuntimeEventRetentionCleanupInterval > 0 {
		interval := cfg.RuntimeEventRetentionCleanupInterval
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				retention, err := store.GetRuntimeEventRetentionDays()
				if err != nil {
					logger.Printf("runtime event retention fetch error: %v", err)
				} else {
					days := retention.Days
					if days <= 0 {
						days = cfg.RuntimeEventRetentionDays
					}
					if days <= 0 {
						days = 30
					}
					cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
					if count, err := store.DeleteRuntimeEventsBefore(cutoff); err != nil {
						logger.Printf("delete runtime events error: %v", err)
					} else if count > 0 {
						logger.Printf("deleted runtime events count=%d cutoff=%s", count, cutoff.Format(time.RFC3339))
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

func tryRunBreakglassCommand(logger *log.Logger) (bool, error) {
	if len(os.Args) < 2 || os.Args[1] != "auth" {
		return false, nil
	}
	if len(os.Args) < 3 || os.Args[2] != "breakglass" {
		return true, fmt.Errorf("unknown auth command; expected: auth breakglass <reset-password|create-admin>")
	}
	if len(os.Args) < 4 {
		return true, fmt.Errorf("missing breakglass subcommand")
	}
	cfg := config.FromEnv()
	if cfg.DatabaseURL == "" {
		return true, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.AuthMode != "" && cfg.AuthMode != auth.ModeLocal {
		return true, fmt.Errorf("breakglass commands require AUTH_MODE=%s", auth.ModeLocal)
	}
	switch os.Args[3] {
	case "reset-password":
		return true, runBreakglassResetPassword(logger, cfg.DatabaseURL, os.Args[4:])
	case "create-admin":
		return true, runBreakglassCreateAdmin(logger, cfg.DatabaseURL, os.Args[4:])
	default:
		return true, fmt.Errorf("unknown breakglass subcommand: %s", os.Args[3])
	}
}

func runBreakglassResetPassword(logger *log.Logger, databaseURL string, args []string) error {
	fs := flag.NewFlagSet("breakglass reset-password", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	email := fs.String("email", "", "target local user email")
	password := fs.String("password", "", "temporary password to set (optional; generated if empty)")
	reason := fs.String("reason", "", "required audit reason")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*email) == "" {
		return fmt.Errorf("--email is required")
	}
	if strings.TrimSpace(*reason) == "" {
		return fmt.Errorf("--reason is required")
	}
	if strings.TrimSpace(*password) == "" {
		generated, err := auth.GenerateBreakglassPassword()
		if err != nil {
			return err
		}
		*password = generated
	}
	pool, st, err := openPostgresStore(databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	user, err := auth.BreakglassResetPassword(st, *email, *password)
	event := newBreakglassAuditEvent("auth.breakglass.password_reset", "user", breakglassTargetID(user, *email), *reason)
	if err != nil {
		event.Status = "error"
		event.Error = err.Error()
		_ = st.CreateAuditEvent(event)
		return err
	}
	event.Status = "success"
	event.AfterJSON = auditJSON(map[string]any{
		"email": user.Email,
	})
	if auditErr := st.CreateAuditEvent(event); auditErr != nil {
		return fmt.Errorf("password reset succeeded but audit failed: %w", auditErr)
	}
	logger.Printf("breakglass password reset complete user=%s email=%s", user.UserID, user.Email)
	fmt.Printf("email: %s\n", user.Email)
	fmt.Printf("temporary_password: %s\n", *password)
	return nil
}

func runBreakglassCreateAdmin(logger *log.Logger, databaseURL string, args []string) error {
	fs := flag.NewFlagSet("breakglass create-admin", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	email := fs.String("email", "", "new local admin email")
	displayName := fs.String("display-name", "Recovery Admin", "display name")
	password := fs.String("password", "", "temporary password to set (optional; generated if empty)")
	reason := fs.String("reason", "", "required audit reason")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*email) == "" {
		return fmt.Errorf("--email is required")
	}
	if strings.TrimSpace(*reason) == "" {
		return fmt.Errorf("--reason is required")
	}
	if strings.TrimSpace(*password) == "" {
		generated, err := auth.GenerateBreakglassPassword()
		if err != nil {
			return err
		}
		*password = generated
	}
	pool, st, err := openPostgresStore(databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	user, err := auth.BreakglassCreateAdmin(st, *email, *displayName, *password)
	event := newBreakglassAuditEvent("auth.breakglass.admin_create", "user", breakglassTargetID(user, *email), *reason)
	if err != nil {
		event.Status = "error"
		event.Error = err.Error()
		_ = st.CreateAuditEvent(event)
		return err
	}
	event.Status = "success"
	event.AfterJSON = auditJSON(map[string]any{
		"email":       user.Email,
		"displayName": user.DisplayName,
		"roles":       []string{"admin"},
	})
	if auditErr := st.CreateAuditEvent(event); auditErr != nil {
		return fmt.Errorf("admin create succeeded but audit failed: %w", auditErr)
	}
	logger.Printf("breakglass admin create complete user=%s email=%s", user.UserID, user.Email)
	fmt.Printf("email: %s\n", user.Email)
	fmt.Printf("temporary_password: %s\n", *password)
	return nil
}

func openPostgresStore(databaseURL string) (*pgxpool.Pool, storepkg.Store, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("db connect: %w", err)
	}
	return pool, postgres.New(pool), nil
}

func newBreakglassAuditEvent(action, targetType, targetID, reason string) storepkg.AuditEvent {
	host, _ := os.Hostname()
	metadata := map[string]any{
		"reason":  reason,
		"host":    host,
		"command": action,
	}
	metadataJSON, _ := json.Marshal(metadata)
	return storepkg.AuditEvent{
		EventID:      fmt.Sprintf("breakglass-%d", time.Now().UTC().UnixNano()),
		OccurredAt:   time.Now().UTC(),
		ActorType:    "breakglass",
		ActorID:      strings.TrimSpace(os.Getenv("USER")),
		AuthMethod:   "local_cli",
		SourceIP:     "local",
		UserAgent:    "hardwareops-control-plane-cli",
		Action:       action,
		TargetType:   targetType,
		TargetID:     targetID,
		MetadataJSON: metadataJSON,
	}
}

func breakglassTargetID(user storepkg.User, fallback string) string {
	if user.UserID != "" {
		return user.UserID
	}
	return strings.TrimSpace(strings.ToLower(fallback))
}

func auditJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

func materializePEMFiles(cfg *config.Config) (func(), error) {
	if cfg == nil {
		return func() {}, nil
	}

	type pemTarget struct {
		path *string
		env  string
		name string
		mode os.FileMode
	}

	targets := []pemTarget{
		{path: &cfg.CACertPath, env: "CA_CERT_PEM", name: "ca.crt", mode: 0o644},
		{path: &cfg.CAKeyPath, env: "CA_KEY_PEM", name: "ca.key", mode: 0o600},
		{path: &cfg.ActiveCACertPath, env: "ACTIVE_CA_CERT_PEM", name: "active-ca.crt", mode: 0o644},
		{path: &cfg.ActiveCAKeyPath, env: "ACTIVE_CA_KEY_PEM", name: "active-ca.key", mode: 0o600},
		{path: &cfg.CABundlePath, env: "CA_BUNDLE_PEM", name: "ca-bundle.crt", mode: 0o644},
		{path: &cfg.TLSClientCA, env: "TLS_CLIENT_CA_PEM", name: "tls-client-ca.crt", mode: 0o644},
	}

	needsTempDir := false
	for _, target := range targets {
		if strings.TrimSpace(*target.path) == "" && strings.TrimSpace(os.Getenv(target.env)) != "" {
			needsTempDir = true
			break
		}
	}
	if !needsTempDir {
		return func() {}, nil
	}

	dir, err := os.MkdirTemp("", "hardwareops-control-plane-pems-*")
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	for _, target := range targets {
		if strings.TrimSpace(*target.path) != "" {
			continue
		}
		value := strings.TrimSpace(os.Getenv(target.env))
		if value == "" {
			continue
		}
		path := filepath.Join(dir, target.name)
		if err := os.WriteFile(path, []byte(value), target.mode); err != nil {
			cleanup()
			return nil, fmt.Errorf("write %s from %s: %w", target.name, target.env, err)
		}
		*target.path = path
	}

	return cleanup, nil
}
