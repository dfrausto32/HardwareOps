package httpapi

import (
	"crypto/x509"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/httpapi/handlers"
)

func NewRouter(logger *log.Logger, deps Dependencies) http.Handler {
	r := chi.NewRouter()
	ConfigureTrustedProxyCIDRs(deps.TrustedProxyCIDRs)
	handlers.ConfigureTrustedProxyCIDRs(deps.TrustedProxyCIDRs)

	r.Use(RequestLogger(logger))
	if len(deps.CORSAllowedOrigins) > 0 {
		r.Use(CORS(deps.CORSAllowedOrigins))
	}
	if deps.Metrics != nil {
		r.Use(deps.Metrics.Middleware)
	}
	if deps.Maintenance != nil {
		r.Use(MaintenanceMiddleware(deps.Maintenance))
	}
	if deps.Auth != nil && deps.Auth.Enabled() {
		r.Use(deps.Auth.Middleware)
	}
	r.Get("/healthz", handlers.Health())

	enrollmentLimiter := NewRateLimiter(deps.RateLimits.EnrollmentTokenRPM, time.Minute, deps.TrustProxy, "enrollment_token", deps.Metrics)
	deviceEnrollLimiter := NewRateLimiter(deps.RateLimits.EnrollRPM, time.Minute, deps.TrustProxy, "device_enroll", deps.Metrics)
	checkinLimiter := NewRateLimiter(deps.RateLimits.CheckinRPM, time.Minute, deps.TrustProxy, "checkin", deps.Metrics)
	applyLimiter := NewRateLimiter(deps.RateLimits.ApplyResultRPM, time.Minute, deps.TrustProxy, "apply_result", deps.Metrics)
	loginLimiter := NewRateLimiter(deps.RateLimits.AuthLoginRPM, time.Minute, deps.TrustProxy, "auth_login", deps.Metrics)
	pendingEnrollmentGuard := handlers.NewPendingEnrollmentGuard(handlers.PendingEnrollmentGuardConfig{
		RequestRPMPerSource:  deps.PendingEnrollmentGuardrails.RequestRPMPerSource,
		RequestRPMPerProfile: deps.PendingEnrollmentGuardrails.RequestRPMPerProfile,
		MaxActive:            deps.PendingEnrollmentGuardrails.MaxActive,
		MaxActivePerProfile:  deps.PendingEnrollmentGuardrails.MaxActivePerProfile,
		MaxActivePerSource:   deps.PendingEnrollmentGuardrails.MaxActivePerSource,
	}, deps.Metrics)
	identityPolicy := handlers.DeviceIdentityPolicy{
		Mode:             deps.DeviceIdentityMode,
		RequireOnEnroll:  deps.DeviceIdentityRequireOnEnroll,
		RequireOnCheckin: deps.DeviceIdentityRequireOnCheckin,
	}

	requireRole := func(role string) func(http.Handler) http.Handler {
		if deps.Auth == nil || !deps.Auth.Enabled() {
			return func(next http.Handler) http.Handler { return next }
		}
		return deps.Auth.RequireRole(role)
	}
	artifactPublisher := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if deps.Auth == nil || !deps.Auth.Enabled() {
				next.ServeHTTP(w, r)
				return
			}
			if user, ok := auth.UserFromContext(r.Context()); ok {
				if auth.HasRole(user.Roles, "operator") {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if token, ok := auth.ServiceTokenFromContext(r.Context()); ok {
				if auth.HasScope(token.Scopes, "artifact.publish") {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
	viewer := requireRole("viewer")
	operator := requireRole("operator")
	admin := requireRole("admin")
	artifactSigPolicy := handlers.ArtifactSignaturePolicy{
		Require:       deps.ArtifactSignatureRequireDefault,
		EnforceIngest: deps.ArtifactSignatureEnforceIngest,
		KeyID:         deps.ArtifactSignatureKeyID,
	}

	if deps.Metrics != nil && deps.MetricsPath != "" {
		r.With(admin).Handle(deps.MetricsPath, deps.Metrics.Handler())
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/bootstrap", handlers.BootstrapStatus(deps.Auth != nil && deps.Auth.Enabled(), deps.BootstrapToken))
		r.Get("/bootstrap/ca", handlers.DownloadBootstrapCA(logger, deps.Store, deps.CertManager, deps.BootstrapToken, deps.TrustProxy))

		r.Get("/auth/status", handlers.AuthStatus(deps.Auth))
		r.With(loginLimiter.Middleware).Post("/auth/login", handlers.Login(logger, deps.Auth, deps.Store, deps.TrustProxy, deps.AuthLoginBackoff))
		r.Post("/auth/register", handlers.Register(logger, deps.Auth, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/auth/me", handlers.GetMe())
		r.With(admin).Post("/auth/vouchers", handlers.CreateAuthVoucher(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/auth/service-tokens", handlers.CreateServiceToken(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/auth/service-tokens", handlers.ListServiceTokens(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/auth/service-tokens/{tokenId}/revoke", handlers.RevokeServiceToken(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/auth/service-tokens/{tokenId}/rotate", handlers.RotateServiceToken(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/users", handlers.CreateUser(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/users", handlers.ListUsers(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Patch("/users/{userId}", handlers.UpdateUser(logger, deps.Store, deps.TrustProxy))

		r.With(viewer).Get("/groups", handlers.ListGroups(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/groups/batch", handlers.BatchGroups(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Put("/groups/{groupId}", handlers.PutGroup(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/groups/{groupId}", handlers.DeleteGroup(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/artifacts", handlers.ListArtifacts(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/artifacts/lifecycle/policy", handlers.GetArtifactLifecyclePolicy(logger, deps.Store))
		r.With(viewer).Get("/artifacts/lifecycle/status", handlers.GetArtifactLifecycleStatus(deps.ArtifactLifecycle))
		r.With(admin).Put("/artifacts/lifecycle/policy", handlers.SetArtifactLifecyclePolicy(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/artifacts/lifecycle/prune", handlers.PruneArtifacts(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy, deps.ArtifactLifecycle))
		r.With(operator).Post("/artifacts", handlers.CreateArtifactWithRealtime(logger, deps.Store, deps.TrustProxy, artifactSigPolicy, deps.ReleaseAutoUpdate, deps.Events))
		r.With(operator).Post("/artifacts/upload", handlers.UploadArtifactWithRealtime(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy, deps.Metrics, artifactSigPolicy, deps.ReleaseAutoUpdate, deps.Events))
		r.With(artifactPublisher).Post("/artifacts/pull", handlers.PullArtifactWithRealtime(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.ArtifactPullHosts, deps.ArtifactPullMaxBytes, deps.ArtifactPullTimeout, deps.ArtifactPullAllowInsecureHTTP, deps.ArtifactPullCreds, deps.TrustProxy, deps.Metrics, artifactSigPolicy, deps.ReleaseAutoUpdate, deps.Events))
		r.With(artifactPublisher).Post("/artifacts/presign-upload", handlers.PresignArtifactUpload(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires, deps.TrustProxy, deps.Metrics))
		r.With(artifactPublisher).Post("/artifacts/complete", handlers.CompleteArtifactUploadWithRealtime(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy, deps.Metrics, artifactSigPolicy, deps.ReleaseAutoUpdate, deps.Events))
		r.With(admin).Get("/artifacts/pull-credentials", handlers.GetPullCredentialStatus(logger, deps.ArtifactPullCredsManager))
		r.With(admin).Post("/artifacts/pull-credentials/reload", handlers.ReloadPullCredentials(logger, deps.Store, deps.ArtifactPullCredsManager, deps.TrustProxy))
		r.With(viewer).Get("/artifacts/{artifactId}", handlers.GetArtifact(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/artifacts/{artifactId}/deprecate", handlers.DeprecateArtifact(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/artifacts/{artifactId}/restore", handlers.RestoreArtifact(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/artifacts/{artifactId}", handlers.DeleteArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy))
		r.Post("/artifacts/{artifactId}/presign", handlers.PresignArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires, deps.TrustProxy, deps.Metrics))

		r.With(admin).Get("/audit", handlers.ListAuditEvents(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/audit.csv", handlers.ExportAuditCSV(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/audit/retention", handlers.GetAuditRetention(logger, deps.Store))
		r.With(admin).Put("/audit/retention", handlers.SetAuditRetention(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/license", handlers.GetLicenseStatus(logger, deps.Store, deps.License))
		r.With(viewer).Get("/release-auto-update", handlers.GetReleaseAutoUpdateStatus(logger, deps.Store, deps.ReleaseAutoUpdate))
		r.With(admin).Put("/release-auto-update", handlers.SetReleaseAutoUpdateSettings(logger, deps.Store, deps.ReleaseAutoUpdate, deps.TrustProxy))
		r.With(admin).Post("/release-auto-update/run", handlers.RunReleaseAutoUpdate(logger, deps.Store, deps.ReleaseAutoUpdate, deps.TrustProxy))
		r.With(viewer).Get("/cert-rotation", handlers.GetCertRotationStatus(logger, deps.Store, deps.CertManager))
		r.With(operator).Post("/cert-rotation/reload", handlers.ReloadCertRotation(logger, deps.Store, deps.CertManager, deps.TrustProxy))
		r.With(operator).Post("/cert-rotation/rotate", handlers.RotateCertRotation(logger, deps.Store, deps.CertManager, deps.TrustProxy, deps.CertRotationGrace))
		r.With(operator).Post("/cert-rotation/cleanup", handlers.CleanupCertRotation(logger, deps.Store, deps.CertManager, deps.TrustProxy))

		r.With(viewer).Get("/devices", handlers.ListDevices(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/devices/{deviceId}", handlers.GetDevice(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/devices/{deviceId}", handlers.DeleteDevice(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/devices/{deviceId}/decommission", handlers.DecommissionDevice(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Patch("/devices/{deviceId}", handlers.PatchDevice(logger, deps.Store, deps.TrustProxy))
		r.With(applyLimiter.Middleware).Post("/devices/{deviceId}/apply-result", handlers.PostApplyResult(logger, deps.Store, deps.Events, deps.TrustProxy, deps.ClientCertHeader, deps.Metrics))
		var activeCAPool func() *x509.CertPool
		if deps.CertManager != nil {
			activeCAPool = deps.CertManager.ActivePool
		}
		r.With(checkinLimiter.Middleware).Post("/devices/checkin", handlers.DeviceCheckin(logger, deps.Store, deps.Events, deps.TrustProxy, deps.ClientCertHeader, activeCAPool, identityPolicy, deps.Metrics, artifactSigPolicy))
		r.With(checkinLimiter.Middleware).Post("/devices/reenroll", handlers.DeviceReenroll(logger, deps.Store, deps.Signer, deps.TrustProxy, deps.ClientCertHeader))
		r.With(enrollmentLimiter.Middleware).With(operator).Post("/enrollments", handlers.CreateEnrollmentToken(logger, deps.Store, deps.License, deps.TrustProxy, deps.Metrics))
		r.With(operator).Get("/enrollment-profiles", handlers.ListEnrollmentProfiles(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/enrollment-profiles", handlers.CreateEnrollmentProfile(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Patch("/enrollment-profiles/{profileId}", handlers.UpdateEnrollmentProfile(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/enrollment-profiles/{profileId}/rotate", handlers.RotateEnrollmentProfileToken(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/enrollment-profiles/{profileId}/disable", handlers.SetEnrollmentProfileDisabled(logger, deps.Store, deps.TrustProxy, true))
		r.With(operator).Post("/enrollment-profiles/{profileId}/enable", handlers.SetEnrollmentProfileDisabled(logger, deps.Store, deps.TrustProxy, false))
		r.With(deviceEnrollLimiter.Middleware).Post("/devices/enroll", handlers.DeviceEnrollWithEvents(logger, deps.Store, deps.License, deps.Signer, identityPolicy, deps.TrustProxy, deps.Metrics, deps.Events))
		r.With(deviceEnrollLimiter.Middleware).Post("/pending-enrollments/request", handlers.RequestPendingEnrollment(logger, deps.Store, identityPolicy, deps.TrustProxy, deps.Metrics, deps.Events, pendingEnrollmentGuard))
		r.With(deviceEnrollLimiter.Middleware).Post("/pending-enrollments/claim", handlers.ClaimPendingEnrollment(logger, deps.Store, deps.License, deps.Signer, identityPolicy, deps.TrustProxy, deps.Metrics, deps.Events))
		r.With(operator).Get("/pending-enrollments", handlers.ListPendingEnrollments(logger, deps.Store, deps.TrustProxy, deps.Metrics))
		r.With(operator).Post("/pending-enrollments/{requestId}/approve", handlers.ApprovePendingEnrollment(logger, deps.Store, deps.TrustProxy, deps.Metrics))
		r.With(operator).Post("/pending-enrollments/{requestId}/deny", handlers.DenyPendingEnrollment(logger, deps.Store, deps.TrustProxy, deps.Metrics))
		r.With(operator).Post("/pending-enrollments/{requestId}/reset", handlers.ResetPendingEnrollment(logger, deps.Store, deps.TrustProxy, deps.Metrics))
		r.With(viewer).Get("/desired-state", handlers.ListDesiredState(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Put("/desired-state/groups/{groupId}", handlers.PutDesiredStateGroup(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/desired-state/groups/{groupId}", handlers.DeleteDesiredStateGroup(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Put("/desired-state/devices/{deviceId}", handlers.PutDesiredStateDevice(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/desired-state/devices/{deviceId}", handlers.DeleteDesiredStateDevice(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/logs/{deviceId}", handlers.GetDeviceLogs(logger, deps.Store, deps.LogDir, deps.TrustProxy))
		r.With(viewer).Get("/events/history", handlers.ListRuntimeEvents(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/events/retention", handlers.GetRuntimeEventRetention(logger, deps.Store))
		r.With(admin).Put("/events/retention", handlers.SetRuntimeEventRetention(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/events", handlers.StreamEvents(logger, deps.Events))
		r.With(viewer).Get("/health/summary", handlers.HealthSummaryHandler(logger, deps.Store))
		r.With(viewer).Get("/maintenance", handlers.GetMaintenance(deps.Maintenance))
		r.With(admin).Put("/maintenance", handlers.SetMaintenance(logger, deps.Store, deps.Maintenance, deps.TrustProxy))
		r.With(admin).Get("/maintenance/backups", handlers.ListBackups(deps.BackupDir))
		r.With(admin).Get("/maintenance/backup", handlers.GetBackupStatus(deps.Backup))
		r.With(admin).Post("/maintenance/backup", handlers.StartBackup(logger, deps.Store, deps.Backup, deps.TrustProxy, deps.Metrics))
		r.With(admin).Get("/maintenance/restore", handlers.GetRestoreStatus(deps.Restore))
		r.With(admin).Post("/maintenance/restore", handlers.StartRestore(logger, deps.Store, deps.Restore, deps.Maintenance, deps.BackupDir, deps.TrustProxy, deps.Metrics))
		r.With(viewer).Get("/maintenance/upgrade/available", handlers.GetUpgradeAvailable(deps.UpgradeUpdatesDir))
		r.With(viewer).Get("/maintenance/upgrade/preflight", handlers.GetUpgradePreflight(deps.Upgrade, deps.UpgradeUpdatesDir))
		r.With(viewer).Get("/maintenance/upgrade", handlers.GetUpgradeStatus(deps.Upgrade))
		r.With(admin).Post("/maintenance/upgrade", handlers.ApplyUpgrade(logger, deps.Store, deps.Upgrade, deps.Maintenance, deps.TrustProxy, deps.UpgradeUpdatesDir, deps.Metrics))
	})

	return r
}
