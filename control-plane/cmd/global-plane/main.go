package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/globalplane"
	globalhttp "github.com/hardwareops/control-plane/internal/globalplane/httpapi"
	"github.com/hardwareops/control-plane/internal/globalplane/sync"
	"github.com/hardwareops/control-plane/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg, err := globalplane.FromEnv()
	if err != nil {
		logger.Fatalf("config: %v", err)
	}

	// Connect to DB.
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	// Run migrations.
	if err := migrate.Apply(context.Background(), pool, cfg.MigrationsDir); err != nil {
		logger.Fatalf("migrate: %v", err)
	}
	logger.Printf("migrations complete (%s)", cfg.MigrationsDir)

	// Store.
	store := globalplane.NewPostgresStore(pool)

	// Auth manager (reuses the same JWT/service-token logic as the control-plane).
	authMode := "local"
	if cfg.JWTSecret == "" {
		authMode = "disabled"
	}
	authMgr, err := auth.NewManager(authMode, cfg.JWTSecret, 12*time.Hour, "hardwareops-global", store)
	if err != nil {
		logger.Fatalf("auth manager: %v", err)
	}

	// Sync manager.
	syncMgr := sync.NewManager(store, cfg.TokenEncryptionKey, logger)

	// Build dependencies.
	deps := globalhttp.Dependencies{
		Store:              store,
		SyncManager:        syncMgr,
		Auth:               authMgr,
		TokenEncryptionKey: cfg.TokenEncryptionKey,
		CORSAllowedOrigins: splitOrigins(cfg.CORSAllowedOrigins),
	}

	// HTTP server.
	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: globalhttp.NewRouter(logger, deps),
	}

	// Start sync manager in background.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := syncMgr.Start(ctx); err != nil {
			logger.Printf("sync manager error: %v", err)
		}
	}()

	// TLS.
	if cfg.EnableTLS && cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertPath, cfg.TLSKeyPath)
		if err != nil {
			logger.Fatalf("load TLS cert/key: %v", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
		srv.TLSConfig = tlsCfg

		// Graceful shutdown on signal.
		go listenForShutdown(logger, srv, cancel)

		logger.Printf("global-plane listening on https://%s", cfg.HTTPAddr)
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server error: %v", err)
		}
		return
	}

	go listenForShutdown(logger, srv, cancel)

	logger.Printf("global-plane listening on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server error: %v", err)
	}
}

func listenForShutdown(logger *log.Logger, srv *http.Server, cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	<-c
	logger.Println("shutting down global-plane...")
	cancel()
	_ = srv.Shutdown(context.Background())
}

func splitOrigins(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
