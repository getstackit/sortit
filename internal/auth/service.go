package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName   = "sortit_session"
	oauthStateCookie    = "sortit_oauth_state"
	oauthReturnToCookie = "sortit_oauth_return_to"
)

type ServiceConfig struct {
	Store      *Store
	Provider   OAuthProvider
	WebOrigin  string
	SessionTTL time.Duration
}

type Service struct {
	store      *Store
	provider   OAuthProvider
	webOrigin  string
	sessionTTL time.Duration
	cliLogins  *cliLoginManager
}

func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("auth store is required")
	}
	if cfg.Provider == nil {
		return nil, fmt.Errorf("oauth provider is required")
	}
	sessionTTL := cfg.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = 30 * 24 * time.Hour
	}
	return &Service{
		store:      cfg.Store,
		provider:   cfg.Provider,
		webOrigin:  strings.TrimRight(strings.TrimSpace(cfg.WebOrigin), "/"),
		sessionTTL: sessionTTL,
		cliLogins:  newCLILoginManager(),
	}, nil
}

func (s *Service) BeginGitHubLogin(w http.ResponseWriter, r *http.Request, callbackPath string) error {
	state, _, err := newSecretToken("st")
	if err != nil {
		return err
	}

	setOAuthReturnToCookie(w, r, sanitizeReturnTo(r.URL.Query().Get("return_to")))

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   int((10 * time.Minute).Seconds()),
	})

	http.Redirect(w, r, s.provider.AuthorizeURL(state, callbackURL(r, callbackPath)), http.StatusFound)
	return nil
}

func (s *Service) CompleteGitHubLogin(w http.ResponseWriter, r *http.Request, callbackPath string) error {
	user, err := s.completeOAuthLogin(r, callbackPath)
	if err != nil {
		return err
	}
	returnTo := readOAuthReturnTo(r)
	if err := s.setSessionCookie(w, r, user.ID); err != nil {
		return err
	}
	clearOAuthReturnToCookie(w, r)
	http.Redirect(w, r, redirectTarget(s.webOrigin, returnTo), http.StatusFound)
	return nil
}

func (s *Service) completeOAuthLogin(r *http.Request, callbackPath string) (User, error) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		return User{}, fmt.Errorf("code and state are required")
	}

	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || strings.TrimSpace(stateCookie.Value) == "" {
		return User{}, ErrUnauthorized
	}
	if state != strings.TrimSpace(stateCookie.Value) {
		return User{}, ErrUnauthorized
	}

	oauthUser, err := s.provider.Exchange(r.Context(), code, callbackURL(r, callbackPath))
	if err != nil {
		return User{}, err
	}

	user, err := s.store.UpsertOAuthUser(r.Context(), oauthUser)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) setSessionCookie(w http.ResponseWriter, r *http.Request, userID string) error {
	clearOAuthStateCookie(w, r)

	rawSession, err := s.store.CreateSession(r.Context(), userID, time.Now().UTC().Add(s.sessionTTL))
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    rawSession,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   int(s.sessionTTL.Seconds()),
	})
	return nil
}

func (s *Service) AuthenticateRequest(r *http.Request, bearerOnly bool) (Principal, error) {
	if rawBearer := bearerToken(r.Header.Get("Authorization")); rawBearer != "" {
		return s.store.LookupAPIToken(r.Context(), rawBearer)
	}
	if bearerOnly {
		return Principal{}, ErrUnauthorized
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return Principal{}, ErrUnauthorized
	}
	return s.store.LookupSession(r.Context(), cookie.Value)
}

func (s *Service) CurrentPrincipal(r *http.Request) (Principal, error) {
	return s.AuthenticateRequest(r, false)
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) error {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && strings.TrimSpace(cookie.Value) != "" {
		if deleteErr := s.store.DeleteSession(r.Context(), cookie.Value); deleteErr != nil {
			return deleteErr
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   -1,
	})
	return nil
}

func (s *Service) CreateAPIToken(ctx context.Context, principal Principal, name string) (APIToken, string, error) {
	return s.store.CreateAPIToken(ctx, principal.UserID, name)
}

func (s *Service) ListAPITokens(ctx context.Context, principal Principal) ([]APIToken, error) {
	return s.store.ListAPITokens(ctx, principal.UserID)
}

func (s *Service) RevokeAPIToken(ctx context.Context, principal Principal, tokenID string) error {
	return s.store.RevokeAPIToken(ctx, principal.UserID, tokenID)
}

func (s *Service) WebOrigin() string {
	return s.webOrigin
}

func callbackURL(r *http.Request, callbackPath string) string {
	scheme := "http"
	if isSecureRequest(r) {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, callbackPath)
}

func redirectTarget(webOrigin, returnTo string) string {
	returnTo = sanitizeReturnTo(returnTo)
	if returnTo != "" {
		if strings.HasPrefix(returnTo, "http://") || strings.HasPrefix(returnTo, "https://") {
			return returnTo
		}
		if webOrigin != "" {
			return webOrigin + returnTo
		}
		return returnTo
	}
	if webOrigin == "" {
		return "/"
	}
	return webOrigin + "/"
}

func clearOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   -1,
	})
}

func setOAuthReturnToCookie(w http.ResponseWriter, r *http.Request, returnTo string) {
	if returnTo == "" {
		clearOAuthReturnToCookie(w, r)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthReturnToCookie,
		Value:    returnTo,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   int((10 * time.Minute).Seconds()),
	})
}

func clearOAuthReturnToCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthReturnToCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   -1,
	})
}

func readOAuthReturnTo(r *http.Request) string {
	cookie, err := r.Cookie(oauthReturnToCookie)
	if err != nil {
		return ""
	}
	return sanitizeReturnTo(cookie.Value)
}

func sanitizeReturnTo(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "/") {
		return value
	}
	return ""
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func bearerToken(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.SplitN(strings.TrimSpace(value), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
