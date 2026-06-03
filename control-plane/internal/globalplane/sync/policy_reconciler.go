package sync

import (
	"context"
	"log"
	"time"

	"github.com/parcel/control-plane/internal/globalplane"
)

// policyReconciler periodically re-pushes global desired-state policies to any
// regional plane that missed the original fan-out or whose last push failed.
// It runs alongside the replicationReconciler in the sync Manager.
type policyReconciler struct {
	store      globalplane.Store
	encKey     []byte
	logger     *log.Logger
	interval   time.Duration
	staleAfter time.Duration // re-push even on success if older than this
}

func newPolicyReconciler(store globalplane.Store, encKey []byte, logger *log.Logger, interval time.Duration) *policyReconciler {
	if interval <= 0 {
		interval = 90 * time.Second
	}
	return &policyReconciler{
		store:      store,
		encKey:     encKey,
		logger:     logger,
		interval:   interval,
		staleAfter: 10 * time.Minute,
	}
}

func (pr *policyReconciler) run(ctx context.Context) {
	ticker := time.NewTicker(pr.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pr.reconcile(ctx)
		}
	}
}

func (pr *policyReconciler) reconcile(ctx context.Context) {
	allStates, err := pr.store.ListGlobalDesiredStatesWithGroups()
	if err != nil {
		pr.logger.Printf("[global-plane] policy reconciler: list desired states: %v", err)
		return
	}
	planes, err := pr.store.ListRegionalPlanes()
	if err != nil {
		pr.logger.Printf("[global-plane] policy reconciler: list planes: %v", err)
		return
	}
	syncStatuses, err := pr.store.ListAllPolicySyncStatus()
	if err != nil {
		pr.logger.Printf("[global-plane] policy reconciler: list sync status: %v", err)
		return
	}

	// Index existing sync statuses by "groupID:planeID".
	statusMap := make(map[string]globalplane.PolicySyncStatus, len(syncStatuses))
	for _, s := range syncStatuses {
		statusMap[s.GroupID+":"+s.PlaneID] = s
	}

	for _, state := range allStates {
		// Skip groups with no desired state configured.
		if state.GlobalDesiredState.ArtifactID == "" && state.GlobalDesiredState.DesiredVersion == "" {
			continue
		}
		for _, plane := range planes {
			if !plane.Enabled {
				continue
			}
			key := state.GlobalGroup.GroupID + ":" + plane.PlaneID
			st, exists := statusMap[key]

			// Skip if recently pushed without error.
			if exists && st.PushError == "" && st.PushedAt != nil && time.Since(*st.PushedAt) < pr.staleAfter {
				continue
			}

			// Apply exponential backoff for consecutive failures.
			if exists && st.PushError != "" && st.LastAttemptAt != nil {
				delay := policyRetryDelay(st.RetryCount)
				if time.Since(*st.LastAttemptAt) < delay {
					continue
				}
			}

			// Compute retry count for this attempt.
			newRetryCount := 1
			if exists && st.PushError != "" {
				newRetryCount = st.RetryCount + 1
			}

			go pr.pushOne(ctx, plane, state.GlobalGroup, state.GlobalDesiredState, newRetryCount)
		}
	}
}

func (pr *policyReconciler) pushOne(
	ctx context.Context,
	plane globalplane.RegionalPlane,
	group globalplane.GlobalGroup,
	state globalplane.GlobalDesiredState,
	retryCount int,
) {
	token, err := globalplane.DecryptToken(plane.EncryptedToken, pr.encKey)
	if err != nil {
		pr.logger.Printf("[global-plane] policy reconciler: decrypt token for %s: %v", plane.Name, err)
		_ = pr.store.UpsertPolicySyncStatus(group.GroupID, plane.PlaneID, nil, "decrypt token: "+err.Error(), retryCount)
		return
	}
	client, err := NewRegionalClient(plane.BaseURL, token, plane.TLSCAPem)
	if err != nil {
		_ = pr.store.UpsertPolicySyncStatus(group.GroupID, plane.PlaneID, nil, "build client: "+err.Error(), retryCount)
		return
	}
	payload := FederationPolicyPayload{
		GroupID:          group.GroupID,
		GroupName:        group.Name,
		SelectorJSON:     group.SelectorJSON,
		ArtifactID:       state.ArtifactID,
		DesiredVersion:   state.DesiredVersion,
		DesiredConfigRev: state.DesiredConfigRev,
		PolicyJSON:       state.PolicyJSON,
		ComponentsJSON:   state.ComponentsJSON,
		CheckinInterval:  state.CheckinInterval,
	}
	pushCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	pushErr := client.PushPolicy(pushCtx, payload)
	now := time.Now()
	if pushErr != nil {
		_ = pr.store.UpsertPolicySyncStatus(group.GroupID, plane.PlaneID, nil, pushErr.Error(), retryCount)
		pr.logger.Printf("[global-plane] policy reconciler: push group %s to plane %s (attempt %d): %v",
			group.GroupID, plane.Name, retryCount, pushErr)
		return
	}
	_ = pr.store.UpsertPolicySyncStatus(group.GroupID, plane.PlaneID, &now, "", 0)
	pr.logger.Printf("[global-plane] policy reconciler: pushed group %s to plane %s", group.GroupID, plane.Name)
}

// policyRetryDelay returns the backoff delay for the given retry count.
// Sequence: 30s, 60s, 120s, 240s, … capped at 30 minutes.
func policyRetryDelay(retryCount int) time.Duration {
	if retryCount <= 0 {
		return 0
	}
	shift := retryCount - 1
	if shift > 5 {
		shift = 5 // cap at 2^5 * 30s = 960s ≈ 16 min; next cap at 30 min
	}
	delay := time.Duration(1<<uint(shift)) * 30 * time.Second
	if delay > 30*time.Minute {
		delay = 30 * time.Minute
	}
	return delay
}
