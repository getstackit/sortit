package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"splat/internal/auth"
)

func newCORSMiddleware(origins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	})
}

func authRequiredMiddleware(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if service == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			principal, err := service.AuthenticateRequest(r, false)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
		})
	}
}

func bearerAuthMiddleware(service *auth.Service, next http.Handler) http.Handler {
	if service == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := service.AuthenticateRequest(r, true)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

// slogLogFormatter adapts chi's middleware.RequestLogger to slog.
type slogLogFormatter struct{}

func (s *slogLogFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	return &slogLogEntry{
		method: r.Method,
		path:   r.URL.Path,
		ctx:    r,
	}
}

type slogLogEntry struct {
	method string
	path   string
	ctx    *http.Request
}

func (e *slogLogEntry) Write(status, bytes int, _ http.Header, elapsed time.Duration, _ any) {
	level := slog.LevelInfo
	if status >= 500 {
		level = slog.LevelError
	}
	slog.LogAttrs(e.ctx.Context(), level, "request",
		slog.String("method", e.method),
		slog.String("path", e.path),
		slog.Int("status", status),
		slog.Int("bytes", bytes),
		slog.Duration("duration", elapsed.Round(time.Millisecond)),
	)
}

func (e *slogLogEntry) Panic(v any, stack []byte) {
	slog.ErrorContext(e.ctx.Context(), "panic",
		slog.String("method", e.method),
		slog.String("path", e.path),
		slog.String("panic", fmt.Sprintf("%v", v)),
		slog.String("stack", string(stack)),
	)
}
