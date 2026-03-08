package api

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"path"
	"sort"
	"strings"

	"splat/internal/issues"
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
	ClosedBy string `json:"closedBy,omitempty"`
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

	storeIssues, err := s.issueStore.List(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to list issues", err)
		return
	}

	issuesByID := make(map[string]issues.Issue, len(storeIssues))
	for _, issue := range storeIssues {
		issuesByID[issue.ID] = issue
	}

	selected := make([]issues.Issue, 0, len(request.IDs))
	for _, id := range request.IDs {
		issue, ok := issuesByID[id]
		if !ok {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		selected = append(selected, issue)
	}

	pairs, average := compareIssueEmbeddings(selected)
	writeJSON(w, http.StatusOK, compareIssuesResponse{
		ComparedIssueCount:          len(selected),
		AverageEmbeddingSimilarity:  average,
		PairwiseEmbeddingSimilarity: pairs,
	})
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
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}

			issue, err := s.issueStore.Get(r.Context(), id)
			if err != nil {
				if errors.Is(err, issues.ErrNotFound) {
					writeError(w, http.StatusNotFound, "issue not found")
					return
				}

				writeInternalError(w, r, "failed to load issue", err)
				return
			}

			writeJSON(w, http.StatusOK, issue)
			return
		}

		switch segments[1] {
		case "close":
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

			closed, err := s.issueStore.CloseIssue(r.Context(), id, request.ClosedBy)
			if err != nil {
				if errors.Is(err, issues.ErrNotFound) {
					writeError(w, http.StatusNotFound, "issue not found")
					return
				}

				writeInternalError(w, r, "failed to close issue", err)
				return
			}

			writeJSON(w, http.StatusOK, closed)
		case "reopen":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}

			reopened, err := s.issueStore.ReopenIssue(r.Context(), id)
			if err != nil {
				if errors.Is(err, issues.ErrNotFound) {
					writeError(w, http.StatusNotFound, "issue not found")
					return
				}

				writeInternalError(w, r, "failed to reopen issue", err)
				return
			}

			writeJSON(w, http.StatusOK, reopened)
		default:
			writeError(w, http.StatusNotFound, "route not found")
		}
	}
}

func (s *Server) handleIssueList(w http.ResponseWriter, r *http.Request) {
	filter, err := ParseIssueStatusFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid status query")
		return
	}

	items, err := s.issueStore.List(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to list issues", err)
		return
	}

	writeJSON(w, http.StatusOK, issuesResponse{Issues: filterIssuesByStatus(items, filter)})
}

func (s *Server) handleIssueCreate(w http.ResponseWriter, r *http.Request) {
	request, err := decodeCreateIssueRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	enriched, err := s.analyzeIssueInput(r.Context(), issues.CreateInput{
		Raw:       request.Raw,
		Tags:      request.Tags,
		CreatedBy: request.CreatedBy,
	})
	if err != nil {
		writeInternalError(w, r, "failed to analyze issue", err)
		return
	}

	created, err := s.issueStore.Create(r.Context(), enriched)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func decodeCreateIssueRequest(r *http.Request) (createIssueRequest, error) {
	defer r.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	var request createIssueRequest
	if err := decoder.Decode(&request); err != nil {
		return createIssueRequest{}, errors.New("invalid request body")
	}

	request.Raw = strings.TrimSpace(request.Raw)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.Raw == "" {
		return createIssueRequest{}, errors.New("raw is required")
	}

	return request, nil
}

func decodeCompareIssuesRequest(r *http.Request) (compareIssuesRequest, error) {
	defer r.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	var request compareIssuesRequest
	if err := decoder.Decode(&request); err != nil {
		return compareIssuesRequest{}, errors.New("invalid request body")
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
	defer r.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	var request closeIssueRequest
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return closeIssueRequest{}, nil
		}
		return closeIssueRequest{}, errors.New("invalid request body")
	}

	request.ClosedBy = strings.TrimSpace(request.ClosedBy)
	return request, nil
}

func compareIssueEmbeddings(items []issues.Issue) ([]pairwiseIssueSimilarity, float64) {
	pairs := make([]pairwiseIssueSimilarity, 0, len(items)*(len(items)-1)/2)
	total := 0.0

	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			similarity := cosineSimilarity(items[i].Embedding, items[j].Embedding)
			total += similarity
			pairs = append(pairs, pairwiseIssueSimilarity{
				SourceID:   items[i].ID,
				TargetID:   items[j].ID,
				Similarity: math.Round(similarity*100) / 100,
			})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Similarity == pairs[j].Similarity {
			if pairs[i].SourceID == pairs[j].SourceID {
				return pairs[i].TargetID < pairs[j].TargetID
			}
			return pairs[i].SourceID < pairs[j].SourceID
		}
		return pairs[i].Similarity > pairs[j].Similarity
	})

	average := 0.0
	if len(pairs) > 0 {
		average = math.Round((total/float64(len(pairs)))*100) / 100
	}
	return pairs, average
}

func issueItemRoute(prefix string) string {
	return path.Join(prefix, "issues") + "/"
}
