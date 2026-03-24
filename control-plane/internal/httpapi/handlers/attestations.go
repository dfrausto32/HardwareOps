package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hardwareops/control-plane/internal/artifacttrust"
	"github.com/hardwareops/control-plane/internal/store"
)

// PostAttestationRequest is the JSON body for submitting an attestation.
type PostAttestationRequest struct {
	// PredicateType is the in-toto predicate type URI (e.g. "https://slsa.dev/provenance/v1").
	PredicateType string `json:"predicateType"`
	// Payload is the raw attestation payload (in-toto statement JSON or similar).
	Payload json.RawMessage `json:"payload"`
	// Signature is the optional base64-encoded signature over the payload.
	Signature string `json:"signature,omitempty"`
	// SignatureType is the signature algorithm ("ed25519", "cosign", "keyless").
	SignatureType string `json:"signatureType,omitempty"`
	// SignatureKeyID is the ID of the signing key (for keyed signatures).
	SignatureKeyID string `json:"signatureKeyId,omitempty"`
	// KeylessBundle is the JSON-encoded KeylessCosignBundle for keyless verification.
	// Required when signatureType == "keyless".
	KeylessBundle string `json:"keylessBundle,omitempty"`
}

// AttestationResponse is the JSON representation of a stored attestation.
type AttestationResponse struct {
	AttestationID  string          `json:"attestationId"`
	ArtifactID     string          `json:"artifactId"`
	PredicateType  string          `json:"predicateType"`
	Payload        json.RawMessage `json:"payload"`
	Signature      string          `json:"signature,omitempty"`
	SignatureType  string          `json:"signatureType,omitempty"`
	SignatureKeyID string          `json:"signatureKeyId,omitempty"`
	BuilderID      string          `json:"builderId,omitempty"`
	BuilderIssuer  string          `json:"builderIssuer,omitempty"`
	VerifiedAt     *time.Time      `json:"verifiedAt,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}

func attestationToResponse(rec store.AttestationRecord) AttestationResponse {
	resp := AttestationResponse{
		AttestationID:  rec.AttestationID,
		ArtifactID:     rec.ArtifactID,
		PredicateType:  rec.PredicateType,
		Payload:        rec.PayloadJSON,
		Signature:      rec.Signature,
		SignatureType:  rec.SignatureType,
		SignatureKeyID: rec.SignatureKeyID,
		BuilderID:      rec.BuilderID,
		BuilderIssuer:  rec.BuilderIssuer,
		CreatedAt:      rec.CreatedAt,
	}
	if !rec.VerifiedAt.IsZero() {
		t := rec.VerifiedAt
		resp.VerifiedAt = &t
	}
	return resp
}

// PostArtifactAttestation accepts an in-toto / SLSA attestation for an artifact.
// If signatureType == "keyless", it performs Sigstore keyless verification and
// stores the extracted builder identity. Otherwise the attestation is stored
// as-is for audit purposes.
func PostArtifactAttestation(
	logger *log.Logger,
	st store.Store,
	fulcioRootCert, rekorURL string,
	requireRekorLog bool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")

		// Verify the artifact exists.
		artifact, ok, err := st.GetArtifact(artifactID)
		if err != nil {
			logger.Printf("PostArtifactAttestation: get artifact %s: %v", artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}

		var req PostAttestationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.PredicateType == "" {
			http.Error(w, "predicateType is required", http.StatusBadRequest)
			return
		}
		if len(req.Payload) == 0 {
			http.Error(w, "payload is required", http.StatusBadRequest)
			return
		}

		rec := store.AttestationRecord{
			ArtifactID:     artifactID,
			PredicateType:  req.PredicateType,
			PayloadJSON:    req.Payload,
			Signature:      req.Signature,
			SignatureType:  req.SignatureType,
			SignatureKeyID: req.SignatureKeyID,
		}

		// Perform keyless verification if requested.
		if req.SignatureType == artifacttrust.SignatureTypeKeyless {
			if req.KeylessBundle == "" {
				http.Error(w, "keylessBundle is required for keyless signatureType", http.StatusBadRequest)
				return
			}
			opts := artifacttrust.KeylessVerifyOptions{
				FulcioRootCertPEM: fulcioRootCert,
				RekorURL:          rekorURL,
				RequireRekorLog:   requireRekorLog,
			}
			builderID, builderIssuer, err := artifacttrust.VerifyArtifactKeyless(
				req.KeylessBundle, artifact.SHA256, opts,
			)
			if err != nil {
				http.Error(w, "keyless verification failed: "+err.Error(), http.StatusUnprocessableEntity)
				return
			}
			rec.BuilderID = builderID
			rec.BuilderIssuer = builderIssuer
			now := time.Now().UTC()
			rec.VerifiedAt = now
		}

		if err := st.CreateAttestation(rec); err != nil {
			logger.Printf("PostArtifactAttestation: create attestation for %s: %v", artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(attestationToResponse(rec))
	}
}

// ListArtifactAttestations returns all attestations for an artifact.
func ListArtifactAttestations(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID := chi.URLParam(r, "artifactId")

		if _, ok, err := st.GetArtifact(artifactID); err != nil {
			logger.Printf("ListArtifactAttestations: get artifact %s: %v", artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}

		recs, err := st.ListAttestations(artifactID)
		if err != nil {
			logger.Printf("ListArtifactAttestations: list for %s: %v", artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		resp := make([]AttestationResponse, 0, len(recs))
		for _, rec := range recs {
			resp = append(resp, attestationToResponse(rec))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
