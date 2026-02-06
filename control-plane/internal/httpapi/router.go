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
	r.Get("/healthz", handlers.Health())

	enrollmentLimiter := NewRateLimiter(deps.RateLimits.EnrollmentTokenRPM, time.Minute, deps.TrustProxy)
	deviceEnrollLimiter := NewRateLimiter(deps.RateLimits.EnrollRPM, time.Minute, deps.TrustProxy)
	checkinLimiter := NewRateLimiter(deps.RateLimits.CheckinRPM, time.Minute, deps.TrustProxy)
	applyLimiter := NewRateLimiter(deps.RateLimits.ApplyResultRPM, time.Minute, deps.TrustProxy)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/groups", handlers.ListGroups(logger, deps.Store))
		r.Put("/groups/{groupId}", handlers.PutGroup(logger, deps.Store))
		r.Delete("/groups/{groupId}", handlers.DeleteGroup(logger, deps.Store))
		r.Get("/artifacts", handlers.ListArtifacts(logger, deps.Store))
		r.Post("/artifacts", handlers.CreateArtifact(logger, deps.Store))
		r.Post("/artifacts/upload", handlers.UploadArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket))
		r.Get("/artifacts/{artifactId}", handlers.GetArtifact(logger, deps.Store))
		r.Delete("/artifacts/{artifactId}", handlers.DeleteArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket))
		r.Post("/artifacts/{artifactId}/presign", handlers.PresignArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires))
		r.Get("/devices", handlers.ListDevices(logger, deps.Store))
		r.Get("/devices/{deviceId}", handlers.GetDevice(logger, deps.Store))
		r.Delete("/devices/{deviceId}", handlers.DeleteDevice(logger, deps.Store))
		r.Patch("/devices/{deviceId}", handlers.PatchDevice(logger, deps.Store))
		r.With(applyLimiter.Middleware).Post("/devices/{deviceId}/apply-result", handlers.PostApplyResult(logger, deps.Store, deps.Events, deps.TrustProxy, deps.ClientCertHeader))
		r.With(checkinLimiter.Middleware).Post("/devices/checkin", handlers.DeviceCheckin(logger, deps.Store, deps.Events, deps.TrustProxy, deps.ClientCertHeader))
		r.With(enrollmentLimiter.Middleware).Post("/enrollments", handlers.CreateEnrollmentToken(logger, deps.Store))
		r.With(deviceEnrollLimiter.Middleware).Post("/devices/enroll", handlers.DeviceEnroll(logger, deps.Store, deps.Signer, deps.TrustProxy))
		r.Get("/desired-state", handlers.ListDesiredState(logger, deps.Store))
		r.Put("/desired-state/groups/{groupId}", handlers.PutDesiredStateGroup(logger, deps.Store))
		r.Delete("/desired-state/groups/{groupId}", handlers.DeleteDesiredStateGroup(logger, deps.Store))
		r.Put("/desired-state/devices/{deviceId}", handlers.PutDesiredStateDevice(logger, deps.Store))
		r.Delete("/desired-state/devices/{deviceId}", handlers.DeleteDesiredStateDevice(logger, deps.Store))
		r.Get("/logs/{deviceId}", handlers.GetDeviceLogs(logger, deps.LogDir))
		r.Get("/events", handlers.StreamEvents(logger, deps.Events))
		r.Get("/maintenance", handlers.GetMaintenance(deps.Maintenance))
		r.Put("/maintenance", handlers.SetMaintenance(deps.Maintenance, deps.MaintenanceToken))
		r.Get("/maintenance/upgrade/available", handlers.GetUpgradeAvailable(deps.UpgradeUpdatesDir))
		r.Get("/maintenance/upgrade", handlers.GetUpgradeStatus(deps.Upgrade))
		r.Post("/maintenance/upgrade", handlers.ApplyUpgrade(deps.Upgrade, deps.Maintenance, deps.MaintenanceToken))
	})

	return r
}
