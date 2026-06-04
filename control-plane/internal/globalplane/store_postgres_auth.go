package globalplane

// This file implements the auth.AuthStore interface on PostgresStore so that
// auth.NewManager can be wired up with the global-plane database directly,
// without depending on the full regional control-plane store.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

// EnsureBootstrapAdmin creates an initial admin user when the users table is
// empty, mirroring auth.EnsureBootstrapAdmin for the global-plane's own store.
// Returns true when a user was created.
func (s *PostgresStore) EnsureBootstrapAdmin(email, password string) (bool, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return false, errors.New("bootstrap email/password required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, err
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO users (user_id, email, password_hash, roles, disabled, created_at)
		 VALUES ($1, $2, $3, 'admin', FALSE, now())`,
		uuid.NewString(), email, hash); err != nil {
		return false, err
	}
	return true, nil
}

func (s *PostgresStore) GetUser(userID string) (store.User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT user_id, email, password_hash, roles, disabled, last_login_at, created_at
		 FROM users WHERE user_id = $1`, userID)
	u, err := scanUser(row)
	if err == pgx.ErrNoRows {
		return store.User{}, false, nil
	}
	return u, err == nil, err
}

func (s *PostgresStore) GetUserByEmail(email string) (store.User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT user_id, email, password_hash, roles, disabled, last_login_at, created_at
		 FROM users WHERE email = $1`, email)
	u, err := scanUser(row)
	if err == pgx.ErrNoRows {
		return store.User{}, false, nil
	}
	return u, err == nil, err
}

func (s *PostgresStore) SetUserLastLogin(userID string, at time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `UPDATE users SET last_login_at = $2 WHERE user_id = $1`, userID, at)
	return err
}

func (s *PostgresStore) GetServiceTokenByTokenHash(tokenHash string) (store.ServiceToken, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := s.pool.QueryRow(ctx,
		`SELECT token_id, name, token_hash, scopes, last_used_at, created_at
		 FROM service_tokens WHERE token_hash = $1 AND NOT disabled`, tokenHash)
	var t store.ServiceToken
	var scopesRaw string
	var lastUsed *time.Time
	err := row.Scan(&t.TokenID, &t.Name, &t.TokenHash, &scopesRaw, &lastUsed, &t.CreatedAt)
	if err == pgx.ErrNoRows {
		return store.ServiceToken{}, false, nil
	}
	if err != nil {
		return store.ServiceToken{}, false, err
	}
	if lastUsed != nil {
		t.LastUsedAt = *lastUsed
	}
	// Store as JSON array for auth.parseScopes compatibility.
	var scopeSlice []string
	if strings.HasPrefix(scopesRaw, "[") {
		_ = json.Unmarshal([]byte(scopesRaw), &scopeSlice)
	} else if scopesRaw != "" {
		scopeSlice = strings.Fields(scopesRaw)
	}
	if len(scopeSlice) > 0 {
		b, _ := json.Marshal(scopeSlice)
		t.ScopesJSON = b
	}
	return t, true, nil
}

func (s *PostgresStore) SetServiceTokenLastUsed(tokenID string, at time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`UPDATE service_tokens SET last_used_at = $2 WHERE token_id = $1`, tokenID, at)
	return err
}

// scanUser scans a users row into a store.User.
func scanUser(row scannable) (store.User, error) {
	var u store.User
	var rolesRaw string
	var lastLogin *time.Time
	var createdAt time.Time
	err := row.Scan(&u.UserID, &u.Email, &u.PasswordHash, &rolesRaw, &u.Disabled, &lastLogin, &createdAt)
	if err != nil {
		return store.User{}, err
	}
	if lastLogin != nil {
		u.LastLoginAt = *lastLogin
	}
	// Store as JSON array for auth.rolesFromJSON compatibility.
	rolesSlice := strings.Fields(rolesRaw)
	if len(rolesSlice) > 0 {
		b, _ := json.Marshal(rolesSlice)
		u.RolesJSON = b
	}
	return u, nil
}
