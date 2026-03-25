package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
	"github.com/hardwareops/control-plane/internal/webhooks"
)

type authFixture struct {
	router  http.Handler
	manager *auth.Manager
	store   *memory.Store
}

func newAuthFixture(t *testing.T) authFixture {
	t.Helper()

	mem := memory.New()
	manager, err := auth.NewManager(auth.ModeLocal, "test-secret", time.Hour, "test-issuer", mem)
	if err != nil {
		t.Fatalf("new auth manager: %v", err)
	}
	encryptionKey := bytes.Repeat([]byte("k"), 32)

	return authFixture{
		router:  NewRouter(logDiscard(), Dependencies{Store: mem, Auth: manager, WebhookEncryptionKey: encryptionKey}),
		manager: manager,
		store:   mem,
	}
}

func (f authFixture) issueUserToken(t *testing.T, roles ...string) string {
	t.Helper()

	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		t.Fatalf("marshal roles: %v", err)
	}

	user := store.User{
		UserID:    uuid.NewString(),
		Email:     uuid.NewString() + "@example.com",
		RolesJSON: rolesJSON,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := f.store.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	token, _, err := f.manager.IssueToken(user)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return token
}

func (f authFixture) issueServiceToken(t *testing.T, scopes []string, expiresAt time.Time, revoked bool) string {
	t.Helper()

	rawToken, tokenHash, err := auth.GenerateVoucherToken()
	if err != nil {
		t.Fatalf("generate service token: %v", err)
	}

	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		t.Fatalf("marshal scopes: %v", err)
	}

	now := time.Now().UTC()
	serviceToken := store.ServiceToken{
		TokenID:    uuid.NewString(),
		Name:       "svc-" + uuid.NewString(),
		TokenHash:  tokenHash,
		ScopesJSON: scopesJSON,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
	}
	if revoked {
		serviceToken.RevokedAt = now
		serviceToken.RevokedBy = "test"
	}
	if err := f.store.CreateServiceToken(serviceToken); err != nil {
		t.Fatalf("create service token: %v", err)
	}

	return rawToken
}

func performRequest(t *testing.T, router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *strings.Reader
	if body == "" {
		reqBody = strings.NewReader("")
	} else {
		reqBody = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestRouterRoleMatrix(t *testing.T) {
	fixture := newAuthFixture(t)
	tokens := map[string]string{
		"viewer":   fixture.issueUserToken(t, "viewer"),
		"operator": fixture.issueUserToken(t, "operator"),
		"admin":    fixture.issueUserToken(t, "admin"),
	}

	testCases := []struct {
		name string
		path string
		body string
		want map[string]int
	}{
		{
			name: "viewer endpoint",
			path: "/api/v1/auth/me",
			want: map[string]int{
				"anonymous": http.StatusUnauthorized,
				"viewer":    http.StatusOK,
				"operator":  http.StatusOK,
				"admin":     http.StatusOK,
			},
		},
		{
			name: "operator endpoint",
			path: "/api/v1/groups/batch",
			body: `{"actions":[{"action":"upsert","name":"ops","selector":{}}]}`,
			want: map[string]int{
				"anonymous": http.StatusUnauthorized,
				"viewer":    http.StatusForbidden,
				"operator":  http.StatusOK,
				"admin":     http.StatusOK,
			},
		},
		{
			name: "admin endpoint",
			path: "/api/v1/auth/vouchers",
			body: `{"email":"new.user@example.com","roles":["viewer"]}`,
			want: map[string]int{
				"anonymous": http.StatusUnauthorized,
				"viewer":    http.StatusForbidden,
				"operator":  http.StatusForbidden,
				"admin":     http.StatusOK,
			},
		},
		{
			name: "artifact publish endpoint",
			path: "/api/v1/artifacts/pull",
			body: `{}`,
			want: map[string]int{
				"anonymous": http.StatusUnauthorized,
				"viewer":    http.StatusForbidden,
				"operator":  http.StatusInternalServerError,
				"admin":     http.StatusInternalServerError,
			},
		},
	}

	identities := []string{"anonymous", "viewer", "operator", "admin"}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodGet
			if tc.body != "" {
				method = http.MethodPost
			}

			for _, identity := range identities {
				token := tokens[identity]
				rec := performRequest(t, fixture.router, method, tc.path, tc.body, token)
				if rec.Code != tc.want[identity] {
					t.Fatalf("%s: expected %d, got %d body=%s", identity, tc.want[identity], rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestArtifactPublishServiceTokenScopes(t *testing.T) {
	fixture := newAuthFixture(t)
	now := time.Now().UTC()

	validToken := fixture.issueServiceToken(t, []string{"artifact.publish"}, now.Add(time.Hour), false)
	wrongScopeToken := fixture.issueServiceToken(t, []string{"inventory.read"}, now.Add(time.Hour), false)
	revokedToken := fixture.issueServiceToken(t, []string{"artifact.publish"}, now.Add(time.Hour), true)
	expiredToken := fixture.issueServiceToken(t, []string{"artifact.publish"}, now.Add(-time.Hour), false)

	routeCases := []struct {
		name string
		path string
	}{
		{name: "pull", path: "/api/v1/artifacts/pull"},
		{name: "presign upload", path: "/api/v1/artifacts/presign-upload"},
		{name: "complete upload", path: "/api/v1/artifacts/complete"},
	}

	tokenCases := []struct {
		name  string
		token string
		want  int
	}{
		{name: "scoped", token: validToken, want: http.StatusInternalServerError},
		{name: "wrong scope", token: wrongScopeToken, want: http.StatusForbidden},
		{name: "revoked", token: revokedToken, want: http.StatusUnauthorized},
		{name: "expired", token: expiredToken, want: http.StatusUnauthorized},
	}

	for _, routeCase := range routeCases {
		t.Run(routeCase.name, func(t *testing.T) {
			for _, tokenCase := range tokenCases {
				rec := performRequest(t, fixture.router, http.MethodPost, routeCase.path, `{}`, tokenCase.token)
				if rec.Code != tokenCase.want {
					t.Fatalf("%s: expected %d, got %d body=%s", tokenCase.name, tokenCase.want, rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestServiceTokenCannotAccessRoleOnlyEndpoints(t *testing.T) {
	fixture := newAuthFixture(t)
	token := fixture.issueServiceToken(t, []string{"artifact.publish"}, time.Now().UTC().Add(time.Hour), false)

	testCases := []struct {
		name string
		path string
		body string
	}{
		{
			name: "viewer only endpoint",
			path: "/api/v1/auth/me",
		},
		{
			name: "operator only endpoint",
			path: "/api/v1/groups/batch",
			body: `{"actions":[{"action":"upsert","name":"ops","selector":{}}]}`,
		},
		{
			name: "admin only endpoint",
			path: "/api/v1/auth/vouchers",
			body: `{"email":"new.user@example.com","roles":["viewer"]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodGet
			if tc.body != "" {
				method = http.MethodPost
			}

			rec := performRequest(t, fixture.router, method, tc.path, tc.body, token)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected %d, got %d body=%s", http.StatusUnauthorized, rec.Code, rec.Body.String())
			}
		})
	}
}

func seedScopedRouteFixtures(t *testing.T, fixture authFixture) (deviceID, groupID, artifactID, webhookID, deliveryID string) {
	t.Helper()

	deviceID = uuid.NewString()
	if err := fixture.store.UpsertDevice(store.Device{
		DeviceID:  deviceID,
		Status:    "active",
		LastSeen:  time.Now().UTC(),
		LabelsJSON: []byte(`{"role":"edge"}`),
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	groupID = uuid.NewString()
	if err := fixture.store.UpsertGroup(store.Group{
		GroupID:      groupID,
		Name:         "edge",
		SelectorJSON: []byte(`{"role":"edge"}`),
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	artifactID = uuid.NewString()
	if err := fixture.store.CreateArtifact(store.Artifact{
		ArtifactID: artifactID,
		Name:       "agent",
		Version:    "1.0.0",
		ObjectKey:  "artifacts/agent.tar.gz",
		SHA256:     "abc",
		SizeBytes:  10,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	webhookID = uuid.NewString()
	encryptedSecret, err := webhooks.EncryptSecret(bytes.Repeat([]byte("k"), 32), "shared-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	if err := fixture.store.CreateWebhook(store.Webhook{
		ID:              webhookID,
		Name:            "ci",
		URL:             "http://example.invalid/hook",
		EncryptedSecret: encryptedSecret,
		EventTypes:      []string{"ping", "deployment.triggered"},
		Enabled:         true,
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed webhook: %v", err)
	}

	deliveryID = uuid.NewString()
	if err := fixture.store.CreateWebhookDelivery(store.WebhookDelivery{
		ID:          deliveryID,
		WebhookID:   webhookID,
		EventType:   "ping",
		PayloadJSON: []byte(`{"eventType":"ping"}`),
		Status:      "failed",
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed webhook delivery: %v", err)
	}

	return deviceID, groupID, artifactID, webhookID, deliveryID
}

func TestServiceTokenScopedRouteAccess(t *testing.T) {
	fixture := newAuthFixture(t)
	deviceID, groupID, artifactID, webhookID, deliveryID := seedScopedRouteFixtures(t, fixture)
	now := time.Now().UTC()

	deviceReadToken := fixture.issueServiceToken(t, []string{auth.ScopeDeviceRead}, now.Add(time.Hour), false)
	artifactReadToken := fixture.issueServiceToken(t, []string{auth.ScopeArtifactRead}, now.Add(time.Hour), false)
	deploymentTriggerToken := fixture.issueServiceToken(t, []string{auth.ScopeDeploymentTrigger}, now.Add(time.Hour), false)
	webhookManageToken := fixture.issueServiceToken(t, []string{auth.ScopeWebhookManage}, now.Add(time.Hour), false)
	wrongScopeToken := fixture.issueServiceToken(t, []string{"inventory.read"}, now.Add(time.Hour), false)
	expiredToken := fixture.issueServiceToken(t, []string{auth.ScopeDeviceRead}, now.Add(-time.Hour), false)
	revokedToken := fixture.issueServiceToken(t, []string{auth.ScopeDeviceRead}, now.Add(time.Hour), true)

	testCases := []struct {
		name      string
		method    string
		path      string
		body      string
		okToken   string
		wantOK    []int
		wrongVerb bool
	}{
		{name: "device.read devices", method: http.MethodGet, path: "/api/v1/devices", okToken: deviceReadToken, wantOK: []int{http.StatusOK}},
		{name: "device.read device detail", method: http.MethodGet, path: "/api/v1/devices/" + deviceID, okToken: deviceReadToken, wantOK: []int{http.StatusOK}},
		{name: "device.read groups", method: http.MethodGet, path: "/api/v1/groups", okToken: deviceReadToken, wantOK: []int{http.StatusOK}},
		{name: "device.read deployment status", method: http.MethodGet, path: "/api/v1/groups/" + groupID + "/deployment-status?artifactId=" + artifactID, okToken: deviceReadToken, wantOK: []int{http.StatusOK}},
		{name: "device.read device vuln scans", method: http.MethodGet, path: "/api/v1/devices/" + deviceID + "/vulnerability-scans", okToken: deviceReadToken, wantOK: []int{http.StatusOK}},
		{name: "artifact.read artifacts", method: http.MethodGet, path: "/api/v1/artifacts", okToken: artifactReadToken, wantOK: []int{http.StatusOK}},
		{name: "artifact.read artifact detail", method: http.MethodGet, path: "/api/v1/artifacts/" + artifactID, okToken: artifactReadToken, wantOK: []int{http.StatusOK}},
		{name: "artifact.read attestations", method: http.MethodGet, path: "/api/v1/artifacts/" + artifactID + "/attestations", okToken: artifactReadToken, wantOK: []int{http.StatusOK}},
		{name: "artifact.read lifecycle status", method: http.MethodGet, path: "/api/v1/artifacts/lifecycle/status", okToken: artifactReadToken, wantOK: []int{http.StatusOK}},
		{name: "artifact.read vuln scans", method: http.MethodGet, path: "/api/v1/artifacts/" + artifactID + "/vulnerability-scans", okToken: artifactReadToken, wantOK: []int{http.StatusOK}},
		{name: "artifact.read presign", method: http.MethodPost, path: "/api/v1/artifacts/" + artifactID + "/presign", body: `{}`, okToken: artifactReadToken, wantOK: []int{http.StatusInternalServerError}},
		{name: "deployment.trigger device", method: http.MethodPost, path: "/api/v1/devices/" + deviceID + "/trigger-apply", body: `{}`, okToken: deploymentTriggerToken, wantOK: []int{http.StatusOK}},
		{name: "deployment.trigger group", method: http.MethodPost, path: "/api/v1/groups/" + groupID + "/trigger-apply", body: `{}`, okToken: deploymentTriggerToken, wantOK: []int{http.StatusOK}},
		{name: "webhook.manage list", method: http.MethodGet, path: "/api/v1/webhooks", okToken: webhookManageToken, wantOK: []int{http.StatusOK}},
		{name: "webhook.manage get", method: http.MethodGet, path: "/api/v1/webhooks/" + webhookID, okToken: webhookManageToken, wantOK: []int{http.StatusOK}},
		{name: "webhook.manage deliveries", method: http.MethodGet, path: "/api/v1/webhooks/" + webhookID + "/deliveries", okToken: webhookManageToken, wantOK: []int{http.StatusOK}},
		{name: "webhook.manage create", method: http.MethodPost, path: "/api/v1/webhooks", body: `{"name":"ci","url":"http://example.invalid/hook","eventTypes":["ping"],"enabled":true}`, okToken: webhookManageToken, wantOK: []int{http.StatusCreated}},
		{name: "webhook.manage update", method: http.MethodPut, path: "/api/v1/webhooks/" + webhookID, body: `{"name":"new-name","enabled":true}`, okToken: webhookManageToken, wantOK: []int{http.StatusOK}},
		{name: "webhook.manage redeliver", method: http.MethodPost, path: "/api/v1/webhooks/" + webhookID + "/deliveries/" + deliveryID + "/redeliver", okToken: webhookManageToken, wantOK: []int{http.StatusOK}},
		{name: "webhook.manage test", method: http.MethodPost, path: "/api/v1/webhooks/" + webhookID + "/test", okToken: webhookManageToken, wantOK: []int{http.StatusOK}},
		{name: "webhook.manage delete", method: http.MethodDelete, path: "/api/v1/webhooks/" + webhookID, okToken: webhookManageToken, wantOK: []int{http.StatusNoContent}},
	}

	hasStatus := func(got int, want []int) bool {
		for _, code := range want {
			if got == code {
				return true
			}
		}
		return false
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := performRequest(t, fixture.router, tc.method, tc.path, tc.body, tc.okToken)
			if !hasStatus(rec.Code, tc.wantOK) {
				t.Fatalf("scoped token: expected one of %v, got %d body=%s", tc.wantOK, rec.Code, rec.Body.String())
			}

			rec = performRequest(t, fixture.router, tc.method, tc.path, tc.body, wrongScopeToken)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("wrong scope: expected %d, got %d body=%s", http.StatusForbidden, rec.Code, rec.Body.String())
			}

			rec = performRequest(t, fixture.router, tc.method, tc.path, tc.body, "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous: expected %d, got %d body=%s", http.StatusUnauthorized, rec.Code, rec.Body.String())
			}
		})
	}

	expiredCases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "expired device.read", method: http.MethodGet, path: "/api/v1/devices"},
		{name: "revoked device.read", method: http.MethodGet, path: "/api/v1/groups"},
	}
	for _, tc := range expiredCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := performRequest(t, fixture.router, tc.method, tc.path, tc.body, expiredToken)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expired: expected %d, got %d body=%s", http.StatusUnauthorized, rec.Code, rec.Body.String())
			}
			rec = performRequest(t, fixture.router, tc.method, tc.path, tc.body, revokedToken)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("revoked: expected %d, got %d body=%s", http.StatusUnauthorized, rec.Code, rec.Body.String())
			}
		})
	}

	rec := performRequest(t, fixture.router, http.MethodPost, "/api/v1/artifacts/presign-upload", `{}`, deploymentTriggerToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("deployment.trigger should not grant artifact upload: got %d body=%s", rec.Code, rec.Body.String())
	}
}
