package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/parcel/control-plane/internal/auth"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Login authenticates a global-plane operator with email/password and returns a
// JWT. It mirrors the regional control-plane's login so the bootstrap admin can
// obtain a token issued for the global plane (the regional and global planes use
// different issuers, so a regional token is not accepted here).
func Login(mgr *auth.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mgr == nil || !mgr.Enabled() {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		_, token, exp, _, err := mgr.Authenticate(req.Email, req.Password)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(loginResponse{Token: token, ExpiresAt: exp})
	}
}
