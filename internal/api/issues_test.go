package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
	issuemap "splat/internal/map"
)

type failingIssueStore struct {
	listErr error
}

func (s failingIssueStore) List(context.Context) ([]issues.Issue, error) {
	return nil, s.listErr
}

func (s failingIssueStore) Get(context.Context, string) (issues.Issue, error) {
	return issues.Issue{}, issues.ErrNotFound
}

func (s failingIssueStore) SaveIssue(context.Context, issues.Issue) error { return nil }
func (s failingIssueStore) SaveIssuePost(context.Context, issues.IssuePost) error { return nil }
func (s failingIssueStore) UpdateIssueFields(context.Context, string, issues.IssueFieldUpdate) error {
	return nil
}
func (s failingIssueStore) SaveOperation(context.Context, issues.IssueOperation) error { return nil }
func (s failingIssueStore) SaveLink(context.Context, issues.IssueLink) error             { return nil }
func (s failingIssueStore) NextIssueID(context.Context) (string, error)     { return "", nil }
func (s failingIssueStore) NextOperationID(context.Context) (string, error) { return "", nil }

func TestIssuesEndpointListsSeededIssues(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  newPostgresIssueStore(t, issues.FixtureIssues()),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue list, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected JSON content type, got %q", got)
	}

	var payload issuesResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode issue list response: %v", err)
	}

	if len(payload.Issues) != len(issues.FixtureIssues()) {
		t.Fatalf("expected %d seeded issues, got %d", len(issues.FixtureIssues()), len(payload.Issues))
	}
	if payload.Issues[0].ID != "sample-1" {
		t.Fatalf("expected newest seeded issue first, got %q", payload.Issues[0].ID)
	}
}

func TestIssuesEndpointStartsEmptyByDefault(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue list, got %d", rec.Code)
	}

	var payload issuesResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode issue list response: %v", err)
	}

	if len(payload.Issues) != 0 {
		t.Fatalf("expected no issues by default, got %d", len(payload.Issues))
	}
}

func TestIssuesEndpointGetsIssueByID(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  newPostgresIssueStore(t, issues.FixtureIssues()),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/issues/sample-3", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue lookup, got %d", rec.Code)
	}

	var payload issues.Issue
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode issue response: %v", err)
	}

	if payload.ID != "sample-3" {
		t.Fatalf("expected sample-3, got %q", payload.ID)
	}
	if payload.Raw == "" {
		t.Fatal("expected issue body in response")
	}
	if len(payload.Discussion) != 1 {
		t.Fatalf("expected initial discussion history, got %#v", payload.Discussion)
	}
}

func TestIssuesEndpointReturnsNotFoundForMissingIssue(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  newPostgresIssueStore(t, issues.FixtureIssues()),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/issues/missing-issue", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing issue, got %d", rec.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if payload.Error != "issue not found" {
		t.Fatalf("expected issue not found error, got %q", payload.Error)
	}
}

func TestIssuesEndpointCreatesIssue(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})
	handler := server.Handler()

	body := bytes.NewBufferString(`{"raw":"  new issue  ","tags":["bug"," bug ",""],"createdBy":"  Casey "}`)
	req := httptest.NewRequest(http.MethodPost, "/api/issues", body)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for issue create, got %d", rec.Code)
	}

	var created issues.Issue
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created issue: %v", err)
	}

	if created.Raw != "new issue" {
		t.Fatalf("expected trimmed raw value, got %q", created.Raw)
	}
	if created.CreatedBy != "Casey" {
		t.Fatalf("expected trimmed createdBy, got %q", created.CreatedBy)
	}
	if created.Status != issues.StatusOpen {
		t.Fatalf("expected created issue to be open, got %q", created.Status)
	}
	if len(created.Tags) != 1 || created.Tags[0] != "bug" {
		t.Fatalf("expected sanitized tags, got %#v", created.Tags)
	}
	if len(created.Discussion) != 1 {
		t.Fatalf("expected initial discussion post, got %#v", created.Discussion)
	}
	if created.Discussion[0].Raw != created.Raw {
		t.Fatalf("expected discussion to preserve original raw, got %q", created.Discussion[0].Raw)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	var payload issuesResponse
	if err := json.NewDecoder(listRec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode issue list response: %v", err)
	}

	if payload.Issues[0].ID != created.ID {
		t.Fatalf("expected created issue at top of list, got %q", payload.Issues[0].ID)
	}

	stored, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("failed to list stored issues: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored issue, got %d", len(stored))
	}
	if len(stored[0].TagScores) == 0 {
		t.Fatal("expected stored issue to include analyzed tag scores")
	}
	if len(stored[0].Embedding) == 0 {
		t.Fatal("expected stored issue to include embedding vector")
	}

	tags, err := store.ListTags(context.Background())
	if err != nil {
		t.Fatalf("failed to list stored tags: %v", err)
	}
	if len(tags) == 0 {
		t.Fatal("expected stored tags to include analyzed taxonomy")
	}
	if len(tags[0].Embedding) == 0 {
		t.Fatal("expected stored tags to include embeddings")
	}
}

func TestIssuesEndpointRefinesIssue(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})
	handler := server.Handler()

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/issues",
		bytes.NewBufferString(`{"raw":"Export fails on iPad"}`),
	)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for issue create, got %d", createRec.Code)
	}

	var created issues.Issue
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created issue: %v", err)
	}

	refineReq := httptest.NewRequest(
		http.MethodPost,
		"/api/issues/"+created.ID+"/refine",
		bytes.NewBufferString(`{"raw":"Customer says it only happens in Safari after tapping share twice","createdBy":"Jordan"}`),
	)
	refineReq.Header.Set("Content-Type", "application/json")
	refineRec := httptest.NewRecorder()
	handler.ServeHTTP(refineRec, refineReq)

	if refineRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue refine, got %d", refineRec.Code)
	}

	var refined issues.Issue
	if err := json.NewDecoder(refineRec.Body).Decode(&refined); err != nil {
		t.Fatalf("decode refined issue: %v", err)
	}

	if len(refined.Discussion) != 2 {
		t.Fatalf("expected 2 discussion posts, got %#v", refined.Discussion)
	}
	if refined.Discussion[1].CreatedBy != "Jordan" {
		t.Fatalf("expected refinement author Jordan, got %q", refined.Discussion[1].CreatedBy)
	}
	if refined.Discussion[1].Raw != "Customer says it only happens in Safari after tapping share twice" {
		t.Fatalf("unexpected refinement raw: %q", refined.Discussion[1].Raw)
	}
	if refined.Raw == created.Raw {
		t.Fatalf("expected canonical raw to update after refinement, got %q", refined.Raw)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/issues/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue lookup, got %d", getRec.Code)
	}

	var loaded issues.Issue
	if err := json.NewDecoder(getRec.Body).Decode(&loaded); err != nil {
		t.Fatalf("decode loaded issue: %v", err)
	}
	if len(loaded.Discussion) != 2 {
		t.Fatalf("expected persisted discussion posts, got %#v", loaded.Discussion)
	}
}

func TestIssuesEndpointFiltersByStatusAndClosesIssue(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})
	handler := server.Handler()

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/issues",
		bytes.NewBufferString(`{"raw":"close me"}`),
	)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for issue create, got %d", createRec.Code)
	}

	var created issues.Issue
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created issue: %v", err)
	}

	closeReq := httptest.NewRequest(
		http.MethodPost,
		"/api/issues/"+created.ID+"/close",
		bytes.NewBufferString(`{"closedBy":"Casey"}`),
	)
	closeReq.Header.Set("Content-Type", "application/json")
	closeRec := httptest.NewRecorder()
	handler.ServeHTTP(closeRec, closeReq)

	if closeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue close, got %d", closeRec.Code)
	}

	var closed issues.Issue
	if err := json.NewDecoder(closeRec.Body).Decode(&closed); err != nil {
		t.Fatalf("decode closed issue: %v", err)
	}
	if closed.Status != issues.StatusClosed {
		t.Fatalf("expected closed status, got %q", closed.Status)
	}
	if closed.ClosedAt == nil {
		t.Fatal("expected closedAt to be set")
	}
	if closed.ClosedBy != "Casey" {
		t.Fatalf("expected closedBy Casey, got %q", closed.ClosedBy)
	}

	openReq := httptest.NewRequest(http.MethodGet, "/api/issues?status=open", nil)
	openRec := httptest.NewRecorder()
	handler.ServeHTTP(openRec, openReq)

	var openPayload issuesResponse
	if err := json.NewDecoder(openRec.Body).Decode(&openPayload); err != nil {
		t.Fatalf("decode open issues response: %v", err)
	}
	if len(openPayload.Issues) != 0 {
		t.Fatalf("expected no open issues, got %d", len(openPayload.Issues))
	}

	closedReq := httptest.NewRequest(http.MethodGet, "/api/issues?status=closed", nil)
	closedListRec := httptest.NewRecorder()
	handler.ServeHTTP(closedListRec, closedReq)

	var closedPayload issuesResponse
	if err := json.NewDecoder(closedListRec.Body).Decode(&closedPayload); err != nil {
		t.Fatalf("decode closed issues response: %v", err)
	}
	if len(closedPayload.Issues) != 1 || closedPayload.Issues[0].ID != created.ID {
		t.Fatalf("expected closed issue in filtered list, got %#v", closedPayload.Issues)
	}
}

func TestIssuesEndpointReopensIssue(t *testing.T) {
	store := newPostgresIssueStore(t, []issues.Issue{
		{
			ID:        "issue-closed",
			Raw:       "closed",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[0].CreatedAt,
			Status:    issues.StatusClosed,
			ClosedAt:  new(issues.FixtureIssues()[0].CreatedAt.Add(time.Hour)),
			ClosedBy:  "Casey",
		},
	})
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/issues/issue-closed/reopen", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue reopen, got %d", rec.Code)
	}

	var reopened issues.Issue
	if err := json.NewDecoder(rec.Body).Decode(&reopened); err != nil {
		t.Fatalf("decode reopened issue: %v", err)
	}
	if reopened.Status != issues.StatusOpen {
		t.Fatalf("expected open status, got %q", reopened.Status)
	}
	if reopened.ClosedAt != nil {
		t.Fatalf("expected closedAt cleared, got %v", reopened.ClosedAt)
	}
	if reopened.ClosedBy != "" {
		t.Fatalf("expected closedBy cleared, got %q", reopened.ClosedBy)
	}
}

func TestIssuesEndpointExploresIssue(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	if err := store.UpsertTags(context.Background(), []issues.Tag{
		{Name: "export", Embedding: []float64{1, 0, 0}},
		{Name: "safari", Embedding: []float64{0.9, 0.1, 0}},
		{Name: "bug", Embedding: []float64{0.8, 0.2, 0}},
		{Name: "feature", Embedding: []float64{0, 1, 0}},
	}); err != nil {
		t.Fatalf("seed tags: %v", err)
	}

	if err := store.Replace(context.Background(), []issues.Issue{
		{
			ID:        "issue-target",
			Raw:       "Safari export crashes on PDF download",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[0].CreatedAt,
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "export", Relevance: 0.95},
				{Tag: "safari", Relevance: 0.92},
				{Tag: "bug", Relevance: 0.9},
			},
			Embedding: []float64{1, 0, 0},
		},
		{
			ID:        "issue-related",
			Raw:       "PDF export fails in Safari for some users",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[1].CreatedAt,
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "export", Relevance: 0.91},
				{Tag: "safari", Relevance: 0.88},
				{Tag: "bug", Relevance: 0.67},
			},
			Embedding: []float64{0.99, 0.1, 0},
		},
		{
			ID:        "issue-other",
			Raw:       "Feature request for dashboard filters",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[2].CreatedAt,
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "feature", Relevance: 0.95},
			},
			Embedding: []float64{0, 1, 0},
		},
		{
			ID:        "issue-closed",
			Raw:       "Closed Safari export bug",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[3].CreatedAt,
			Status:    issues.StatusClosed,
			TagScores: []issues.TagRelevance{
				{Tag: "export", Relevance: 0.96},
				{Tag: "safari", Relevance: 0.94},
			},
			Embedding: []float64{1, 0, 0},
		},
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/issues/issue-target/explore?limit=2", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue explore, got %d", rec.Code)
	}

	var payload issuemap.ExploreResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode explore response: %v", err)
	}

	if payload.Issue.ID != "issue-target" {
		t.Fatalf("expected target issue in response, got %q", payload.Issue.ID)
	}
	if len(payload.RelatedIssues) != 2 {
		t.Fatalf("expected 2 related issues, got %d", len(payload.RelatedIssues))
	}
	if payload.RelatedIssues[0].ID != "issue-related" {
		t.Fatalf("expected most related issue first, got %q", payload.RelatedIssues[0].ID)
	}
	if payload.RelatedIssues[0].CombinedSimilarity < payload.RelatedIssues[1].CombinedSimilarity {
		t.Fatalf("expected related issues sorted by combined similarity: %+v", payload.RelatedIssues)
	}
	for _, item := range payload.RelatedIssues {
		if item.ID == "issue-target" || item.ID == "issue-closed" {
			t.Fatalf("expected target and closed issues excluded from related results, got %+v", payload.RelatedIssues)
		}
	}
	if payload.RelatedIssues[0].SemanticSimilarity == 0 {
		t.Fatal("expected semantic similarity to be populated")
	}
	if payload.RelatedIssues[0].FactorSimilarity == 0 {
		t.Fatal("expected factor similarity to be populated")
	}
	if len(payload.Opportunities) == 0 {
		t.Fatal("expected at least one structured opportunity")
	}
	if payload.Opportunities[0].IssueIDs[0] != "issue-target" {
		t.Fatalf("expected opportunity to include target issue first, got %+v", payload.Opportunities[0].IssueIDs)
	}
	if len(payload.Opportunities[0].Issues) == 0 {
		t.Fatalf("expected opportunity issues with descriptions, got %+v", payload.Opportunities[0])
	}
	if payload.Opportunities[0].Issues[0].Description != "Safari export crashes on PDF download" {
		t.Fatalf("expected target issue description in opportunity, got %+v", payload.Opportunities[0].Issues)
	}
	if len(payload.Opportunities[0].SharedTags) == 0 {
		t.Fatalf("expected opportunity shared tags, got %+v", payload.Opportunities[0])
	}
}

func TestIssuesEndpointSearchesIssues(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	if err := store.UpsertTags(context.Background(), []issues.Tag{
		{Name: "frontend", Embedding: []float64{1, 0, 0}},
		{Name: "improvement", Embedding: []float64{0.8, 0.2, 0}},
		{Name: "backend", Embedding: []float64{0, 1, 0}},
	}); err != nil {
		t.Fatalf("seed tags: %v", err)
	}

	if err := store.Replace(context.Background(), []issues.Issue{
		{
			ID:        "issue-frontend",
			Raw:       "Frontend polish for the issue detail page",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[0].CreatedAt,
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "frontend", Relevance: 0.95},
				{Tag: "improvement", Relevance: 0.86},
			},
			Embedding: []float64{1, 0, 0},
		},
		{
			ID:        "issue-ui",
			Raw:       "UI cleanup for issue cards and spacing",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[1].CreatedAt,
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "frontend", Relevance: 0.74},
				{Tag: "improvement", Relevance: 0.71},
			},
			Embedding: []float64{0.86, 0.14, 0},
		},
		{
			ID:        "issue-backend",
			Raw:       "Backend queue for async enrichment",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[2].CreatedAt,
			Status:    issues.StatusOpen,
			TagScores: []issues.TagRelevance{
				{Tag: "backend", Relevance: 0.93},
			},
			Embedding: []float64{0, 1, 0},
		},
		{
			ID:        "issue-closed",
			Raw:       "Closed frontend cleanup follow-up",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[3].CreatedAt,
			Status:    issues.StatusClosed,
			TagScores: []issues.TagRelevance{
				{Tag: "frontend", Relevance: 0.97},
			},
			Embedding: []float64{1, 0, 0},
		},
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		Analyzer: ai.NewAnalyzer(&fakeTagger{
			scores: []ai.TagScore{
				{Tag: "frontend", Relevance: 0.96},
				{Tag: "improvement", Relevance: 0.79},
			},
		}, &fakeEmbedder{
			result: ai.EmbeddingResult{
				Vector: []float32{1, 0, 0},
				Info: ai.EmbeddingInfo{
					Dimensions:          3,
					Preview:             []float32{1, 0, 0},
					ChunkCount:          1,
					EstimatedTokenCount: 3,
					PooledFromChunks:    false,
				},
			},
		}),
		IssueStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/issues/search?q=front%20end%20changes&limit=2", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue search, got %d", rec.Code)
	}

	var payload issuemap.SearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode search response: %v", err)
	}

	if payload.Query.Raw != "front end changes" {
		t.Fatalf("expected query echo, got %q", payload.Query.Raw)
	}
	if len(payload.Query.Tags) < 2 || payload.Query.Tags[0].Tag != "frontend" {
		t.Fatalf("expected query tags in response, got %+v", payload.Query.Tags)
	}
	if len(payload.RelatedIssues) != 2 {
		t.Fatalf("expected 2 related issues, got %d", len(payload.RelatedIssues))
	}
	if payload.RelatedIssues[0].ID != "issue-frontend" {
		t.Fatalf("expected issue-frontend first, got %q", payload.RelatedIssues[0].ID)
	}
	if payload.RelatedIssues[1].ID != "issue-ui" {
		t.Fatalf("expected issue-ui second, got %q", payload.RelatedIssues[1].ID)
	}
	for _, item := range payload.RelatedIssues {
		if item.Status != issues.StatusOpen {
			t.Fatalf("expected only open issues in search results, got %+v", payload.RelatedIssues)
		}
	}
}

func TestIssuesEndpointSearchRequiresQuery(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/issues/search?limit=2", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing query, got %d", rec.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error != "query is required" {
		t.Fatalf("expected query error, got %q", payload.Error)
	}
}

func TestIssuesEndpointExploreRejectsInvalidLimit(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  newPostgresIssueStore(t, issues.FixtureIssues()),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/issues/sample-1/explore?limit=0", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid limit query, got %d", rec.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error != "invalid limit query" {
		t.Fatalf("expected invalid limit error, got %q", payload.Error)
	}
}

//go:fix inline
func ptrToTime(value time.Time) *time.Time {
	return new(value)
}

func TestIssuesEndpointRejectsInvalidBody(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/issues", bytes.NewBufferString(`{"text":"missing raw"}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid issue create body, got %d", rec.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if payload.Error == "" {
		t.Fatal("expected error message in response")
	}
}

func TestIssuesEndpointLogsInternalServerErrors(t *testing.T) {
	var logOutput bytes.Buffer
	originalWriter := log.Writer()
	log.SetOutput(&logOutput)
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
	})

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore: failingIssueStore{
			listErr: errors.New("database offline"),
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if payload.Error != "failed to list issues" {
		t.Fatalf("expected stable client error message, got %q", payload.Error)
	}

	logged := logOutput.String()
	if !strings.Contains(logged, "500 GET /api/issues: failed to list issues: database offline") {
		t.Fatalf("expected internal error log, got %q", logged)
	}
}

func TestIssuesCompareEndpointReturnsEmbeddingSimilarity(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	if err := store.Replace(context.Background(), []issues.Issue{
		{
			ID:        "issue-a",
			Raw:       "first",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[0].CreatedAt,
			Embedding: []float64{1, 0},
		},
		{
			ID:        "issue-b",
			Raw:       "second",
			CreatedBy: "Casey",
			CreatedAt: issues.FixtureIssues()[1].CreatedAt,
			Embedding: []float64{0.6, 0.8},
		},
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/issues/compare",
		bytes.NewBufferString(`{"ids":["issue-a","issue-b"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload compareIssuesResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode compare payload: %v", err)
	}

	if payload.ComparedIssueCount != 2 {
		t.Fatalf("expected 2 compared issues, got %d", payload.ComparedIssueCount)
	}
	if payload.AverageEmbeddingSimilarity != 0.6 {
		t.Fatalf("expected average embedding similarity 0.6, got %v", payload.AverageEmbeddingSimilarity)
	}
	if len(payload.PairwiseEmbeddingSimilarity) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(payload.PairwiseEmbeddingSimilarity))
	}
	if payload.PairwiseEmbeddingSimilarity[0].Similarity != 0.6 {
		t.Fatalf("expected pair similarity 0.6, got %v", payload.PairwiseEmbeddingSimilarity[0].Similarity)
	}
}
