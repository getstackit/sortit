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
	"splat/internal/auth"
	"splat/internal/commands"
	"splat/internal/issues"
	mcpserver "splat/internal/mcp"
	"splat/internal/queries"
	"splat/internal/services"
)

type ServerConfig struct {
	Port        int
	CORSOrigins []string
	APIPrefixes []string
	Analyzer    *ai.Analyzer
	IssueStore  issues.Store
	Auth        *auth.Service
}

type Server struct {
	config              ServerConfig
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
	apiMux := http.NewServeMux()
	apiRoutes := make(map[string]struct{})
	publicAPIRoutes := make(map[string]struct{})

	for _, prefix := range normalizeAPIPrefixes(s.config.APIPrefixes) {
		s.registerDedicatedAPIRoutes(apiMux, apiRoutes, publicAPIRoutes, prefix)
	}
	s.registerUIRoutes(apiMux, apiRoutes, publicAPIRoutes, "/api/ui")

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

	handler := authMiddleware(s.authService, publicAPIRoutes, root)
	handler = corsMiddleware(s.config.CORSOrigins, apiRoutes, handler)
	return loggingMiddleware(handler)
}

func (s *Server) registerDedicatedAPIRoutes(
	apiMux *http.ServeMux,
	apiRoutes map[string]struct{},
	publicAPIRoutes map[string]struct{},
	prefix string,
) {
	healthRoute := path.Join(prefix, "health")
	authGitHubStartRoute := path.Join(prefix, "auth", "github", "start")
	authGitHubCallbackRoute := path.Join(prefix, "auth", "github", "callback")
	authSessionRoute := path.Join(prefix, "auth", "session")
	authLogoutRoute := path.Join(prefix, "auth", "logout")
	authTokensRoute := path.Join(prefix, "auth", "tokens")
	authTokenItemSubtreeRoute := authTokenItemRoute(prefix)
	authCLILoginRoute := path.Join(prefix, "auth", "cli", "login")
	authCLILoginExchangeSubtreeRoute := authCLILoginExchangeRoute(prefix)
	issuesRoute := path.Join(prefix, "issues")
	issuesSearchRoute := path.Join(prefix, "issues", "search")
	issuesCombineRoute := path.Join(prefix, "issues", "combine")
	issuesLinkRoute := path.Join(prefix, "issues", "link")
	issuesRefineRoute := path.Join(prefix, "issues", "refine")
	issuesProgressRoute := path.Join(prefix, "issues", "progress")
	issuesCloseRoute := path.Join(prefix, "issues", "close")
	issuesAssignRoute := path.Join(prefix, "issues", "assign")
	issueItemSubtreeRoute := issueItemRoute(prefix)
	tagsRoute := path.Join(prefix, "tags")
	peopleSubtreeRoute := path.Join(prefix, "people") + "/"
	peopleCorrelationsRoute := path.Join(prefix, "people", "correlations")

	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, healthRoute, s.handleHealth)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authGitHubStartRoute, s.handleAuthGitHubStart)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authGitHubCallbackRoute, s.handleAuthGitHubCallback)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authSessionRoute, s.handleAuthSession)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authLogoutRoute, s.handleAuthLogout)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authCLILoginRoute, s.handleAuthCLILogin)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authCLILoginExchangeSubtreeRoute, s.handleAuthCLILoginExchange(authCLILoginExchangeSubtreeRoute))

	registerProtectedRoute(apiMux, apiRoutes, authTokensRoute, s.handleAuthTokens)
	registerProtectedRoute(apiMux, apiRoutes, authTokenItemSubtreeRoute, s.handleAuthTokenByID(authTokenItemSubtreeRoute))
	registerProtectedRoute(apiMux, apiRoutes, issuesRoute, s.handleIssues)
	registerProtectedRoute(apiMux, apiRoutes, issuesSearchRoute, s.handleIssueSearch)
	registerProtectedRoute(apiMux, apiRoutes, issuesCombineRoute, s.handleIssueCombine)
	registerProtectedRoute(apiMux, apiRoutes, issuesLinkRoute, s.handleIssueLink)
	registerProtectedRoute(apiMux, apiRoutes, issuesRefineRoute, s.handleIssueRefineBatch)
	registerProtectedRoute(apiMux, apiRoutes, issuesProgressRoute, s.handleIssueProgressBatch)
	registerProtectedRoute(apiMux, apiRoutes, issuesCloseRoute, s.handleIssueCloseBatch)
	registerProtectedRoute(apiMux, apiRoutes, issuesAssignRoute, s.handleIssueAssignBatch)
	registerProtectedRoute(apiMux, apiRoutes, issueItemSubtreeRoute, s.handleIssueByID(issueItemSubtreeRoute))
	registerProtectedRoute(apiMux, apiRoutes, tagsRoute, s.handleTags)
	registerProtectedRoute(apiMux, apiRoutes, peopleCorrelationsRoute, s.handleWorkCorrelations)
	registerProtectedRoute(apiMux, apiRoutes, peopleSubtreeRoute, s.handlePersonProfile(peopleSubtreeRoute))
}

func (s *Server) registerUIRoutes(
	apiMux *http.ServeMux,
	apiRoutes map[string]struct{},
	publicAPIRoutes map[string]struct{},
	prefix string,
) {
	healthRoute := path.Join(prefix, "health")
	authGitHubStartRoute := path.Join(prefix, "auth", "github", "start")
	authGitHubCallbackRoute := path.Join(prefix, "auth", "github", "callback")
	authSessionRoute := path.Join(prefix, "auth", "session")
	authLogoutRoute := path.Join(prefix, "auth", "logout")
	authTokensRoute := path.Join(prefix, "auth", "tokens")
	authTokenItemSubtreeRoute := authTokenItemRoute(prefix)
	authCLILoginCompleteSubtreeRoute := authCLILoginCompleteRoute(prefix)
	issuesRoute := path.Join(prefix, "issues")
	activityRoute := path.Join(prefix, "activity")
	issuesCompareRoute := path.Join(prefix, "issues", "compare")
	issuesSearchRoute := path.Join(prefix, "issues", "search")
	searchRoute := path.Join(prefix, "search")
	issueItemSubtreeRoute := issueItemRoute(prefix)
	tagsRoute := path.Join(prefix, "tags")
	revisionRoute := path.Join(prefix, "revision")
	mapRoute := path.Join(prefix, "map")
	mapEdgesRoute := path.Join(prefix, "map", "edges")
	debugAnalyzeRoute := path.Join(prefix, "debug", "issues", "analyze")
	debugInvalidateMapProjectionRoute := path.Join(prefix, "debug", "map-projection", "invalidate")
	peopleSubtreeRoute := path.Join(prefix, "people") + "/"
	peopleCorrelationsRoute := path.Join(prefix, "people", "correlations")

	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, healthRoute, s.handleHealth)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authGitHubStartRoute, s.handleAuthGitHubStart)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authGitHubCallbackRoute, s.handleAuthGitHubCallback)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authSessionRoute, s.handleAuthSession)
	registerPublicRoute(apiMux, apiRoutes, publicAPIRoutes, authLogoutRoute, s.handleAuthLogout)

	registerProtectedRoute(apiMux, apiRoutes, authTokensRoute, s.handleAuthTokens)
	registerProtectedRoute(apiMux, apiRoutes, authTokenItemSubtreeRoute, s.handleAuthTokenByID(authTokenItemSubtreeRoute))
	registerProtectedRoute(apiMux, apiRoutes, authCLILoginCompleteSubtreeRoute, s.handleAuthCLILoginComplete(authCLILoginCompleteSubtreeRoute))
	registerProtectedRoute(apiMux, apiRoutes, issuesRoute, s.handleIssues)
	registerProtectedRoute(apiMux, apiRoutes, activityRoute, s.handleActivity)
	registerProtectedRoute(apiMux, apiRoutes, issuesCompareRoute, s.handleIssueCompare)
	registerProtectedRoute(apiMux, apiRoutes, issuesSearchRoute, s.handleIssueSearch)
	registerProtectedRoute(apiMux, apiRoutes, searchRoute, s.handleUnifiedSearch)
	registerProtectedRoute(apiMux, apiRoutes, issueItemSubtreeRoute, s.handleIssueByID(issueItemSubtreeRoute))
	registerProtectedRoute(apiMux, apiRoutes, tagsRoute, s.handleTags)
	registerProtectedRoute(apiMux, apiRoutes, revisionRoute, s.handleRevision)
	registerProtectedRoute(apiMux, apiRoutes, mapRoute, s.handleMap)
	registerProtectedRoute(apiMux, apiRoutes, mapEdgesRoute, s.handleMapEdges)
	registerProtectedRoute(apiMux, apiRoutes, peopleCorrelationsRoute, s.handleWorkCorrelations)
	registerProtectedRoute(apiMux, apiRoutes, peopleSubtreeRoute, s.handlePersonProfile(peopleSubtreeRoute))
	registerProtectedRoute(apiMux, apiRoutes, debugAnalyzeRoute, s.handleDebugIssueAnalyze)
	registerProtectedRoute(apiMux, apiRoutes, debugInvalidateMapProjectionRoute, s.handleDebugInvalidateMapProjection)
}

func registerProtectedRoute(
	apiMux *http.ServeMux,
	apiRoutes map[string]struct{},
	route string,
	handler http.HandlerFunc,
) {
	apiRoutes[route] = struct{}{}
	apiMux.HandleFunc(route, handler)
}

func registerPublicRoute(
	apiMux *http.ServeMux,
	apiRoutes map[string]struct{},
	publicAPIRoutes map[string]struct{},
	route string,
	handler http.HandlerFunc,
) {
	registerProtectedRoute(apiMux, apiRoutes, route, handler)
	publicAPIRoutes[route] = struct{}{}
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
				log.Printf("issue enrichment worker stopped: %v", err)
			}
		}()
	}

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.config.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("splat server listening on http://localhost:%d", s.config.Port)
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

func (s *Server) handleRevision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

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
	if err != nil {
		log.Printf("500 %s %s: %s: %v", r.Method, r.URL.Path, message, err) //nolint:gosec
	} else {
		log.Printf("500 %s %s: %s", r.Method, r.URL.Path, message) //nolint:gosec
	}
	writeError(w, http.StatusInternalServerError, message)
}

func NewServer(cfg ServerConfig) *Server {
	baseStore := cfg.IssueStore
	if baseStore == nil {
		baseStore = issues.NewInMemoryStore(nil)
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
	catalog := services.NewCatalogService(tagStore, commandAnalyzer)
	enricher := services.NewIssueEnricher(commandAnalyzer, catalog)
	var enrichmentWorker *services.IssueEnrichmentWorker
	if claimer := enrichmentJobClaimerFromStore(baseStore); claimer != nil && uowBeginner != nil {
		invalidator := mapProjectionInvalidatorFromIssueStore(baseStore)
		enrichmentWorker = &services.IssueEnrichmentWorker{
			Store:    baseStore,
			DB:       uowBeginner,
			Jobs:     claimer,
			Enricher: enricher,
			OnStateChange: func(ctx context.Context, applied bool) {
				revisions.Bump()
				if applied && invalidator != nil {
					if err := invalidator.InvalidateMapProjections(ctx); err != nil {
						log.Printf("failed to invalidate map projections after enrichment: %v", err)
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
		startedAt:           time.Now().UTC(),
		revisions:           revisions,
		mapProjectionLoader: mapProjectionLoader,
		enrichmentWorker:    enrichmentWorker,
		createIssue: commands.CreateIssueHandler{
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
		getIssue:      queries.GetIssueHandler{Store: store},
		compareIssues: queries.CompareIssuesHandler{Store: store},
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
		exploreIssue:      queries.ExploreIssueHandler{Store: baseStore, Catalog: catalog},
		listTags:          queries.ListTagsHandler{Catalog: catalog},
		getMap:            queries.MapHandler{IssueStore: store, Catalog: catalog, Projection: mapProjectionLoader},
		getMapEdges:       queries.EdgeHandler{IssueStore: store, Catalog: catalog, Projection: mapProjectionLoader},
		debugAnalyzeIssue: queries.DebugAnalyzeIssueHandler{Analyzer: cfg.Analyzer, Catalog: catalog, Store: store},
		getPersonProfile:  queries.GetPersonProfileHandler{Store: store},
		getPersonDetail:   queries.GetPersonDetailHandler{Store: store},
		workCorrelations:  queries.WorkCorrelationsHandler{Store: store},
		authService:       cfg.Auth,
		catalog:           catalog,
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
			log.Printf(
				"failed to invalidate map projections after %s event for issue %s: %v",
				event.Kind,
				event.IssueID,
				err,
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
