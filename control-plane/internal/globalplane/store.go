package globalplane

import "time"

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
}
