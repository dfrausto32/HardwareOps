package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
)

// Per-chunk hash manifest for resumable downloads (roadmap T4).
//
// A device that finishes a download whose full SHA-256 does not match can use
// this manifest to find which fixed-size chunks are corrupt and re-fetch only
// those, instead of discarding the whole file. The manifest is an
// optimisation and is deliberately NOT part of the signed material: the
// device still verifies the full SHA-256 (and the signature over it) before
// applying anything, so a wrong manifest can only cause extra re-downloads or
// a failed apply, never acceptance of bad content.
const (
	defaultChunkSize = 1 << 20  // 1 MiB
	minChunkSize     = 64 << 10 // 64 KiB
	maxChunkSize     = 8 << 20  // 8 MiB
	chunkCacheMax    = 128      // manifests kept in memory
	maxChunkCount    = 1 << 20  // refuse absurd manifests (≈64 MiB of hex)
)

// ChunkManifest is the response for GET /devices/artifacts/{id}/chunks.
type ChunkManifest struct {
	ArtifactID string   `json:"artifactId"`
	SHA256     string   `json:"sha256"`
	Size       int64    `json:"size"`
	ChunkSize  int      `json:"chunkSize"`
	Chunks     []string `json:"chunks"` // lowercase hex SHA-256 of each chunk; last may be short
}

// chunkManifestCache memoises manifests (artifact content is immutable, so
// they never go stale) and de-duplicates concurrent computations so a fleet
// asking at once costs one pass over the object.
type chunkManifestCache struct {
	mu       sync.Mutex
	done     map[string]*ChunkManifest
	order    []string
	inflight map[string]*chunkCall
}

type chunkCall struct {
	wg  sync.WaitGroup
	val *ChunkManifest
	err error
}

func newChunkManifestCache() *chunkManifestCache {
	return &chunkManifestCache{done: map[string]*ChunkManifest{}, inflight: map[string]*chunkCall{}}
}

func (c *chunkManifestCache) get(key string, compute func() (*ChunkManifest, error)) (*ChunkManifest, error) {
	c.mu.Lock()
	if m, ok := c.done[key]; ok {
		c.mu.Unlock()
		return m, nil
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		call.wg.Wait()
		return call.val, call.err
	}
	call := &chunkCall{}
	call.wg.Add(1)
	c.inflight[key] = call
	c.mu.Unlock()

	call.val, call.err = compute()

	c.mu.Lock()
	delete(c.inflight, key)
	if call.err == nil {
		c.done[key] = call.val
		c.order = append(c.order, key)
		if len(c.order) > chunkCacheMax {
			delete(c.done, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.mu.Unlock()
	call.wg.Done()
	return call.val, call.err
}

// computeChunkManifest streams r once, hashing each chunkSize block and the
// whole stream (constant memory regardless of object size).
func computeChunkManifest(r io.Reader, chunkSize int) (chunks []string, fullSHA string, size int64, err error) {
	buf := make([]byte, chunkSize)
	full := sha256.New()
	for {
		n, rerr := io.ReadFull(r, buf)
		if n > 0 {
			sum := sha256.Sum256(buf[:n])
			chunks = append(chunks, hex.EncodeToString(sum[:]))
			full.Write(buf[:n])
			size += int64(n)
			if len(chunks) > maxChunkCount {
				return nil, "", 0, fmt.Errorf("object too large for chunk size %d", chunkSize)
			}
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			return nil, "", 0, rerr
		}
	}
	return chunks, hex.EncodeToString(full.Sum(nil)), size, nil
}

// GetAssignedArtifactChunks serves the chunk manifest for an artifact assigned
// to the calling device (mTLS), computing it on first request.
func GetAssignedArtifactChunks(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, trustProxy bool, clientCertHeader string) http.HandlerFunc {
	cache := newChunkManifestCache()
	return func(w http.ResponseWriter, r *http.Request) {
		device, err := deviceFromMTLS(r, st, trustProxy, clientCertHeader)
		if err != nil {
			deviceArtifactAuthError(logger, w, err)
			return
		}
		artifactID := chi.URLParam(r, "artifactId")
		if _, err := uuid.Parse(artifactID); err != nil {
			http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
			return
		}
		if objStore == nil || bucket == "" {
			http.Error(w, "object store not configured", http.StatusInternalServerError)
			return
		}
		chunkSize := defaultChunkSize
		if raw := strings.TrimSpace(r.URL.Query().Get("chunkSize")); raw != "" {
			n, err := strconv.Atoi(raw)
			// Powers of two only: bounds the number of distinct manifests (and
			// full passes over the object) a client can force the server to compute.
			if err != nil || n < minChunkSize || n > maxChunkSize || n&(n-1) != 0 {
				http.Error(w, fmt.Sprintf("chunkSize must be a power of two between %d and %d", minChunkSize, maxChunkSize), http.StatusBadRequest)
				return
			}
			chunkSize = n
		}

		allowed, err := deviceAssignedArtifact(st, device.DeviceID, artifactID)
		if err != nil {
			logger.Printf("resolve device artifact assignment error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "artifact not assigned", http.StatusForbidden)
			return
		}
		artifact, ok, err := st.GetArtifact(artifactID)
		if err != nil {
			logger.Printf("get artifact error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		key := strings.ToLower(artifact.SHA256) + ":" + strconv.Itoa(chunkSize)
		// Compute detached from this request: a slow device link may give up
		// before the pass over a large object finishes, and the next retry
		// should find the finished manifest rather than restart the work.
		bg := context.WithoutCancel(r.Context())
		manifest, err := cache.get(key, func() (*ChunkManifest, error) {
			rc, err := objStore.GetObject(bg, bucket, artifact.ObjectKey)
			if err != nil {
				return nil, fmt.Errorf("object read error")
			}
			defer rc.Close()
			chunks, fullSHA, size, err := computeChunkManifest(rc, chunkSize)
			if err != nil {
				return nil, fmt.Errorf("object read error")
			}
			// Never publish a manifest for content that is not the registered
			// artifact (object replaced/corrupted at rest).
			if !strings.EqualFold(fullSHA, strings.TrimSpace(artifact.SHA256)) ||
				(artifact.SizeBytes > 0 && size != artifact.SizeBytes) {
				return nil, fmt.Errorf("artifact object does not match registered sha256/size")
			}
			return &ChunkManifest{ArtifactID: artifactID, SHA256: fullSHA, Size: size, ChunkSize: chunkSize, Chunks: chunks}, nil
		})
		if err != nil {
			logger.Printf("chunk manifest error artifact=%s: %v", artifactID, err)
			http.Error(w, "chunk manifest unavailable", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorDevice(device.DeviceID), "artifact.device_chunks", "artifact", artifactID)
		event.MetadataJSON = auditJSON(map[string]any{"deviceId": device.DeviceID, "chunkSize": chunkSize, "chunks": len(manifest.Chunks)})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manifest)
	}
}
