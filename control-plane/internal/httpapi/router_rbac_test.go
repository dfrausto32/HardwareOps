package httpapi

import (
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

	return authFixture{
		router:  NewRouter(logDiscard(), Dependencies{Store: mem, Auth: manager}),
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
