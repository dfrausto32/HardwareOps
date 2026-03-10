package store

import (
	"errors"
	"time"
)

var (
	ErrEnrollmentTokenInvalid     = errors.New("enrollment token invalid or expired")
	ErrDeviceLimitExceeded        = errors.New("device limit exceeded")
	ErrEnrollmentProfileNotFound  = errors.New("enrollment profile not found")
	ErrEnrollmentProfileInvalid   = errors.New("enrollment profile invalid or expired")
	ErrEnrollmentProfileExhausted = errors.New("enrollment profile max uses reached")
	ErrPendingEnrollmentNotFound  = errors.New("pending enrollment not found")
	ErrPendingEnrollmentToken     = errors.New("pending enrollment token invalid")
	ErrPendingEnrollmentState     = errors.New("pending enrollment state invalid")
	ErrPendingEnrollmentThrottled = errors.New("pending enrollment approval throttled")
)

type Device struct {
	DeviceID        string
	CertFingerprint string
	Status          string
	LastSeen        time.Time
	LabelsJSON      []byte
	MetadataJSON    []byte
}

type DeviceState struct {
	DeviceID            string
	CurrentVersion      string
	CurrentConfigRev    string
	ServicesJSON        []byte
	HealthJSON          []byte
	ComponentsJSON      []byte
	UpdatedAt           time.Time
	LastApplyStatus     string
	LastApplyError      string
	LastApplyAt         time.Time
	LastApplyArtifactID string
	LastPreApplyStatus  string
	LastPreApplyError   string
	LastPreApplyAt      time.Time
}

type EnrollmentToken struct {
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type EnrollmentProfile struct {
	ProfileID              string
	Name                   string
	TokenHash              string
	PreviousTokenHash      string
	RequireApproval        bool
	AllowUntrustedHW       bool
	ChallengeHash          string
	ChallengeHint          string
	ApprovalDelaySec       int
	MaxUses                int
	Uses                   int
	ExpiresAt              time.Time
	PreviousTokenExpiresAt time.Time
	TokenRotatedAt         time.Time
	CreatedAt              time.Time
	CreatedBy              string
	Disabled               bool
	DefaultLabelsJSON      []byte
}

type EnrollmentProfileUpdate struct {
	Name              string
	RequireApproval   bool
	AllowUntrustedHW  bool
	ChallengeHash     string
	ChallengeHint     string
	ApprovalDelaySec  int
	MaxUses           int
	DefaultLabelsJSON []byte
}

type PendingEnrollment struct {
	RequestID           string
	ProfileID           string
	Status              string
	CSR                 string
	ClaimTokenHash      string
	CapabilitiesJSON    []byte
	MetadataJSON        []byte
	SourceIP            string
	UserAgent           string
	AgentVersion        string
	HardwareID          string
	DeniedReason        string
	ExpiresAt           time.Time
	ApprovalAvailableAt time.Time
	ApprovedAt          time.Time
	ApprovedByUserID    string
	DeniedAt            time.Time
	DeniedByUserID      string
	IssuedAt            time.Time
	IssuedDeviceID      string
	CreatedAt           time.Time
}

type PendingEnrollmentFilter struct {
	Status string
	Limit  int
	Offset int
}

type Group struct {
	GroupID      string
	Name         string
	SelectorJSON []byte
	CreatedAt    time.Time
}

type Artifact struct {
	ArtifactID   string
	Name         string
	Version      string
	Type         string
	Status       string
	ObjectKey    string
	SHA256       string
	Signature    string
	SizeBytes    int64
	MetadataJSON []byte
	CreatedAt    time.Time
	DeprecatedAt time.Time
	DeleteAfter  time.Time
}

type ArtifactStats struct {
	Count     int
	SizeBytes int64
}

type ArtifactLifecyclePolicy struct {
	DeprecatedDeleteAfterDays int
	UpdatedAt                 time.Time
}

type ReleaseAutoUpdateSettings struct {
	Enabled         bool
	AllowUnsigned   bool
	UpdatedAt       time.Time
	UpdatedByUserID string
}

type ApplyResult struct {
	ApplyID          string
	DeviceID         string
	ArtifactID       string
	Component        string
	Status           string
	AppliedVersion   string
	AppliedConfigRev string
	Error            string
	PreApplyStatus   string
	PreApplyError    string
	CreatedAt        time.Time
}

type AuditEvent struct {
	EventID        string
	OccurredAt     time.Time
	ActorType      string
	ActorID        string
	ActorEmail     string
	ActorRolesJSON []byte
	AuthMethod     string
	SourceIP       string
	UserAgent      string
	RequestID      string
	Action         string
	TargetType     string
	TargetID       string
	Status         string
	Error          string
	BeforeJSON     []byte
	AfterJSON      []byte
	MetadataJSON   []byte
}

type AuditRetention struct {
	Days      int
	UpdatedAt time.Time
}

type RuntimeEvent struct {
	EventID     string
	OccurredAt  time.Time
	Type        string
	DeviceID    string
	PayloadJSON []byte
}

type RuntimeEventFilter struct {
	Type     string
	DeviceID string
	Since    time.Time
	Until    time.Time
	Limit    int
	Offset   int
}

type RuntimeEventRetention struct {
	Days      int
	UpdatedAt time.Time
}

type CertRotationState struct {
	ActiveFingerprint   string
	PreviousFingerprint string
	RotatedAt           time.Time
	GracePeriodSeconds  int64
	CleanedAt           time.Time
	CleanedReason       string
}

type AuditEventFilter struct {
	Action     string
	ActorType  string
	ActorID    string
	ActorEmail string
	TargetType string
	TargetID   string
	Status     string
	Since      time.Time
	Until      time.Time
	Limit      int
	Offset     int
}

type User struct {
	UserID       string
	Email        string
	DisplayName  string
	PasswordHash string
	RolesJSON    []byte
	Disabled     bool
	AuthProvider string
	ExternalID   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  time.Time
}

type UserUpdate struct {
	UserID       string
	DisplayName  *string
	RolesJSON    []byte
	Disabled     *bool
	PasswordHash *string
}

type AuthVoucher struct {
	VoucherID string
	TokenHash string
	Email     string
	RolesJSON []byte
	ExpiresAt time.Time
	CreatedAt time.Time
	CreatedBy string
	UsedAt    time.Time
	UsedBy    string
	Revoked   bool
}

type ServiceToken struct {
	TokenID    string
	Name       string
	TokenHash  string
	ScopesJSON []byte
	ExpiresAt  time.Time
	CreatedAt  time.Time
	CreatedBy  string
	LastUsedAt time.Time
	RevokedAt  time.Time
	RevokedBy  string
}

type DesiredStateGroup struct {
	GroupID          string
	ArtifactID       string
	DesiredVersion   string
	DesiredConfigRev string
	PolicyJSON       []byte
	ComponentsJSON   []byte
	CheckinInterval  int
	UpdatedAt        time.Time
}

type DesiredStateDevice struct {
	DeviceID         string
	ArtifactID       string
	DesiredVersion   string
	DesiredConfigRev string
	PolicyJSON       []byte
	ComponentsJSON   []byte
	CheckinInterval  int
	Source           string
	UpdatedAt        time.Time
}

type ListDevicesFilter struct {
	Status string
	Limit  int
	Offset int
}

type Store interface {
	UpsertDevice(device Device) error
	UpsertDeviceState(state DeviceState) error
	CreateEnrollmentToken(tokenHash string, expiresAt time.Time) error
	ConsumeEnrollmentToken(tokenHash string) (bool, error)
	EnrollDeviceWithToken(tokenHash string, device Device, maxDevices int) error
	CreateEnrollmentProfile(profile EnrollmentProfile) error
	ListEnrollmentProfiles() ([]EnrollmentProfile, error)
	GetEnrollmentProfile(profileID string) (EnrollmentProfile, error)
	UpdateEnrollmentProfile(profileID string, update EnrollmentProfileUpdate) (EnrollmentProfile, error)
	GetEnrollmentProfileByTokenHash(profileTokenHash string) (EnrollmentProfile, error)
	SetEnrollmentProfileDisabled(profileID string, disabled bool) (EnrollmentProfile, error)
	RotateEnrollmentProfileToken(profileID, tokenHash string, previousTokenValidUntil time.Time) (EnrollmentProfile, error)
	CreatePendingEnrollmentForProfileToken(profileTokenHash string, pending PendingEnrollment) (EnrollmentProfile, error)
	ExpirePendingEnrollments(before time.Time) (int, error)
	CountActivePendingEnrollments(profileID, sourceIP string, now time.Time) (int, error)
	ListPendingEnrollments(filter PendingEnrollmentFilter) ([]PendingEnrollment, error)
	ApprovePendingEnrollment(requestID, approvedByUserID string, at time.Time) (PendingEnrollment, error)
	DenyPendingEnrollment(requestID, reason, deniedByUserID string, at time.Time) (PendingEnrollment, error)
	ConflictPendingEnrollment(requestID, reason string, at time.Time) (PendingEnrollment, error)
	ResetPendingEnrollment(requestID string, expiresAt time.Time) (PendingEnrollment, error)
	GetPendingEnrollmentForClaim(requestID, claimTokenHash string) (PendingEnrollment, bool, error)
	MarkPendingEnrollmentIssued(requestID, claimTokenHash string, device Device, maxDevices int, issuedAt time.Time) (PendingEnrollment, error)
	CreateDevice(device Device) error
	GetDevice(deviceID string) (Device, bool, error)
	GetDeviceByFingerprint(fingerprint string) (Device, bool, error)
	GetDeviceByHardwareID(hardwareID string) (Device, bool, error)
	GetDeviceState(deviceID string) (DeviceState, bool, error)
	ListDevices(filter ListDevicesFilter) ([]Device, error)
	CountDevices() (int, error)
	CountDevicesByStatus(status string) (int, error)
	LatestDeviceSeen() (time.Time, error)
	DeleteDevice(deviceID string) error
	DeleteStaleDevices(cutoff time.Time) (int, error)
	UpdateDeviceStatuses(staleCutoff, offlineCutoff time.Time) (int, error)
	UpsertGroup(group Group) error
	ListGroups() ([]Group, error)
	DeleteGroup(groupID string) error
	UpsertDesiredStateGroup(state DesiredStateGroup) error
	UpsertDesiredStateDevice(state DesiredStateDevice) error
	GetDesiredStateDevice(deviceID string) (DesiredStateDevice, bool, error)
	GetDesiredStateGroupForDevice(deviceID string) (DesiredStateGroup, bool, error)
	ListDesiredStateGroups() ([]DesiredStateGroup, error)
	ListDesiredStateDevices() ([]DesiredStateDevice, error)
	DeleteDesiredStateDevice(deviceID string) error
	DeleteDesiredStateGroup(groupID string) error
	CreateArtifact(artifact Artifact) error
	GetArtifact(artifactID string) (Artifact, bool, error)
	ListArtifacts(name, version string, limit, offset int) ([]Artifact, error)
	DeprecateArtifact(artifactID string, deprecatedAt, deleteAfter time.Time) error
	RestoreArtifact(artifactID string) error
	ListArtifactsForPrune(cutoff time.Time, limit int) ([]Artifact, error)
	CountArtifactReferences(artifactID string) (int, error)
	GetArtifactLifecyclePolicy() (ArtifactLifecyclePolicy, error)
	SetArtifactLifecyclePolicy(days int) (ArtifactLifecyclePolicy, error)
	GetReleaseAutoUpdateSettings() (ReleaseAutoUpdateSettings, error)
	SetReleaseAutoUpdateSettings(settings ReleaseAutoUpdateSettings) (ReleaseAutoUpdateSettings, error)
	DeleteArtifact(artifactID string) error
	GetArtifactStats() (ArtifactStats, error)
	CreateApplyResult(result ApplyResult) error
	CreateRuntimeEvent(event RuntimeEvent) error
	ListRuntimeEvents(filter RuntimeEventFilter) ([]RuntimeEvent, error)
	DeleteRuntimeEventsBefore(cutoff time.Time) (int, error)
	EnsureRuntimeEventRetentionDays(days int) error
	GetRuntimeEventRetentionDays() (RuntimeEventRetention, error)
	SetRuntimeEventRetentionDays(days int) (RuntimeEventRetention, error)
	CreateAuditEvent(event AuditEvent) error
	ListAuditEvents(filter AuditEventFilter) ([]AuditEvent, error)
	DeleteAuditEventsBefore(cutoff time.Time) (int, error)
	EnsureAuditRetentionDays(days int) error
	GetAuditRetentionDays() (AuditRetention, error)
	SetAuditRetentionDays(days int) (AuditRetention, error)
	GetCertRotationState() (CertRotationState, bool, error)
	SetCertRotationState(state CertRotationState) error
	CreateUser(user User) error
	GetUser(userID string) (User, bool, error)
	GetUserByEmail(email string) (User, bool, error)
	GetUserByExternalID(provider, externalID string) (User, bool, error)
	ListUsers(limit, offset int) ([]User, error)
	UpdateUser(update UserUpdate) error
	SetUserLastLogin(userID string, at time.Time) error
	CreateAuthVoucher(voucher AuthVoucher) error
	GetAuthVoucherByTokenHash(tokenHash string) (AuthVoucher, bool, error)
	MarkAuthVoucherUsed(voucherID, usedBy string, at time.Time) (bool, error)
	CreateServiceToken(token ServiceToken) error
	GetServiceToken(tokenID string) (ServiceToken, bool, error)
	GetServiceTokenByTokenHash(tokenHash string) (ServiceToken, bool, error)
	ListServiceTokens(limit, offset int) ([]ServiceToken, error)
	SetServiceTokenLastUsed(tokenID string, at time.Time) error
	RevokeServiceToken(tokenID, revokedBy string, at time.Time) (bool, error)
}
