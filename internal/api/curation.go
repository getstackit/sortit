package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"sortit/internal/curation"
	"sortit/internal/domain"
	"sortit/internal/issues"
)

type curationProposalsResponse struct {
	Proposals []domain.CurationProposal `json:"proposals"`
}

type createCurationProposalRequest struct {
	Kind       string                 `json:"kind"`
	Payload    domain.CurationPayload `json:"payload"`
	Rationale  string                 `json:"rationale,omitempty"`
	Confidence float64                `json:"confidence,omitempty"`
	SourceRefs []string               `json:"sourceRefs,omitempty"`
	CreatedBy  string                 `json:"createdBy,omitempty"`
}

func (s *Server) handleCurationProposalList(w http.ResponseWriter, r *http.Request) {
	if s.curation == nil {
		writeError(w, http.StatusServiceUnavailable, "curation is not available")
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if strings.EqualFold(status, "all") {
		status = ""
	}

	proposals, err := s.curation.ListProposals(r.Context(), domain.CurationProposalStatus(status))
	if err != nil {
		writeInternalError(w, r, "failed to list curation proposals", err)
		return
	}

	writeJSON(w, http.StatusOK, curationProposalsResponse{Proposals: proposals})
}

func (s *Server) handleGetCurationProposal(w http.ResponseWriter, r *http.Request) {
	if s.curation == nil {
		writeError(w, http.StatusServiceUnavailable, "curation is not available")
		return
	}

	proposal, err := s.curation.GetProposal(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, issues.ErrCurationProposalNotFound) {
			writeError(w, http.StatusNotFound, "curation proposal not found")
			return
		}
		writeInternalError(w, r, "failed to load curation proposal", err)
		return
	}

	writeJSON(w, http.StatusOK, proposal)
}

func (s *Server) handleCurationProposalCreate(w http.ResponseWriter, r *http.Request) {
	if s.curation == nil {
		writeError(w, http.StatusServiceUnavailable, "curation is not available")
		return
	}

	request, err := decodeJSON[createCurationProposalRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := s.curation.CreateProposal(r.Context(), curation.CreateProposalInput{
		Kind:       domain.CurationKind(strings.TrimSpace(request.Kind)),
		Payload:    request.Payload,
		Rationale:  request.Rationale,
		Confidence: request.Confidence,
		SourceRefs: request.SourceRefs,
		CreatedBy:  actorForRequest(r, request.CreatedBy),
	})
	if err != nil {
		switch {
		case errors.Is(err, curation.ErrUnknownKind), errors.Is(err, curation.ErrInvalidPayload):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeInternalError(w, r, "failed to create curation proposal", err)
		}
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleCurationProposalAccept(w http.ResponseWriter, r *http.Request) {
	if s.curation == nil {
		writeError(w, http.StatusServiceUnavailable, "curation is not available")
		return
	}

	proposal, err := s.curation.AcceptProposal(r.Context(), chi.URLParam(r, "id"), actorForRequest(r, ""))
	if err != nil {
		switch {
		case errors.Is(err, issues.ErrCurationProposalNotFound):
			writeError(w, http.StatusNotFound, "curation proposal not found")
		case errors.Is(err, curation.ErrProposalNotPending):
			writeError(w, http.StatusConflict, "curation proposal is not pending")
		default:
			writeInternalError(w, r, "failed to accept curation proposal", err)
		}
		return
	}

	writeJSON(w, http.StatusOK, proposal)
}

func (s *Server) handleCurationProposalReject(w http.ResponseWriter, r *http.Request) {
	if s.curation == nil {
		writeError(w, http.StatusServiceUnavailable, "curation is not available")
		return
	}

	proposal, err := s.curation.RejectProposal(r.Context(), chi.URLParam(r, "id"), actorForRequest(r, ""))
	if err != nil {
		switch {
		case errors.Is(err, issues.ErrCurationProposalNotFound):
			writeError(w, http.StatusNotFound, "curation proposal not found")
		case errors.Is(err, curation.ErrProposalNotPending):
			writeError(w, http.StatusConflict, "curation proposal is not pending")
		default:
			writeInternalError(w, r, "failed to reject curation proposal", err)
		}
		return
	}

	writeJSON(w, http.StatusOK, proposal)
}
