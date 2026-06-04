package globalplane

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the global-plane.
type Config struct {
	// Database
	DatabaseURL string

	// HTTP
	HTTPAddr string

	// Auth — reuses the same JWT secret pattern as the control-plane.
	JWTSecret string

	// Token encryption key (32 bytes, base64-encoded) for encrypting regional plane
	// service tokens at rest. Generate with: openssl rand -base64 32
	TokenEncryptionKey []byte

	// Migrations
	MigrationsDir string

	// CORS
	CORSAllowedOrigins string

	// Sync
	DefaultSyncInterval time.Duration

	// TLS
	EnableTLS   bool
	TLSCertPath string
	TLSKeyPath  string

	// MinIO (global artifact storage)
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOUseTLS    bool
	PresignExpires time.Duration

	// PublicBaseURL is the externally-reachable base URL of this global-plane,
	// sent to regional planes as the globalPresignBaseUrl in federation metadata.
	PublicBaseURL string

	// OIDC SSO (same env vars as the regional control-plane)
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
	OIDCScopes       string
	OIDCGroupClaim   string
	OIDCRoleMap      string
	OIDCDefaultRole  string

	// PostLoginURL is where the OIDC callback redirects the browser after a
	// successful SSO login, with ?global_oidc_token=<jwt> appended.
	// Typically the regional UI origin, e.g. https://regional.example.com.
	PostLoginURL string
}

// FromEnv reads Config from environment variables, applying defaults.
func FromEnv() (Config, error) {
	cfg := Config{
		DatabaseURL:         os.Getenv("GLOBAL_DATABASE_URL"),
		HTTPAddr:            envOr("GLOBAL_HTTP_ADDR", ":8090"),
		JWTSecret:           os.Getenv("AUTH_JWT_SECRET"),
		MigrationsDir:       envOr("GLOBAL_MIGRATIONS_DIR", "migrations/global"),
		CORSAllowedOrigins:  os.Getenv("CORS_ALLOWED_ORIGINS"),
		EnableTLS:           os.Getenv("ENABLE_TLS") == "1",
		TLSCertPath:         os.Getenv("TLS_CERT_PATH"),
		TLSKeyPath:          os.Getenv("TLS_KEY_PATH"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("GLOBAL_DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("AUTH_JWT_SECRET is required")
	}

	rawKey := os.Getenv("GLOBAL_TOKEN_ENCRYPTION_KEY")
	if rawKey == "" {
		return Config{}, errors.New("GLOBAL_TOKEN_ENCRYPTION_KEY is required (base64-encoded 32-byte AES key)")
	}
	key, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil {
		return Config{}, fmt.Errorf("GLOBAL_TOKEN_ENCRYPTION_KEY: invalid base64: %w", err)
	}
	if len(key) != 32 {
		return Config{}, fmt.Errorf("GLOBAL_TOKEN_ENCRYPTION_KEY: must be 32 bytes when decoded (got %d)", len(key))
	}
	cfg.TokenEncryptionKey = key

	intervalStr := envOr("GLOBAL_SYNC_DEFAULT_INTERVAL", "60")
	secs, err := strconv.Atoi(intervalStr)
	if err != nil || secs < 10 {
		return Config{}, fmt.Errorf("GLOBAL_SYNC_DEFAULT_INTERVAL: must be an integer >= 10 (got %q)", intervalStr)
	}
	cfg.DefaultSyncInterval = time.Duration(secs) * time.Second

	cfg.OIDCIssuer = os.Getenv("AUTH_OIDC_ISSUER")
	cfg.OIDCClientID = os.Getenv("AUTH_OIDC_CLIENT_ID")
	cfg.OIDCClientSecret = os.Getenv("AUTH_OIDC_CLIENT_SECRET")
	cfg.OIDCRedirectURL = os.Getenv("AUTH_OIDC_REDIRECT_URL")
	cfg.OIDCScopes = envOr("AUTH_OIDC_SCOPES", "openid email profile groups")
	cfg.OIDCGroupClaim = envOr("AUTH_OIDC_GROUP_CLAIM", "groups")
	cfg.OIDCRoleMap = os.Getenv("AUTH_OIDC_ROLE_MAP")
	cfg.OIDCDefaultRole = envOr("AUTH_OIDC_DEFAULT_ROLE", "viewer")
	cfg.PostLoginURL = os.Getenv("GLOBAL_POST_LOGIN_URL")

	cfg.MinIOEndpoint = os.Getenv("GLOBAL_MINIO_ENDPOINT")
	cfg.MinIOAccessKey = os.Getenv("GLOBAL_MINIO_ACCESS_KEY")
	cfg.MinIOSecretKey = os.Getenv("GLOBAL_MINIO_SECRET_KEY")
	cfg.MinIOBucket = envOr("GLOBAL_MINIO_BUCKET", "global-artifacts")
	cfg.MinIOUseTLS = os.Getenv("GLOBAL_MINIO_USE_TLS") == "1"
	cfg.PublicBaseURL = os.Getenv("GLOBAL_PUBLIC_BASE_URL")

	presignSecs := envOr("GLOBAL_PRESIGN_EXPIRES_SECONDS", "3600")
	ps, _ := strconv.Atoi(presignSecs)
	if ps <= 0 {
		ps = 3600
	}
	cfg.PresignExpires = time.Duration(ps) * time.Second

	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EncryptToken encrypts a plaintext service token using AES-256-GCM.
func EncryptToken(plaintext string, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// DecryptToken decrypts a service token encrypted by EncryptToken.
func DecryptToken(ciphertext []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
