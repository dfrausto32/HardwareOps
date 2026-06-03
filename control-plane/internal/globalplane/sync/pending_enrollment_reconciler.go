package sync

import (
	"context"
	"log"
	"time"

	"github.com/parcel/control-plane/internal/globalplane"
)

// pendingEnrollmentReconciler polls all enabled regional planes for pending
// enrollments and caches them in global_pending_enrollments_cache for unified
// approval from the global UI.
type pendingEnrollmentReconciler struct {
	store    globalplane.Store
	encKey   []byte
	logger   *log.Logger
	interval time.Duration
}

func newPendingEnrollmentReconciler(store globalplane.Store, encKey []byte, logger *log.Logger, interval time.Duration) *pendingEnrollmentReconciler {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &pendingEnrollmentReconciler{
		store:    store,
		encKey:   encKey,
		logger:   logger,
		interval: interval,
	}
}

func (r *pendingEnrollmentReconciler) run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *pendingEnrollmentReconciler) reconcile(ctx context.Context) {
	planes, err := r.store.ListRegionalPlanes()
	if err != nil {
		r.logger.Printf("[global-plane] pending enrollment reconciler: list planes: %v", err)
		return
	}
	for _, plane := range planes {
		if !plane.Enabled {
			continue
		}
		go r.pollPlane(ctx, plane)
	}
}

func (r *pendingEnrollmentReconciler) pollPlane(ctx context.Context, plane globalplane.RegionalPlane) {
	token, err := globalplane.DecryptToken(plane.EncryptedToken, r.encKey)
	if err != nil {
		r.logger.Printf("[global-plane] pending enrollment reconciler: decrypt token for %s: %v", plane.Name, err)
		return
	}
	client, err := newRegionalClient(plane.BaseURL, token, plane.TLSCAPem)
	if err != nil {
		return
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	rows, err := client.FetchPendingEnrollments(fetchCtx)
	if err != nil {
		r.logger.Printf("[global-plane] pending enrollment reconciler: fetch from %s: %v", plane.Name, err)
		return
	}

	// Replace cache for this plane.
	if err := r.store.PurgeGlobalPendingEnrollmentsForPlane(plane.PlaneID); err != nil {
		r.logger.Printf("[global-plane] pending enrollment reconciler: purge for %s: %v", plane.Name, err)
		return
	}
	if len(rows) == 0 {
		return
	}

	items := make([]globalplane.GlobalPendingEnrollment, len(rows))
	for i, row := range rows {
		items[i] = globalplane.GlobalPendingEnrollment{
			PlaneID:             plane.PlaneID,
			RequestID:           row.RequestID,
			ProfileID:           row.ProfileID,
			Status:              row.Status,
			SourceIP:            row.SourceIP,
			AgentVersion:        row.AgentVersion,
			HardwareID:          row.HardwareID,
			MetadataJSON:        []byte(row.MetadataJSON),
			CapabilitiesJSON:    []byte(row.CapabilitiesJSON),
			DeniedReason:        row.DeniedReason,
			ExpiresAt:           row.ExpiresAt,
			ApprovalAvailableAt: row.ApprovalAvailableAt,
			CreatedAt:           row.CreatedAt,
		}
	}
	if err := r.store.UpsertGlobalPendingEnrollments(plane.PlaneID, items); err != nil {
		r.logger.Printf("[global-plane] pending enrollment reconciler: upsert for %s: %v", plane.Name, err)
		return
	}
	r.logger.Printf("[global-plane] pending enrollment reconciler: cached %d pending enrollments from %s", len(items), plane.Name)
}
