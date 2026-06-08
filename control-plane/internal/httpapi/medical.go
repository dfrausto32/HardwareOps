package httpapi

import (
	"encoding/json"
	"net/http"
)

// MedicalOnly returns a middleware that responds 404 for any deployment profile
// other than "medical". Wire this around all Phase G medical-only routes so that
// standard deployments cannot accidentally reach them.
func MedicalOnly(profile string) func(http.Handler) http.Handler {
	isMedical := profile == "medical"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMedical {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "not found",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MedicalProfileStatus is the response body for GET /api/v1/medical/status.
type MedicalProfileStatus struct {
	Profile string `json:"profile"`
	Medical bool   `json:"medical"`
}

// MedicalStatus returns a handler that exposes the active deployment profile.
// Reachable only when the MedicalOnly middleware passes.
func MedicalStatus(profile string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MedicalProfileStatus{
			Profile: profile,
			Medical: profile == "medical",
		})
	}
}
