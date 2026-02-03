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
	DeviceID         string
	CurrentVersion   string
	CurrentConfigRev string
	ServicesJSON     []byte
	HealthJSON       []byte
	UpdatedAt        time.Time
	LastApplyStatus  string
	LastApplyError   string
	LastApplyAt      time.Time
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
	ArtifactID string
	Name       string
	Version    string
	ObjectKey  string
	SHA256     string
	Signature  string
	SizeBytes  int64
	CreatedAt  time.Time
}

type ApplyResult struct {
	ApplyID          string
	DeviceID         string
	Status           string
	AppliedVersion   string
	AppliedConfigRev string
	Error            string
	CreatedAt        time.Time
}

type DesiredStateGroup struct {
	GroupID          string
	ArtifactID       string
	DesiredVersion   string
	DesiredConfigRev string
	PolicyJSON       []byte
	CheckinInterval  int
	UpdatedAt        time.Time
}

type DesiredStateDevice struct {
	DeviceID         string
	ArtifactID       string
	DesiredVersion   string
	DesiredConfigRev string
	PolicyJSON       []byte
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
	UpsertGroup(group Group) error
	ListGroups() ([]Group, error)
	UpsertDesiredStateGroup(state DesiredStateGroup) error
	UpsertDesiredStateDevice(state DesiredStateDevice) error
	GetDesiredStateDevice(deviceID string) (DesiredStateDevice, bool, error)
	GetDesiredStateGroupForDevice(deviceID string) (DesiredStateGroup, bool, error)
	ListDesiredStateGroups() ([]DesiredStateGroup, error)
	ListDesiredStateDevices() ([]DesiredStateDevice, error)
	CreateArtifact(artifact Artifact) error
	GetArtifact(artifactID string) (Artifact, bool, error)
	ListArtifacts(name, version string, limit, offset int) ([]Artifact, error)
	CreateApplyResult(result ApplyResult) error
}
