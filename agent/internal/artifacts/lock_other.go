//go:build !unix

package artifacts

import (
	"fmt"
	"os"
	"sync"
)

var (
	heldMu sync.Mutex
	held   = map[string]bool{}
)

// acquirePartLock is the non-unix fallback: in-process exclusion only (the
// agent targets Linux/systemd, where the flock implementation is used).
func acquirePartLock(path string) (func(), error) {
	heldMu.Lock()
	defer heldMu.Unlock()
	if held[path] {
		return nil, fmt.Errorf("another download of this artifact is already in progress")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	held[path] = true
	return func() {
		heldMu.Lock()
		delete(held, path)
		heldMu.Unlock()
		f.Close()
	}, nil
}
