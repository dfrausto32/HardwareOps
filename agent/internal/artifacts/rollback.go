package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
)

// RollbackToVersion switches the current symlink back to a prior version.
func RollbackToVersion(root, version string) error {
	if version == "" {
		return fmt.Errorf("previous version empty")
	}
	versionDir := filepath.Join(root, "versions", version)
	if _, err := os.Stat(versionDir); err != nil {
		return fmt.Errorf("previous version not found: %w", err)
	}
	current := filepath.Join(root, "current")
	_, err := switchSymlink(current, versionDir)
	return err
}
