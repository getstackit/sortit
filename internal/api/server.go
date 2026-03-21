package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"splat/internal/ai"
	"splat/internal/auth"
	"splat/internal/commands"
	"splat/internal/issues"
	mcpserver "splat/internal/mcp"
	"splat/internal/queries"
	"splat/internal/services"
	"splat/internal/tracing"
)

type ServerConfig struct {
	Port        int
	CORSOrigins []string
	APIPrefixes []string
	Logger      *slog.Logger
	Analyzer    *ai.Analyzer
	IssueStore  issues.Store
	Auth        *auth.Service
}

type Server struct {
	config              ServerConfig
	logger              *slog.Logger
	httpServer          *http.Server
	startedAt           time.Time
	revisions           *issues.RevisionTracker
	mapProjectionLoader *queries.MapProjectionLoader
	enrichmentWorker    *services.IssueEnrichmentWorker
	enrichmentCancel    context.CancelFunc
	enrichmentDone      chan struct{}
	createIssue         commands.CreateIssueHandler
	refineIssue         commands.RefineIssueHandler
	progressIssue       commands.ProgressIssueHandler
	closeIssue          commands.CloseIssueHandler
	reopenIssue         commands.ReopenIssueHandler
	assignIssue         commands.AssignIssueHandler
	splitIssue          commands.SplitIssueHandler
	combineIssues       commands.CombineIssuesHandler
	linkIssues          commands.LinkIssuesHandler
	listIssues          queries.ListIssuesHandler
	listActivity        queries.ListActivityHandler
	getIssue            queries.GetIssueHandler
	compareIssues       queries.CompareIssuesHandler
	searchIssues        queries.SearchIssuesHandler
	searchUnified       queries.SearchUnifiedHandler
	listTags            queries.ListTagsHandler
	getMap              queries.MapHandler
	getMapEdges         queries.EdgeHandler
	debugAnalyzeIssue   queries.DebugAnalyzeIssueHandler
	debugFactorWeights  queries.DebugFactorWeightsHandler
	exploreIssue        queries.ExploreIssueHandler
	getPersonProfile    queries.GetPersonProfileHandler
	getPersonDetail     queries.GetPersonDetailHandler
	workCorrelations    queries.WorkCorrelationsHandler
	authService         *auth.Service
	catalog             *services.CatalogService
}

type issueTagStore interface {
	ListTags(context.Context) ([]issues.Tag, error)
	UpsertTags(context.Context, []issues.Tag) error
	UpdateTagSpecificity(ctx context.Context, name string, specificity, llm, embedding *float64, computedAt *time.Time) error
}

type issueStoreLoggerSetter interface {
	SetLogger(*slog.Logger)
}

func mapProjectionStoreFromIssueStore(store issues.Store) issues.MapProjectionStorePersistence {
	if projectionStore, ok := store.(issues.MapProjectionStorePersistence); ok {
		return projectionStore
	}
	return nil
}

func mapProjectionInvalidatorFromIssueStore(store issues.Store) issues.MapProjectionInvalidator {
	if invalidator, ok := store.(issues.MapProjectionInvalidator); ok {
		return invalidator
	}
	return nil
}

func unitOfWorkBeginnerFromStore(store issues.Store) issues.UnitOfWorkBeginner {
	if beginner, ok := store.(issues.UnitOfWorkBeginner); ok {
		return beginner
	}
	return nil
}

func semanticSearchStoreFromStore(store issues.Store) issues.SemanticSearchStore {
	searchStore, ok := store.(issues.SemanticSearchStore)
	if !ok {
		return nil
	}
	return searchStore
}

func enrichmentJobClaimerFromStore(store issues.Store) issues.EnrichmentJobClaimer {
	claimer, ok := store.(issues.EnrichmentJobClaimer)
	if !ok {
		return nil
	}
	return claimer
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

type revisionResponse struct {
	Revision uint64 `json:"revision"`
}

func tagStoreFromIssueStore(store issues.Store) issueTagStore {
	tagStore, ok := store.(issueTagStore)
	if !ok {
		return nil
	}
	return tagStore
}

func (s *Server) Initialize(ctx context.Context) error {
	return s.catalog.EnsureStoredTags(ctx, issues.DefaultTags())
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// Global middleware stack (outermost → innermost).
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(tracing.Middleware)
	r.Use(middleware.RequestLogger(&slogLogFormatter{}))
	r.Use(middleware.Compress(5))

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed))
	})

	mcpHandler := mcpserver.NewHandler(mcpserver.ServerConfig{
		CreateIssue:      s.createIssue,
		RefineIssue:      s.refineIssue,
		ProgressIssue:    s.progressIssue,
		CloseIssue:       s.closeIssue,
		AssignIssue:      s.assignIssue,
		SplitIssue:       s.splitIssue,
		CombineIssues:    s.combineIssues,
		LinkIssues:       s.linkIssues,
		GetIssue:         s.getIssue,
		ListTags:         s.listTags,
		SearchIssues:     s.searchIssues,
		ExploreIssue:     s.exploreIssue,
		GetPersonProfile: s.getPersonProfile,
		WorkCorrelations: s.workCorrelations,
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"name":   "splat-server",
			"status": "ok",
		})
	})

	r.Handle("/mcp", bearerAuthMiddleware(s.authService, mcpHandler))

	for _, prefix := range normalizeAPIPrefixes(s.config.APIPrefixes) {
		r.Route(prefix, func(r chi.Router) {
			r.Use(newCORSMiddleware(s.config.CORSOrigins))
			s.registerDedicatedAPIRoutes(r)
		})
	}

	r.Route("/api/ui", func(r chi.Router) {
		r.Use(newCORSMiddleware(s.config.CORSOrigins))
		s.registerUIRoutes(r)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		http.NotFound(w, r)
	})

	s.logRoutes(r)

	return r
}

func (s *Server) registerPublicRoutes(r chi.Router) {
	r.Get("/health", s.handleHealth)

	r.Route("/auth", func(r chi.Router) {
		r.Get("/github/start", s.handleAuthGitHubStart)
		r.Get("/github/callback", s.handleAuthGitHubCallback)
		r.Get("/session", s.handleAuthSession)
		r.Post("/logout", s.handleAuthLogout)
	})
}

func (s *Server) registerAuthRoutes(r chi.Router) {
	r.Get("/auth/tokens", s.handleAuthTokenListOrCreate)
	r.Post("/auth/tokens", s.handleAuthTokenListOrCreate)
	r.Post("/auth/tokens/{tokenID}/revoke", s.handleAuthTokenRevoke)
}

func (s *Server) registerDedicatedAPIRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		s.registerPublicRoutes(r)
		r.Post("/auth/cli/login", s.handleAuthCLILogin)
		r.Post("/auth/cli/login/{loginID}/exchange", s.handleAuthCLILoginExchange)
	})

	r.Group(func(r chi.Router) {
		r.Use(authRequiredMiddleware(s.authService))
		s.registerAuthRoutes(r)
		s.registerIssueRoutes(r)
		s.registerTagRoutes(r)
		r.Get("/people/correlations", s.handleWorkCorrelations)
		r.Get("/people/{person}", s.handlePersonDetail)
		r.Get("/people/{person}/profile", s.handlePersonProfileRoute)
	})
}

func (s *Server) registerUIRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		s.registerPublicRoutes(r)
	})

	r.Group(func(r chi.Router) {
		r.Use(authRequiredMiddleware(s.authService))
		s.registerAuthRoutes(r)
		r.Post("/auth/cli/login/{loginID}/complete", s.handleAuthCLILoginComplete)
		s.registerIssueRoutes(r)
		r.Get("/activity", s.handleActivity)
		r.Post("/issues/compare", s.handleIssueCompare)
		r.Get("/search", s.handleUnifiedSearch)
		s.registerTagRoutes(r)
		r.Get("/revision", s.handleRevision)
		r.Get("/map", s.handleMap)
		r.Get("/map/edges", s.handleMapEdges)
		r.Get("/people/correlations", s.handleWorkCorrelations)
		r.Get("/people/{person}", s.handlePersonDetail)
		r.Get("/people/{person}/profile", s.handlePersonProfileRoute)
		r.Route("/debug", func(r chi.Router) {
			r.Use(middleware.Timeout(debugRequestTimeout))
			r.Post("/issues/analyze", s.handleDebugIssueAnalyze)
			r.Post("/map-projection/invalidate", s.handleDebugInvalidateMapProjection)
			r.Post("/tags/rescore", s.handleDebugRescoreTags)
			r.Get("/factor-weights", s.handleDebugFactorWeights)
		})
	})
}

func (s *Server) registerIssueRoutes(r chi.Router) {
	r.Get("/issues", s.handleIssueList)
	r.Post("/issues", s.handleIssueCreate)
	r.Get("/issues/search", s.handleIssueSearch)
	r.Post("/issues/combine", s.handleIssueCombine)
	r.Post("/issues/link", s.handleIssueLink)
	r.Post("/issues/refine", s.handleIssueRefineBatch)
	r.Post("/issues/progress", s.handleIssueProgressBatch)
	r.Post("/issues/close", s.handleIssueCloseBatch)
	r.Post("/issues/assign", s.handleIssueAssignBatch)
	r.Get("/issues/{id}", s.handleGetIssue)
	r.Post("/issues/{id}/close", s.handleCloseIssue)
	r.Post("/issues/{id}/refine", s.handleRefineIssue)
	r.Get("/issues/{id}/explore", s.handleExploreIssue)
	r.Post("/issues/{id}/progress", s.handleProgressIssue)
	r.Post("/issues/{id}/reopen", s.handleReopenIssue)
	r.Post("/issues/{id}/assign", s.handleAssignIssue)
	r.Post("/issues/{id}/split", s.handleSplitIssue)
}

func (s *Server) registerTagRoutes(r chi.Router) {
	r.Get("/tags", s.handleTags)
	r.Post("/tags/merge", s.handleTagMerge)
	r.Post("/tags/dismiss", s.handleTagDismiss)
	r.Get("/tags/dismissed", s.handleTagDismissedList)
}

const debugRequestTimeout = 120 * time.Second

func (s *Server) logRoutes(r *chi.Mux) {
	walkFn := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		s.logger.Info("route registered", "method", method, "path", route)
		return nil
	}
	if err := chi.Walk(r, walkFn); err != nil {
		s.logger.Error("failed to walk routes", "error", err)
	}
}

func (s *Server) Start() error {
	handler := s.Handler()
	if s.enrichmentWorker != nil && s.enrichmentCancel == nil {
		runCtx, cancel := context.WithCancel(context.Background())
		s.enrichmentCancel = cancel
		s.enrichmentDone = make(chan struct{})
		go func() {
			defer close(s.enrichmentDone)
			if err := s.enrichmentWorker.Run(runCtx); err != nil && runCtx.Err() == nil {
				s.logger.Error("issue enrichment worker stopped", "error", err)
			}
		}()
	}

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.config.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.logger.Info("server listening", "port", s.config.Port)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.enrichmentCancel != nil {
		s.enrichmentCancel()
		if s.enrichmentDone != nil {
			select {
			case <-s.enrichmentDone:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		s.enrichmentCancel = nil
		s.enrichmentDone = nil
	}
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *Server) ProcessPendingEnrichment(ctx context.Context) error {
	if s.enrichmentWorker == nil {
		return nil
	}
	for {
		processed, err := s.enrichmentWorker.ProcessOne(ctx)
		if err != nil {
			return err
		}
		if !processed {
			return nil
		}
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Name:      "splat-server",
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(s.startedAt).Round(time.Second).String(),
	})
}

func (s *Server) handleRevision(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, revisionResponse{
		Revision: s.revisions.Revision(),
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
	attrs := []any{
		"status", 500,
		"method", r.Method,
		"path", r.URL.Path,
	}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	slog.ErrorContext(r.Context(), message, attrs...)
	writeError(w, http.StatusInternalServerError, message)
}

func NewServer(cfg ServerConfig) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	baseStore := cfg.IssueStore
	if baseStore == nil {
		baseStore = issues.NewInMemoryStore(nil)
	}
	if loggerSetter, ok := baseStore.(issueStoreLoggerSetter); ok {
		loggerSetter.SetLogger(logger)
	}
	revisions := issues.NewRevisionTracker()
	observed := issues.NewObservedStore(baseStore, revisions)
	store := observed

	// EventStore: available on ObservedStore which wraps both Store and EventStore
	var events issues.EventStore = observed

	// CommandRunner manages DB transaction lifecycle for command handlers.
	uowBeginner := unitOfWorkBeginnerFromStore(baseStore)
	eventBus := issues.NewEventBus()
	if listener := projectionInvalidationListener(
		mapProjectionInvalidatorFromIssueStore(baseStore),
	); listener != nil {
		eventBus.Subscribe(listener)
	}
	runner := &commands.CommandRunner{
		DB:       uowBeginner,
		OnCommit: func() { revisions.Bump() },
	}

	tagStore := tagStoreFromIssueStore(store)
	commandAnalyzer := services.FallbackAnalyzer(cfg.Analyzer)
	catalogLogger := logger.With("component", "catalog")
	catalog := services.NewCatalogService(tagStore, commandAnalyzer, catalogLogger)
	enricherLogger := logger.With("component", "enricher")
	enricher := services.NewIssueEnricher(commandAnalyzer, catalog, enricherLogger)
	var enrichmentWorker *services.IssueEnrichmentWorker
	workerLogger := logger.With("component", "enrichment_worker")
	if claimer := enrichmentJobClaimerFromStore(baseStore); claimer != nil && uowBeginner != nil {
		invalidator := mapProjectionInvalidatorFromIssueStore(baseStore)
		enrichmentWorker = &services.IssueEnrichmentWorker{
			Logger:   workerLogger,
			Store:    baseStore,
			DB:       uowBeginner,
			Jobs:     claimer,
			Enricher: enricher,
			Catalog:  catalog,
			OnStateChange: func(ctx context.Context, applied bool) {
				revisions.Bump()
				if applied && invalidator != nil {
					if err := invalidator.InvalidateMapProjections(ctx); err != nil {
						workerLogger.ErrorContext(ctx, "failed to invalidate map projections after enrichment", "error", err)
					}
				}
			},
		}
	}
	mapProjectionLoader := &queries.MapProjectionLoader{
		Store:       store,
		Catalog:     catalog,
		Revisions:   revisions,
		Projections: mapProjectionStoreFromIssueStore(baseStore),
	}

	return &Server{
		config:              cfg,
		logger:              logger,
		startedAt:           time.Now().UTC(),
		revisions:           revisions,
		mapProjectionLoader: mapProjectionLoader,
		enrichmentWorker:    enrichmentWorker,
		createIssue: commands.CreateIssueHandler{
			Logger:   logger.With("command", "create_issue"),
			Runner:   runner,
			Enricher: enricher,
			Events:   eventBus,
		},
		refineIssue: commands.RefineIssueHandler{
			Runner:   runner,
			Store:    store,
			Enricher: enricher,
			Events:   eventBus,
		},
		progressIssue: commands.ProgressIssueHandler{Runner: runner, Events: eventBus},
		closeIssue:    commands.CloseIssueHandler{Runner: runner, Events: eventBus},
		reopenIssue: commands.ReopenIssueHandler{
			Runner: runner,
			Events: eventBus,
		},
		assignIssue: commands.AssignIssueHandler{
			Runner: runner,
			Events: eventBus,
		},
		splitIssue: commands.SplitIssueHandler{
			Runner:   runner,
			Enricher: enricher,
			Events:   eventBus,
		},
		combineIssues: commands.CombineIssuesHandler{
			Runner:   runner,
			Store:    store,
			Enricher: enricher,
			Events:   eventBus,
		},
		linkIssues: commands.LinkIssuesHandler{
			Runner: runner,
			Events: eventBus,
		},
		listIssues:    queries.ListIssuesHandler{Store: store},
		listActivity:  queries.ListActivityHandler{Events: events},
		getIssue:      queries.GetIssueHandler{Store: store, Logger: logger.With("query", "get_issue")},
		compareIssues: queries.CompareIssuesHandler{Reader: store},
		searchIssues: queries.SearchIssuesHandler{
			Analyzer: commandAnalyzer,
			Catalog:  catalog,
			Store:    baseStore,
		},
		searchUnified: queries.SearchUnifiedHandler{
			Analyzer: commandAnalyzer,
			Catalog:  catalog,
			Store:    baseStore,
		},
		exploreIssue: queries.ExploreIssueHandler{
			Reader:       store,
			DetailReader: store,
			SearchStore:  semanticSearchStoreFromStore(baseStore),
			Catalog:      catalog,
		},
		listTags:           queries.ListTagsHandler{Catalog: catalog},
		getMap:             queries.MapHandler{IssueStore: store, Catalog: catalog, Projection: mapProjectionLoader},
		getMapEdges:        queries.EdgeHandler{IssueStore: store, Catalog: catalog, Projection: mapProjectionLoader},
		debugAnalyzeIssue:  queries.DebugAnalyzeIssueHandler{Analyzer: cfg.Analyzer, Catalog: catalog, Store: store},
		debugFactorWeights: queries.DebugFactorWeightsHandler{Store: store, Catalog: catalog},
		getPersonProfile:   queries.GetPersonProfileHandler{Store: store, Catalog: catalog},
		getPersonDetail:    queries.GetPersonDetailHandler{Store: store, Catalog: catalog},
		workCorrelations:   queries.WorkCorrelationsHandler{Store: store, Catalog: catalog},
		authService:        cfg.Auth,
		catalog:            catalog,
	}
}

func projectionInvalidationListener(
	invalidator issues.MapProjectionInvalidator,
) issues.EventListener {
	if invalidator == nil {
		return nil
	}

	return func(ctx context.Context, event issues.Event) {
		if !shouldInvalidateMapProjection(event.Kind) {
			return
		}

		if err := invalidator.InvalidateMapProjections(ctx); err != nil {
			slog.ErrorContext(ctx, "failed to invalidate map projections",
				"event_kind", event.Kind,
				"issue_id", event.IssueID,
				"error", err,
			)
		}
	}
}

func shouldInvalidateMapProjection(eventKind string) bool {
	switch eventKind {
	case "assigned", "closed", "progress", "reopened":
		return false
	default:
		return true
	}
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
