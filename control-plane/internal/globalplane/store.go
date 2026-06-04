package globalplane

import (
	"time"

	"github.com/parcel/control-plane/internal/store"
)

// RegionalPlane is a registered regional control plane.
type RegionalPlane struct {
	PlaneID              string
	Name                 string
	BaseURL              string
	EncryptedToken       []byte // AES-256-GCM encrypted service token
	TLSCAPem             string // optional PEM CA cert for self-signed regional CPs
	Enabled              bool
	SyncIntervalSeconds  int
	LastSyncAt           *time.Time
	LastSyncError        string
	CreatedBy            string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// CachedDevice is a device snapshot pulled from a regional plane.
type CachedDevice struct {
	CacheID  string
	PlaneID  string
	DeviceID string
	Status   string
	LastSeen *time.Time
	Labels   map[string]string
	Metadata map[string]string
	SyncedAt time.Time
}

// CachedArtifact is an artifact metadata snapshot pulled from a regional plane.
type CachedArtifact struct {
	CacheID      string
	PlaneID      string
	ArtifactID   string
	Name         string
	Version      string
	ArtifactType string
	Status       string
	SHA256       string
	SizeBytes    int64
	CreatedAt    *time.Time
	SyncedAt     time.Time
}

// HealthSnapshot is the health summary for a single regional plane.
type HealthSnapshot struct {
	PlaneID        string
	TotalDevices   int
	ActiveDevices  int
	StaleDevices   int
	OfflineDevices int
	DegradedDevices int
	LastDeviceSeen *time.Time
	SyncedAt       time.Time
}

// DeviceCacheFilter controls which cached devices are returned.
type DeviceCacheFilter struct {
	PlaneID string // empty = all planes
	Status  string // empty = all statuses
}

// ArtifactCacheFilter controls which cached artifacts are returned.
type ArtifactCacheFilter struct {
	PlaneID string // empty = all planes
	Name    string // empty = all names
}

// GlobalArtifact is an artifact uploaded directly to the global plane.
type GlobalArtifact struct {
	ArtifactID      string
	Name            string
	Version         string
	ArtifactType    string
	Status          string
	ObjectKey       string
	SHA256          string
	Signature       string
	SignatureType   string
	SignatureKeyID  string
	SizeBytes       int64
	MetadataJSON    []byte
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ArtifactReplicationStatus tracks per-artifact per-region blob replication state.
// BlobStatus values: pending | replicating | confirmed | failed
type ArtifactReplicationStatus struct {
	ReplicationID      string
	ArtifactID         string
	PlaneID            string
	PlaneName          string // populated on read by joining regional_planes
	MetadataPushedAt   *time.Time
	MetadataPushError  string
	BlobStatus         string
	BlobConfirmedAt    *time.Time
	BlobCheckError     string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// GlobalGroup is an operator-defined group on the global plane with a label selector.
type GlobalGroup struct {
	GroupID      string    `json:"groupId"`
	Name         string    `json:"name"`
	SelectorJSON []byte    `json:"selectorJson,omitempty"`
	CreatedBy    string    `json:"createdBy,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// GlobalDesiredState is the desired artifact state for a global group.
type GlobalDesiredState struct {
	GroupID          string    `json:"groupId"`
	ArtifactID       string    `json:"artifactId,omitempty"`
	DesiredVersion   string    `json:"desiredVersion,omitempty"`
	DesiredConfigRev string    `json:"desiredConfigRev,omitempty"`
	PolicyJSON       []byte    `json:"policyJson,omitempty"`
	ComponentsJSON   []byte    `json:"componentsJson,omitempty"`
	CheckinInterval  int       `json:"checkinInterval,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt"`
	UpdatedBy        string    `json:"updatedBy,omitempty"`
}

// GlobalDesiredStateWithGroup pairs a group with its desired state for fan-out.
type GlobalDesiredStateWithGroup struct {
	GlobalGroup
	GlobalDesiredState
}

// GlobalEnrollmentProfile is an enrollment profile created on the global plane.
type GlobalEnrollmentProfile struct {
	ProfileID         string    `json:"profileId"`
	Name              string    `json:"name"`
	RequireApproval   bool      `json:"requireApproval"`
	AllowUntrustedHW  bool      `json:"allowUntrustedHw"`
	ChallengeHint     string    `json:"challengeHint,omitempty"`
	ApprovalDelaySec  int       `json:"approvalDelaySec,omitempty"`
	MaxUses           int       `json:"maxUses,omitempty"`
	CertValidityDays  int       `json:"certValidityDays,omitempty"`
	DefaultLabelsJSON []byte    `json:"defaultLabels,omitempty"`
	Disabled          bool      `json:"disabled"`
	CreatedBy         string    `json:"createdBy,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// GlobalEnrollmentProfileUpdate carries the mutable fields of a global enrollment profile.
type GlobalEnrollmentProfileUpdate struct {
	Name              string
	RequireApproval   bool
	AllowUntrustedHW  bool
	ChallengeHint     string
	ApprovalDelaySec  int
	MaxUses           int
	CertValidityDays  int
	DefaultLabelsJSON []byte
}

// GlobalPendingEnrollment is a pending enrollment cached from a regional plane.
type GlobalPendingEnrollment struct {
	CacheID             string
	PlaneID             string
	PlaneName           string // populated on read via join with regional_planes
	RequestID           string
	ProfileID           string
	Status              string
	SourceIP            string
	AgentVersion        string
	HardwareID          string
	MetadataJSON        []byte
	CapabilitiesJSON    []byte
	DeniedReason        string
	ExpiresAt           *time.Time
	ApprovalAvailableAt *time.Time
	CreatedAt           time.Time
	SyncedAt            time.Time
}

// GlobalPendingEnrollmentFilter controls list results.
type GlobalPendingEnrollmentFilter struct {
	Status  string // empty = all statuses
	PlaneID string // empty = all planes
}

// PolicySyncStatus tracks the last policy push result for a (group, plane) pair.
// The reconciler uses this to detect missed fan-outs and schedule retries.
type PolicySyncStatus struct {
	SyncID        string
	GroupID       string
	PlaneID       string
	PushedAt      *time.Time // last successful push; nil if never succeeded
	PushError     string     // non-empty if last attempt failed
	RetryCount    int        // consecutive failure count; reset to 0 on success
	LastAttemptAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Store is the data access interface for the global aggregation plane.
type Store interface {
	// Regional plane registry
	CreateRegionalPlane(p RegionalPlane) error
	GetRegionalPlane(planeID string) (RegionalPlane, bool, error)
	ListRegionalPlanes() ([]RegionalPlane, error)
	UpdateRegionalPlane(planeID string, name, baseURL string, encryptedToken []byte, tlsCAPem string, syncIntervalSeconds int) error
	UpdateRegionalPlaneSyncStatus(planeID string, syncedAt time.Time, syncErr string) error
	DeleteRegionalPlane(planeID string) error

	// Device directory cache
	UpsertDeviceCacheEntries(planeID string, devices []CachedDevice) error
	ListDeviceCache(filter DeviceCacheFilter) ([]CachedDevice, error)
	PurgeDeviceCacheForPlane(planeID string) error

	// Artifact cache
	UpsertArtifactCacheEntries(planeID string, artifacts []CachedArtifact) error
	ListArtifactCache(filter ArtifactCacheFilter) ([]CachedArtifact, error)

	// Health cache
	UpsertHealthCache(snapshot HealthSnapshot) error
	ListHealthCache() ([]HealthSnapshot, error)

	// Global artifacts (federated upload)
	CreateGlobalArtifact(a GlobalArtifact) error
	GetGlobalArtifact(artifactID string) (GlobalArtifact, bool, error)
	ListGlobalArtifacts(nameFilter string) ([]GlobalArtifact, error)

	// Artifact replication status
	CreateReplicationStatusRows(artifactID string, planeIDs []string) error
	UpdateReplicationStatusMetadataPush(artifactID, planeID string, pushedAt *time.Time, pushErr string) error
	UpdateReplicationStatusBlobConfirmed(artifactID, planeID string, confirmedAt time.Time) error
	UpdateReplicationStatusBlobCheckError(artifactID, planeID, checkErr string) error
	ListReplicationStatus(artifactID string) ([]ArtifactReplicationStatus, error)
	ListPendingReplicationRows() ([]ArtifactReplicationStatus, error)

	// Global groups
	CreateGlobalGroup(g GlobalGroup) (GlobalGroup, error)
	GetGlobalGroup(groupID string) (GlobalGroup, bool, error)
	ListGlobalGroups() ([]GlobalGroup, error)
	DeleteGlobalGroup(groupID string) error

	// Global desired state
	UpsertGlobalDesiredState(state GlobalDesiredState) error
	GetGlobalDesiredState(groupID string) (GlobalDesiredState, bool, error)
	ListGlobalDesiredStatesWithGroups() ([]GlobalDesiredStateWithGroup, error)
	DeleteGlobalDesiredState(groupID string) error

	// Policy sync status (E4 reconciler)
	UpsertPolicySyncStatus(groupID, planeID string, pushedAt *time.Time, pushErr string, retryCount int) error
	GetPolicySyncStatus(groupID, planeID string) (PolicySyncStatus, bool, error)
	ListAllPolicySyncStatus() ([]PolicySyncStatus, error)

	// Global enrollment profiles (E5)
	CreateGlobalEnrollmentProfile(p GlobalEnrollmentProfile) (GlobalEnrollmentProfile, error)
	GetGlobalEnrollmentProfile(profileID string) (GlobalEnrollmentProfile, bool, error)
	ListGlobalEnrollmentProfiles() ([]GlobalEnrollmentProfile, error)
	UpdateGlobalEnrollmentProfile(profileID string, u GlobalEnrollmentProfileUpdate) (GlobalEnrollmentProfile, error)
	DeleteGlobalEnrollmentProfile(profileID string) error

	// Pending enrollment cache (E5 reconciler)
	UpsertGlobalPendingEnrollments(planeID string, items []GlobalPendingEnrollment) error
	PurgeGlobalPendingEnrollmentsForPlane(planeID string) error
	ListGlobalPendingEnrollments(filter GlobalPendingEnrollmentFilter) ([]GlobalPendingEnrollment, error)

	// Audit
	CreateAuditEvent(event store.AuditEvent) error
	ListAuditEvents(filter store.AuditEventFilter) ([]store.AuditEvent, error)
	DeleteAuditEventsBefore(cutoff time.Time) (int, error)

	// OIDC user management (satisfies auth.OIDCStore)
	GetUserByExternalID(provider, externalID string) (store.User, bool, error)
	CreateUser(user store.User) error
	UpdateUser(update store.UserUpdate) error
}
