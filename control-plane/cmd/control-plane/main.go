package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"net/http"
	"os"

	"github.com/hardwareops/control-plane/internal/config"
	cpcrypto "github.com/hardwareops/control-plane/internal/crypto"
	"github.com/hardwareops/control-plane/internal/httpapi"
	"github.com/hardwareops/control-plane/internal/migrate"
	"github.com/hardwareops/control-plane/internal/objectstore"
	"github.com/hardwareops/control-plane/internal/store/postgres"
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

	var signer httpapi.CertSigner
	if cfg.CACertPath != "" && cfg.CAKeyPath != "" {
		certPEM, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			logger.Fatalf("read CA cert: %v", err)
		}
		keyPEM, err := os.ReadFile(cfg.CAKeyPath)
		if err != nil {
			logger.Fatalf("read CA key: %v", err)
		}
		ca, err := cpcrypto.LoadCA(certPEM, keyPEM)
		if err != nil {
			logger.Fatalf("load CA: %v", err)
		}
		signer = &cpcrypto.CASigner{CA: ca, CAPEM: certPEM}
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

	deps := httpapi.Dependencies{
		Store:            postgres.New(pool),
		Signer:           signer,
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
	}

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: httpapi.NewRouter(logger, deps),
	}

	if cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		clientCAPath := cfg.TLSClientCA
		if clientCAPath == "" {
			clientCAPath = cfg.CACertPath
		}
		if clientCAPath == "" {
			logger.Fatal("TLS enabled but TLS_CLIENT_CA_PATH or CA_CERT_PATH not set")
		}
		caPEM, err := os.ReadFile(clientCAPath)
		if err != nil {
			logger.Fatalf("read client CA: %v", err)
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(caPEM); !ok {
			logger.Fatal("invalid client CA cert")
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven

		srv.TLSConfig = tlsConfig
		logger.Printf("control-plane listening on https://%s", cfg.HTTPAddr)
		if err := srv.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server error: %v", err)
		}
		return
	}

	logger.Printf("control-plane listening on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server error: %v", err)
	}
}
