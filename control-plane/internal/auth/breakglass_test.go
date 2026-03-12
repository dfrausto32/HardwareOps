package auth

import (
	"testing"
	"time"

	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
	"golang.org/x/crypto/bcrypt"
)

func TestBreakglassResetPassword(t *testing.T) {
	mem := memory.New()
	passwordHash, err := HashPassword("old-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := store.User{
		UserID:       "user-1",
		Email:        "admin@example.com",
		PasswordHash: passwordHash,
		RolesJSON:    []byte(`["admin"]`),
		AuthProvider: ModeLocal,
		CreatedAt:    time.Now().UTC(),
	}
	if err := mem.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	updated, err := BreakglassResetPassword(mem, user.Email, "new-password")
	if err != nil {
		t.Fatalf("breakglass reset password: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("new-password")); err != nil {
		t.Fatalf("expected updated password hash")
	}
}

func TestBreakglassCreateAdmin(t *testing.T) {
	mem := memory.New()
	user, err := BreakglassCreateAdmin(mem, "recovery@example.com", "", "temp-password")
	if err != nil {
		t.Fatalf("breakglass create admin: %v", err)
	}
	if user.Email != "recovery@example.com" {
		t.Fatalf("unexpected email: %s", user.Email)
	}
	if string(user.RolesJSON) != `["admin"]` {
		t.Fatalf("expected admin role, got %s", string(user.RolesJSON))
	}
	if user.DisplayName != "Recovery Admin" {
		t.Fatalf("unexpected display name: %s", user.DisplayName)
	}
}
