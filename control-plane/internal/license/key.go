package license

// EmbeddedPublicKey can be injected at build time via:
// -ldflags "-X github.com/hardwareops/control-plane/internal/license.EmbeddedPublicKey=<b64>"
// If set, it enables "locked" license verification without env overrides.
var EmbeddedPublicKey string
