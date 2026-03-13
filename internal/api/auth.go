package api

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"splat/internal/auth"
)

type authSessionResponse struct {
	User auth.User `json:"user"`
}

type createAPITokenResponse struct {
	Token    string        `json:"token"`
	Metadata auth.APIToken `json:"metadata"`
}

type apiTokensResponse struct {
	Tokens []auth.APIToken `json:"tokens"`
}

type cliLoginStartResponse struct {
	LoginID       string    `json:"loginId"`
	StartURL      string    `json:"startUrl"`
	ExchangeToken string    `json:"exchangeToken"`
	IntervalMS    int       `json:"intervalMs"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type cliLoginExchangeRequest struct {
	ExchangeToken string `json:"exchangeToken"`
}

type cliLoginExchangeResponse struct {
	Status   string        `json:"status"`
	User     auth.User     `json:"user,omitempty"`
	Token    string        `json:"token,omitempty"`
	Metadata auth.APIToken `json:"metadata,omitempty"`
}

func (s *Server) handleAuthGitHubStart(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		writeError(w, http.StatusNotImplemented, "authentication is not configured")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if err := s.authService.BeginGitHubLogin(w, r, authCallbackPath(r.URL.Path)); err != nil {
		writeInternalError(w, r, "failed to start github login", err)
		return
	}
}

func (s *Server) handleAuthGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		writeError(w, http.StatusNotImplemented, "authentication is not configured")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if err := s.authService.CompleteGitHubLogin(w, r, r.URL.Path); err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "authentication failed")
			return
		}
		writeInternalError(w, r, "failed to complete github login", err)
		return
	}
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		writeError(w, http.StatusNotImplemented, "authentication is not configured")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	principal, err := s.authService.CurrentPrincipal(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, authSessionResponse{User: principal.User()})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		writeError(w, http.StatusNotImplemented, "authentication is not configured")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if err := s.authService.Logout(w, r); err != nil {
		writeInternalError(w, r, "failed to logout", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthTokens(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		writeError(w, http.StatusNotImplemented, "authentication is not configured")
		return
	}

	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		tokens, err := s.authService.ListAPITokens(r.Context(), principal)
		if err != nil {
			writeInternalError(w, r, "failed to list api tokens", err)
			return
		}
		if tokens == nil {
			tokens = []auth.APIToken{}
		}
		writeJSON(w, http.StatusOK, apiTokensResponse{Tokens: tokens})
	case http.MethodPost:
		token, rawToken, err := s.authService.CreateAPIToken(r.Context(), principal)
		if err != nil {
			writeInternalError(w, r, "failed to create api token", err)
			return
		}
		writeJSON(w, http.StatusCreated, createAPITokenResponse{
			Token:    rawToken,
			Metadata: token,
		})
	default:
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAuthTokenByID(route string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authService == nil {
			writeError(w, http.StatusNotImplemented, "authentication is not configured")
			return
		}

		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		tail := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, route))
		segments := strings.Split(tail, "/")
		if len(segments) != 2 || strings.TrimSpace(segments[0]) == "" || segments[1] != "revoke" {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		if err := s.authService.RevokeAPIToken(r.Context(), principal, segments[0]); err != nil {
			if errors.Is(err, auth.ErrTokenNotFound) {
				writeError(w, http.StatusNotFound, "token not found")
				return
			}
			writeInternalError(w, r, "failed to revoke api token", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	}
}

func (s *Server) handleAuthCLILogin(w http.ResponseWriter, r *http.Request) {
	if s.authService == nil {
		writeError(w, http.StatusNotImplemented, "authentication is not configured")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	start, err := s.authService.BeginCLILogin()
	if err != nil {
		writeInternalError(w, r, "failed to start cli login", err)
		return
	}

	writeJSON(w, http.StatusCreated, cliLoginStartResponse{
		LoginID:       start.ID,
		StartURL:      cliLoginPageURL(s.authService.WebOrigin(), start.ID),
		ExchangeToken: start.ExchangeToken,
		IntervalMS:    int(start.PollInterval / time.Millisecond),
		ExpiresAt:     start.ExpiresAt,
	})
}

func (s *Server) handleAuthCLILoginExchange(route string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authService == nil {
			writeError(w, http.StatusNotImplemented, "authentication is not configured")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		loginID, err := parseCLILoginID(route, r.URL.Path, "exchange")
		if err != nil {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		request, err := decodeJSON[cliLoginExchangeRequest](r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		result, err := s.authService.ExchangeCLILogin(loginID, strings.TrimSpace(request.ExchangeToken))
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrCLILoginPending):
				writeJSON(w, http.StatusAccepted, cliLoginExchangeResponse{Status: "pending"})
				return
			case errors.Is(err, auth.ErrCLILoginNotFound), errors.Is(err, auth.ErrCLILoginConsumed):
				writeError(w, http.StatusNotFound, "cli login not found")
				return
			case errors.Is(err, auth.ErrCLILoginExpired):
				writeError(w, http.StatusGone, "cli login expired")
				return
			case errors.Is(err, auth.ErrUnauthorized):
				writeError(w, http.StatusUnauthorized, "authentication failed")
				return
			default:
				writeInternalError(w, r, "failed to exchange cli login", err)
				return
			}
		}

		writeJSON(w, http.StatusOK, cliLoginExchangeResponse{
			Status:   "complete",
			User:     result.User,
			Token:    result.Token,
			Metadata: result.Metadata,
		})
	}
}

func (s *Server) handleAuthCLILoginComplete(route string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authService == nil {
			writeError(w, http.StatusNotImplemented, "authentication is not configured")
			return
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		loginID, err := parseCLILoginID(route, r.URL.Path, "complete")
		if err != nil {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}

		result, err := s.authService.CompleteCLILogin(r.Context(), loginID, principal.User())
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrCLILoginNotFound):
				writeError(w, http.StatusNotFound, "cli login not found")
			case errors.Is(err, auth.ErrCLILoginExpired):
				writeError(w, http.StatusGone, "cli login expired")
			default:
				writeInternalError(w, r, "failed to complete cli login", err)
			}
			return
		}

		writeJSON(w, http.StatusOK, cliLoginExchangeResponse{
			Status:   "complete",
			User:     result.User,
			Metadata: result.Metadata,
		})
	}
}

func authCallbackPath(startPath string) string {
	callbackPath := path.Clean(strings.TrimSuffix(startPath, "/start") + "/callback")
	if !strings.HasPrefix(callbackPath, "/") {
		return "/" + callbackPath
	}
	return callbackPath
}

func authTokenItemRoute(prefix string) string {
	return path.Join(prefix, "auth", "tokens") + "/"
}

func authCLILoginExchangeRoute(prefix string) string {
	return path.Join(prefix, "auth", "cli", "login") + "/"
}

func authCLILoginCompleteRoute(prefix string) string {
	return path.Join(prefix, "auth", "cli", "login") + "/"
}

func parseCLILoginAction(route, requestPath string) (string, string, error) {
	tail := strings.TrimSpace(strings.TrimPrefix(requestPath, route))
	segments := strings.Split(tail, "/")
	if len(segments) != 2 || strings.TrimSpace(segments[0]) == "" || strings.TrimSpace(segments[1]) == "" {
		return "", "", fmt.Errorf("invalid cli auth route")
	}
	return segments[0], segments[1], nil
}

func parseCLILoginID(route, requestPath, action string) (string, error) {
	loginID, gotAction, err := parseCLILoginAction(route, requestPath)
	if err != nil {
		return "", err
	}
	if gotAction != action {
		return "", fmt.Errorf("invalid cli auth action")
	}
	return loginID, nil
}

func cliLoginPageURL(webOrigin, loginID string) string {
	base := strings.TrimRight(strings.TrimSpace(webOrigin), "/")
	if base == "" {
		base = "http://localhost:3000"
	}
	return fmt.Sprintf("%s/auth/cli-login?login_id=%s", base, loginID)
}
