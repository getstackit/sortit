package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName = "splat_session"
	oauthStateCookie  = "splat_oauth_state"
)

type ServiceConfig struct {
	Store      *SQLiteStore
	Provider   OAuthProvider
	WebOrigin  string
	SessionTTL time.Duration
}

type Service struct {
	store      *SQLiteStore
	provider   OAuthProvider
	webOrigin  string
	sessionTTL time.Duration
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
	}, nil
}

func (s *Service) BeginGitHubLogin(w http.ResponseWriter, r *http.Request, callbackPath string) error {
	state, _, err := newSecretToken("st")
	if err != nil {
		return err
	}

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
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		return fmt.Errorf("code and state are required")
	}

	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || strings.TrimSpace(stateCookie.Value) == "" {
		return ErrUnauthorized
	}
	if state != strings.TrimSpace(stateCookie.Value) {
		return ErrUnauthorized
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   -1,
	})

	oauthUser, err := s.provider.Exchange(r.Context(), code, callbackURL(r, callbackPath))
	if err != nil {
		return err
	}

	user, err := s.store.UpsertOAuthUser(r.Context(), oauthUser)
	if err != nil {
		return err
	}

	rawSession, err := s.store.CreateSession(r.Context(), user.ID, time.Now().UTC().Add(s.sessionTTL))
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

	http.Redirect(w, r, redirectTarget(s.webOrigin), http.StatusFound)
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

func (s *Service) CreateAPIToken(ctx context.Context, principal Principal) (APIToken, string, error) {
	return s.store.CreateAPIToken(ctx, principal.UserID)
}

func (s *Service) ListAPITokens(ctx context.Context, principal Principal) ([]APIToken, error) {
	return s.store.ListAPITokens(ctx, principal.UserID)
}

func (s *Service) RevokeAPIToken(ctx context.Context, principal Principal, tokenID string) error {
	return s.store.RevokeAPIToken(ctx, principal.UserID, tokenID)
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

func redirectTarget(webOrigin string) string {
	if webOrigin == "" {
		return "/"
	}
	return webOrigin + "/"
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
