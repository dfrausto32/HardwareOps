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

	r.Get("/healthz", handlers.Health())

	enrollmentLimiter := NewRateLimiter(deps.RateLimits.EnrollmentTokenRPM, time.Minute, deps.TrustProxy)
	deviceEnrollLimiter := NewRateLimiter(deps.RateLimits.EnrollRPM, time.Minute, deps.TrustProxy)
	checkinLimiter := NewRateLimiter(deps.RateLimits.CheckinRPM, time.Minute, deps.TrustProxy)
	applyLimiter := NewRateLimiter(deps.RateLimits.ApplyResultRPM, time.Minute, deps.TrustProxy)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/groups", handlers.ListGroups(logger, deps.Store))
		r.Put("/groups/{groupId}", handlers.PutGroup(logger, deps.Store))
		r.Get("/artifacts", handlers.ListArtifacts(logger, deps.Store))
		r.Post("/artifacts", handlers.CreateArtifact(logger, deps.Store))
		r.Post("/artifacts/upload", handlers.UploadArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket))
		r.Get("/artifacts/{artifactId}", handlers.GetArtifact(logger, deps.Store))
		r.Post("/artifacts/{artifactId}/presign", handlers.PresignArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires))
		r.Get("/devices", handlers.ListDevices(logger, deps.Store))
		r.Get("/devices/{deviceId}", handlers.GetDevice(logger, deps.Store))
		r.Patch("/devices/{deviceId}", handlers.PatchDevice(logger, deps.Store))
		r.With(applyLimiter.Middleware).Post("/devices/{deviceId}/apply-result", handlers.PostApplyResult(logger, deps.Store, deps.TrustProxy, deps.ClientCertHeader))
		r.With(checkinLimiter.Middleware).Post("/devices/checkin", handlers.DeviceCheckin(logger, deps.Store, deps.TrustProxy, deps.ClientCertHeader))
		r.With(enrollmentLimiter.Middleware).Post("/enrollments", handlers.CreateEnrollmentToken(logger, deps.Store))
		r.With(deviceEnrollLimiter.Middleware).Post("/devices/enroll", handlers.DeviceEnroll(logger, deps.Store, deps.Signer, deps.TrustProxy))
		r.Get("/desired-state", handlers.ListDesiredState(logger, deps.Store))
		r.Put("/desired-state/groups/{groupId}", handlers.PutDesiredStateGroup(logger, deps.Store))
		r.Put("/desired-state/devices/{deviceId}", handlers.PutDesiredStateDevice(logger, deps.Store))
	})

	return r
}
