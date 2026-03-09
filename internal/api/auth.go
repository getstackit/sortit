package api

import (
	"errors"
	"net/http"
	"path"
	"strings"

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
