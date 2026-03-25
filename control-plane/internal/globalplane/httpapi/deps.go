package httpapi

import (
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/globalplane"
	"github.com/hardwareops/control-plane/internal/globalplane/sync"
)

// Dependencies holds all runtime dependencies for the global-plane HTTP layer.
type Dependencies struct {
	Store              globalplane.Store
	SyncManager        *sync.Manager
	Auth               *auth.Manager
	TokenEncryptionKey []byte
	CORSAllowedOrigins []string
}
