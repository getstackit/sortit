package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"splat/internal/auth"
	"splat/internal/issues"
)

type fakeOAuthProvider struct {
	user auth.OAuthUser
}

func (p fakeOAuthProvider) AuthorizeURL(state, redirectURL string) string {
	values := url.Values{}
	values.Set("state", state)
	values.Set("redirect_uri", redirectURL)
	return "https://github.test/login?" + values.Encode()
}

func (p fakeOAuthProvider) Exchange(_ context.Context, code, redirectURL string) (auth.OAuthUser, error) {
	_ = code
	_ = redirectURL
	return p.user, nil
}

func newAuthenticatedServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()

	store := newSQLiteIssueStore(t, nil)
	authService, err := auth.NewService(auth.ServiceConfig{
		Store: auth.NewSQLiteStore(store.DB()),
		Provider: fakeOAuthProvider{
			user: auth.OAuthUser{
				Provider:       "github",
				ProviderUserID: "12345",
				Login:          "octocat",
				DisplayName:    "The Octocat",
				AvatarURL:      "https://example.com/avatar.png",
				Email:          "octocat@example.com",
			},
		},
		WebOrigin: "http://localhost:3000",
	})
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
		Auth:        authService,
	})
	handler := server.Handler()
	return server, handler
}

func authenticateSessionCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()

	startReq := httptest.NewRequest(http.MethodGet, "/api/auth/github/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusFound {
		t.Fatalf("expected 302 for login start, got %d", startRec.Code)
	}

	var stateCookie *http.Cookie
	for _, cookie := range startRec.Result().Cookies() {
		if cookie.Name == "splat_oauth_state" {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected oauth state cookie")
	}

	location, err := startRec.Result().Location()
	if err != nil {
		t.Fatalf("start redirect location: %v", err)
	}
	state := location.Query().Get("state")
	if state == "" {
		t.Fatal("expected state in redirect location")
	}

	callbackReq := httptest.NewRequest(
		http.MethodGet,
		"/api/auth/github/callback?code=test-code&state="+url.QueryEscape(state),
		nil,
	)
	callbackReq.AddCookie(stateCookie)
	callbackRec := httptest.NewRecorder()
	handler.ServeHTTP(callbackRec, callbackReq)
	if callbackRec.Code != http.StatusFound {
		t.Fatalf("expected 302 for callback, got %d", callbackRec.Code)
	}

	for _, cookie := range callbackRec.Result().Cookies() {
		if cookie.Name == "splat_session" {
			return cookie
		}
	}
	t.Fatal("expected session cookie")
	return nil
}

func TestProtectedRoutesRequireAuthentication(t *testing.T) {
	_, handler := newAuthenticatedServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestGitHubCallbackCreatesSessionAndSessionEndpoint(t *testing.T) {
	_, handler := newAuthenticatedServer(t)
	sessionCookie := authenticateSessionCookie(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload authSessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if payload.User.Login != "octocat" {
		t.Fatalf("expected octocat login, got %q", payload.User.Login)
	}
}

func TestAuthenticatedActorOverridesRequestCreatedBy(t *testing.T) {
	_, handler := newAuthenticatedServer(t)
	sessionCookie := authenticateSessionCookie(t, handler)

	body := strings.NewReader(`{"raw":"GitHub auth should own attribution","createdBy":"Mallory"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/issues", body)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var created issues.Issue
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.CreatedBy != "The Octocat" {
		t.Fatalf("expected authenticated actor, got %q", created.CreatedBy)
	}
}

func TestAPITokenCanReadProtectedRoutesAndRevocationStopsIt(t *testing.T) {
	_, handler := newAuthenticatedServer(t)
	sessionCookie := authenticateSessionCookie(t, handler)

	createTokenReq := httptest.NewRequest(http.MethodPost, "/api/auth/tokens", strings.NewReader(`{}`))
	createTokenReq.AddCookie(sessionCookie)
	createTokenRec := httptest.NewRecorder()
	handler.ServeHTTP(createTokenRec, createTokenReq)
	if createTokenRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for token creation, got %d", createTokenRec.Code)
	}

	var createdToken createAPITokenResponse
	if err := json.NewDecoder(createTokenRec.Body).Decode(&createdToken); err != nil {
		t.Fatalf("decode token response: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	listReq.Header.Set("Authorization", "Bearer "+createdToken.Token)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 with bearer token, got %d", listRec.Code)
	}

	revokeReq := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/tokens/"+createdToken.Metadata.ID+"/revoke",
		strings.NewReader(`{}`),
	)
	revokeReq.AddCookie(sessionCookie)
	revokeRec := httptest.NewRecorder()
	handler.ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for revoke, got %d", revokeRec.Code)
	}

	revokedReq := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	revokedReq.Header.Set("Authorization", "Bearer "+createdToken.Token)
	revokedRec := httptest.NewRecorder()
	handler.ServeHTTP(revokedRec, revokedReq)
	if revokedRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked token, got %d", revokedRec.Code)
	}
}
