package artifacts

import (
	"fmt"
	"time"
)

type ApplyOptions struct {
	// RefreshDownloadURL returns a fresh download locator when the current
	// presigned URL has expired mid-transfer (401/403). Optional.
	RefreshDownloadURL func() (string, error)
	// PartialMaxAge / PartialMaxBytes bound abandoned partial downloads kept
	// for resume; zero selects defaults (7 days, 2 GiB).
	PartialMaxAge   time.Duration
	PartialMaxBytes int64
	// Transfer controls for constrained links (roadmap T3); zero = unlimited.
	TransferWindow *TransferWindow // downloads only start inside this window; nil = always
	MaxBytesPerSec int64           // pace downloads to at most this rate
	ByteBudget     int64 // max bytes fetched per apply; the rest resumes next check-in
	// OnProgress is called with (committed, total) as bytes are persisted.
	OnProgress func(committed, total int64)

	AllowUnsupported      bool
	SigningPublicKeyPath  string
	SigningKeyID          string
	RequireSignature      bool
	VerificationMode      string
	AllowedSigningKeyIDs  []string
	AllowedSignatureTypes []string
	TrustKeys             []TrustKey
}

type TrustKey struct {
	KeyID        string
	Algorithm    string
	PublicKeyPEM string
}

type ApplyOutcome struct {
	PreApplyStatus string
	PreApplyError  string
}

type ApplyContext struct {
	Root       string
	VersionDir string
	Desired    Desired
	Meta       ArtifactMeta
	Manifest   Manifest
	Logger     Logger
}

type FirmwareApplier interface {
	ApplyFirmware(ctx ApplyContext) error
}

type ContainerImageApplier interface {
	ApplyContainerImage(ctx ApplyContext) error
}

type ErrApplyNotImplemented struct {
	Type string
}

func (e ErrApplyNotImplemented) Error() string {
	return fmt.Sprintf("%s apply not implemented", e.Type)
}

type firmwareApplierStub struct{}

func (firmwareApplierStub) ApplyFirmware(_ ApplyContext) error {
	return ErrApplyNotImplemented{Type: "firmware"}
}

type containerImageApplierStub struct{}

func (containerImageApplierStub) ApplyContainerImage(_ ApplyContext) error {
	return ErrApplyNotImplemented{Type: "container_image"}
}

var defaultFirmwareApplier FirmwareApplier = firmwareApplierStub{}
var defaultContainerImageApplier ContainerImageApplier = containerImageApplierStub{}

// SetFirmwareApplier replaces the stub with a real implementation (Phase F2).
// Called once at agent startup when firmware flashing is configured.
func SetFirmwareApplier(applier FirmwareApplier) {
	if applier != nil {
		defaultFirmwareApplier = applier
	}
}
