package sync

import (
	"context"
	"log"
	"time"

	"github.com/parcel/control-plane/internal/globalplane"
)

// replicationReconciler polls pending/replicating blob statuses and marks
// them confirmed once the regional plane reports local presence.
type replicationReconciler struct {
	store   globalplane.Store
	encKey  []byte
	logger  *log.Logger
	interval time.Duration
}

func newReplicationReconciler(store globalplane.Store, encKey []byte, logger *log.Logger, interval time.Duration) *replicationReconciler {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &replicationReconciler{store: store, encKey: encKey, logger: logger, interval: interval}
}

func (rr *replicationReconciler) run(ctx context.Context) {
	ticker := time.NewTicker(rr.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rr.reconcile(ctx)
		}
	}
}

func (rr *replicationReconciler) reconcile(ctx context.Context) {
	pending, err := rr.store.ListPendingReplicationRows()
	if err != nil {
		rr.logger.Printf("[global-plane] replication reconciler: list pending: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	// Build a plane lookup for decrypting tokens.
	planes, err := rr.store.ListRegionalPlanes()
	if err != nil {
		rr.logger.Printf("[global-plane] replication reconciler: list planes: %v", err)
		return
	}
	planeMap := make(map[string]globalplane.RegionalPlane, len(planes))
	for _, p := range planes {
		planeMap[p.PlaneID] = p
	}

	for _, row := range pending {
		plane, ok := planeMap[row.PlaneID]
		if !ok {
			continue
		}
		token, err := globalplane.DecryptToken(plane.EncryptedToken, rr.encKey)
		if err != nil {
			rr.logger.Printf("[global-plane] replication reconciler: decrypt token for %s: %v", plane.Name, err)
			continue
		}
		client, err := newRegionalClient(plane.BaseURL, token, plane.TLSCAPem)
		if err != nil {
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		confirmed, checkErr := client.GetBlobStatus(checkCtx, row.ArtifactID)
		cancel()
		if checkErr != nil {
			_ = rr.store.UpdateReplicationStatusBlobCheckError(row.ArtifactID, row.PlaneID, checkErr.Error())
			continue
		}
		if confirmed {
			now := time.Now()
			_ = rr.store.UpdateReplicationStatusBlobConfirmed(row.ArtifactID, row.PlaneID, now)
			rr.logger.Printf("[global-plane] artifact %s confirmed in plane %s", row.ArtifactID, plane.Name)
		}
	}
}
