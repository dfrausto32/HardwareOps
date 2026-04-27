package httpapi

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/globalplane/httpapi/handlers"
)

// syncManagerIface is the subset of sync.Manager used by the router.
type syncManagerIface interface {
	Refresh(ctx interface{ Done() <-chan struct{} }) error
}

// NewRouter constructs the global-plane chi router.
func NewRouter(logger *log.Logger, deps Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(requestLogger(logger))
	if len(deps.CORSAllowedOrigins) > 0 {
		r.Use(corsMiddleware(deps.CORSAllowedOrigins))
	}
	r.Use(securityHeaders())
	if deps.Auth != nil && deps.Auth.Enabled() {
		r.Use(deps.Auth.Middleware)
	}

	requireRole := func(role string) func(http.Handler) http.Handler {
		if deps.Auth == nil || !deps.Auth.Enabled() {
			return func(next http.Handler) http.Handler { return next }
		}
		return deps.Auth.RequireRole(role)
	}

	requireScopeOrRole := func(scope, minRole string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if deps.Auth == nil || !deps.Auth.Enabled() {
					next.ServeHTTP(w, r)
					return
				}
				if user, ok := auth.UserFromContext(r.Context()); ok {
					if auth.HasRole(user.Roles, minRole) {
						next.ServeHTTP(w, r)
						return
					}
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if token, ok := auth.ServiceTokenFromContext(r.Context()); ok {
					if auth.HasScope(token.Scopes, scope) {
						next.ServeHTTP(w, r)
						return
					}
				}
				http.Error(w, "forbidden", http.StatusForbidden)
			})
		}
	}

	// Liveness — no auth.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// syncMgrAdapter wraps sync.Manager so the handlers can call Refresh
	// without a circular import on context.Context.
	type ctxDone interface{ Done() <-chan struct{} }
	syncAdapter := &syncAdapter{mgr: deps.SyncManager}

	r.Route("/api/v1", func(r chi.Router) {
		// Plane management — requires admin role or federation.manage scope.
		r.With(requireScopeOrRole(auth.ScopeFederationManage, "admin")).
			Get("/planes", handlers.ListPlanes(deps.Store))

		r.With(requireScopeOrRole(auth.ScopeFederationManage, "admin")).
			Post("/planes", handlers.RegisterPlane(deps.Store, deps.TokenEncryptionKey, syncAdapter))

		r.Route("/planes/{planeId}", func(r chi.Router) {
			r.With(requireScopeOrRole(auth.ScopeFederationManage, "admin")).
				Get("/", handlers.GetPlane(deps.Store))
			r.With(requireRole("admin")).
				Patch("/", handlers.UpdatePlane(deps.Store, deps.TokenEncryptionKey, syncAdapter))
			r.With(requireRole("admin")).
				Delete("/", handlers.DeletePlane(deps.Store, syncAdapter))
		})

		// Aggregated read endpoints — viewer or device.read / artifact.read scope.
		r.With(requireScopeOrRole(auth.ScopeDeviceRead, "viewer")).
			Get("/devices", handlers.ListDevices(deps.Store))

		r.With(requireScopeOrRole(auth.ScopeDeviceRead, "viewer")).
			Get("/health/summary", handlers.HealthSummary(deps.Store))

		r.With(requireScopeOrRole(auth.ScopeArtifactRead, "viewer")).
			Get("/artifacts", handlers.ListArtifacts(deps.Store))

		// Global groups and desired state.
		r.With(requireScopeOrRole(auth.ScopeArtifactRead, "viewer")).
			Get("/groups", handlers.ListGlobalGroups(deps.Store))
		r.With(requireRole("operator")).
			Post("/groups", handlers.CreateGlobalGroup(deps.Store, logger))
		r.With(requireRole("admin")).
			Delete("/groups/{groupId}", handlers.DeleteGlobalGroup(deps.Store, deps.TokenEncryptionKey, logger))
		r.With(requireScopeOrRole(auth.ScopeArtifactRead, "viewer")).
			Get("/groups/{groupId}/desired-state", handlers.GetGlobalDesiredState(deps.Store))
		r.With(requireRole("operator")).
			Put("/groups/{groupId}/desired-state", handlers.PutGlobalDesiredState(deps.Store, deps.TokenEncryptionKey, logger))
		r.With(requireRole("operator")).
			Delete("/groups/{groupId}/desired-state", handlers.DeleteGlobalDesiredState(deps.Store, deps.TokenEncryptionKey, logger))
		r.With(requireScopeOrRole(auth.ScopeArtifactRead, "viewer")).
			Get("/desired-state", handlers.ListGlobalDesiredStates(deps.Store))

		// Federated artifact management.
		r.With(requireRole("operator")).
			Post("/federation/artifacts/upload", handlers.UploadFederatedArtifact(
				deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires,
				deps.PublicBaseURL, deps.TokenEncryptionKey, logger,
			))
		r.With(requireScopeOrRole(auth.ScopeArtifactRead, "viewer")).
			Get("/federation/artifacts", handlers.ListFederatedArtifacts(deps.Store))
		r.Route("/federation/artifacts/{artifactId}", func(r chi.Router) {
			r.With(requireScopeOrRole(auth.ScopeArtifactRead, "viewer")).
				Get("/", handlers.GetFederatedArtifact(deps.Store))
			r.With(requireScopeOrRole(auth.ScopeArtifactRead, "viewer")).
				Get("/replication-status", handlers.GetReplicationStatus(deps.Store))
			r.With(requireScopeOrRole(auth.ScopeArtifactRead, "viewer")).
				Get("/presign", handlers.PresignFederatedArtifact(deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires))
		})
	})

	_ = context.Background // satisfy import if unused elsewhere
	return r
}

// syncAdapter adapts sync.Manager.Refresh to the interface expected by handlers.
// handlers receive an interface{Done() <-chan struct{}} to avoid an import cycle.
type syncAdapter struct {
	mgr interface {
		Refresh(ctx context.Context) error
	}
}

func (a *syncAdapter) Refresh(ctx interface{ Done() <-chan struct{} }) error {
	// The handlers pass r.Context() which satisfies context.Context.
	return a.mgr.Refresh(ctx.(context.Context))
}

// requestLogger is a minimal request logger middleware.
func requestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Printf("[global-plane] %s %s", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware adds CORS headers for the allowed origins.
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			for _, o := range allowedOrigins {
				if o == origin || o == "*" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
					break
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// securityHeaders adds minimal security response headers.
func securityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			next.ServeHTTP(w, r)
		})
	}
}

// splitOrigins splits a comma-separated CORS_ALLOWED_ORIGINS string.
func splitOrigins(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
