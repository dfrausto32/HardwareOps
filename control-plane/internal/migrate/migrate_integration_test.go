package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyMigrations(t *testing.T) {
	baseURL := os.Getenv("TEST_DATABASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("DATABASE_URL")
	}
	if baseURL == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL not set")
	}

	schema := "ho_mig_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	url := baseURL
	if strings.Contains(baseURL, "?") {
		url += "&options=-csearch_path%3D" + schema
	} else {
		url += "?options=-csearch_path%3D" + schema
	}

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")

	migrationsPath := filepath.Join("..", "..", "..", "migrations")
	if err := Apply(ctx, pool, migrationsPath); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("schema_migrations check: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected schema_migrations to have entries")
	}
}
