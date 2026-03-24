package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Apply(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return err
	}

	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}

	applied, err := loadApplied(ctx, pool)
	if err != nil {
		return err
	}

	for _, file := range files {
		if applied[file] {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return err
		}

		if requiresNoTransaction(string(content)) {
			conn, err := pool.Acquire(ctx)
			if err != nil {
				return err
			}
			for _, stmt := range splitStatements(string(content)) {
				if _, err := conn.Exec(ctx, stmt, pgx.QueryExecModeSimpleProtocol); err != nil {
					conn.Release()
					return fmt.Errorf("apply %s: %w", file, err)
				}
			}
			if _, err := conn.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, pgx.QueryExecModeSimpleProtocol, file); err != nil {
				conn.Release()
				return err
			}
			conn.Release()
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", file, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, file); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}

	return nil
}

func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`)
	return err
}

func migrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".sql") {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no migration files found in %s", dir)
	}
	sort.Strings(files)
	return files, nil
}

func loadApplied(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return applied, nil
}

func requiresNoTransaction(content string) bool {
	return strings.Contains(strings.ToUpper(content), "CONCURRENTLY")
}

func splitStatements(content string) []string {
	parts := strings.Split(content, ";")
	stmts := make([]string, 0, len(parts))
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt == "" {
			continue
		}
		stmts = append(stmts, stmt)
	}
	return stmts
}
