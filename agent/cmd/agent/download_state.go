package main

import (
	"time"

	"github.com/parcel/agent/internal/state"
)

// heartbeatHook, when set by the main loop, sends a throttled check-in while a
// long download runs so the control plane sees fresh progress (and the device
// does not look offline for the duration of the transfer). Nil in tests.
var heartbeatHook func(st *state.State)

// currentDownload returns the component's Download record if it belongs to
// artifactID, else a fresh one (a new artifact resets timers and attempts).
func currentDownload(cs state.ComponentState, artifactID string, now time.Time) *state.DownloadState {
	if d := cs.Download; d != nil && d.ArtifactID == artifactID {
		cp := *d
		return &cp
	}
	return &state.DownloadState{ArtifactID: artifactID, StartedAt: now, LastProgressAt: now}
}

// beginDownload marks the start of a download attempt for a component.
func beginDownload(st *state.State, component, artifactID string) {
	st.EnsureComponents()
	now := time.Now().UTC()
	cs := st.Components[component]
	d := currentDownload(cs, artifactID, now)
	d.State = state.DownloadStateDownloading
	d.Reason = ""
	d.Attempts++
	cs.Download = d
	st.Components[component] = cs
}

// recordDownloadProgress updates byte counters; LastProgressAt only advances
// when bytes actually advanced, so operators can tell stalled from progressing.
func recordDownloadProgress(st *state.State, component, artifactID string, done, total int64) {
	st.EnsureComponents()
	now := time.Now().UTC()
	cs := st.Components[component]
	d := currentDownload(cs, artifactID, now)
	if done > d.BytesDone {
		d.LastProgressAt = now
	}
	d.State = state.DownloadStateDownloading
	d.BytesDone = done
	if total > 0 {
		d.BytesTotal = total
	}
	cs.Download = d
	st.Components[component] = cs
}

// deferDownload records that a download is waiting (outside the transfer
// window, byte budget reached, link stalled). reason must be a short,
// non-sensitive string: it is sent to the control plane, so never pass raw
// error text (HTTP errors can embed presigned URLs).
func deferDownload(st *state.State, component, artifactID, reason string, done, total int64) {
	st.EnsureComponents()
	now := time.Now().UTC()
	cs := st.Components[component]
	d := currentDownload(cs, artifactID, now)
	d.State = state.DownloadStateWaiting
	d.Reason = reason
	if done > d.BytesDone {
		d.BytesDone = done
		d.LastProgressAt = now
	}
	if total > 0 {
		d.BytesTotal = total
	}
	cs.Download = d
	st.Components[component] = cs
}
