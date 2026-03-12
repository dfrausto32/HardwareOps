package auth

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
)

func BreakglassResetPassword(st store.Store, email, password string) (store.User, error) {
	if st == nil {
		return store.User{}, errors.New("store required")
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return store.User{}, errors.New("email and password required")
	}
	user, ok, err := st.GetUserByEmail(email)
	if err != nil {
		return store.User{}, err
	}
	if !ok {
		return store.User{}, errors.New("user not found")
	}
	if user.AuthProvider != "" && user.AuthProvider != ModeLocal {
		return store.User{}, errors.New("breakglass reset only supports local users")
	}
	if user.Disabled {
		return store.User{}, errors.New("user is disabled")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return store.User{}, err
	}
	if err := st.UpdateUser(store.UserUpdate{
		UserID:       user.UserID,
		PasswordHash: &hash,
	}); err != nil {
		return store.User{}, err
	}
	user.PasswordHash = hash
	user.UpdatedAt = time.Now().UTC()
	return user, nil
}

func BreakglassCreateAdmin(st store.Store, email, displayName, password string) (store.User, error) {
	if st == nil {
		return store.User{}, errors.New("store required")
	}
	email = strings.TrimSpace(strings.ToLower(email))
	displayName = strings.TrimSpace(displayName)
	if email == "" || password == "" {
		return store.User{}, errors.New("email and password required")
	}
	if displayName == "" {
		displayName = "Recovery Admin"
	}
	if existing, ok, err := st.GetUserByEmail(email); err != nil {
		return store.User{}, err
	} else if ok {
		if existing.AuthProvider == ModeLocal {
			return store.User{}, errors.New("local user already exists")
		}
		return store.User{}, errors.New("user already exists")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return store.User{}, err
	}
	rolesJSON, _ := json.Marshal([]string{"admin"})
	now := time.Now().UTC()
	user := store.User{
		UserID:       uuid.NewString(),
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: hash,
		RolesJSON:    rolesJSON,
		AuthProvider: ModeLocal,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := st.CreateUser(user); err != nil {
		return store.User{}, err
	}
	return user, nil
}

func GenerateBreakglassPassword() (string, error) {
	token, _, err := GenerateVoucherToken()
	if err != nil {
		return "", err
	}
	return token, nil
}
