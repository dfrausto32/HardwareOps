package handlers

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hardwareops/control-plane/internal/artifacttrust"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestTrustedSigningKeyHandlers_CreateListPatchRetire(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	pemKey := generateTrustedKeyFixture(t)
	keyID, err := artifacttrust.ComputeKeyIDFromPublicKeyPEM(pemKey)
	if err != nil {
		t.Fatalf("compute key id: %v", err)
	}

	createBody, _ := json.Marshal(map[string]any{
		"displayName":  "release key",
		"algorithm":    "ed25519",
		"publicKeyPem": pemKey,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/trusted-signing-keys", bytes.NewReader(createBody))
	createW := httptest.NewRecorder()
	PutTrustedSigningKey(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusOK {
		t.Fatalf("create key expected 200, got %d body=%s", createW.Code, createW.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/trusted-signing-keys", nil)
	listW := httptest.NewRecorder()
	ListTrustedSigningKeys(logger, mem, false).ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list keys expected 200, got %d", listW.Code)
	}
	if !bytes.Contains(listW.Body.Bytes(), []byte(keyID)) || !bytes.Contains(listW.Body.Bytes(), []byte(`"displayName":"release key"`)) {
		t.Fatalf("list response missing created key: %s", listW.Body.String())
	}

	patchBody := []byte(`{"displayName":"release key 2","notes":"rotated"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/trusted-signing-keys/"+keyID, bytes.NewReader(patchBody))
	patchReq = withURLParam(patchReq, "keyId", keyID)
	patchW := httptest.NewRecorder()
	PatchTrustedSigningKey(logger, mem, false).ServeHTTP(patchW, patchReq)
	if patchW.Code != http.StatusOK {
		t.Fatalf("patch key expected 200, got %d body=%s", patchW.Code, patchW.Body.String())
	}

	retireReq := httptest.NewRequest(http.MethodPost, "/api/v1/trusted-signing-keys/"+keyID+"/retire", nil)
	retireReq = withURLParam(retireReq, "keyId", keyID)
	retireW := httptest.NewRecorder()
	RetireTrustedSigningKey(logger, mem, false).ServeHTTP(retireW, retireReq)
	if retireW.Code != http.StatusOK {
		t.Fatalf("retire key expected 200, got %d body=%s", retireW.Code, retireW.Body.String())
	}
	if !bytes.Contains(retireW.Body.Bytes(), []byte(`"state":"retired"`)) {
		t.Fatalf("expected retired state, got %s", retireW.Body.String())
	}
}

func TestArtifactTrustPolicyHandlers_DefaultAndUpdate(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	sigPolicy := ArtifactSignaturePolicy{Store: mem}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/artifact-trust/policy", nil)
	getW := httptest.NewRecorder()
	GetArtifactTrustPolicy(logger, mem, false, sigPolicy).ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get policy expected 200, got %d body=%s", getW.Code, getW.Body.String())
	}
	if !bytes.Contains(getW.Body.Bytes(), []byte(`"verificationMode":"warn_unsigned"`)) {
		t.Fatalf("expected warn_unsigned default, got %s", getW.Body.String())
	}

	putBody := []byte(`{"verificationMode":"require_verified","allowedSignatureTypes":["ed25519"]}`)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/artifact-trust/policy", bytes.NewReader(putBody))
	putW := httptest.NewRecorder()
	PutArtifactTrustPolicy(logger, mem, false, sigPolicy).ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("put policy expected 200, got %d body=%s", putW.Code, putW.Body.String())
	}
	if !bytes.Contains(putW.Body.Bytes(), []byte(`"verificationMode":"require_verified"`)) {
		t.Fatalf("expected require_verified, got %s", putW.Body.String())
	}
}

func generateTrustedKeyFixture(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pkix, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix}))
}
