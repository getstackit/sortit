package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"sortit/internal/auth"
	"sortit/internal/issues"
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

func newAuthenticatedServer(t *testing.T) (*Server, http.Handler) { //nolint:unparam
	t.Helper()

	store := newPostgresIssueStore(t, nil)
	authService, err := auth.NewService(auth.ServiceConfig{
		Store: auth.NewStore(store.DB()),
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
		if cookie.Name == "sortit_oauth_state" {
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
		if cookie.Name == "sortit_session" {
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

func TestGitHubCallbackHonorsReturnTo(t *testing.T) {
	_, handler := newAuthenticatedServer(t)

	startReq := httptest.NewRequest(http.MethodGet, "/api/ui/auth/github/start?return_to=%2Fauth%2Fcli-login%3Flogin_id%3Dabc", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusFound {
		t.Fatalf("expected 302 for login start, got %d", startRec.Code)
	}

	var stateCookie *http.Cookie
	var returnToCookie *http.Cookie
	for _, cookie := range startRec.Result().Cookies() {
		if cookie.Name == "sortit_oauth_state" {
			stateCookie = cookie
		}
		if cookie.Name == "sortit_oauth_return_to" {
			returnToCookie = cookie
		}
	}
	if stateCookie == nil || returnToCookie == nil {
		t.Fatalf("expected oauth cookies, got state=%v returnTo=%v", stateCookie, returnToCookie)
	}

	location, err := startRec.Result().Location()
	if err != nil {
		t.Fatalf("start redirect location: %v", err)
	}
	state := location.Query().Get("state")

	callbackReq := httptest.NewRequest(
		http.MethodGet,
		"/api/ui/auth/github/callback?code=test-code&state="+url.QueryEscape(state),
		nil,
	)
	callbackReq.AddCookie(stateCookie)
	callbackReq.AddCookie(returnToCookie)
	callbackRec := httptest.NewRecorder()
	handler.ServeHTTP(callbackRec, callbackReq)
	if callbackRec.Code != http.StatusFound {
		t.Fatalf("expected 302 for callback, got %d", callbackRec.Code)
	}

	callbackLocation, err := callbackRec.Result().Location()
	if err != nil {
		t.Fatalf("callback redirect location: %v", err)
	}
	if callbackLocation.String() != "http://localhost:3000/auth/cli-login?login_id=abc" {
		t.Fatalf("unexpected callback redirect: %s", callbackLocation.String())
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

func TestAuthenticatedActorOverridesAssignCreatedBy(t *testing.T) {
	_, handler := newAuthenticatedServer(t)
	sessionCookie := authenticateSessionCookie(t, handler)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/issues",
		strings.NewReader(`{"raw":"Assignment attribution should use the authenticated actor"}`),
	)
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var created issues.Issue
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	assignReq := httptest.NewRequest(
		http.MethodPost,
		"/api/issues/"+created.ID+"/assign",
		strings.NewReader(`{"assignedTo":"Jordan","createdBy":"Mallory"}`),
	)
	assignReq.AddCookie(sessionCookie)
	assignRec := httptest.NewRecorder()
	handler.ServeHTTP(assignRec, assignReq)
	if assignRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", assignRec.Code)
	}

	var assigned issues.Issue
	if err := json.NewDecoder(assignRec.Body).Decode(&assigned); err != nil {
		t.Fatalf("decode assign response: %v", err)
	}
	if len(assigned.Discussion) < 2 {
		t.Fatalf("expected assignment discussion post, got %#v", assigned.Discussion)
	}
	if assigned.Discussion[1].CreatedBy != "The Octocat" {
		t.Fatalf("expected authenticated assignment actor, got %q", assigned.Discussion[1].CreatedBy)
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

func TestCLILoginCanBeCompletedFromBrowserSession(t *testing.T) {
	_, handler := newAuthenticatedServer(t)

	startReq := httptest.NewRequest(http.MethodPost, "/api/auth/cli/login", strings.NewReader(`{}`))
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for cli login start, got %d", startRec.Code)
	}

	var startPayload cliLoginStartResponse
	if err := json.NewDecoder(startRec.Body).Decode(&startPayload); err != nil {
		t.Fatalf("decode cli login start: %v", err)
	}
	if !strings.Contains(startPayload.StartURL, "/auth/cli-login?login_id="+url.QueryEscape(startPayload.LoginID)) {
		t.Fatalf("unexpected cli login start URL: %s", startPayload.StartURL)
	}

	sessionCookie := authenticateSessionCookie(t, handler)

	completeReq := httptest.NewRequest(
		http.MethodPost,
		"/api/ui/auth/cli/login/"+url.PathEscape(startPayload.LoginID)+"/complete",
		strings.NewReader(`{}`),
	)
	completeReq.Header.Set("Content-Type", "application/json")
	completeReq.AddCookie(sessionCookie)
	completeRec := httptest.NewRecorder()
	handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for cli login completion, got %d", completeRec.Code)
	}

	exchangeReq := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/cli/login/"+url.PathEscape(startPayload.LoginID)+"/exchange",
		strings.NewReader(`{"exchangeToken":"`+startPayload.ExchangeToken+`"}`),
	)
	exchangeReq.Header.Set("Content-Type", "application/json")
	exchangeRec := httptest.NewRecorder()
	handler.ServeHTTP(exchangeRec, exchangeReq)
	if exchangeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for cli login exchange, got %d", exchangeRec.Code)
	}

	var exchangePayload cliLoginExchangeResponse
	if err := json.NewDecoder(exchangeRec.Body).Decode(&exchangePayload); err != nil {
		t.Fatalf("decode cli login exchange: %v", err)
	}
	if exchangePayload.Status != "complete" || exchangePayload.Token == "" {
		t.Fatalf("unexpected cli login exchange payload: %+v", exchangePayload)
	}

	protectedReq := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	protectedReq.Header.Set("Authorization", "Bearer "+exchangePayload.Token)
	protectedRec := httptest.NewRecorder()
	handler.ServeHTTP(protectedRec, protectedReq)
	if protectedRec.Code != http.StatusOK {
		t.Fatalf("expected bearer token to access protected routes, got %d", protectedRec.Code)
	}
}
