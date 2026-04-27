package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
)

func EnsureBootstrapAdmin(st store.Store, email, password string) (bool, error) {
	if st == nil {
		return false, errors.New("store required")
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return false, errors.New("bootstrap email/password required")
	}
	users, err := st.ListUsers(1, 0)
	if err != nil {
		return false, err
	}
	if len(users) > 0 {
		return false, nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return false, err
	}
	rolesJSON := []byte(`["admin"]`)
	now := time.Now().UTC()
	user := store.User{
		UserID:       uuid.NewString(),
		Email:        email,
		DisplayName:  "Bootstrap Admin",
		PasswordHash: hash,
		RolesJSON:    rolesJSON,
		Disabled:     false,
		AuthProvider: "local",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := st.CreateUser(user); err != nil {
		return false, err
	}
	return true, nil
}
