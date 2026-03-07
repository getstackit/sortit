package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"bored/internal/ai"
	"bored/internal/issues"
	issuemap "bored/internal/map"
)

const debugAnalyzeTimeout = 45 * time.Second

type debugIssueAnalyzeRequest struct {
	Text string   `json:"text"`
	Tags []string `json:"tags,omitempty"`
}

type replaceableIssueStore interface {
	Replace(context.Context, []issues.Issue) error
}

type debugIssueStoreResponse struct {
	IssueCount int `json:"issueCount"`
}

func (s *Server) handleDebugIssueAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	request, err := decodeDebugIssueAnalyzeRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), debugAnalyzeTimeout)
	defer cancel()

	analysis, err := s.analyzer.AnalyzeIssue(ctx, request.Text, debugTags(request.Tags))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ai.ErrNotConfigured) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, analysis)
}

func (s *Server) handleDebugIssueSampleLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	store, ok := s.issueStore.(replaceableIssueStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "issue store cannot be reset")
		return
	}

	samples := issues.SeedIssues()
	enriched, err := s.analyzeSeedIssues(r.Context(), samples)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to analyze sample issues")
		return
	}

	if err := store.Replace(r.Context(), enriched); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load sample issues")
		return
	}

	writeJSON(w, http.StatusOK, debugIssueStoreResponse{IssueCount: len(samples)})
}

func (s *Server) handleDebugIssueReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	store, ok := s.issueStore.(replaceableIssueStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "issue store cannot be reset")
		return
	}

	if err := store.Replace(r.Context(), nil); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear issues")
		return
	}

	writeJSON(w, http.StatusOK, debugIssueStoreResponse{IssueCount: 0})
}

func decodeDebugIssueAnalyzeRequest(r *http.Request) (debugIssueAnalyzeRequest, error) {
	defer r.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	var request debugIssueAnalyzeRequest
	if err := decoder.Decode(&request); err != nil {
		return debugIssueAnalyzeRequest{}, errors.New("invalid request body")
	}

	request.Text = strings.TrimSpace(request.Text)
	if request.Text == "" {
		return debugIssueAnalyzeRequest{}, errors.New("text is required")
	}
	return request, nil
}

func debugTags(rawTags []string) []ai.Tag {
	if len(rawTags) == 0 {
		definitions := issuemap.AllTagDefinitions()
		tags := make([]ai.Tag, 0, len(definitions))
		for _, definition := range definitions {
			if definition.Name == "" {
				continue
			}
			tags = append(tags, ai.Tag{
				Name:        definition.Name,
				Description: definition.Description,
			})
		}
		return tags
	}

	seen := make(map[string]struct{}, len(rawTags))
	tags := make([]ai.Tag, 0, len(rawTags))
	for _, raw := range rawTags {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tags = append(tags, ai.Tag{Name: name})
	}

	if len(tags) == 0 {
		return ai.TagsFromNames(issuemap.AllTags())
	}
	return tags
}
