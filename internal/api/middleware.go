package api

import (
	"log"
	"net/http"
	"strings"
	"time"

	"splat/internal/auth"
)

func corsMiddleware(origins []string, apiRoutes map[string]struct{}, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin == "" {
			continue
		}
		allowed[origin] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimRight(r.Header.Get("Origin"), "/")
		_, originAllowed := allowed[origin]
		if originAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}

		if r.Method == http.MethodOptions {
			if originAllowed && isCORSPreflight(r) && isRegisteredAPIRoute(r.URL.Path, apiRoutes) {
				w.Header().Set("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func authMiddleware(service *auth.Service, publicAPIRoutes map[string]struct{}, next http.Handler) http.Handler {
	if service == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || isPublicRoute(path, publicAPIRoutes) {
			next.ServeHTTP(w, r)
			return
		}

		bearerOnly := path == "/mcp"
		if !bearerOnly && !strings.HasPrefix(path, "/api/") && path != "/api" {
			next.ServeHTTP(w, r)
			return
		}

		principal, err := service.AuthenticateRequest(r, bearerOnly)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

func isPublicRoute(requestPath string, publicAPIRoutes map[string]struct{}) bool {
	if _, ok := publicAPIRoutes[requestPath]; ok {
		return true
	}
	return false
}

func isCORSPreflight(r *http.Request) bool {
	return r.Header.Get("Access-Control-Request-Method") != ""
}

func isRegisteredAPIRoute(requestPath string, apiRoutes map[string]struct{}) bool {
	if _, ok := apiRoutes[requestPath]; ok {
		return true
	}

	for route := range apiRoutes {
		if strings.HasSuffix(route, "/") && strings.HasPrefix(requestPath, route) {
			return true
		}
	}

	return false
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		log.Printf(
			"%s %s %d %s",
			r.Method,
			r.URL.Path,
			recorder.status,
			time.Since(start).Round(time.Millisecond),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
