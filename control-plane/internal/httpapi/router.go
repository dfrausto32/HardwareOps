package httpapi

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hardwareops/control-plane/internal/httpapi/handlers"
)

func NewRouter(logger *log.Logger, deps Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(RequestLogger(logger))
	if len(deps.CORSAllowedOrigins) > 0 {
		r.Use(CORS(deps.CORSAllowedOrigins))
	}
	if deps.Maintenance != nil {
		r.Use(MaintenanceMiddleware(deps.Maintenance))
	}
	if deps.Auth != nil && deps.Auth.Enabled() {
		r.Use(deps.Auth.Middleware)
	}
	r.Get("/healthz", handlers.Health())

	enrollmentLimiter := NewRateLimiter(deps.RateLimits.EnrollmentTokenRPM, time.Minute, deps.TrustProxy)
	deviceEnrollLimiter := NewRateLimiter(deps.RateLimits.EnrollRPM, time.Minute, deps.TrustProxy)
	checkinLimiter := NewRateLimiter(deps.RateLimits.CheckinRPM, time.Minute, deps.TrustProxy)
	applyLimiter := NewRateLimiter(deps.RateLimits.ApplyResultRPM, time.Minute, deps.TrustProxy)

	requireRole := func(role string) func(http.Handler) http.Handler {
		if deps.Auth == nil || !deps.Auth.Enabled() {
			return func(next http.Handler) http.Handler { return next }
		}
		return deps.Auth.RequireRole(role)
	}
	viewer := requireRole("viewer")
	operator := requireRole("operator")
	admin := requireRole("admin")

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/auth/status", handlers.AuthStatus(deps.Auth))
		r.Post("/auth/login", handlers.Login(logger, deps.Auth, deps.Store, deps.TrustProxy))
		r.Post("/auth/register", handlers.Register(logger, deps.Auth, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/auth/me", handlers.GetMe())
		r.With(admin).Post("/auth/vouchers", handlers.CreateAuthVoucher(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/users", handlers.CreateUser(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/users", handlers.ListUsers(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Patch("/users/{userId}", handlers.UpdateUser(logger, deps.Store, deps.TrustProxy))

		r.With(viewer).Get("/groups", handlers.ListGroups(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Put("/groups/{groupId}", handlers.PutGroup(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/groups/{groupId}", handlers.DeleteGroup(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/artifacts", handlers.ListArtifacts(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/artifacts", handlers.CreateArtifact(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/artifacts/upload", handlers.UploadArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy))
		r.With(viewer).Get("/artifacts/{artifactId}", handlers.GetArtifact(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/artifacts/{artifactId}", handlers.DeleteArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy))
		r.Post("/artifacts/{artifactId}/presign", handlers.PresignArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires, deps.TrustProxy))

		r.With(admin).Get("/audit", handlers.ListAuditEvents(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/audit.csv", handlers.ExportAuditCSV(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/audit/retention", handlers.GetAuditRetention(logger, deps.Store))
		r.With(admin).Put("/audit/retention", handlers.SetAuditRetention(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/license", handlers.GetLicenseStatus(logger, deps.Store, deps.License))

		r.With(viewer).Get("/devices", handlers.ListDevices(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/devices/{deviceId}", handlers.GetDevice(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/devices/{deviceId}", handlers.DeleteDevice(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Patch("/devices/{deviceId}", handlers.PatchDevice(logger, deps.Store, deps.TrustProxy))
		r.With(applyLimiter.Middleware).Post("/devices/{deviceId}/apply-result", handlers.PostApplyResult(logger, deps.Store, deps.Events, deps.TrustProxy, deps.ClientCertHeader))
		r.With(checkinLimiter.Middleware).Post("/devices/checkin", handlers.DeviceCheckin(logger, deps.Store, deps.Events, deps.TrustProxy, deps.ClientCertHeader))
		r.With(enrollmentLimiter.Middleware).With(operator).Post("/enrollments", handlers.CreateEnrollmentToken(logger, deps.Store, deps.License, deps.TrustProxy))
		r.With(deviceEnrollLimiter.Middleware).Post("/devices/enroll", handlers.DeviceEnroll(logger, deps.Store, deps.License, deps.Signer, deps.TrustProxy))
		r.With(viewer).Get("/desired-state", handlers.ListDesiredState(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Put("/desired-state/groups/{groupId}", handlers.PutDesiredStateGroup(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/desired-state/groups/{groupId}", handlers.DeleteDesiredStateGroup(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Put("/desired-state/devices/{deviceId}", handlers.PutDesiredStateDevice(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/desired-state/devices/{deviceId}", handlers.DeleteDesiredStateDevice(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/logs/{deviceId}", handlers.GetDeviceLogs(logger, deps.Store, deps.LogDir, deps.TrustProxy))
		r.With(viewer).Get("/events", handlers.StreamEvents(logger, deps.Events))
		r.With(viewer).Get("/maintenance", handlers.GetMaintenance(deps.Maintenance))
		r.With(admin).Put("/maintenance", handlers.SetMaintenance(logger, deps.Store, deps.Maintenance, deps.MaintenanceToken, deps.TrustProxy))
		r.With(viewer).Get("/maintenance/upgrade/available", handlers.GetUpgradeAvailable(deps.UpgradeUpdatesDir))
		r.With(viewer).Get("/maintenance/upgrade/preflight", handlers.GetUpgradePreflight(deps.Upgrade, deps.UpgradeUpdatesDir))
		r.With(viewer).Get("/maintenance/upgrade", handlers.GetUpgradeStatus(deps.Upgrade))
		r.With(admin).Post("/maintenance/upgrade", handlers.ApplyUpgrade(logger, deps.Store, deps.Upgrade, deps.Maintenance, deps.MaintenanceToken, deps.TrustProxy, deps.UpgradeUpdatesDir))
	})

	return r
}
