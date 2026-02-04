package artifacts

import "fmt"

type ApplyOptions struct {
	AllowUnsupported bool
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
