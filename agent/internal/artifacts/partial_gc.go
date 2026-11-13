package artifacts

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultPartialMaxAge   = 7 * 24 * time.Hour
	defaultPartialMaxBytes = int64(2) << 30 // 2 GiB across abandoned partials
)

type partialEntry struct {
	partPath string
	mtime    time.Time
	size     int64
}

// pruneStalePartials removes abandoned partial downloads in dir so interrupted
// transfers cannot exhaust device storage. A partial is abandoned when it has
// not grown for maxAge; if the survivors still exceed maxBytes, the least
// recently written are removed first. keepPart (the partial about to be
// used) and any partial currently locked by another downloader are never
// touched. Best-effort: errors are ignored.
func pruneStalePartials(dir, keepPart string, maxAge time.Duration, maxBytes int64) {
	if maxAge <= 0 {
		maxAge = defaultPartialMaxAge
	}
	if maxBytes <= 0 {
		maxBytes = defaultPartialMaxBytes
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	var live []partialEntry
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		full := filepath.Join(dir, name)
		if full == keepPart {
			continue
		}
		switch {
		case strings.HasSuffix(name, ".part"):
			fi, err := e.Info()
			if err != nil {
				continue
			}
			if now.Sub(fi.ModTime()) > maxAge {
				removePartial(full)
				continue
			}
			live = append(live, partialEntry{full, fi.ModTime(), fi.Size()})
		case strings.HasSuffix(name, ".part.meta"), strings.HasSuffix(name, ".part.lock"):
			// Orphaned sidecar/lock whose .part is gone.
			base := strings.TrimSuffix(strings.TrimSuffix(full, ".meta"), ".lock")
			if _, err := os.Stat(base); os.IsNotExist(err) {
				if fi, err := e.Info(); err == nil && now.Sub(fi.ModTime()) > maxAge {
					_ = os.Remove(full)
				}
			}
		}
	}

	var total int64
	for _, p := range live {
		total += p.size
	}
	if total <= maxBytes {
		return
	}
	sort.Slice(live, func(i, j int) bool { return live[i].mtime.Before(live[j].mtime) })
	for _, p := range live {
		if total <= maxBytes {
			return
		}
		if removePartial(p.partPath) {
			total -= p.size
		}
	}
}

// removePartial deletes a partial and its sidecars unless another downloader
// holds its lock. Reports whether it was removed.
func removePartial(partPath string) bool {
	release, err := acquirePartLock(partPath + ".lock")
	if err != nil {
		return false
	}
	_ = os.Remove(partPath)
	_ = os.Remove(partPath + ".meta")
	_ = os.Remove(partPath + ".lock")
	release()
	return true
}
