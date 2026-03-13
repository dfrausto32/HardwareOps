package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/metrics"
	"github.com/hardwareops/control-plane/internal/store"
)

func effectiveDesiredComponentsForDevice(st store.Store, deviceID string) (map[string]DesiredComponentResponse, error) {
	desired, _, err := st.GetDesiredStateDevice(deviceID)
	if err != nil {
		return nil, err
	}
	groupDesired, _, err := st.GetDesiredStateGroupForDevice(deviceID)
	if err != nil {
		return nil, err
	}

	desiredComponents := mergeLegacyDesiredComponents(
		decodeDesiredComponents(desired.ComponentsJSON),
		desired.ArtifactID,
		desired.DesiredVersion,
		desired.DesiredConfigRev,
		desired.PolicyJSON,
		desired.Source,
	)
	groupComponents := mergeLegacyDesiredComponents(
		decodeDesiredComponents(groupDesired.ComponentsJSON),
		groupDesired.ArtifactID,
		groupDesired.DesiredVersion,
		groupDesired.DesiredConfigRev,
		groupDesired.PolicyJSON,
		"",
	)

	out := map[string]DesiredComponentResponse{}
	keys := map[string]struct{}{}
	for key := range desiredComponents {
		keys[key] = struct{}{}
	}
	for key := range groupComponents {
		keys[key] = struct{}{}
	}
	for key := range keys {
		if comp, ok := desiredComponents[key]; ok && comp.Source == "manual" {
			out[key] = comp
			continue
		}
		if comp, ok := groupComponents[key]; ok && desiredComponentAssigned(comp) {
			out[key] = comp
			continue
		}
		if comp, ok := desiredComponents[key]; ok {
			out[key] = comp
		}
	}
	return out, nil
}

func desiredComponentAssigned(comp DesiredComponentResponse) bool {
	return comp.ArtifactID != "" || comp.DesiredVersion != "" || comp.DesiredConfigRev != "" || len(comp.Policy) != 0
}

func deviceAssignedArtifact(st store.Store, deviceID, artifactID string) (bool, error) {
	components, err := effectiveDesiredComponentsForDevice(st, deviceID)
	if err != nil {
		return false, err
	}
	for _, comp := range components {
		if comp.ArtifactID == artifactID {
			return true, nil
		}
	}
	return false, nil
}

func GetAssignedArtifact(logger *log.Logger, st store.Store, trustProxy bool, clientCertHeader string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		device, err := deviceFromMTLS(r, st, trustProxy, clientCertHeader)
		if err != nil {
			deviceArtifactAuthError(logger, w, err)
			return
		}

		artifactID := chi.URLParam(r, "artifactId")
		if artifactID == "" {
			http.Error(w, "artifactId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(artifactID); err != nil {
			http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
			return
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

		refs, err := st.CountArtifactReferences(artifactID)
		if err != nil {
			logger.Printf("count artifact refs error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		resp := artifactToResponse(artifact, refs)
		event := buildAuditEvent(r, trustProxy, actorDevice(device.DeviceID), "artifact.device_read", "artifact", artifactID)
		event.MetadataJSON = auditJSON(map[string]any{
			"name":     artifact.Name,
			"version":  artifact.Version,
			"type":     artifact.Type,
			"deviceId": device.DeviceID,
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func PresignAssignedArtifact(logger *log.Logger, st store.Store, objStore ObjectStore, bucket string, expires time.Duration, trustProxy bool, clientCertHeader string, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status string) {
			if metricsCollector != nil {
				metricsCollector.IncArtifactPresign(status)
			}
		}

		device, err := deviceFromMTLS(r, st, trustProxy, clientCertHeader)
		if err != nil {
			record("error")
			deviceArtifactAuthError(logger, w, err)
			return
		}

		artifactID := chi.URLParam(r, "artifactId")
		if artifactID == "" {
			record("error")
			http.Error(w, "artifactId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(artifactID); err != nil {
			record("error")
			http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
			return
		}
		if objStore == nil || bucket == "" {
			record("error")
			http.Error(w, "object store not configured", http.StatusInternalServerError)
			return
		}

		allowed, err := deviceAssignedArtifact(st, device.DeviceID, artifactID)
		if err != nil {
			logger.Printf("resolve device artifact assignment error: %v", err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !allowed {
			record("error")
			http.Error(w, "artifact not assigned", http.StatusForbidden)
			return
		}

		artifact, ok, err := st.GetArtifact(artifactID)
		if err != nil {
			logger.Printf("get artifact error: %v", err)
			record("error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			record("error")
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		exp := expires
		if exp <= 0 {
			exp = 15 * time.Minute
		}
		downloadURL, err := objStore.PresignGet(r.Context(), bucket, artifact.ObjectKey, exp)
		if err != nil {
			logger.Printf("presign error: %v", err)
			record("error")
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorDevice(device.DeviceID), "artifact.device_presign", "artifact", artifactID), err)
			http.Error(w, "presign error", http.StatusInternalServerError)
			return
		}
		record("success")

		event := buildAuditEvent(r, trustProxy, actorDevice(device.DeviceID), "artifact.device_presign", "artifact", artifactID)
		event.MetadataJSON = auditJSON(map[string]any{
			"deviceId":  device.DeviceID,
			"expiresAt": time.Now().UTC().Add(exp),
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PresignResponse{
			DownloadURL: downloadURL,
			ExpiresAt:   time.Now().UTC().Add(exp),
		})
	}
}

func deviceArtifactAuthError(logger *log.Logger, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUnknownDeviceCert):
		logger.Printf("device artifact auth failed: unknown device certificate")
		http.Error(w, "unknown device certificate", http.StatusUnauthorized)
	case errors.Is(err, errClientCertRequired):
		logger.Printf("device artifact auth failed: client certificate required")
		http.Error(w, "client certificate required", http.StatusUnauthorized)
	default:
		logger.Printf("device artifact auth failed: %v", err)
		http.Error(w, "client certificate required", http.StatusUnauthorized)
	}
}
