package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/migrate"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStore_Integration(t *testing.T) {
	baseURL := os.Getenv("TEST_DATABASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("DATABASE_URL")
	}
	if baseURL == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL not set")
	}

	schema := "ho_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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

	applyMigrations(t, pool)

	s := New(pool)
	deviceID := uuid.NewString()
	if err := s.UpsertDevice(store.Device{DeviceID: deviceID, Status: "active", LastSeen: time.Now().UTC(), LabelsJSON: []byte(`{"region":"west"}`)}); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	if err := s.UpsertDeviceState(store.DeviceState{DeviceID: deviceID, CurrentVersion: "v1", UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("upsert state: %v", err)
	}

	if _, ok, err := s.GetDevice(deviceID); err != nil || !ok {
		t.Fatalf("get device: ok=%v err=%v", ok, err)
	}
	if _, ok, err := s.GetDeviceState(deviceID); err != nil || !ok {
		t.Fatalf("get device_state: ok=%v err=%v", ok, err)
	}
	if items, err := s.ListDevices(store.ListDevicesFilter{Status: "active", Limit: 10, Offset: 0}); err != nil || len(items) == 0 {
		t.Fatalf("list devices: count=%d err=%v", len(items), err)
	}

	groupID := uuid.NewString()
	if err := s.UpsertGroup(store.Group{GroupID: groupID, Name: "g1", SelectorJSON: []byte(`{"region":"west"}`), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("upsert group: %v", err)
	}
	if err := s.UpsertDesiredStateGroup(store.DesiredStateGroup{GroupID: groupID, DesiredVersion: "v1", UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("upsert desired group: %v", err)
	}
	if err := s.UpsertDesiredStateDevice(store.DesiredStateDevice{DeviceID: deviceID, DesiredVersion: "v1", Source: "agent", UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("upsert desired device: %v", err)
	}
	if _, ok, err := s.GetDesiredStateDevice(deviceID); err != nil || !ok {
		t.Fatalf("get desired device: ok=%v err=%v", ok, err)
	}
	if _, ok, err := s.GetDesiredStateGroupForDevice(deviceID); err != nil || !ok {
		t.Fatalf("get desired group for device: ok=%v err=%v", ok, err)
	}
	if groups, err := s.ListDesiredStateGroups(); err != nil || len(groups) == 0 {
		t.Fatalf("list desired groups: count=%d err=%v", len(groups), err)
	}
	if devices, err := s.ListDesiredStateDevices(); err != nil || len(devices) == 0 {
		t.Fatalf("list desired devices: count=%d err=%v", len(devices), err)
	}

	artifactID := uuid.NewString()
	if err := s.CreateArtifact(store.Artifact{ArtifactID: artifactID, Name: "agent", Version: "1.0.0", ObjectKey: "artifacts/a.tar.gz", SHA256: "abc", SizeBytes: 10, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if _, ok, err := s.GetArtifact(artifactID); err != nil || !ok {
		t.Fatalf("get artifact: ok=%v err=%v", ok, err)
	}
	if items, err := s.ListArtifacts("agent", "1.0.0", 10, 0); err != nil || len(items) == 0 {
		t.Fatalf("list artifacts: count=%d err=%v", len(items), err)
	}

	hash := "abcd"
	if err := s.CreateEnrollmentToken(hash, time.Now().UTC().Add(1*time.Hour)); err != nil {
		t.Fatalf("create token: %v", err)
	}
	ok, err := s.ConsumeEnrollmentToken(hash)
	if err != nil || !ok {
		t.Fatalf("consume token: ok=%v err=%v", ok, err)
	}
}

func applyMigrations(t *testing.T, pool *pgxpool.Pool) {
	migrationsDir := filepath.Join("..", "..", "..", "migrations")
	if err := migrate.Apply(context.Background(), pool, migrationsDir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
}
