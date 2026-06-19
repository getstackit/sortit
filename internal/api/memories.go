package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/memories"
)

type memoriesResponse struct {
	Memories []domain.Memory `json:"memories"`
}

type createMemoryRequest struct {
	Title          string   `json:"title,omitempty"`
	Body           string   `json:"body"`
	Kind           string   `json:"kind,omitempty"`
	AnchorTags     []string `json:"anchorTags,omitempty"`
	AnchorRegion   string   `json:"anchorRegion,omitempty"`
	CreatedBy      string   `json:"createdBy,omitempty"`
	SourceIssueIDs []string `json:"sourceIssueIds,omitempty"`
}

func (s *Server) handleMemoryCreate(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
		return
	}

	request, err := decodeJSON[createMemoryRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Body = strings.TrimSpace(request.Body)
	if request.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	created, err := s.memories.CreateMemory(r.Context(), memories.CreateMemoryInput{
		Title:          request.Title,
		Body:           request.Body,
		Kind:           domain.MemoryKind(strings.TrimSpace(request.Kind)),
		AnchorTags:     request.AnchorTags,
		AnchorRegion:   request.AnchorRegion,
		CreatedBy:      actorForRequest(r, request.CreatedBy),
		Source:         domain.MemorySourceManual,
		SourceIssueIDs: request.SourceIssueIDs,
	})
	if err != nil {
		writeInternalError(w, r, "failed to create memory", err)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleMemoryList(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
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

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if strings.EqualFold(status, "all") {
		status = ""
	}

	opts := issues.MemoryListOptions{Status: domain.MemoryStatus(status)}
	if limit != nil {
		opts.Limit = *limit
	}
	if offset != nil {
		opts.Offset = *offset
	}

	items, err := s.memories.ListMemories(r.Context(), opts)
	if err != nil {
		writeInternalError(w, r, "failed to list memories", err)
		return
	}

	writeJSON(w, http.StatusOK, memoriesResponse{Memories: items})
}

func (s *Server) handleGetMemory(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
		return
	}

	memory, err := s.memories.GetMemory(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, issues.ErrMemoryNotFound) {
			writeError(w, http.StatusNotFound, "memory not found")
			return
		}
		writeInternalError(w, r, "failed to load memory", err)
		return
	}

	writeJSON(w, http.StatusOK, memory)
}
