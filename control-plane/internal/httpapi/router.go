package httpapi

import (
	"crypto/x509"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/parcel/control-plane/internal/artifacttrust"
	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/httpapi/handlers"
)

func NewRouter(logger *log.Logger, deps Dependencies) http.Handler {
	r := chi.NewRouter()
	ConfigureTrustedProxyCIDRs(deps.TrustedProxyCIDRs)
	handlers.ConfigureTrustedProxyCIDRs(deps.TrustedProxyCIDRs)

	r.Use(RequestLogger(logger))
	if len(deps.CORSAllowedOrigins) > 0 {
		r.Use(CORS(deps.CORSAllowedOrigins))
	}
	r.Use(SecurityHeaders())
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
	// requireScopeOrRole allows a user with minRole OR a service token with the
	// given scope to access the route.
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
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			})
		}
	}
	artifactPublisher := requireScopeOrRole(auth.ScopeArtifactPublish, "operator")
	deploymentTrigger := requireScopeOrRole(auth.ScopeDeploymentTrigger, "operator")
	webhookManager := requireScopeOrRole(auth.ScopeWebhookManage, "operator")
	deviceViewer   := requireScopeOrRole(auth.ScopeDeviceRead, "viewer")
	artifactViewer := requireScopeOrRole(auth.ScopeArtifactRead, "viewer")
	viewer := requireRole("viewer")
	operator := requireRole("operator")
	admin := requireRole("admin")
	// vulnScanTrigger is nil-safe: when ArtifactScanJob is nil we pass nil so
	// the handlers' nil-check works correctly.
	var vulnScanTrigger handlers.ArtifactVulnScanTrigger
	if deps.ArtifactScanJob != nil {
		vulnScanTrigger = deps.ArtifactScanJob
	}
	// sbomGenTrigger follows the same nil-safety pattern.
	var sbomGenTrigger handlers.ArtifactSBOMGenerator
	if deps.ArtifactSBOMJob != nil {
		sbomGenTrigger = deps.ArtifactSBOMJob
	}
	// nessusTrigger follows the same nil-safety pattern.
	var nessusTrigger handlers.NessusSyncJobTrigger
	if deps.NessusSyncJob != nil {
		nessusTrigger = deps.NessusSyncJob
	}

	artifactSigPolicy := handlers.ArtifactSignaturePolicy{
		Store:                 deps.Store,
		VerificationMode:      deps.ArtifactTrustVerificationMode,
		AllowedSigningKeyIDs:  deps.ArtifactTrustAllowedKeyIDs,
		AllowedSignatureTypes: deps.ArtifactTrustAllowedSigTypes,
		Require:               deps.ArtifactSignatureRequireDefault,
		EnforceIngest:         deps.ArtifactSignatureEnforceIngest,
		KeyID:                 deps.ArtifactSignatureKeyID,
		Hardened:              deps.HardenedProfile,
		KeylessOpts: artifacttrust.KeylessVerifyOptions{
			FulcioRootCertPEM: deps.ArtifactFulcioRootCert,
			RekorURL:          deps.ArtifactRekorURL,
			RequireRekorLog:   deps.ArtifactRequireRekorLog,
		},
	}

	if deps.Metrics != nil && deps.MetricsPath != "" {
		r.With(admin).Handle(deps.MetricsPath, deps.Metrics.Handler())
	}

	// Derive OIDC login URL for use in status endpoint.
	oidcLoginURL := ""
	if deps.OIDCProvider != nil {
		oidcLoginURL = "/api/v1/auth/oidc/login"
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/bootstrap", handlers.BootstrapStatus(deps.Auth != nil && deps.Auth.Enabled(), deps.BootstrapToken))
		r.Get("/bootstrap/ca", handlers.DownloadBootstrapCA(logger, deps.Store, deps.CertManager, deps.BootstrapToken, deps.TrustProxy))

		r.Get("/auth/status", handlers.AuthStatus(deps.Auth, oidcLoginURL, deps.LDAPProvider != nil, deps.SMTPEnabled))
		r.With(loginLimiter.Middleware).Post("/auth/login", handlers.Login(logger, deps.Auth, deps.Store, deps.TrustProxy, deps.AuthLoginBackoff))
		r.Post("/auth/register", handlers.Register(logger, deps.Auth, deps.Store, deps.TrustProxy))
		r.With(loginLimiter.Middleware).Post("/auth/password-reset/complete", handlers.CompletePasswordResetToken(logger, deps.Auth, deps.Store, deps.TrustProxy))
		r.With(loginLimiter.Middleware).Post("/auth/forgot-password", handlers.ForgotPassword(logger, deps.Auth, deps.Store, deps.Mailer, deps.AppPublicURL, deps.TrustProxy))
		if deps.WorkloadIdentity != nil {
			r.With(loginLimiter.Middleware).Post("/auth/workload-identity/exchange", handlers.ExchangeWorkloadIdentityToken(logger, deps.Auth, deps.WorkloadIdentity, deps.Store, deps.TrustProxy))
			if statusManager, ok := deps.WorkloadIdentity.(*auth.WorkloadIdentityManager); ok {
				r.With(admin).Get("/auth/workload-identity/status", handlers.GetWorkloadIdentityStatus(statusManager))
			}
		}
		if deps.OIDCProvider != nil {
			r.Get("/auth/oidc/login", handlers.OIDCLogin(deps.OIDCProvider))
			r.Get("/auth/oidc/callback", handlers.OIDCCallback(logger, deps.OIDCProvider, deps.Store, deps.TrustProxy))
		}
		if deps.LDAPProvider != nil {
			r.With(loginLimiter.Middleware).Post("/auth/ldap/login", handlers.LDAPLogin(logger, deps.LDAPProvider, deps.Store, deps.TrustProxy))
		}
		r.With(viewer).Get("/auth/me", handlers.GetMe(deps.Store))
		r.With(viewer).Post("/auth/recovery-codes/generate", handlers.GenerateRecoveryCodes(logger, deps.Auth, deps.Store, deps.TrustProxy))
		r.With(loginLimiter.Middleware).Post("/auth/recovery-codes/reset", handlers.ResetPasswordWithRecoveryCode(logger, deps.Auth, deps.Store, deps.TrustProxy))
		r.With(viewer).Post("/auth/totp/enroll", handlers.TOTPEnroll(logger, deps.Auth, deps.Store, deps.TOTPEncryptionKey, deps.TrustProxy))
		r.With(viewer).Post("/auth/totp/confirm", handlers.TOTPConfirm(logger, deps.Auth, deps.Store, deps.TOTPEncryptionKey, deps.TrustProxy))
		r.With(loginLimiter.Middleware).Post("/auth/totp/verify", handlers.TOTPVerify(logger, deps.Auth, deps.Store, deps.TOTPEncryptionKey, deps.TrustProxy))
		r.With(viewer).Post("/auth/totp/disable", handlers.TOTPDisable(logger, deps.Auth, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/auth/vouchers", handlers.CreateAuthVoucher(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/auth/service-tokens", handlers.CreateServiceToken(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/auth/service-tokens", handlers.ListServiceTokens(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/auth/service-tokens/{tokenId}/revoke", handlers.RevokeServiceToken(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/auth/service-tokens/{tokenId}/rotate", handlers.RotateServiceToken(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/users", handlers.CreateUser(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/users", handlers.ListUsers(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Patch("/users/{userId}", handlers.UpdateUser(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/users/{userId}/password-reset-token", handlers.CreatePasswordResetToken(logger, deps.Auth, deps.Store, deps.Mailer, deps.AppPublicURL, deps.TrustProxy))
		r.With(admin).Post("/users/{userId}/invite", handlers.SendUserInvite(logger, deps.Auth, deps.Store, deps.Mailer, deps.AppPublicURL, deps.TrustProxy))

		r.With(deviceViewer).Get("/groups", handlers.ListGroups(logger, deps.Store, deps.TrustProxy))
		r.With(deviceViewer).Get("/groups/{groupId}/deployment-status", handlers.GetGroupDeploymentStatus(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/groups/batch", handlers.BatchGroups(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Put("/groups/{groupId}", handlers.PutGroup(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/groups/{groupId}", handlers.DeleteGroup(logger, deps.Store, deps.TrustProxy))
		r.With(artifactViewer).Get("/artifacts", handlers.ListArtifacts(logger, deps.Store, deps.TrustProxy))
		r.With(artifactViewer).Get("/artifacts/lifecycle/policy", handlers.GetArtifactLifecyclePolicy(logger, deps.Store))
		r.With(artifactViewer).Get("/artifacts/lifecycle/status", handlers.GetArtifactLifecycleStatus(deps.ArtifactLifecycle))
		r.With(admin).Put("/artifacts/lifecycle/policy", handlers.SetArtifactLifecyclePolicy(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/artifacts/lifecycle/prune", handlers.PruneArtifacts(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy, deps.ArtifactLifecycle))
		r.With(operator).Get("/trusted-signing-keys", handlers.ListTrustedSigningKeys(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/trusted-signing-keys", handlers.PutTrustedSigningKey(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Patch("/trusted-signing-keys/{keyId}", handlers.PatchTrustedSigningKey(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/trusted-signing-keys/{keyId}/retire", handlers.RetireTrustedSigningKey(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Get("/artifact-trust/policy", handlers.GetArtifactTrustPolicy(logger, deps.Store, deps.TrustProxy, artifactSigPolicy))
		r.With(admin).Put("/artifact-trust/policy", handlers.PutArtifactTrustPolicy(logger, deps.Store, deps.TrustProxy, artifactSigPolicy))
		r.With(operator).Post("/artifacts", handlers.CreateArtifactWithRealtime(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy, artifactSigPolicy, deps.ReleaseAutoUpdate, deps.Events, vulnScanTrigger, deps.VulnSkipArtifactTypes, sbomGenTrigger, deps.SBOMSkipArtifactTypes))
		r.With(operator).Post("/artifacts/upload", handlers.UploadArtifactWithRealtime(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy, deps.Metrics, artifactSigPolicy, deps.ReleaseAutoUpdate, deps.Events, vulnScanTrigger, deps.VulnSkipArtifactTypes, sbomGenTrigger, deps.SBOMSkipArtifactTypes))
		r.With(artifactPublisher).Post("/artifacts/pull", handlers.PullArtifactWithRealtime(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.ArtifactPullHosts, deps.ArtifactPullMaxBytes, deps.ArtifactPullTimeout, deps.ArtifactPullAllowInsecureHTTP, deps.ArtifactPullCreds, deps.TrustProxy, deps.Metrics, artifactSigPolicy, deps.ReleaseAutoUpdate, deps.Events, vulnScanTrigger, deps.VulnSkipArtifactTypes, sbomGenTrigger, deps.SBOMSkipArtifactTypes))
		r.With(artifactPublisher).Post("/artifacts/presign-upload", handlers.PresignArtifactUpload(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires, deps.TrustProxy, deps.Metrics))
		r.With(artifactPublisher).Post("/artifacts/complete", handlers.CompleteArtifactUploadWithRealtime(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy, deps.Metrics, artifactSigPolicy, deps.ReleaseAutoUpdate, deps.Events, vulnScanTrigger, deps.VulnSkipArtifactTypes, sbomGenTrigger, deps.SBOMSkipArtifactTypes))
		r.With(admin).Get("/artifacts/pull-credentials", handlers.GetPullCredentialStatus(logger, deps.ArtifactPullCredsManager))
		r.With(admin).Post("/artifacts/pull-credentials/reload", handlers.ReloadPullCredentials(logger, deps.Store, deps.ArtifactPullCredsManager, deps.TrustProxy))
		r.With(artifactViewer).Get("/artifacts/{artifactId}", handlers.GetArtifact(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/artifacts/{artifactId}/deprecate", handlers.DeprecateArtifact(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Post("/artifacts/{artifactId}/restore", handlers.RestoreArtifact(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Delete("/artifacts/{artifactId}", handlers.DeleteArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.TrustProxy))
		r.With(artifactViewer).Post("/artifacts/{artifactId}/presign", handlers.PresignArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires, deps.TrustProxy, deps.Metrics))
		r.With(artifactViewer).Post("/artifacts/{artifactId}/sbom/presign", handlers.PresignArtifactSBOM(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires, deps.TrustProxy))
		r.With(operator).Post("/artifacts/{artifactId}/attestations", handlers.PostArtifactAttestation(logger, deps.Store, deps.ArtifactFulcioRootCert, deps.ArtifactRekorURL, deps.ArtifactRequireRekorLog))
		r.With(artifactViewer).Get("/artifacts/{artifactId}/attestations", handlers.ListArtifactAttestations(logger, deps.Store))
		r.With(artifactViewer).Get("/artifacts/{artifactId}/vulnerability-scans", handlers.ListArtifactVulnScans(logger, deps.Store))
		r.With(artifactViewer).Get("/artifacts/{artifactId}/vulnerability-scans/latest", handlers.GetLatestArtifactVulnScan(logger, deps.Store))
		r.With(operator).Post("/artifacts/{artifactId}/vulnerability-scans", handlers.TriggerArtifactVulnScan(logger, deps.Store, vulnScanTrigger))

		r.With(admin).Get("/audit", handlers.ListAuditEvents(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/audit.csv", handlers.ExportAuditCSV(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/audit/retention", handlers.GetAuditRetention(logger, deps.Store))
		r.With(admin).Put("/audit/retention", handlers.SetAuditRetention(logger, deps.Store, deps.TrustProxy, deps.AuditMinRetentionDays))
		r.With(admin).Get("/license", handlers.GetLicenseStatus(logger, deps.Store, deps.License))
		r.With(viewer).Get("/release-auto-update", handlers.GetReleaseAutoUpdateStatus(logger, deps.Store, deps.ReleaseAutoUpdate))
		r.With(admin).Put("/release-auto-update", handlers.SetReleaseAutoUpdateSettings(logger, deps.Store, deps.ReleaseAutoUpdate, deps.TrustProxy))
		r.With(admin).Post("/release-auto-update/run", handlers.RunReleaseAutoUpdate(logger, deps.Store, deps.ReleaseAutoUpdate, deps.TrustProxy))
		r.With(viewer).Get("/cert-rotation", handlers.GetCertRotationStatus(logger, deps.Store, deps.CertManager))
		r.With(operator).Post("/cert-rotation/reload", handlers.ReloadCertRotation(logger, deps.Store, deps.CertManager, deps.TrustProxy))
		r.With(operator).Post("/cert-rotation/rotate", handlers.RotateCertRotation(logger, deps.Store, deps.CertManager, deps.TrustProxy, deps.CertRotationGrace))
		r.With(operator).Post("/cert-rotation/cleanup", handlers.CleanupCertRotation(logger, deps.Store, deps.CertManager, deps.TrustProxy))

		r.With(deviceViewer).Get("/devices", handlers.ListDevices(logger, deps.Store, deps.TrustProxy))
		r.With(deviceViewer).Get("/devices/{deviceId}", handlers.GetDevice(logger, deps.Store, deps.TrustProxy))
		r.With(deviceViewer).Get("/devices/{deviceId}/vulnerability-scans", handlers.ListDeviceVulnScans(logger, deps.Store))
		r.With(deviceViewer).Get("/devices/{deviceId}/vulnerability-scans/latest", handlers.GetLatestDeviceVulnScan(logger, deps.Store))
		r.With(operator).Delete("/devices/{deviceId}", handlers.DeleteDevice(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Post("/devices/{deviceId}/decommission", handlers.DecommissionDevice(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Patch("/devices/{deviceId}", handlers.PatchDevice(logger, deps.Store, deps.TrustProxy))
		deviceIDKey := func(r *http.Request) string {
			return chi.URLParam(r, "deviceId")
		}
		r.Method(http.MethodPost, "/devices/{deviceId}/apply-result", applyLimiter.MiddlewareWithKey(deviceIDKey, handlers.PostApplyResult(logger, deps.Store, deps.Events, deps.TrustProxy, deps.ClientCertHeader, deps.Metrics)))
		var activeCAPool func() *x509.CertPool
		if deps.CertManager != nil {
			activeCAPool = deps.CertManager.ActivePool
		}
		r.With(checkinLimiter.Middleware).Post("/devices/checkin", handlers.DeviceCheckin(logger, deps.Store, deps.Events, deps.TrustProxy, deps.ClientCertHeader, activeCAPool, identityPolicy, deps.Metrics, artifactSigPolicy))
		r.With(checkinLimiter.Middleware).Post("/devices/reenroll", handlers.DeviceReenroll(logger, deps.Store, deps.Signer, deps.TrustProxy, deps.ClientCertHeader))
		r.Get("/devices/artifacts/{artifactId}", handlers.GetAssignedArtifact(logger, deps.Store, deps.TrustProxy, deps.ClientCertHeader))
		r.Post("/devices/artifacts/{artifactId}/presign", handlers.PresignAssignedArtifact(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires, deps.TrustProxy, deps.ClientCertHeader, deps.Metrics))
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
		r.With(requireScopeOrRole(auth.ScopeFederationPush, "operator")).Get("/pending-enrollments", handlers.ListPendingEnrollments(logger, deps.Store, deps.TrustProxy, deps.Metrics))
		r.With(requireScopeOrRole(auth.ScopeFederationPush, "operator")).Post("/pending-enrollments/{requestId}/approve", handlers.ApprovePendingEnrollment(logger, deps.Store, deps.TrustProxy, deps.Metrics))
		r.With(requireScopeOrRole(auth.ScopeFederationPush, "operator")).Post("/pending-enrollments/{requestId}/deny", handlers.DenyPendingEnrollment(logger, deps.Store, deps.TrustProxy, deps.Metrics))
		r.With(operator).Post("/pending-enrollments/{requestId}/reset", handlers.ResetPendingEnrollment(logger, deps.Store, deps.TrustProxy, deps.Metrics))
		r.With(viewer).Get("/desired-state", handlers.ListDesiredState(logger, deps.Store, deps.TrustProxy))
		// In the medical profile the IEC 62304 change-approval gate applies to the
		// standard desired-state routes too — not just the /medical/ aliases.
		putDesiredGroup := handlers.PutDesiredStateGroupWithPolicy(logger, deps.Store, deps.TrustProxy, artifactSigPolicy, deps.ReleaseAutoUpdate)
		putDesiredDevice := handlers.PutDesiredStateDeviceWithPolicy(logger, deps.Store, deps.TrustProxy, artifactSigPolicy, deps.ReleaseAutoUpdate)
		if deps.RequireChangeApproval {
			putDesiredGroup = handlers.PutDesiredStateGroupWithPolicyMedical(logger, deps.Store, deps.TrustProxy, artifactSigPolicy, deps.ReleaseAutoUpdate)
			putDesiredDevice = handlers.PutDesiredStateDeviceWithPolicyMedical(logger, deps.Store, deps.TrustProxy, artifactSigPolicy, deps.ReleaseAutoUpdate)
		}
		r.With(operator).Put("/desired-state/groups/{groupId}", putDesiredGroup)
		r.With(operator).Delete("/desired-state/groups/{groupId}", handlers.DeleteDesiredStateGroup(logger, deps.Store, deps.TrustProxy))
		r.With(operator).Put("/desired-state/devices/{deviceId}", putDesiredDevice)
		r.With(operator).Delete("/desired-state/devices/{deviceId}", handlers.DeleteDesiredStateDevice(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/logs/{deviceId}", handlers.GetDeviceLogs(logger, deps.Store, deps.LogDir, deps.TrustProxy))
		r.With(viewer).Get("/events/history", handlers.ListRuntimeEvents(logger, deps.Store, deps.TrustProxy))
		r.With(admin).Get("/events/retention", handlers.GetRuntimeEventRetention(logger, deps.Store))
		r.With(admin).Put("/events/retention", handlers.SetRuntimeEventRetention(logger, deps.Store, deps.TrustProxy))
		r.With(viewer).Get("/events", handlers.StreamEvents(logger, deps.Events))
		r.With(deviceViewer).Get("/health/summary", handlers.HealthSummaryHandler(logger, deps.Store))
		r.With(viewer).Get("/maintenance", handlers.GetMaintenance(deps.Maintenance))
		r.With(admin).Put("/maintenance", handlers.SetMaintenance(logger, deps.Store, deps.Maintenance, deps.TrustProxy))
		r.With(admin).Get("/maintenance/backups", handlers.ListBackups(deps.BackupDir))
		r.With(admin).Get("/maintenance/backup", handlers.GetBackupStatus(deps.Backup))
		r.With(admin).Post("/maintenance/backup", handlers.StartBackup(logger, deps.Store, deps.Backup, deps.TrustProxy, deps.Metrics))
		r.With(admin).Get("/maintenance/restore", handlers.GetRestoreStatus(deps.Restore))
		r.With(admin).Post("/maintenance/restore", handlers.StartRestore(logger, deps.Store, deps.Restore, deps.Maintenance, deps.BackupDir, deps.TrustProxy, deps.Metrics))
		r.With(admin).Post("/vulnerability-scans/nessus/sync", handlers.TriggerNessusSync(logger, nessusTrigger))
		r.With(admin).Get("/vulnerability-scans/nessus/status", handlers.GetNessusSyncStatus(logger, nessusTrigger))

		// Federation ingest — called by the global plane.
		federationPush := requireScopeOrRole(auth.ScopeFederationPush, "admin")
		r.With(federationPush).Post("/federation/artifacts", handlers.ReceiveFederatedArtifact(deps.Store, logger))
		r.With(federationPush).Get("/federation/artifacts/{artifactId}/blob-status", handlers.GetFederatedBlobStatus(deps.Store, deps.ObjectStore, deps.S3Bucket, logger))
		r.With(federationPush).Post("/federation/policies", handlers.ReceiveFederatedPolicy(deps.Store, logger))
		r.With(federationPush).Get("/federation/policies", handlers.ListFederatedPolicies(deps.Store))
		r.With(federationPush).Delete("/federation/policies/{groupId}", handlers.DeleteFederatedPolicy(deps.Store, logger))

		// Webhook registration and outbound delivery.
		r.With(webhookManager).Post("/webhooks", handlers.CreateWebhook(logger, deps.Store, deps.WebhookEncryptionKey, deps.TrustProxy))
		r.With(webhookManager).Get("/webhooks", handlers.ListWebhooks(logger, deps.Store))
		r.With(webhookManager).Get("/webhooks/{webhookId}", handlers.GetWebhook(logger, deps.Store))
		r.With(webhookManager).Put("/webhooks/{webhookId}", handlers.UpdateWebhook(logger, deps.Store, deps.TrustProxy))
		r.With(webhookManager).Delete("/webhooks/{webhookId}", handlers.DeleteWebhook(logger, deps.Store, deps.TrustProxy))
		r.With(webhookManager).Get("/webhooks/{webhookId}/deliveries", handlers.ListWebhookDeliveries(logger, deps.Store))
		r.With(webhookManager).Post("/webhooks/{webhookId}/deliveries/{deliveryId}/redeliver", handlers.RedeliverWebhookDelivery(logger, deps.Store, deps.WebhookEncryptionKey))
		r.With(webhookManager).Post("/webhooks/{webhookId}/test", handlers.TestWebhook(logger, deps.Store, deps.WebhookEncryptionKey))

		// Deploy triggers — signal devices/groups to apply their desired state ASAP.
		r.With(deploymentTrigger).Post("/devices/{deviceId}/trigger-apply", handlers.TriggerDeviceApply(logger, deps.Store, deps.Events, deps.TrustProxy, deps.TriggerFanoutLimit))
		r.With(deploymentTrigger).Post("/groups/{groupId}/trigger-apply", handlers.TriggerGroupApply(logger, deps.Store, deps.Events, deps.TrustProxy, deps.TriggerFanoutLimit))

		r.With(viewer).Get("/maintenance/upgrade/available", handlers.GetUpgradeAvailable(deps.UpgradeUpdatesDir))
		r.With(viewer).Get("/maintenance/upgrade/preflight", handlers.GetUpgradePreflight(deps.Upgrade, deps.UpgradeUpdatesDir))
		r.With(viewer).Get("/maintenance/upgrade", handlers.GetUpgradeStatus(deps.Upgrade))
		r.With(admin).Post("/maintenance/upgrade", handlers.ApplyUpgrade(logger, deps.Store, deps.Upgrade, deps.Maintenance, deps.TrustProxy, deps.UpgradeUpdatesDir, deps.Metrics))

		// Medical-only subrouter (Phase G). Returns 404 on standard deployments.
		medicalOnly := MedicalOnly(deps.DeploymentProfile)
		r.Route("/medical", func(r chi.Router) {
			r.Use(medicalOnly)
			r.With(viewer).Get("/status", MedicalStatus(deps.DeploymentProfile))

			// IEC 62304 change records (Phase G2).
			r.With(operator).Post("/artifacts/{artifactId}/change-record", handlers.UpsertChangeRecord(logger, deps.Store, deps.TrustProxy))
			r.With(viewer).Get("/artifacts/{artifactId}/change-record", handlers.GetChangeRecord(logger, deps.Store))
			r.With(operator).Post("/artifacts/{artifactId}/change-record/submit", handlers.SubmitChangeRecord(logger, deps.Store, deps.TrustProxy))
			r.With(admin).Post("/artifacts/{artifactId}/change-record/approve", handlers.ApproveChangeRecord(logger, deps.Store, deps.TrustProxy))
			r.With(admin).Post("/artifacts/{artifactId}/change-record/reject", handlers.RejectChangeRecord(logger, deps.Store, deps.TrustProxy))

			// QMS evidence bundle (Phase G5).
			r.With(operator).Post("/artifacts/{artifactId}/qms-package", handlers.PostQMSPackage(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires, deps.TrustProxy))

			// HIPAA audit export (Phase G4).
			r.With(admin).Get("/audit/hipaa-export", handlers.HIPAAExportAudit(logger, deps.Store, deps.TrustProxy))

			// VEX document endpoints (Phase G3).
			r.With(artifactViewer).Post("/artifacts/{artifactId}/sbom/vex/presign", handlers.PostVexPresign(logger, deps.Store, deps.ObjectStore, deps.S3Bucket, deps.PresignExpires, deps.TrustProxy))
			r.With(operator).Post("/artifacts/{artifactId}/sbom/vex/assertions", handlers.UpsertVexAssertion(logger, deps.Store, deps.TrustProxy))

			// Desired-state with change-approval gate (medical profile overrides standard routes).
			r.With(operator).Put("/desired-state/groups/{groupId}", handlers.PutDesiredStateGroupWithPolicyMedical(logger, deps.Store, deps.TrustProxy, artifactSigPolicy, deps.ReleaseAutoUpdate))
			r.With(operator).Put("/desired-state/devices/{deviceId}", handlers.PutDesiredStateDeviceWithPolicyMedical(logger, deps.Store, deps.TrustProxy, artifactSigPolicy, deps.ReleaseAutoUpdate))
		})
	})

	return r
}
