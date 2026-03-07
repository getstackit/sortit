package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
)

type ServerConfig struct {
	Port        int
	CORSOrigins []string
	APIPrefixes []string
}

type Server struct {
	config     ServerConfig
	httpServer *http.Server
	startedAt  time.Time
}

type healthResponse struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Uptime    string `json:"uptime"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewServer(cfg ServerConfig) *Server {
	return &Server{
		config:    cfg,
		startedAt: time.Now().UTC(),
	}
}

func (s *Server) Handler() http.Handler {
	apiMux := http.NewServeMux()
	apiRoutes := make(map[string]struct{})

	for _, prefix := range normalizeAPIPrefixes(s.config.APIPrefixes) {
		healthRoute := path.Join(prefix, "health")
		mapRoute := path.Join(prefix, "map")
		mapEdgesRoute := path.Join(prefix, "map", "edges")

		apiRoutes[healthRoute] = struct{}{}
		apiRoutes[mapRoute] = struct{}{}
		apiRoutes[mapEdgesRoute] = struct{}{}

		apiMux.HandleFunc(healthRoute, s.handleHealth)
		apiMux.HandleFunc(mapRoute, s.handleMap)
		apiMux.HandleFunc(mapEdgesRoute, s.handleMapEdges)
	}

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRegisteredAPIRoute(r.URL.Path, apiRoutes) {
			apiMux.ServeHTTP(w, r)
			return
		}

		if r.URL.Path == "/" {
			writeJSON(w, http.StatusOK, map[string]string{
				"name":   "bored-server",
				"status": "ok",
			})
			return
		}

		http.NotFound(w, r)
	})

	handler := corsMiddleware(s.config.CORSOrigins, apiRoutes, root)
	return loggingMiddleware(handler)
}

func (s *Server) Start() error {
	handler := s.Handler()

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.config.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("bored server listening on http://localhost:%d", s.config.Port)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, healthResponse{
		Name:      "bored-server",
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(s.startedAt).Round(time.Second).String(),
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func normalizeAPIPrefixes(prefixes []string) []string {
	if len(prefixes) == 0 {
		return []string{"/api/v1", "/api"}
	}

	seen := make(map[string]struct{}, len(prefixes))
	normalized := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		if len(prefix) > 1 {
			prefix = strings.TrimRight(prefix, "/")
		}
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		normalized = append(normalized, prefix)
	}

	if len(normalized) == 0 {
		return []string{"/api/v1", "/api"}
	}
	return normalized
}
