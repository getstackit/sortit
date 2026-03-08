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

	"splat/internal/ai"
	"splat/internal/issues"
	mcpserver "splat/internal/mcp"
)

type ServerConfig struct {
	Port        int
	CORSOrigins []string
	APIPrefixes []string
	Analyzer    *ai.Analyzer
	IssueStore  issues.Store
}

type Server struct {
	config        ServerConfig
	httpServer    *http.Server
	startedAt     time.Time
	analyzer      *ai.Analyzer
	issueAnalyzer *ai.Analyzer
	issueStore    issues.Store
	tagStore      issueTagStore
}

type issueTagStore interface {
	ListTags(context.Context) ([]issues.Tag, error)
	UpsertTags(context.Context, []issues.Tag) error
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
	store := cfg.IssueStore
	if store == nil {
		store = issues.NewInMemoryStore(nil)
	}

	return &Server{
		config:        cfg,
		startedAt:     time.Now().UTC(),
		analyzer:      cfg.Analyzer,
		issueAnalyzer: fallbackAnalyzer(cfg.Analyzer),
		issueStore:    store,
		tagStore:      tagStoreFromIssueStore(store),
	}
}

func tagStoreFromIssueStore(store issues.Store) issueTagStore {
	tagStore, ok := store.(issueTagStore)
	if !ok {
		return nil
	}
	return tagStore
}

func fallbackAnalyzer(analyzer *ai.Analyzer) *ai.Analyzer {
	if analyzer != nil {
		return analyzer
	}
	return defaultIssueAnalyzer()
}

func (s *Server) Initialize(ctx context.Context) error {
	return s.ensureStoredTags(ctx, issues.DefaultTags())
}

func (s *Server) Handler() http.Handler {
	apiMux := http.NewServeMux()
	apiRoutes := make(map[string]struct{})

	for _, prefix := range normalizeAPIPrefixes(s.config.APIPrefixes) {
		healthRoute := path.Join(prefix, "health")
		issuesRoute := path.Join(prefix, "issues")
		issuesCompareRoute := path.Join(prefix, "issues", "compare")
		issueItemSubtreeRoute := issueItemRoute(prefix)
		tagsRoute := path.Join(prefix, "tags")
		mapRoute := path.Join(prefix, "map")
		mapEdgesRoute := path.Join(prefix, "map", "edges")
		debugAnalyzeRoute := path.Join(prefix, "debug", "issues", "analyze")
		debugSampleRoute := path.Join(prefix, "debug", "issues", "sample")
		debugResetRoute := path.Join(prefix, "debug", "issues", "reset")

		apiRoutes[healthRoute] = struct{}{}
		apiRoutes[issuesRoute] = struct{}{}
		apiRoutes[issuesCompareRoute] = struct{}{}
		apiRoutes[issueItemSubtreeRoute] = struct{}{}
		apiRoutes[tagsRoute] = struct{}{}
		apiRoutes[mapRoute] = struct{}{}
		apiRoutes[mapEdgesRoute] = struct{}{}
		apiRoutes[debugAnalyzeRoute] = struct{}{}
		apiRoutes[debugSampleRoute] = struct{}{}
		apiRoutes[debugResetRoute] = struct{}{}

		apiMux.HandleFunc(healthRoute, s.handleHealth)
		apiMux.HandleFunc(issuesRoute, s.handleIssues)
		apiMux.HandleFunc(issuesCompareRoute, s.handleIssueCompare)
		apiMux.HandleFunc(issueItemSubtreeRoute, s.handleIssueByID(issueItemSubtreeRoute))
		apiMux.HandleFunc(tagsRoute, s.handleTags)
		apiMux.HandleFunc(mapRoute, s.handleMap)
		apiMux.HandleFunc(mapEdgesRoute, s.handleMapEdges)
		apiMux.HandleFunc(debugAnalyzeRoute, s.handleDebugIssueAnalyze)
		apiMux.HandleFunc(debugSampleRoute, s.handleDebugIssueSampleLoad)
		apiMux.HandleFunc(debugResetRoute, s.handleDebugIssueReset)
	}

	mcpHandler := mcpserver.NewHandler(mcpserver.ServerConfig{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d%s", s.config.Port, normalizeAPIPrefixes(s.config.APIPrefixes)[0]),
	})

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRegisteredAPIRoute(r.URL.Path, apiRoutes) {
			apiMux.ServeHTTP(w, r)
			return
		}

		if r.URL.Path == "/mcp" {
			mcpHandler.ServeHTTP(w, r)
			return
		}

		if r.URL.Path == "/" {
			writeJSON(w, http.StatusOK, map[string]string{
				"name":   "splat-server",
				"status": "ok",
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			writeError(w, http.StatusNotFound, "route not found")
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

	log.Printf("splat server listening on http://localhost:%d", s.config.Port)
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
		Name:      "splat-server",
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

func writeInternalError(w http.ResponseWriter, r *http.Request, message string, err error) {
	if err != nil {
		log.Printf("500 %s %s: %s: %v", r.Method, r.URL.Path, message, err)
	} else {
		log.Printf("500 %s %s: %s", r.Method, r.URL.Path, message)
	}
	writeError(w, http.StatusInternalServerError, message)
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
