package httpapi

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/certs"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestBreakglassServiceTokenRevokeAllowsOperatorAndAudits(t *testing.T) {
	mem := memory.New()
	manager, tokens := newBreakglassAuthFixture(t, mem)
	tokenID := seedServiceToken(t, mem, "ci-publisher", []string{"artifact.publish"}, time.Now().UTC().Add(2*time.Hour))
	router := NewRouter(logDiscard(), Dependencies{Store: mem, Auth: manager})

	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/service-tokens/"+tokenID+"/revoke", bytes.NewReader([]byte(`{"reason":"credential suspected compromised"}`)))
	viewerReq.Header.Set("Authorization", "Bearer "+tokens["viewer"])
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerRec := httptest.NewRecorder()
	router.ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("viewer revoke expected 403, got %d (%s)", viewerRec.Code, viewerRec.Body.String())
	}

	operatorReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/service-tokens/"+tokenID+"/revoke", bytes.NewReader([]byte(`{"reason":"credential suspected compromised"}`)))
	operatorReq.Header.Set("Authorization", "Bearer "+tokens["operator"])
	operatorReq.Header.Set("Content-Type", "application/json")
	operatorRec := httptest.NewRecorder()
	router.ServeHTTP(operatorRec, operatorReq)
	if operatorRec.Code != http.StatusNoContent {
		t.Fatalf("operator revoke expected 204, got %d (%s)", operatorRec.Code, operatorRec.Body.String())
	}

	token, ok, err := mem.GetServiceToken(tokenID)
	if err != nil {
		t.Fatalf("get revoked token: %v", err)
	}
	if !ok || token.RevokedAt.IsZero() {
		t.Fatalf("expected token to be revoked: %+v", token)
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "auth.service_token.revoke", Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 revoke audit event, got %d", len(events))
	}
	if events[0].ActorID != "operator-1" {
		t.Fatalf("expected actor operator-1, got %q", events[0].ActorID)
	}
	if events[0].Status != "success" {
		t.Fatalf("expected success audit status, got %q", events[0].Status)
	}
	if events[0].TargetID != tokenID {
		t.Fatalf("expected target %q, got %q", tokenID, events[0].TargetID)
	}
	meta := decodeAuditJSON(t, events[0].MetadataJSON)
	if meta["reason"] != "credential suspected compromised" {
		t.Fatalf("expected revoke reason in audit metadata, got %+v", meta)
	}
	if meta["breakGlass"] != true {
		t.Fatalf("expected breakGlass=true in audit metadata, got %+v", meta)
	}
}

func TestBreakglassServiceTokenRotateAuditsReplacement(t *testing.T) {
	mem := memory.New()
	manager, tokens := newBreakglassAuthFixture(t, mem)
	tokenID := seedServiceToken(t, mem, "deploy-token", []string{"artifact.publish"}, time.Now().UTC().Add(4*time.Hour))
	router := NewRouter(logDiscard(), Dependencies{Store: mem, Auth: manager})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/service-tokens/"+tokenID+"/rotate", bytes.NewReader([]byte(`{"reason":"token leaked","ttlHours":1}`)))
	req.Header.Set("Authorization", "Bearer "+tokens["operator"])
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		ReplacedTokenID string    `json:"replacedTokenId"`
		TokenID         string    `json:"tokenId"`
		Name            string    `json:"name"`
		Scopes          []string  `json:"scopes"`
		ExpiresAt       time.Time `json:"expiresAt"`
		Token           string    `json:"token"`
		RevokedAt       time.Time `json:"revokedAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode rotate response: %v", err)
	}
	if resp.ReplacedTokenID != tokenID {
		t.Fatalf("expected replaced token %q, got %q", tokenID, resp.ReplacedTokenID)
	}
	if resp.TokenID == "" || resp.TokenID == tokenID {
		t.Fatalf("expected new token id, got %+v", resp)
	}
	if resp.Token == "" {
		t.Fatalf("expected raw replacement token in response")
	}
	if resp.Name != "deploy-token" {
		t.Fatalf("expected replacement token to keep name, got %q", resp.Name)
	}

	oldToken, ok, err := mem.GetServiceToken(tokenID)
	if err != nil {
		t.Fatalf("get original token: %v", err)
	}
	if !ok || oldToken.RevokedAt.IsZero() {
		t.Fatalf("expected original token to be revoked: %+v", oldToken)
	}
	newToken, ok, err := mem.GetServiceToken(resp.TokenID)
	if err != nil {
		t.Fatalf("get replacement token: %v", err)
	}
	if !ok {
		t.Fatalf("replacement token %q not found", resp.TokenID)
	}
	if newToken.Name != "deploy-token" {
		t.Fatalf("expected replacement name to be preserved, got %q", newToken.Name)
	}
	if newToken.RevokedAt != (time.Time{}) {
		t.Fatalf("expected replacement token to remain active, got %+v", newToken)
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "auth.service_token.rotate", Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 rotate audit event, got %d", len(events))
	}
	if events[0].Status != "success" {
		t.Fatalf("expected success audit status, got %q", events[0].Status)
	}
	meta := decodeAuditJSON(t, events[0].MetadataJSON)
	if meta["reason"] != "token leaked" {
		t.Fatalf("expected rotate reason in metadata, got %+v", meta)
	}
	if meta["replacementTokenId"] != resp.TokenID {
		t.Fatalf("expected replacement token id in metadata, got %+v", meta)
	}
	if got, ok := meta["ttlHours"].(float64); !ok || got != 1 {
		t.Fatalf("expected ttlHours=1 in metadata, got %+v", meta)
	}
}

func TestBreakglassCertReloadAllowsOperatorAndAudits(t *testing.T) {
	mem := memory.New()
	manager, tokens := newBreakglassAuthFixture(t, mem)
	certManager := newBreakglassCertManager(t)
	router := NewRouter(logDiscard(), Dependencies{
		Store:       mem,
		Auth:        manager,
		CertManager: certManager,
	})

	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/cert-rotation/reload", bytes.NewReader([]byte(`{"reason":"validate emergency CA reload"}`)))
	viewerReq.Header.Set("Authorization", "Bearer "+tokens["viewer"])
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerRec := httptest.NewRecorder()
	router.ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("viewer reload expected 403, got %d (%s)", viewerRec.Code, viewerRec.Body.String())
	}

	operatorReq := httptest.NewRequest(http.MethodPost, "/api/v1/cert-rotation/reload", bytes.NewReader([]byte(`{"reason":"validate emergency CA reload"}`)))
	operatorReq.Header.Set("Authorization", "Bearer "+tokens["operator"])
	operatorReq.Header.Set("Content-Type", "application/json")
	operatorRec := httptest.NewRecorder()
	router.ServeHTTP(operatorRec, operatorReq)
	if operatorRec.Code != http.StatusOK {
		t.Fatalf("operator reload expected 200, got %d (%s)", operatorRec.Code, operatorRec.Body.String())
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "certs.reload", Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 cert reload audit event, got %d", len(events))
	}
	if events[0].ActorID != "operator-1" {
		t.Fatalf("expected actor operator-1, got %q", events[0].ActorID)
	}
	meta := decodeAuditJSON(t, events[0].MetadataJSON)
	if meta["reason"] != "validate emergency CA reload" {
		t.Fatalf("expected reload reason in metadata, got %+v", meta)
	}
	if meta["breakGlass"] != true {
		t.Fatalf("expected breakGlass=true in metadata, got %+v", meta)
	}
}

func TestBreakglassCertCleanupConflictAuditsOutcome(t *testing.T) {
	mem := memory.New()
	manager, tokens := newBreakglassAuthFixture(t, mem)
	certManager := newBreakglassCertManager(t)
	activeFingerprint := certManager.State().ActiveFingerprint
	router := NewRouter(logDiscard(), Dependencies{
		Store:       mem,
		Auth:        manager,
		CertManager: certManager,
	})

	deviceID := uuid.NewString()
	if err := mem.UpsertDevice(store.Device{
		DeviceID:        deviceID,
		CertFingerprint: "device-fp",
		Status:          "active",
		LastSeen:        time.Now().UTC(),
		MetadataJSON:    []byte(`{"hwops":{"cert":{"active":false}}}`),
	}); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	if err := mem.SetCertRotationState(store.CertRotationState{
		ActiveFingerprint:   activeFingerprint,
		PreviousFingerprint: "previous-fingerprint",
		RotatedAt:           time.Now().UTC(),
		GracePeriodSeconds:  int64(time.Hour.Seconds()),
	}); err != nil {
		t.Fatalf("set rotation state: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cert-rotation/cleanup", bytes.NewReader([]byte(`{"reason":"remove compromised CA"}`)))
	req.Header.Set("Authorization", "Bearer "+tokens["operator"])
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("cleanup expected 409, got %d (%s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		Cleanup struct {
			Eligible bool   `json:"eligible"`
			Reason   string `json:"reason"`
		} `json:"cleanup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode cleanup response: %v", err)
	}
	if resp.Cleanup.Eligible {
		t.Fatalf("expected cleanup to remain ineligible")
	}
	if resp.Cleanup.Reason != "waiting" {
		t.Fatalf("expected cleanup reason waiting, got %q", resp.Cleanup.Reason)
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "certs.cleanup", Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 cert cleanup audit event, got %d", len(events))
	}
	if events[0].Status != "denied" {
		t.Fatalf("expected denied cleanup audit status, got %q", events[0].Status)
	}
	if events[0].Error != "waiting" {
		t.Fatalf("expected cleanup audit error waiting, got %q", events[0].Error)
	}
	meta := decodeAuditJSON(t, events[0].MetadataJSON)
	if meta["reason"] != "remove compromised CA" {
		t.Fatalf("expected cleanup reason in metadata, got %+v", meta)
	}
	if meta["cleanupReason"] != "waiting" {
		t.Fatalf("expected cleanupReason=waiting in metadata, got %+v", meta)
	}
}

func newBreakglassAuthFixture(t *testing.T, mem *memory.Store) (*auth.Manager, map[string]string) {
	t.Helper()

	manager, err := auth.NewManager("local", "0123456789abcdef0123456789abcdef", 12*time.Hour, "parcel", mem)
	if err != nil {
		t.Fatalf("new auth manager: %v", err)
	}

	tokens := map[string]string{}
	tokens["viewer"] = createBreakglassUserToken(t, mem, manager, "viewer-1", "viewer@example.com", []string{"viewer"})
	tokens["operator"] = createBreakglassUserToken(t, mem, manager, "operator-1", "operator@example.com", []string{"operator"})
	tokens["admin"] = createBreakglassUserToken(t, mem, manager, "admin-1", "admin@example.com", []string{"admin"})
	return manager, tokens
}

func createBreakglassUserToken(t *testing.T, mem *memory.Store, manager *auth.Manager, userID, email string, roles []string) string {
	t.Helper()

	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		t.Fatalf("marshal roles: %v", err)
	}
	user := store.User{
		UserID:       userID,
		Email:        email,
		RolesJSON:    rolesJSON,
		AuthProvider: "local",
		CreatedAt:    time.Now().UTC(),
	}
	if err := mem.CreateUser(user); err != nil {
		t.Fatalf("create user %s: %v", userID, err)
	}
	token, _, err := manager.IssueToken(user)
	if err != nil {
		t.Fatalf("issue token for %s: %v", userID, err)
	}
	return token
}

func seedServiceToken(t *testing.T, mem *memory.Store, name string, scopes []string, expiresAt time.Time) string {
	t.Helper()

	rawToken, tokenHash, err := auth.GenerateVoucherToken()
	if err != nil {
		t.Fatalf("generate service token: %v", err)
	}
	_ = rawToken
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		t.Fatalf("marshal scopes: %v", err)
	}
	tokenID := uuid.NewString()
	if err := mem.CreateServiceToken(store.ServiceToken{
		TokenID:    tokenID,
		Name:       name,
		TokenHash:  tokenHash,
		ScopesJSON: scopesJSON,
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now().UTC(),
		CreatedBy:  "seed",
	}); err != nil {
		t.Fatalf("create service token: %v", err)
	}
	return tokenID
}

func newBreakglassCertManager(t *testing.T) *certs.Manager {
	t.Helper()

	certPEM, keyPEM := newBreakglassCACert(t)
	dir := t.TempDir()
	activeCertPath := filepath.Join(dir, "active-ca.crt")
	activeKeyPath := filepath.Join(dir, "active-ca.key")
	clientCAPath := filepath.Join(dir, "client-ca.crt")

	if err := os.WriteFile(activeCertPath, certPEM, 0644); err != nil {
		t.Fatalf("write active cert: %v", err)
	}
	if err := os.WriteFile(activeKeyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write active key: %v", err)
	}
	if err := os.WriteFile(clientCAPath, certPEM, 0644); err != nil {
		t.Fatalf("write client bundle: %v", err)
	}

	manager := certs.NewManager()
	if _, err := manager.Load(activeCertPath, activeKeyPath, clientCAPath); err != nil {
		t.Fatalf("load cert manager: %v", err)
	}
	return manager
}

func newBreakglassCACert(t *testing.T) ([]byte, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "Parcel Breakglass Test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

func decodeAuditJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode audit json: %v", err)
	}
	return out
}
