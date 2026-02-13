package store

import "time"

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
	ObjectKey    string
	SHA256       string
	Signature    string
	SizeBytes    int64
	MetadataJSON []byte
	CreatedAt    time.Time
}

type ArtifactStats struct {
	Count     int
	SizeBytes int64
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
	CreateDevice(device Device) error
	GetDevice(deviceID string) (Device, bool, error)
	GetDeviceByFingerprint(fingerprint string) (Device, bool, error)
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
	DeleteArtifact(artifactID string) error
	GetArtifactStats() (ArtifactStats, error)
	CreateApplyResult(result ApplyResult) error
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
	ListUsers(limit, offset int) ([]User, error)
	UpdateUser(update UserUpdate) error
	SetUserLastLogin(userID string, at time.Time) error
	CreateAuthVoucher(voucher AuthVoucher) error
	GetAuthVoucherByTokenHash(tokenHash string) (AuthVoucher, bool, error)
	MarkAuthVoucherUsed(voucherID, usedBy string, at time.Time) (bool, error)
}
