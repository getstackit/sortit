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
		healthRoute := path.Join(prefix, "health")
		authGitHubStartRoute := path.Join(prefix, "auth", "github", "start")
		authGitHubCallbackRoute := path.Join(prefix, "auth", "github", "callback")
		authSessionRoute := path.Join(prefix, "auth", "session")
		authLogoutRoute := path.Join(prefix, "auth", "logout")
		authTokensRoute := path.Join(prefix, "auth", "tokens")
		authTokenItemSubtreeRoute := authTokenItemRoute(prefix)
		issuesRoute := path.Join(prefix, "issues")
		activityRoute := path.Join(prefix, "activity")
		issuesCompareRoute := path.Join(prefix, "issues", "compare")
		issuesCombineRoute := path.Join(prefix, "issues", "combine")
		issuesLinkRoute := path.Join(prefix, "issues", "link")
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
		apiRoutes[healthRoute] = struct{}{}
		apiRoutes[issuesRoute] = struct{}{}
		apiRoutes[activityRoute] = struct{}{}
		apiRoutes[issuesCompareRoute] = struct{}{}
		apiRoutes[issuesCombineRoute] = struct{}{}
		apiRoutes[issuesLinkRoute] = struct{}{}
		apiRoutes[issuesSearchRoute] = struct{}{}
		apiRoutes[searchRoute] = struct{}{}
		apiRoutes[issueItemSubtreeRoute] = struct{}{}
		apiRoutes[tagsRoute] = struct{}{}
		apiRoutes[revisionRoute] = struct{}{}
		apiRoutes[mapRoute] = struct{}{}
		apiRoutes[mapEdgesRoute] = struct{}{}
		apiRoutes[peopleSubtreeRoute] = struct{}{}
		apiRoutes[peopleCorrelationsRoute] = struct{}{}
		apiRoutes[debugAnalyzeRoute] = struct{}{}
		apiRoutes[debugInvalidateMapProjectionRoute] = struct{}{}
		apiRoutes[authGitHubStartRoute] = struct{}{}
		apiRoutes[authGitHubCallbackRoute] = struct{}{}
		apiRoutes[authSessionRoute] = struct{}{}
		apiRoutes[authLogoutRoute] = struct{}{}
		apiRoutes[authTokensRoute] = struct{}{}
		apiRoutes[authTokenItemSubtreeRoute] = struct{}{}
		publicAPIRoutes[healthRoute] = struct{}{}
		publicAPIRoutes[authGitHubStartRoute] = struct{}{}
		publicAPIRoutes[authGitHubCallbackRoute] = struct{}{}
		publicAPIRoutes[authSessionRoute] = struct{}{}
		publicAPIRoutes[authLogoutRoute] = struct{}{}

		apiMux.HandleFunc(healthRoute, s.handleHealth)
		apiMux.HandleFunc(authGitHubStartRoute, s.handleAuthGitHubStart)
		apiMux.HandleFunc(authGitHubCallbackRoute, s.handleAuthGitHubCallback)
		apiMux.HandleFunc(authSessionRoute, s.handleAuthSession)
		apiMux.HandleFunc(authLogoutRoute, s.handleAuthLogout)
		apiMux.HandleFunc(authTokensRoute, s.handleAuthTokens)
		apiMux.HandleFunc(authTokenItemSubtreeRoute, s.handleAuthTokenByID(authTokenItemSubtreeRoute))
		apiMux.HandleFunc(issuesRoute, s.handleIssues)
		apiMux.HandleFunc(activityRoute, s.handleActivity)
		apiMux.HandleFunc(issuesCompareRoute, s.handleIssueCompare)
		apiMux.HandleFunc(issuesCombineRoute, s.handleIssueCombine)
		apiMux.HandleFunc(issuesLinkRoute, s.handleIssueLink)
		apiMux.HandleFunc(issuesSearchRoute, s.handleIssueSearch)
		apiMux.HandleFunc(searchRoute, s.handleUnifiedSearch)
		apiMux.HandleFunc(issueItemSubtreeRoute, s.handleIssueByID(issueItemSubtreeRoute))
		apiMux.HandleFunc(tagsRoute, s.handleTags)
		apiMux.HandleFunc(revisionRoute, s.handleRevision)
		apiMux.HandleFunc(mapRoute, s.handleMap)
		apiMux.HandleFunc(mapEdgesRoute, s.handleMapEdges)
		apiMux.HandleFunc(peopleCorrelationsRoute, s.handleWorkCorrelations)
		apiMux.HandleFunc(peopleSubtreeRoute, s.handlePersonProfile(peopleSubtreeRoute))
		apiMux.HandleFunc(debugAnalyzeRoute, s.handleDebugIssueAnalyze)
		apiMux.HandleFunc(debugInvalidateMapProjectionRoute, s.handleDebugInvalidateMapProjection)
	}

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
		log.Printf("500 %s %s: %s: %v", r.Method, r.URL.Path, message, err)
	} else {
		log.Printf("500 %s %s: %s", r.Method, r.URL.Path, message)
	}
	writeError(w, http.StatusInternalServerError, message)
}

func NewServer(cfg ServerConfig) *Server {
	store := cfg.IssueStore
	if store == nil {
		store = issues.NewInMemoryStore(nil)
	}
	revisions := issues.NewRevisionTracker()
	observed := issues.NewObservedStore(store, revisions)
	store = observed

	// EventStore: available on ObservedStore which wraps both Store and EventStore
	var events issues.EventStore = observed

	// CommandRunner manages DB transaction lifecycle for command handlers.
	uowBeginner := unitOfWorkBeginnerFromStore(cfg.IssueStore)
	eventBus := issues.NewEventBus()
	if listener := projectionInvalidationListener(
		mapProjectionInvalidatorFromIssueStore(cfg.IssueStore),
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
	mapProjectionLoader := &queries.MapProjectionLoader{
		Store:       store,
		Catalog:     catalog,
		Revisions:   revisions,
		Projections: mapProjectionStoreFromIssueStore(cfg.IssueStore),
	}

	return &Server{
		config:              cfg,
		startedAt:           time.Now().UTC(),
		revisions:           revisions,
		mapProjectionLoader: mapProjectionLoader,
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
			Store:    cfg.IssueStore,
		},
		searchUnified: queries.SearchUnifiedHandler{
			Analyzer: commandAnalyzer,
			Catalog:  catalog,
			Store:    cfg.IssueStore,
		},
		exploreIssue:      queries.ExploreIssueHandler{Store: cfg.IssueStore, Catalog: catalog},
		listTags:          queries.ListTagsHandler{Catalog: catalog},
		getMap:            queries.MapHandler{IssueStore: store, Catalog: catalog, Projection: mapProjectionLoader},
		getMapEdges:       queries.EdgeHandler{IssueStore: store, Catalog: catalog, Projection: mapProjectionLoader},
		debugAnalyzeIssue: queries.DebugAnalyzeIssueHandler{Analyzer: cfg.Analyzer, Catalog: catalog, Store: store},
		getPersonProfile:  queries.GetPersonProfileHandler{Store: store},
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
