//go:build unix

package artifacts

import (
	"fmt"
	"os"
	"syscall"
)

// acquirePartLock takes a non-blocking exclusive flock on path. The kernel
// drops the lock if the process dies, so a crashed agent never leaves a stale
// lock behind (unlike an O_EXCL lock file).
func acquirePartLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, fmt.Errorf("another download of this artifact is already in progress")
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
