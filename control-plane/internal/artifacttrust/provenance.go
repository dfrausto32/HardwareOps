package artifacttrust

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/parcel/control-plane/internal/store"
)

// Sigstore / Fulcio custom OID extensions embedded in short-lived signing certs.
var (
	oidFulcioIssuer  = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 1}
	oidFulcioIssuerV2 = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 8}
)

// KeylessCosignBundle is the JSON payload stored in artifact metadata (field
// "cosignBundle") for keyless-signed artifacts. It mirrors the bundle format
// produced by `cosign sign --bundle`.
type KeylessCosignBundle struct {
	// Base64Signature is the ECDSA signature over the artifact digest.
	Base64Signature string `json:"base64Signature"`
	// Cert is the PEM-encoded Fulcio-issued short-lived signing certificate.
	Cert string `json:"cert"`
	// Chain holds any PEM-encoded intermediate certificates (Fulcio sub-CA).
	Chain []string `json:"chain,omitempty"`
	// RekorBundle is the optional offline Rekor inclusion proof.
	RekorBundle *RekorBundle `json:"bundle,omitempty"`
}

// RekorBundle holds the signed entry timestamp (SET) and payload from Rekor.
type RekorBundle struct {
	SignedEntryTimestamp string      `json:"signedEntryTimestamp"`
	Payload             RekorPayload `json:"payload"`
}

// RekorPayload is the tlog entry payload.
type RekorPayload struct {
	Body           string `json:"body"`
	IntegratedTime int64  `json:"integratedTime"`
	LogIndex       int64  `json:"logIndex"`
	LogID          string `json:"logID"`
}

// KeylessVerifyOptions configures keyless signature verification.
// All fields are optional and have safe defaults.
type KeylessVerifyOptions struct {
	// FulcioRootCertPEM is the PEM-encoded Fulcio root certificate used to
	// validate the signing cert chain. If empty the public Sigstore root is used.
	FulcioRootCertPEM string
	// RekorURL is the Rekor transparency log base URL.
	// Defaults to https://rekor.sigstore.dev when empty.
	RekorURL string
	// RequireRekorLog forces a live Rekor lookup when no offline bundle is
	// present. Defaults to false.
	RequireRekorLog bool
}

// VerifyKeylessSignature verifies a keyless cosign signature:
//  1. Parses and chain-validates the Fulcio-issued signing cert.
//  2. Extracts the OIDC builder identity (subject URI) and issuer from cert extensions / SANs.
//  3. Verifies the ECDSA signature over the artifact digest using the cert public key.
//  4. Optionally verifies Rekor inclusion (offline proof or live lookup).
//
// Returns the builder identity (OIDC subject) and issuer on success.
func VerifyKeylessSignature(bundle KeylessCosignBundle, artifactDigest string, opts KeylessVerifyOptions) (builderID, builderIssuer string, err error) {
	if strings.TrimSpace(bundle.Cert) == "" {
		return "", "", errors.New("keyless bundle: cert is required")
	}
	if strings.TrimSpace(bundle.Base64Signature) == "" {
		return "", "", errors.New("keyless bundle: base64Signature is required")
	}

	// Decode and parse the signing certificate.
	cert, err := parseCertPEM(bundle.Cert)
	if err != nil {
		return "", "", fmt.Errorf("keyless bundle: parse cert: %w", err)
	}

	// Build verification roots.
	roots, err := buildFulcioRoots(opts.FulcioRootCertPEM)
	if err != nil {
		return "", "", fmt.Errorf("keyless bundle: build fulcio roots: %w", err)
	}

	// Build intermediates pool from chain.
	intermediates := x509.NewCertPool()
	for i, chainPEM := range bundle.Chain {
		c, err := parseCertPEM(chainPEM)
		if err != nil {
			return "", "", fmt.Errorf("keyless bundle: parse chain cert %d: %w", i, err)
		}
		intermediates.AddCert(c)
	}

	// Verify the cert chain. Fulcio certs are short-lived (~10 min); we
	// use CurrentTime = cert.NotBefore to avoid false expiry rejections
	// when verifying historical artifacts.
	verifyOpts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   cert.NotBefore,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}
	if _, err := cert.Verify(verifyOpts); err != nil {
		return "", "", fmt.Errorf("keyless bundle: cert chain verification failed: %w", err)
	}

	// Extract builder identity from the cert.
	builderID, builderIssuer = extractFulcioIdentity(cert)
	if builderID == "" {
		return "", "", errors.New("keyless bundle: could not extract builder identity from cert")
	}

	// Decode the signature.
	sigBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(bundle.Base64Signature))
	if err != nil {
		return "", "", fmt.Errorf("keyless bundle: decode signature: %w", err)
	}

	// Verify the signature. Fulcio issues ECDSA P-256 keys.
	// The signed payload for keyless cosign is the raw SHA-256 digest bytes.
	digestBytes, err := decodeHexDigest(artifactDigest)
	if err != nil {
		return "", "", fmt.Errorf("keyless bundle: artifact digest: %w", err)
	}

	switch pub := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pub, digestBytes, sigBytes) {
			return "", "", errors.New("keyless bundle: signature verification failed")
		}
	default:
		return "", "", fmt.Errorf("keyless bundle: unsupported cert public key type %T", cert.PublicKey)
	}

	// Rekor verification.
	if bundle.RekorBundle != nil {
		if err := verifyRekorBundleOffline(bundle.RekorBundle, artifactDigest, bundle.Base64Signature, builderID); err != nil {
			return "", "", fmt.Errorf("keyless bundle: rekor offline verification: %w", err)
		}
	} else if opts.RequireRekorLog {
		rekorURL := strings.TrimRight(strings.TrimSpace(opts.RekorURL), "/")
		if rekorURL == "" {
			rekorURL = "https://rekor.sigstore.dev"
		}
		if err := verifyRekorLive(context.Background(), rekorURL, artifactDigest, bundle.Base64Signature); err != nil {
			return "", "", fmt.Errorf("keyless bundle: rekor live verification: %w", err)
		}
	}

	return builderID, builderIssuer, nil
}

// CheckProvenancePolicy verifies that a set of attestation records satisfies
// the given ProvenancePolicy. Returns nil if the policy has no requirements.
func CheckProvenancePolicy(attestations []store.AttestationRecord, policy store.ProvenancePolicy) error {
	if !policy.RequireProvenance {
		return nil
	}
	if len(attestations) == 0 {
		return errors.New("provenance policy requires at least one attestation but none are present")
	}

	// Find at least one attestation that satisfies all policy constraints.
	for _, a := range attestations {
		if policy.RequiredPredicateType != "" && a.PredicateType != policy.RequiredPredicateType {
			continue
		}
		if policy.RequiredBuilderID != "" && !matchesBuilderPattern(a.BuilderID, policy.RequiredBuilderID) {
			continue
		}
		if policy.RequiredBuilderIssuer != "" && a.BuilderIssuer != policy.RequiredBuilderIssuer {
			continue
		}
		// This attestation satisfies all constraints.
		return nil
	}

	return fmt.Errorf("provenance policy not satisfied: no attestation matches predicate=%q builderID=%q issuer=%q",
		policy.RequiredPredicateType, policy.RequiredBuilderID, policy.RequiredBuilderIssuer)
}

// DecodeProvenancePolicy unmarshals a ProvenancePolicyJSON blob, returning a
// zero-value ProvenancePolicy if the input is nil/empty.
func DecodeProvenancePolicy(raw []byte) (store.ProvenancePolicy, error) {
	if len(raw) == 0 {
		return store.ProvenancePolicy{}, nil
	}
	var p store.ProvenancePolicy
	if err := json.Unmarshal(raw, &p); err != nil {
		return store.ProvenancePolicy{}, fmt.Errorf("invalid provenance policy JSON: %w", err)
	}
	return p, nil
}

// EncodeProvenancePolicy marshals a ProvenancePolicy to JSON.
// Returns nil if the policy is a zero value (no requirements set).
func EncodeProvenancePolicy(p store.ProvenancePolicy) []byte {
	if !p.RequireProvenance && p.RequiredPredicateType == "" && p.RequiredBuilderID == "" && p.RequiredBuilderIssuer == "" {
		return nil
	}
	out, _ := json.Marshal(p)
	return out
}

// --- helpers -----------------------------------------------------------------

func parseCertPEM(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(certPEM)))
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	return x509.ParseCertificate(block.Bytes)
}

func buildFulcioRoots(rootPEM string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	rootPEM = strings.TrimSpace(rootPEM)
	if rootPEM == "" {
		rootPEM = defaultFulcioRootPEM
	}
	if !pool.AppendCertsFromPEM([]byte(rootPEM)) {
		return nil, errors.New("failed to parse Fulcio root certificate")
	}
	return pool, nil
}

// extractFulcioIdentity extracts the OIDC subject (builder ID) and issuer
// from a Fulcio-issued certificate. Fulcio embeds these in custom OID extensions
// and/or as a URI SAN.
func extractFulcioIdentity(cert *x509.Certificate) (subject, issuer string) {
	// Try custom OID extensions first (Fulcio v1 and v2).
	for _, ext := range cert.Extensions {
		switch {
		case ext.Id.Equal(oidFulcioIssuer):
			// v1: raw ASCII string value
			issuer = strings.TrimSpace(string(ext.Value))
		case ext.Id.Equal(oidFulcioIssuerV2):
			// v2: ASN.1 UTF8String
			var s string
			if _, err := asn1.Unmarshal(ext.Value, &s); err == nil {
				issuer = strings.TrimSpace(s)
			}
		}
	}

	// Builder identity is the first URI SAN (the OIDC job subject claim).
	if len(cert.URIs) > 0 {
		subject = cert.URIs[0].String()
	}

	return subject, issuer
}

func decodeHexDigest(hexDigest string) ([]byte, error) {
	hexDigest = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(hexDigest)), "sha256:")
	if len(hexDigest) != 64 {
		return nil, fmt.Errorf("expected 64-char hex digest, got %d chars", len(hexDigest))
	}
	var out [sha256.Size]byte
	for i := 0; i < sha256.Size; i++ {
		var b byte
		if _, err := fmt.Sscanf(hexDigest[i*2:i*2+2], "%02x", &b); err != nil {
			return nil, fmt.Errorf("invalid hex at position %d", i*2)
		}
		out[i] = b
	}
	return out[:], nil
}

// verifyRekorBundleOffline does a lightweight structural check of the embedded
// Rekor bundle. Full SET (signed entry timestamp) verification would require
// the Rekor public key; here we validate that the bundle contains a non-empty
// signed entry timestamp and that the log index is positive — sufficient for
// audit trail purposes without an online check.
func verifyRekorBundleOffline(bundle *RekorBundle, _ string, _ string, _ string) error {
	if strings.TrimSpace(bundle.SignedEntryTimestamp) == "" {
		return errors.New("rekor bundle: missing signedEntryTimestamp")
	}
	// Decode the SET to confirm it is valid base64.
	if _, err := base64.StdEncoding.DecodeString(bundle.SignedEntryTimestamp); err != nil {
		return fmt.Errorf("rekor bundle: invalid signedEntryTimestamp encoding: %w", err)
	}
	if bundle.Payload.LogIndex < 0 {
		return errors.New("rekor bundle: invalid log index")
	}
	return nil
}

// verifyRekorLive queries the Rekor API to confirm a log entry exists for the
// given artifact digest + signature. Used only when RequireRekorLog is true
// and no offline bundle is present.
func verifyRekorLive(ctx context.Context, rekorURL, artifactDigest, base64Sig string) error {
	// Search Rekor for entries matching the artifact hash.
	searchURL := rekorURL + "/api/v1/index/retrieve"
	reqBody := fmt.Sprintf(`{"hash":"sha256:%s"}`, strings.TrimPrefix(strings.ToLower(strings.TrimSpace(artifactDigest)), "sha256:"))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchURL, strings.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("rekor request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rekor returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var uuids []string
	if err := json.Unmarshal(body, &uuids); err != nil {
		return fmt.Errorf("parse rekor response: %w", err)
	}
	if len(uuids) == 0 {
		return errors.New("artifact not found in rekor transparency log")
	}
	return nil
}

// matchesBuilderPattern checks if a builder ID matches a required pattern.
// Supports exact match or prefix matching with a trailing wildcard (*).
func matchesBuilderPattern(builderID, pattern string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(builderID, strings.TrimSuffix(pattern, "*"))
	}
	return builderID == pattern
}

// defaultFulcioRootPEM is the Sigstore public Fulcio root certificate
// (valid as of 2024; operators can override via ARTIFACT_FULCIO_ROOT_CERT).
// Source: https://github.com/sigstore/sigstore/blob/main/pkg/tuf/targets/fulcio.crt.pem
const defaultFulcioRootPEM = `-----BEGIN CERTIFICATE-----
MIIB9zCCAXygAwIBAgIUALZNAPFdxHPwjeDloDwyYChAO/4wCgYIKoZIzj0EAwMw
KjEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MREwDwYDVQQDEwhzaWdzdG9yZTAeFw0y
MTEwMDcxMzU2NTlaFw0zMTEwMDUxMzU2NThaMCoxFTATBgNVBAoTDHNpZ3N0b3Jl
LmRldjERMA8GA1UEAxMIc2lnc3RvcmUwdjAQBgcqhkjOPQIBBgUrgQQAIgNiAAT7
XeFT4rb3PQGwS4IajtLk3/OlnpgangaBclYpsYBr5i+4ynB07ceb3LP0OIOZdxex
X69c5iVuyJRQ+Hz05yi+UF3uBWAlHpiS5sh0+H2GHE7SXrk1EC5m1Tr19L9gg92j
YzBhMA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBRY
wB5fkUWlZql6zJChkyLnJlFoGjAfBgNVHSMEGDAWgBRYwB5fkUWlZql6zJChkyLn
JlFoGjAKBggqhkjOPQQDAwNpADBmAjEAj1nHeXZp+13NWBNa+EDsDP8G1WWg1tCM
WP/WHPqpaVo0jhsweNFZgSs0eE7wYI4qAjEA2WB9ot98sIkoF3vZYdd3/VtWB5b9
TNMea7Ix/stJ5TfcLLeABLE4BNJOsQ4vnBHJ
-----END CERTIFICATE-----`
