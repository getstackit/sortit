package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"sortit/internal/ai"
	"sortit/internal/auth"
	"sortit/internal/centering"
	"sortit/internal/curation"
	"sortit/internal/diagnostics"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issueevents"
	"sortit/internal/issues"
	issuecmd "sortit/internal/issues/commands"
	issueviews "sortit/internal/issues/views"
	"sortit/internal/issuexray"
	"sortit/internal/mapview"
	mcpserver "sortit/internal/mcp"
	"sortit/internal/memories"
	"sortit/internal/people"
	"sortit/internal/regions"
	"sortit/internal/ridgelambda"
	"sortit/internal/search"
	"sortit/internal/tagcooccurrence"
	"sortit/internal/tags"
	"sortit/internal/tracing"
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
	config                 ServerConfig
	logger                 *slog.Logger
	httpServer             *http.Server
	startedAt              time.Time
	revisions              *issues.RevisionTracker
	mapProjectionLoader    *mapview.MapProjectionLoader
	enrichmentWorker       *issueenrichment.IssueEnrichmentWorker
	enrichmentCancel       context.CancelFunc
	enrichmentDone         chan struct{}
	createIssue            issuecmd.CreateIssueHandler
	refineIssue            issuecmd.RefineIssueHandler
	progressIssue          issuecmd.ProgressIssueHandler
	closeIssue             issuecmd.CloseIssueHandler
	reopenIssue            issuecmd.ReopenIssueHandler
	assignIssue            issuecmd.AssignIssueHandler
	reEnrichIssue          issuecmd.ReEnrichIssueHandler
	splitIssue             issuecmd.SplitIssueHandler
	combineIssues          issuecmd.CombineIssuesHandler
	linkIssues             issuecmd.LinkIssuesHandler
	listIssues             issueviews.ListIssuesHandler
	listActivity           issueviews.ListActivityHandler
	getIssue               issueviews.GetIssueHandler
	compareIssues          issueviews.CompareIssuesHandler
	searchIssues           search.SearchIssuesHandler
	searchUnified          search.SearchUnifiedHandler
	listTags               issueviews.ListTagsHandler
	getMap                 mapview.MapHandler
	getMapEdges            mapview.EdgeHandler
	debugAnalyzeIssue      diagnostics.DebugAnalyzeIssueHandler
	debugEvalTags          diagnostics.DebugEvalTagsHandler
	debugFactorWeights     diagnostics.DebugFactorWeightsHandler
	debugTagHealth         diagnostics.DebugTagHealthHandler
	debugIssueR2           diagnostics.DebugIssueR2Handler
	debugTagCooccurrence   diagnostics.DebugTagCooccurrenceHandler
	debugRidgeScore        diagnostics.DebugRidgeScoreHandler
	debugEmbeddingFalls    diagnostics.DebugEmbeddingFallbacksHandler
	exploreIssue           mapview.ExploreIssueHandler
	getPersonProfile       people.GetPersonProfileHandler
	getPersonDetail        people.GetPersonDetailHandler
	workCorrelations       people.WorkCorrelationsHandler
	regions                *regions.Handler
	customRegions          *regions.CustomHandler
	cooccurrenceCache      *tagcooccurrence.Cache
	issueXRay              *issuexray.Handler
	authService            *auth.Service
	catalog                *tags.CatalogService
	memories               *memories.Service
	curation               *curation.Service
	curationDetector       *curation.Detector
	curationMemoryDetector *curation.MemoryDetector
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
	// Deployed behind a trusted reverse proxy that sets X-Forwarded-For; safe here.
	r.Use(middleware.RealIP) //nolint:staticcheck // SA1019: see GHSA-3fxj-6jh8-hvhx
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
		Memories:         s.memories,
		GetPersonProfile: s.getPersonProfile,
		WorkCorrelations: s.workCorrelations,
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"name":   "sortit-server",
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
		s.registerMemoryRoutes(r)
		s.registerCurationRoutes(r)
		s.registerTagRoutes(r)
		r.Get("/people/correlations", s.handleWorkCorrelations)
		r.Get("/people/{person}", s.handlePersonDetail)
		r.Get("/people/{person}/profile", s.handlePersonProfileRoute)
		r.Get("/regions", s.handleRegionsList)
		r.Get("/regions/orphans", s.handleRegionOrphans)
		r.Get("/regions/custom", s.handleCustomRegionList)
		r.Post("/regions/custom", s.handleCustomRegionCreate)
		r.Put("/regions/custom/{id}", s.handleCustomRegionUpdate)
		r.Delete("/regions/custom/{id}", s.handleCustomRegionDelete)
		r.Get("/regions/custom/{id}/definition", s.handleCustomRegionGet)
		r.Get("/regions/{kind}/{id}", s.handleRegionGet)
		r.Route("/debug", func(r chi.Router) {
			r.Use(middleware.Timeout(debugRequestTimeout))
			r.Get("/eval-tags", s.handleDebugEvalTags)
			r.Get("/factor-weights", s.handleDebugFactorWeights)
			r.Get("/tag-health", s.handleDebugTagHealth)
			r.Get("/issues/{id}/r2", s.handleDebugIssueR2)
			r.Get("/tag-cooccurrence", s.handleDebugTagCooccurrence)
			r.Get("/issues/{id}/ridge", s.handleDebugRidgeScore)
			r.Get("/embedding-fallbacks", s.handleDebugEmbeddingFallbacks)
		})
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
		s.registerMemoryRoutes(r)
		s.registerCurationRoutes(r)
		r.Get("/activity", s.handleActivity)
		r.Post("/issues/compare", s.handleIssueCompare)
		r.Get("/search", s.handleUnifiedSearch)
		s.registerTagRoutes(r)
		r.Get("/revision", s.handleRevision)
		r.Get("/revision/stream", s.handleRevisionStream)
		r.Get("/map", s.handleMap)
		r.Get("/map/edges", s.handleMapEdges)
		r.Get("/people/correlations", s.handleWorkCorrelations)
		r.Get("/people/{person}", s.handlePersonDetail)
		r.Get("/people/{person}/profile", s.handlePersonProfileRoute)
		r.Get("/regions", s.handleRegionsList)
		r.Get("/regions/orphans", s.handleRegionOrphans)
		r.Get("/regions/custom", s.handleCustomRegionList)
		r.Post("/regions/custom", s.handleCustomRegionCreate)
		r.Put("/regions/custom/{id}", s.handleCustomRegionUpdate)
		r.Delete("/regions/custom/{id}", s.handleCustomRegionDelete)
		r.Get("/regions/custom/{id}/definition", s.handleCustomRegionGet)
		r.Get("/regions/{kind}/{id}", s.handleRegionGet)
		r.Route("/debug", func(r chi.Router) {
			r.Use(middleware.Timeout(debugRequestTimeout))
			r.Post("/issues/analyze", s.handleDebugIssueAnalyze)
			r.Post("/map-projection/invalidate", s.handleDebugInvalidateMapProjection)
			r.Post("/tags/rescore", s.handleDebugRescoreTags)
			r.Get("/eval-tags", s.handleDebugEvalTags)
			r.Get("/factor-weights", s.handleDebugFactorWeights)
			r.Get("/tag-health", s.handleDebugTagHealth)
			r.Get("/issues/{id}/r2", s.handleDebugIssueR2)
			r.Get("/tag-cooccurrence", s.handleDebugTagCooccurrence)
			r.Get("/issues/{id}/ridge", s.handleDebugRidgeScore)
			r.Get("/embedding-fallbacks", s.handleDebugEmbeddingFallbacks)
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
	r.Post("/issues/re-enrich", s.handleReEnrichIssueBatch)
	r.Post("/issues/progress", s.handleIssueProgressBatch)
	r.Post("/issues/close", s.handleIssueCloseBatch)
	r.Post("/issues/assign", s.handleIssueAssignBatch)
	r.Get("/issues/{id}", s.handleGetIssue)
	r.Get("/issues/{id}/xray", s.handleIssueXRay)
	r.Post("/issues/{id}/close", s.handleCloseIssue)
	r.Post("/issues/{id}/refine", s.handleRefineIssue)
	r.Get("/issues/{id}/explore", s.handleExploreIssue)
	r.Post("/issues/{id}/progress", s.handleProgressIssue)
	r.Post("/issues/{id}/reopen", s.handleReopenIssue)
	r.Post("/issues/{id}/assign", s.handleAssignIssue)
	r.Post("/issues/{id}/re-enrich", s.handleReEnrichIssue)
	r.Post("/issues/{id}/split", s.handleSplitIssue)
	r.Get("/issues/{id}/r2", s.handleDebugIssueR2)
}

func (s *Server) registerMemoryRoutes(r chi.Router) {
	r.Get("/memories", s.handleMemoryList)
	r.Get("/memories/search", s.handleMemorySearch)
	r.Post("/memories", s.handleMemoryCreate)
	r.Get("/memories/proposals", s.handleMemoryProposalList)
	r.Post("/memories/proposals/synthesize", s.handleMemoryProposalSynthesize)
	r.Post("/memories/proposals/{id}/accept", s.handleMemoryProposalAccept)
	r.Post("/memories/proposals/{id}/reject", s.handleMemoryProposalReject)
	r.Get("/memories/{id}", s.handleGetMemory)
	r.Post("/memories/{id}/supersede", s.handleMemorySupersede)
	r.Post("/memories/{id}/archive", s.handleMemoryArchive)
}

func (s *Server) registerCurationRoutes(r chi.Router) {
	r.Get("/curation/candidates/duplicates", s.handleCurationCandidatesDuplicates)
	r.Get("/curation/candidates/stale", s.handleCurationCandidatesStale)
	r.Get("/curation/candidates/health", s.handleCurationCandidatesHealth)
	r.Get("/curation/candidates/memories", s.handleCurationCandidatesMemories)
	r.Get("/curation/proposals", s.handleCurationProposalList)
	r.Post("/curation/proposals", s.handleCurationProposalCreate)
	r.Get("/curation/proposals/{id}", s.handleGetCurationProposal)
	r.Post("/curation/proposals/{id}/accept", s.handleCurationProposalAccept)
	r.Post("/curation/proposals/{id}/reject", s.handleCurationProposalReject)
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
		Name:      "sortit-server",
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

const revisionStreamKeepalive = 25 * time.Second

func (s *Server) handleRevisionStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeInternalError(w, r, "streaming unsupported", nil)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// Defeat buffering by intermediaries (e.g. nginx) if they are ever
	// introduced; harmless when no proxy is present.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch, cancel := s.revisions.Subscribe()
	defer cancel()

	// Send the current revision immediately so clients get a deterministic
	// first value without waiting for the next Bump.
	if err := writeRevisionStreamEvent(w, s.revisions.Revision()); err != nil {
		return
	}
	flusher.Flush()

	keepalive := time.NewTicker(revisionStreamKeepalive)
	defer keepalive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case revision, ok := <-ch:
			if !ok {
				return
			}
			if err := writeRevisionStreamEvent(w, revision); err != nil {
				return
			}
			flusher.Flush()
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ":keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeRevisionStreamEvent(w http.ResponseWriter, revision uint64) error {
	// #nosec G705 -- revision is a server-generated uint64 counter, not user input.
	_, err := w.Write([]byte("data: " + strconv.FormatUint(revision, 10) + "\n\n"))
	return err
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
	eventBus := issueevents.NewEventBus()
	if listener := projectionInvalidationListener(
		mapProjectionInvalidatorFromIssueStore(baseStore),
	); listener != nil {
		eventBus.Subscribe(listener)
	}
	runner := &issuecmd.CommandRunner{
		DB:       uowBeginner,
		OnCommit: func() { revisions.Bump() },
	}

	tagStore := tagStoreFromIssueStore(store)
	commandAnalyzer := tags.FallbackAnalyzer(cfg.Analyzer)
	catalogLogger := logger.With("component", "catalog")
	catalog := tags.NewCatalogService(tagStore, commandAnalyzer, catalogLogger)
	enricherLogger := logger.With("component", "enricher")
	enricher := issueenrichment.NewIssueEnricher(commandAnalyzer, catalog, enricherLogger)
	enricher.UseMemoryContext(store)
	memoryService := memories.NewService(store, enricher, logger)
	memoryService.UseSynthesis(store, store)
	var enrichmentWorker *issueenrichment.IssueEnrichmentWorker
	workerLogger := logger.With("component", "enrichment_worker")
	if claimer := enrichmentJobClaimerFromStore(baseStore); claimer != nil && uowBeginner != nil {
		invalidator := mapProjectionInvalidatorFromIssueStore(baseStore)
		enrichmentWorker = &issueenrichment.IssueEnrichmentWorker{
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
	mapProjectionLoader := &mapview.MapProjectionLoader{
		Store:       store,
		Catalog:     catalog,
		Revisions:   revisions,
		Projections: mapProjectionStoreFromIssueStore(baseStore),
		Memories:    store,
	}
	cooccurrenceCache := &tagcooccurrence.Cache{
		Store:     store,
		Revisions: revisions,
	}
	centeringCache := &centering.Cache{
		Store:     store,
		Tags:      catalog,
		Revisions: revisions,
	}
	ridgeLambdaCache := &ridgelambda.Cache{
		Store:     store,
		Tags:      catalog,
		Revisions: revisions,
		Centering: centeringCache,
	}
	var customRegionStore regions.CustomRegionStore = store
	var customRegionWriter regions.CustomRegionWriter = store
	regionsLoader := &regions.Loader{
		Store:         store,
		Tags:          catalog,
		MapProjection: mapProjectionLoader,
		CustomStore:   customRegionStore,
		Revisions:     revisions,
	}
	regionsHandler := &regions.Handler{Loader: regionsLoader}
	customRegionHandler := &regions.CustomHandler{Store: customRegionWriter}

	// Hoisted so the curation dispatcher and the Server fields share one
	// instance of each handler.
	closeIssueHandler := issuecmd.CloseIssueHandler{Runner: runner, Events: eventBus}
	reEnrichIssueHandler := issuecmd.ReEnrichIssueHandler{Runner: runner, Store: store, Events: eventBus}
	combineIssuesHandler := issuecmd.CombineIssuesHandler{
		Runner:   runner,
		Store:    store,
		Enricher: enricher,
		Events:   eventBus,
	}
	curationService := curation.NewService(store, curation.HandlerDispatcher{
		CombineHandler:  combineIssuesHandler,
		CloseHandler:    closeIssueHandler,
		ReenrichHandler: reEnrichIssueHandler,
		Memories:        memoryService,
	}, logger)
	exploreHandler := mapview.ExploreIssueHandler{
		Reader:       store,
		DetailReader: store,
		SearchStore:  semanticSearchStoreFromStore(baseStore),
		Catalog:      catalog,
		RidgeLambda:  ridgeLambdaCache,
	}
	curationDetector := curation.NewDetector(store, store, exploreHandler,
		diagnostics.DebugFactorWeightsHandler{Store: store, Catalog: catalog},
		diagnostics.DebugTagHealthHandler{Store: store, Catalog: catalog, Centering: centeringCache}, logger)
	curationMemoryDetector := curation.NewMemoryDetector(store, store, logger)

	return &Server{
		config:              cfg,
		logger:              logger,
		startedAt:           time.Now().UTC(),
		revisions:           revisions,
		mapProjectionLoader: mapProjectionLoader,
		enrichmentWorker:    enrichmentWorker,
		createIssue: issuecmd.CreateIssueHandler{
			Logger:   logger.With("command", "create_issue"),
			Runner:   runner,
			Enricher: enricher,
			Events:   eventBus,
		},
		refineIssue: issuecmd.RefineIssueHandler{
			Runner:   runner,
			Store:    store,
			Enricher: enricher,
			Events:   eventBus,
		},
		progressIssue: issuecmd.ProgressIssueHandler{Runner: runner, Events: eventBus},
		closeIssue:    closeIssueHandler,
		reopenIssue: issuecmd.ReopenIssueHandler{
			Runner: runner,
			Events: eventBus,
		},
		assignIssue: issuecmd.AssignIssueHandler{
			Runner: runner,
			Events: eventBus,
		},
		reEnrichIssue: reEnrichIssueHandler,
		splitIssue: issuecmd.SplitIssueHandler{
			Runner:   runner,
			Enricher: enricher,
			Events:   eventBus,
		},
		combineIssues: combineIssuesHandler,
		linkIssues: issuecmd.LinkIssuesHandler{
			Runner: runner,
			Events: eventBus,
		},
		listIssues:    issueviews.ListIssuesHandler{Store: store},
		listActivity:  issueviews.ListActivityHandler{Events: events},
		getIssue:      issueviews.GetIssueHandler{Store: store, Logger: logger.With("query", "get_issue")},
		compareIssues: issueviews.CompareIssuesHandler{Reader: store},
		searchIssues: search.SearchIssuesHandler{
			Analyzer:     commandAnalyzer,
			Catalog:      catalog,
			Store:        baseStore,
			Cooccurrence: cooccurrenceCache,
			Centering:    centeringCache,
			RidgeLambda:  ridgeLambdaCache,
		},
		searchUnified: search.SearchUnifiedHandler{
			Analyzer:    commandAnalyzer,
			Catalog:     catalog,
			Store:       baseStore,
			Centering:   centeringCache,
			RidgeLambda: ridgeLambdaCache,
		},
		exploreIssue:         exploreHandler,
		listTags:             issueviews.ListTagsHandler{Catalog: catalog},
		getMap:               mapview.MapHandler{IssueStore: store, Catalog: catalog, Projection: mapProjectionLoader},
		getMapEdges:          mapview.EdgeHandler{IssueStore: store, Catalog: catalog, Projection: mapProjectionLoader},
		debugAnalyzeIssue:    diagnostics.DebugAnalyzeIssueHandler{Analyzer: cfg.Analyzer, Catalog: catalog, Enricher: enricher, Store: store},
		debugEvalTags:        diagnostics.DebugEvalTagsHandler{Analyzer: cfg.Analyzer, Catalog: catalog, Enricher: enricher},
		debugFactorWeights:   diagnostics.DebugFactorWeightsHandler{Store: store, Catalog: catalog},
		debugTagHealth:       diagnostics.DebugTagHealthHandler{Store: store, Catalog: catalog, Centering: centeringCache},
		debugIssueR2:         diagnostics.DebugIssueR2Handler{Store: store, Catalog: catalog},
		debugTagCooccurrence: diagnostics.DebugTagCooccurrenceHandler{Store: store, Cache: cooccurrenceCache},
		debugRidgeScore:      diagnostics.DebugRidgeScoreHandler{Store: store, Catalog: catalog, Centering: centeringCache},
		debugEmbeddingFalls:  diagnostics.DebugEmbeddingFallbacksHandler{},
		getPersonProfile:     people.GetPersonProfileHandler{Store: store, Catalog: catalog},
		getPersonDetail:      people.GetPersonDetailHandler{Store: store, Catalog: catalog, RidgeLambda: ridgeLambdaCache},
		workCorrelations:     people.WorkCorrelationsHandler{Store: store, Catalog: catalog},
		regions:              regionsHandler,
		customRegions:        customRegionHandler,
		cooccurrenceCache:    cooccurrenceCache,
		issueXRay: &issuexray.Handler{
			Issues:       store,
			Tags:         catalog,
			Cooccurrence: cooccurrenceCache,
		},
		authService:            cfg.Auth,
		catalog:                catalog,
		memories:               memoryService,
		curation:               curationService,
		curationDetector:       curationDetector,
		curationMemoryDetector: curationMemoryDetector,
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
