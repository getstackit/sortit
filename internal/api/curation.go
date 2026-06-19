package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"sortit/internal/curation"
	"sortit/internal/domain"
	"sortit/internal/issues"
)

type duplicateCandidatesResponse struct {
	Clusters []curation.DuplicateCluster `json:"clusters"`
}

type staleCandidatesResponse struct {
	Issues []curation.StaleIssue `json:"issues"`
}

// optionalFloatQuery returns (value, true, nil) when key is present and valid,
// (0, false, nil) when absent, and an error when present but unparseable.
func optionalFloatQuery(q url.Values, key string) (float64, bool, error) {
	if strings.TrimSpace(q.Get(key)) == "" {
		return 0, false, nil
	}
	f, err := parseFloatQuery(q, key)
	if err != nil {
		return 0, false, err
	}
	return f, true, nil
}

func (s *Server) handleCurationCandidatesDuplicates(w http.ResponseWriter, r *http.Request) {
	if s.curationDetector == nil {
		writeError(w, http.StatusServiceUnavailable, "curation is not available")
		return
	}

	q := r.URL.Query()
	var params curation.DuplicateParams
	if v, ok, err := optionalFloatQuery(q, "minSimilarity"); err != nil {
		writeError(w, http.StatusBadRequest, "invalid minSimilarity query")
		return
	} else if ok {
		params.MinSimilarity = v
	}
	if v, err := ParsePositiveIntQuery(q, "maxSeeds"); err != nil {
		writeError(w, http.StatusBadRequest, "invalid maxSeeds query")
		return
	} else if v != nil {
		params.MaxSeeds = *v
	}
	if v, err := ParsePositiveIntQuery(q, "exploreLimit"); err != nil {
		writeError(w, http.StatusBadRequest, "invalid exploreLimit query")
		return
	} else if v != nil {
		params.ExploreLimit = *v
	}
	if v, err := ParsePositiveIntQuery(q, "limit"); err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit query")
		return
	} else if v != nil {
		params.Limit = *v
	}

	clusters, err := s.curationDetector.DetectDuplicates(r.Context(), params)
	if err != nil {
		writeInternalError(w, r, "failed to detect duplicate candidates", err)
		return
	}
	writeJSON(w, http.StatusOK, duplicateCandidatesResponse{Clusters: clusters})
}

func (s *Server) handleCurationCandidatesStale(w http.ResponseWriter, r *http.Request) {
	if s.curationDetector == nil {
		writeError(w, http.StatusServiceUnavailable, "curation is not available")
		return
	}

	q := r.URL.Query()
	var params curation.StaleParams
	if v, ok, err := optionalFloatQuery(q, "minDaysInactive"); err != nil {
		writeError(w, http.StatusBadRequest, "invalid minDaysInactive query")
		return
	} else if ok {
		params.MinDaysInactive = v
	}
	if v, ok, err := optionalFloatQuery(q, "maxVelocity"); err != nil {
		writeError(w, http.StatusBadRequest, "invalid maxVelocity query")
		return
	} else if ok {
		params.MaxVelocity = v
	}
	if v, err := ParsePositiveIntQuery(q, "limit"); err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit query")
		return
	} else if v != nil {
		params.Limit = *v
	}

	stale, err := s.curationDetector.DetectStaleIssues(r.Context(), params)
	if err != nil {
		writeInternalError(w, r, "failed to detect stale candidates", err)
		return
	}
	writeJSON(w, http.StatusOK, staleCandidatesResponse{Issues: stale})
}

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
