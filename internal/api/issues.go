package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"bored/internal/issues"
)

type issuesResponse struct {
	Issues []issues.Issue `json:"issues"`
}

type createIssueRequest struct {
	Raw       string   `json:"raw"`
	Tags      []string `json:"tags,omitempty"`
	CreatedBy string   `json:"createdBy,omitempty"`
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

func (s *Server) handleIssueByID(route string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, route))
		if id == "" || strings.Contains(id, "/") {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}

		issue, err := s.issueStore.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, issues.ErrNotFound) {
				writeError(w, http.StatusNotFound, "issue not found")
				return
			}

			writeError(w, http.StatusInternalServerError, "failed to load issue")
			return
		}

		writeJSON(w, http.StatusOK, issue)
	}
}

func (s *Server) handleIssueList(w http.ResponseWriter, r *http.Request) {
	items, err := s.issueStore.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list issues")
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

	enriched, err := s.analyzeIssueInput(r.Context(), issues.CreateInput{
		Raw:       request.Raw,
		Tags:      request.Tags,
		CreatedBy: request.CreatedBy,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to analyze issue")
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

func issueItemRoute(prefix string) string {
	return path.Join(prefix, "issues") + "/"
}
