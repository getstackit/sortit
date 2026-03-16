package api

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"splat/internal/auth"
	"splat/internal/commands"
	"splat/internal/issues"
	"splat/internal/queries"
)

type issuesResponse struct {
	Issues []issues.Issue `json:"issues"`
}

type createIssueRequest struct {
	Raw       string   `json:"raw"`
	Tags      []string `json:"tags,omitempty"`
	CreatedBy string   `json:"createdBy,omitempty"`
}

type closeIssueRequest struct {
	ClosedBy   string `json:"closedBy,omitempty"`
	Reason     string `json:"reason,omitempty"`
	ReasonNote string `json:"reasonNote,omitempty"`
}

type refineIssueRequest struct {
	Raw       string `json:"raw"`
	CreatedBy string `json:"createdBy,omitempty"`
}

type progressIssueRequest struct {
	Raw       string `json:"raw"`
	CreatedBy string `json:"createdBy,omitempty"`
}

type assignIssueRequest struct {
	AssignedTo string `json:"assignedTo"`
	CreatedBy  string `json:"createdBy,omitempty"`
}

type batchCloseIssuesRequest struct {
	IDs        []string `json:"ids"`
	ClosedBy   string   `json:"closedBy,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	ReasonNote string   `json:"reasonNote,omitempty"`
}

type batchRefineIssuesRequest struct {
	IDs       []string `json:"ids"`
	Raw       string   `json:"raw"`
	CreatedBy string   `json:"createdBy,omitempty"`
}

type batchProgressIssuesRequest struct {
	IDs       []string `json:"ids"`
	Raw       string   `json:"raw"`
	CreatedBy string   `json:"createdBy,omitempty"`
}

type batchAssignIssuesRequest struct {
	IDs        []string `json:"ids"`
	AssignedTo string   `json:"assignedTo"`
	CreatedBy  string   `json:"createdBy,omitempty"`
}

type splitIssueChildRequest struct {
	Raw  string   `json:"raw"`
	Tags []string `json:"tags,omitempty"`
}

type splitIssueRequest struct {
	Children    []splitIssueChildRequest `json:"children"`
	CreatedBy   string                   `json:"createdBy,omitempty"`
	Note        string                   `json:"note,omitempty"`
	CloseSource bool                     `json:"closeSource"`
}

type combineIssuesRequest struct {
	IDs       []string `json:"ids"`
	CreatedBy string   `json:"createdBy,omitempty"`
	Note      string   `json:"note,omitempty"`
}

type linkIssuesRequest struct {
	SourceID  string `json:"sourceId"`
	TargetID  string `json:"targetId"`
	Type      string `json:"type"`
	CreatedBy string `json:"createdBy,omitempty"`
	Note      string `json:"note,omitempty"`
}

type compareIssuesRequest struct {
	IDs []string `json:"ids"`
}

type compareIssuesResponse struct {
	ComparedIssueCount          int                       `json:"comparedIssueCount"`
	AverageEmbeddingSimilarity  float64                   `json:"averageEmbeddingSimilarity"`
	PairwiseEmbeddingSimilarity []pairwiseIssueSimilarity `json:"pairwiseEmbeddingSimilarity"`
}

type pairwiseIssueSimilarity struct {
	SourceID   string  `json:"sourceId"`
	TargetID   string  `json:"targetId"`
	Similarity float64 `json:"similarity"`
}

func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleIssueList(w, r)
	case http.MethodPost:
		s.handleIssueCreate(w, r)
	default:
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleIssueCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeCompareIssuesRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.compareIssues.Handle(r.Context(), queries.CompareIssues{IDs: request.IDs})
	if err != nil {
		if queries.NotFound(err) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeInternalError(w, r, "failed to compare issues", err)
		return
	}

	writeJSON(w, http.StatusOK, compareIssuesResponse{
		ComparedIssueCount:         result.ComparedIssueCount,
		AverageEmbeddingSimilarity: result.AverageEmbeddingSimilarity,
		PairwiseEmbeddingSimilarity: toPairwiseIssueSimilarityResponse(
			result.PairwiseEmbeddingSimilarity,
		),
	})
}

func (s *Server) handleIssueSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		query = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	if query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	filter, err := ParseIssueStatusFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid status query")
		return
	}

	limit, err := ParsePositiveIntQuery(r.URL.Query(), "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit query")
		return
	}

	searchLimit := 0
	if limit != nil {
		searchLimit = *limit
	}
	offset, err := ParseNonNegativeIntQuery(r.URL.Query(), "offset")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid offset query")
		return
	}

	searchOffset := 0
	if offset != nil {
		searchOffset = *offset
	}

	sortBy, err := parseIssueSearchSortBy(firstQueryValue(r, "sortBy", "sort_by"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.searchIssues.Handle(r.Context(), queries.SearchIssues{
		Query:      query,
		Limit:      searchLimit,
		Offset:     searchOffset,
		Status:     filter,
		AssignedTo: firstQueryValue(r, "assignedTo", "assigned_to"),
		Tags:       ParseCSV(r.URL.Query().Get("tags")),
		SortBy:     sortBy,
	})
	if err != nil {
		writeInternalError(w, r, "failed to search issues", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleIssueCombine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeCombineIssuesRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.combineIssues.Handle(r.Context(), commands.CombineIssues{
		SourceIDs: request.IDs,
		CreatedBy: actorForRequest(r, request.CreatedBy),
		Note:      request.Note,
	})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeInternalError(w, r, "failed to combine issues", err)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleIssueLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeLinkIssuesRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.linkIssues.Handle(r.Context(), commands.LinkIssues{
		SourceID:  request.SourceID,
		TargetID:  request.TargetID,
		Type:      issues.IssueLinkType(request.Type),
		CreatedBy: actorForRequest(r, request.CreatedBy),
		Note:      request.Note,
	})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		if errors.Is(err, issues.ErrInvalidIssueLink) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, issues.ErrDuplicateIssueLink) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeInternalError(w, r, "failed to link issues", err)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleIssueRefineBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeBatchRefineIssuesRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := commands.RunIssueMutationBatch(request.IDs, func(id string) (issues.Issue, error) {
		return s.refineIssue.Handle(r.Context(), commands.RefineIssue{
			ID:        id,
			Raw:       request.Raw,
			CreatedBy: actorForRequest(r, request.CreatedBy),
		})
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleIssueProgressBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeBatchProgressIssuesRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := commands.RunIssueMutationBatch(request.IDs, func(id string) (issues.Issue, error) {
		return s.progressIssue.Handle(r.Context(), commands.ProgressIssue{
			ID:        id,
			Raw:       request.Raw,
			CreatedBy: actorForRequest(r, request.CreatedBy),
		})
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleIssueCloseBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeBatchCloseIssuesRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := commands.RunIssueMutationBatch(request.IDs, func(id string) (issues.Issue, error) {
		return s.closeIssue.Handle(r.Context(), commands.CloseIssue{
			ID:         id,
			ClosedBy:   actorForRequest(r, request.ClosedBy),
			Reason:     request.Reason,
			ReasonNote: request.ReasonNote,
		})
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleIssueAssignBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeBatchAssignIssuesRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := commands.RunIssueMutationBatch(request.IDs, func(id string) (issues.Issue, error) {
		return s.assignIssue.Handle(r.Context(), commands.AssignIssue{
			ID:         id,
			AssignedTo: request.AssignedTo,
			CreatedBy:  actorForRequest(r, request.CreatedBy),
		})
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleIssueByID(route string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tail := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, route))
		if tail == "" {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}

		segments := strings.Split(tail, "/")
		if len(segments) == 0 || len(segments) > 2 {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}

		id := strings.TrimSpace(segments[0])
		if id == "" {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}

		if len(segments) == 1 {
			s.handleGetIssue(w, r, id)
			return
		}

		switch segments[1] {
		case "close":
			s.handleCloseIssue(w, r, id)
		case "refine":
			s.handleRefineIssue(w, r, id)
		case "explore":
			s.handleExploreIssue(w, r, id)
		case "progress":
			s.handleProgressIssue(w, r, id)
		case "reopen":
			s.handleReopenIssue(w, r, id)
		case "assign":
			s.handleAssignIssue(w, r, id)
		case "split":
			s.handleSplitIssue(w, r, id)
		default:
			writeError(w, http.StatusNotFound, "route not found")
		}
	}
}

func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	issue, err := s.getIssue.Handle(r.Context(), queries.GetIssue{ID: id})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeInternalError(w, r, "failed to load issue", err)
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

func (s *Server) handleCloseIssue(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeCloseIssueRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	closed, err := s.closeIssue.Handle(r.Context(), commands.CloseIssue{
		ID:         id,
		ClosedBy:   actorForRequest(r, request.ClosedBy),
		Reason:     request.Reason,
		ReasonNote: request.ReasonNote,
	})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeInternalError(w, r, "failed to close issue", err)
		return
	}

	writeJSON(w, http.StatusOK, closed)
}

func (s *Server) handleRefineIssue(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeRefineIssueRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	refined, err := s.refineIssue.Handle(r.Context(), commands.RefineIssue{
		ID:        id,
		Raw:       request.Raw,
		CreatedBy: actorForRequest(r, request.CreatedBy),
	})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		if errors.Is(err, issues.ErrIssueClosed) {
			writeError(w, http.StatusConflict, "issue is closed")
			return
		}
		writeInternalError(w, r, "failed to refine issue", err)
		return
	}

	writeJSON(w, http.StatusOK, refined)
}

func (s *Server) handleExploreIssue(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	limit, err := ParsePositiveIntQuery(r.URL.Query(), "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit query")
		return
	}

	exploreLimit := 0
	if limit != nil {
		exploreLimit = *limit
	}

	result, err := s.exploreIssue.Handle(r.Context(), queries.ExploreIssue{
		ID:    id,
		Limit: exploreLimit,
	})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeInternalError(w, r, "failed to explore issue", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleProgressIssue(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeProgressIssueRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	progressed, err := s.progressIssue.Handle(r.Context(), commands.ProgressIssue{
		ID:        id,
		Raw:       request.Raw,
		CreatedBy: actorForRequest(r, request.CreatedBy),
	})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		if errors.Is(err, issues.ErrIssueClosed) {
			writeError(w, http.StatusConflict, "issue is closed")
			return
		}
		writeInternalError(w, r, "failed to post progress", err)
		return
	}

	writeJSON(w, http.StatusOK, progressed)
}

func (s *Server) handleReopenIssue(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	reopened, err := s.reopenIssue.Handle(r.Context(), commands.ReopenIssue{ID: id})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeInternalError(w, r, "failed to reopen issue", err)
		return
	}

	writeJSON(w, http.StatusOK, reopened)
}

func (s *Server) handleAssignIssue(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeAssignIssueRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	assigned, err := s.assignIssue.Handle(r.Context(), commands.AssignIssue{
		ID:         id,
		AssignedTo: request.AssignedTo,
		CreatedBy:  actorForRequest(r, request.CreatedBy),
	})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeInternalError(w, r, "failed to assign issue", err)
		return
	}

	writeJSON(w, http.StatusOK, assigned)
}

func (s *Server) handleSplitIssue(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeSplitIssueRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	children := make([]commands.SplitIssueChild, 0, len(request.Children))
	for _, child := range request.Children {
		children = append(children, commands.SplitIssueChild{
			Raw:  child.Raw,
			Tags: child.Tags,
		})
	}

	result, err := s.splitIssue.Handle(r.Context(), commands.SplitIssue{
		SourceID:    id,
		Children:    children,
		CreatedBy:   actorForRequest(r, request.CreatedBy),
		Note:        request.Note,
		CloseSource: request.CloseSource,
	})
	if err != nil {
		if errors.Is(err, issues.ErrNotFound) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeInternalError(w, r, "failed to split issue", err)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleIssueList(w http.ResponseWriter, r *http.Request) {
	filter, err := ParseIssueStatusFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid status query")
		return
	}

	limit, err := ParsePositiveIntQuery(r.URL.Query(), "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit query")
		return
	}
	offset, err := ParseNonNegativeIntQuery(r.URL.Query(), "offset")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid offset query")
		return
	}

	listLimit := 0
	if limit != nil {
		listLimit = *limit
	}
	listOffset := 0
	if offset != nil {
		listOffset = *offset
	}

	items, err := s.listIssues.Handle(r.Context(), queries.ListIssuesQuery{
		Status:     filter,
		AssignedTo: firstQueryValue(r, "assignedTo", "assigned_to"),
		Tags:       ParseCSV(r.URL.Query().Get("tags")),
		Limit:      listLimit,
		Offset:     listOffset,
	})
	if err != nil {
		writeInternalError(w, r, "failed to list issues", err)
		return
	}

	writeJSON(w, http.StatusOK, issuesResponse{Issues: items})
}

func (s *Server) handleIssueCreate(w http.ResponseWriter, r *http.Request) {
	request, err := decodeCreateIssueRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := s.createIssue.Handle(r.Context(), commands.CreateIssue{
		Raw:       request.Raw,
		Tags:      request.Tags,
		CreatedBy: actorForRequest(r, request.CreatedBy),
	})
	if err != nil {
		writeInternalError(w, r, "failed to create issue", err)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func decodeCreateIssueRequest(r *http.Request) (createIssueRequest, error) {
	request, err := decodeJSON[createIssueRequest](r)
	if err != nil {
		return createIssueRequest{}, err
	}

	request.Raw = strings.TrimSpace(request.Raw)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.Raw == "" {
		return createIssueRequest{}, errors.New("raw is required")
	}

	return request, nil
}

func decodeBatchCloseIssuesRequest(r *http.Request) (batchCloseIssuesRequest, error) {
	request, err := decodeJSON[batchCloseIssuesRequest](r)
	if err != nil {
		return batchCloseIssuesRequest{}, err
	}

	request.IDs = sanitizeIssueIDs(request.IDs)
	request.ClosedBy = strings.TrimSpace(request.ClosedBy)
	request.Reason = strings.TrimSpace(request.Reason)
	request.ReasonNote = strings.TrimSpace(request.ReasonNote)
	if len(request.IDs) == 0 {
		return batchCloseIssuesRequest{}, errors.New("at least one issue id is required")
	}

	return request, nil
}

func decodeBatchRefineIssuesRequest(r *http.Request) (batchRefineIssuesRequest, error) {
	request, err := decodeJSON[batchRefineIssuesRequest](r)
	if err != nil {
		return batchRefineIssuesRequest{}, err
	}

	request.IDs = sanitizeIssueIDs(request.IDs)
	request.Raw = strings.TrimSpace(request.Raw)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if len(request.IDs) == 0 {
		return batchRefineIssuesRequest{}, errors.New("at least one issue id is required")
	}
	if request.Raw == "" {
		return batchRefineIssuesRequest{}, errors.New("raw is required")
	}

	return request, nil
}

func decodeBatchProgressIssuesRequest(r *http.Request) (batchProgressIssuesRequest, error) {
	request, err := decodeJSON[batchProgressIssuesRequest](r)
	if err != nil {
		return batchProgressIssuesRequest{}, err
	}

	request.IDs = sanitizeIssueIDs(request.IDs)
	request.Raw = strings.TrimSpace(request.Raw)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if len(request.IDs) == 0 {
		return batchProgressIssuesRequest{}, errors.New("at least one issue id is required")
	}
	if request.Raw == "" {
		return batchProgressIssuesRequest{}, errors.New("raw is required")
	}

	return request, nil
}

func decodeBatchAssignIssuesRequest(r *http.Request) (batchAssignIssuesRequest, error) {
	request, err := decodeJSON[batchAssignIssuesRequest](r)
	if err != nil {
		return batchAssignIssuesRequest{}, err
	}

	request.IDs = sanitizeIssueIDs(request.IDs)
	request.AssignedTo = strings.TrimSpace(request.AssignedTo)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if len(request.IDs) == 0 {
		return batchAssignIssuesRequest{}, errors.New("at least one issue id is required")
	}

	return request, nil
}

func decodeCompareIssuesRequest(r *http.Request) (compareIssuesRequest, error) {
	request, err := decodeJSON[compareIssuesRequest](r)
	if err != nil {
		return compareIssuesRequest{}, err
	}

	seen := make(map[string]struct{}, len(request.IDs))
	normalized := make([]string, 0, len(request.IDs))
	for _, id := range request.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}

	if len(normalized) < 2 {
		return compareIssuesRequest{}, errors.New("at least two issue ids are required")
	}

	request.IDs = normalized
	return request, nil
}

func decodeCloseIssueRequest(r *http.Request) (closeIssueRequest, error) {
	request, err := decodeJSON[closeIssueRequest](r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return closeIssueRequest{}, nil
		}
		return closeIssueRequest{}, err
	}

	request.ClosedBy = strings.TrimSpace(request.ClosedBy)
	request.Reason = strings.TrimSpace(request.Reason)
	request.ReasonNote = strings.TrimSpace(request.ReasonNote)
	return request, nil
}

func decodeRefineIssueRequest(r *http.Request) (refineIssueRequest, error) {
	request, err := decodeJSON[refineIssueRequest](r)
	if err != nil {
		return refineIssueRequest{}, err
	}

	request.Raw = strings.TrimSpace(request.Raw)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.Raw == "" {
		return refineIssueRequest{}, errors.New("raw is required")
	}

	return request, nil
}

func decodeProgressIssueRequest(r *http.Request) (progressIssueRequest, error) {
	request, err := decodeJSON[progressIssueRequest](r)
	if err != nil {
		return progressIssueRequest{}, err
	}

	request.Raw = strings.TrimSpace(request.Raw)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.Raw == "" {
		return progressIssueRequest{}, errors.New("raw is required")
	}

	return request, nil
}

func decodeAssignIssueRequest(r *http.Request) (assignIssueRequest, error) {
	request, err := decodeJSON[assignIssueRequest](r)
	if err != nil {
		return assignIssueRequest{}, err
	}

	request.AssignedTo = strings.TrimSpace(request.AssignedTo)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	return request, nil
}

func decodeSplitIssueRequest(r *http.Request) (splitIssueRequest, error) {
	request, err := decodeJSON[splitIssueRequest](r)
	if err != nil {
		return splitIssueRequest{}, err
	}

	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	request.Note = strings.TrimSpace(request.Note)
	children := make([]splitIssueChildRequest, 0, len(request.Children))
	for _, child := range request.Children {
		child.Raw = strings.TrimSpace(child.Raw)
		if child.Raw == "" {
			continue
		}
		children = append(children, child)
	}
	if len(children) == 0 {
		return splitIssueRequest{}, errors.New("at least one child issue is required")
	}
	request.Children = children
	return request, nil
}

func decodeCombineIssuesRequest(r *http.Request) (combineIssuesRequest, error) {
	request, err := decodeJSON[combineIssuesRequest](r)
	if err != nil {
		return combineIssuesRequest{}, err
	}

	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	request.Note = strings.TrimSpace(request.Note)
	request.IDs = sanitizeIssueIDs(request.IDs)
	if len(request.IDs) < 2 {
		return combineIssuesRequest{}, errors.New("at least two issue ids are required")
	}
	return request, nil
}

func decodeLinkIssuesRequest(r *http.Request) (linkIssuesRequest, error) {
	request, err := decodeJSON[linkIssuesRequest](r)
	if err != nil {
		return linkIssuesRequest{}, err
	}

	request.SourceID = strings.TrimSpace(request.SourceID)
	request.TargetID = strings.TrimSpace(request.TargetID)
	request.Type = strings.TrimSpace(request.Type)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	request.Note = strings.TrimSpace(request.Note)
	if request.SourceID == "" || request.TargetID == "" {
		return linkIssuesRequest{}, errors.New("sourceId and targetId are required")
	}
	if issues.IssueLinkType(request.Type) == "" {
		return linkIssuesRequest{}, errors.New("type is required")
	}
	if issues.IssueLinkType(request.Type) != issues.IssueLinkTypeParentOf &&
		issues.IssueLinkType(request.Type) != issues.IssueLinkTypeChildOf &&
		issues.IssueLinkType(request.Type) != issues.IssueLinkTypeMergedInto &&
		issues.IssueLinkType(request.Type) != issues.IssueLinkTypeDerivedFrom &&
		issues.IssueLinkType(request.Type) != issues.IssueLinkTypeRelatedTo &&
		issues.IssueLinkType(request.Type) != issues.IssueLinkTypeDuplicateOf {
		return linkIssuesRequest{}, errors.New("invalid link type")
	}
	return request, nil
}

func issueItemRoute(prefix string) string {
	return path.Join(prefix, "issues") + "/"
}

func actorForRequest(r *http.Request, fallback string) string {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if ok {
		return principal.ActorName()
	}
	return fallback
}

func toPairwiseIssueSimilarityResponse(items []queries.PairwiseIssueSimilarity) []pairwiseIssueSimilarity {
	out := make([]pairwiseIssueSimilarity, len(items))
	for i, item := range items {
		out[i] = pairwiseIssueSimilarity{
			SourceID:   item.SourceID,
			TargetID:   item.TargetID,
			Similarity: item.Similarity,
		}
	}
	return out
}

func sanitizeIssueIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func firstQueryValue(r *http.Request, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func parseIssueSearchSortBy(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "relevance":
		return "", nil
	case "created_at":
		return "created_at", nil
	default:
		return "", errors.New("sortBy must be one of: relevance, created_at")
	}
}
