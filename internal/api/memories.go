package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/memories"
)

type memoriesResponse struct {
	Memories []domain.Memory `json:"memories"`
}

type recalledMemoriesResponse struct {
	Memories []memories.RecalledMemory `json:"memories"`
}

// handleMemorySearch recalls active memories most similar to a query — the read
// side of the memory loop, so prior decisions surface before an agent acts.
func (s *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
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

	limit, err := ParsePositiveIntQuery(r.URL.Query(), "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit query")
		return
	}

	opts := memories.RecallOptions{}
	if limit != nil {
		opts.Limit = *limit
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("min_similarity")); raw != "" {
		minSimilarity, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid min_similarity query")
			return
		}
		opts.MinSimilarity = minSimilarity
	}

	results, err := s.memories.RecallMemories(r.Context(), query, opts)
	if err != nil {
		writeInternalError(w, r, "failed to recall memories", err)
		return
	}

	writeJSON(w, http.StatusOK, recalledMemoriesResponse{Memories: results})
}

type createMemoryRequest struct {
	Title          string   `json:"title,omitempty"`
	Body           string   `json:"body"`
	Kind           string   `json:"kind,omitempty"`
	SubjectTag     string   `json:"subjectTag,omitempty"`
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
		SubjectTag:     strings.TrimSpace(request.SubjectTag),
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

// handleMemoryConcept returns the active concept memory bound to a subject tag,
// powering the tag↔concept cross-link on tag detail pages.
func (s *Server) handleMemoryConcept(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
		return
	}
	subjectTag := strings.TrimSpace(r.URL.Query().Get("subjectTag"))
	if subjectTag == "" {
		writeError(w, http.StatusBadRequest, "subjectTag is required")
		return
	}

	memory, err := s.memories.ConceptForTag(r.Context(), subjectTag)
	if err != nil {
		if errors.Is(err, issues.ErrMemoryNotFound) {
			writeError(w, http.StatusNotFound, "no concept for tag")
			return
		}
		writeInternalError(w, r, "failed to load concept", err)
		return
	}

	writeJSON(w, http.StatusOK, memory)
}

type supersedeMemoryRequest struct {
	SupersededBy string `json:"supersededBy,omitempty"`
}

func (s *Server) handleMemorySupersede(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
		return
	}

	request, err := decodeJSON[supersedeMemoryRequest](r)
	if err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	memory, err := s.memories.SupersedeMemory(r.Context(), chi.URLParam(r, "id"), request.SupersededBy)
	if err != nil {
		if errors.Is(err, issues.ErrMemoryNotFound) {
			writeError(w, http.StatusNotFound, "memory not found")
			return
		}
		writeInternalError(w, r, "failed to supersede memory", err)
		return
	}

	writeJSON(w, http.StatusOK, memory)
}

func (s *Server) handleMemoryArchive(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
		return
	}

	memory, err := s.memories.ArchiveMemory(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, issues.ErrMemoryNotFound) {
			writeError(w, http.StatusNotFound, "memory not found")
			return
		}
		writeInternalError(w, r, "failed to archive memory", err)
		return
	}

	writeJSON(w, http.StatusOK, memory)
}

type memoryProposalsResponse struct {
	Proposals []domain.MemoryProposal `json:"proposals"`
}

func (s *Server) handleMemoryProposalList(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if strings.EqualFold(status, "all") {
		status = ""
	}

	proposals, err := s.memories.ListProposals(r.Context(), domain.MemoryProposalStatus(status))
	if err != nil {
		writeInternalError(w, r, "failed to list memory proposals", err)
		return
	}

	writeJSON(w, http.StatusOK, memoryProposalsResponse{Proposals: proposals})
}

func (s *Server) handleMemoryProposalSynthesize(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
		return
	}

	proposals, err := s.memories.SynthesizeProposals(r.Context())
	if err != nil {
		writeInternalError(w, r, "failed to synthesize memory proposals", err)
		return
	}

	writeJSON(w, http.StatusOK, memoryProposalsResponse{Proposals: proposals})
}

func (s *Server) handleMemoryProposalAccept(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
		return
	}

	memory, err := s.memories.AcceptProposal(r.Context(), chi.URLParam(r, "id"), actorForRequest(r, ""))
	if err != nil {
		switch {
		case errors.Is(err, issues.ErrMemoryProposalNotFound):
			writeError(w, http.StatusNotFound, "memory proposal not found")
		case errors.Is(err, memories.ErrProposalNotPending):
			writeError(w, http.StatusConflict, "memory proposal is not pending")
		default:
			writeInternalError(w, r, "failed to accept memory proposal", err)
		}
		return
	}

	writeJSON(w, http.StatusCreated, memory)
}

func (s *Server) handleMemoryProposalReject(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memories are not available")
		return
	}

	proposal, err := s.memories.RejectProposal(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		switch {
		case errors.Is(err, issues.ErrMemoryProposalNotFound):
			writeError(w, http.StatusNotFound, "memory proposal not found")
		case errors.Is(err, memories.ErrProposalNotPending):
			writeError(w, http.StatusConflict, "memory proposal is not pending")
		default:
			writeInternalError(w, r, "failed to reject memory proposal", err)
		}
		return
	}

	writeJSON(w, http.StatusOK, proposal)
}
