package api

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"splat/internal/ai"
	"splat/internal/issues"
	issuemap "splat/internal/map"
)

type integrationMockTagger struct{}

func (t *integrationMockTagger) Score(_ context.Context, text string, _ []ai.Tag) ([]ai.TagScore, error) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "signup"):
		return []ai.TagScore{
			{Tag: "onboarding", Relevance: 0.97},
			{Tag: "wizard", Relevance: 0.91, Suggested: true, Description: "multi-step setup flow"},
			{Tag: "ux", Relevance: 0.42},
		}, nil
	case strings.Contains(lower, "export"):
		return []ai.TagScore{
			{Tag: "export", Relevance: 0.96},
			{Tag: "csv", Relevance: 0.9, Suggested: true, Description: "csv export format"},
			{Tag: "performance", Relevance: 0.45},
		}, nil
	default:
		return []ai.TagScore{
			{Tag: "idea", Relevance: 0.61},
		}, nil
	}
}

func (t *integrationMockTagger) Provider() string {
	return "mock"
}

func (t *integrationMockTagger) Model() string {
	return "integration-tagger"
}

type integrationMockEmbedder struct{}

type integrationMockCanonicalizer struct{}

func (e *integrationMockEmbedder) EmbedText(_ context.Context, text string) (ai.EmbeddingResult, error) {
	vector := integrationVector(strings.ToLower(strings.TrimSpace(text)))
	return ai.EmbeddingResult{
		Vector: vector,
		Info: ai.EmbeddingInfo{
			Dimensions:          len(vector),
			Preview:             append([]float32(nil), vector...),
			ChunkCount:          1,
			EstimatedTokenCount: len(strings.Fields(text)),
			PooledFromChunks:    false,
		},
	}, nil
}

func (e *integrationMockEmbedder) Provider() string {
	return "mock"
}

func (e *integrationMockEmbedder) Model() string {
	return "integration-embedder"
}

func (c *integrationMockCanonicalizer) CanonicalizeDiscussion(_ context.Context, posts []string) (string, error) {
	return strings.Join(posts, "\n\n"), nil
}

func integrationVector(text string) []float32 {
	vector := make([]float32, 4)
	for i, r := range text {
		vector[i%len(vector)] += float32((int(r)%17)+1) / 17
	}

	norm := float32(0)
	for _, value := range vector {
		norm += value * value
	}
	if norm == 0 {
		return []float32{1, 0, 0, 0}
	}

	norm = float32(math.Sqrt(float64(norm)))
	for i := range vector {
		vector[i] /= norm
	}
	return vector
}

func TestAPIIntegrationCreateLookupMapAndTagsWithMockAIBackend(t *testing.T) {
	store := newSQLiteIssueStore(t, nil)
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		Analyzer: ai.NewAnalyzer(
			&integrationMockTagger{},
			&integrationMockEmbedder{},
		),
		IssueStore: store,
	})
	if err := server.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize server: %v", err)
	}

	handler := server.Handler()

	first := createIssueViaAPI(t, handler, createIssueRequest{
		Raw:       "Signup flow needs a clearer multi-step wizard for first-time teams",
		CreatedBy: "Casey",
	})
	second := createIssueViaAPI(t, handler, createIssueRequest{
		Raw:       "CSV export is too slow for large workspaces",
		CreatedBy: "Jordan",
	})

	if !slices.Equal(first.Tags, []string{"onboarding", "wizard", "ux"}) {
		t.Fatalf("unexpected tags for first issue: %#v", first.Tags)
	}
	if !slices.Equal(second.Tags, []string{"export", "csv", "performance"}) {
		t.Fatalf("unexpected tags for second issue: %#v", second.Tags)
	}

	storedFirst := getIssueViaAPI(t, handler, first.ID)
	storedSecond := getIssueViaAPI(t, handler, second.ID)

	if storedFirst.Raw != first.Raw || storedFirst.CreatedBy != first.CreatedBy {
		t.Fatalf("unexpected first issue payload: %+v", storedFirst)
	}
	if storedSecond.Raw != second.Raw || storedSecond.CreatedBy != second.CreatedBy {
		t.Fatalf("unexpected second issue payload: %+v", storedSecond)
	}

	mapReq := httptest.NewRequest(http.MethodGet, "/api/map", nil)
	mapRec := httptest.NewRecorder()
	handler.ServeHTTP(mapRec, mapReq)

	if mapRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for map, got %d", mapRec.Code)
	}

	var mapPayload issuemap.MapResponse
	if err := json.NewDecoder(mapRec.Body).Decode(&mapPayload); err != nil {
		t.Fatalf("decode map response: %v", err)
	}
	if len(mapPayload.Issues) != 2 {
		t.Fatalf("expected 2 issues in map response, got %d", len(mapPayload.Issues))
	}

	firstMapIssue := findMapIssue(t, mapPayload.Issues, first.ID)
	secondMapIssue := findMapIssue(t, mapPayload.Issues, second.ID)
	if firstMapIssue.Tags[0].Tag != "onboarding" {
		t.Fatalf("expected first map issue to use analyzed tags, got %+v", firstMapIssue.Tags)
	}
	if secondMapIssue.Tags[0].Tag != "export" {
		t.Fatalf("expected second map issue to use analyzed tags, got %+v", secondMapIssue.Tags)
	}

	tagsReq := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	tagsRec := httptest.NewRecorder()
	handler.ServeHTTP(tagsRec, tagsReq)

	if tagsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for tags, got %d", tagsRec.Code)
	}

	var tagsPayload tagsResponse
	if err := json.NewDecoder(tagsRec.Body).Decode(&tagsPayload); err != nil {
		t.Fatalf("decode tags response: %v", err)
	}

	tagNames := make([]string, 0, len(tagsPayload.Tags))
	tagDescriptions := make(map[string]string, len(tagsPayload.Tags))
	tagEmbeddings := make(map[string][]float64, len(tagsPayload.Tags))
	for _, tag := range tagsPayload.Tags {
		tagNames = append(tagNames, tag.Name)
		tagDescriptions[tag.Name] = tag.Description
		tagEmbeddings[tag.Name] = tag.Embedding
	}

	for _, name := range []string{"onboarding", "wizard", "export", "csv", "ux", "performance"} {
		if !slices.Contains(tagNames, name) {
			t.Fatalf("expected tags response to include %q, got %#v", name, tagNames)
		}
	}
	if tagDescriptions["wizard"] != "multi-step setup flow" {
		t.Fatalf("unexpected wizard tag description: %q", tagDescriptions["wizard"])
	}
	if tagDescriptions["csv"] != "csv export format" {
		t.Fatalf("unexpected csv tag description: %q", tagDescriptions["csv"])
	}
	if len(tagEmbeddings["wizard"]) == 0 {
		t.Fatal("expected wizard tag embedding in tags response")
	}
}

func TestAPIIntegrationCreateAndCloseIssue(t *testing.T) {
	store := newSQLiteIssueStore(t, nil)
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		Analyzer: ai.NewAnalyzer(
			&integrationMockTagger{},
			&integrationMockEmbedder{},
		),
		IssueStore: store,
	})
	if err := server.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize server: %v", err)
	}

	handler := server.Handler()
	created := createIssueViaAPI(t, handler, createIssueRequest{
		Raw:       "Export flow fails after download starts",
		CreatedBy: "Casey",
	})
	if created.Status != issues.StatusOpen {
		t.Fatalf("expected created issue to be open, got %q", created.Status)
	}

	closed := closeIssueViaAPI(t, handler, created.ID, closeIssueRequest{
		ClosedBy: "Jordan",
	})
	if closed.Status != issues.StatusClosed {
		t.Fatalf("expected closed issue status, got %q", closed.Status)
	}
	if closed.ClosedAt == nil {
		t.Fatal("expected closedAt to be set")
	}
	if closed.ClosedBy != "Jordan" {
		t.Fatalf("expected closedBy Jordan, got %q", closed.ClosedBy)
	}

	stored := getIssueViaAPI(t, handler, created.ID)
	if stored.Status != issues.StatusClosed {
		t.Fatalf("expected stored issue to be closed, got %q", stored.Status)
	}
	if stored.ClosedAt == nil {
		t.Fatal("expected stored issue closedAt to be set")
	}
	if stored.ClosedBy != "Jordan" {
		t.Fatalf("expected stored issue closedBy Jordan, got %q", stored.ClosedBy)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/issues?status=closed", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for closed issue list, got %d", rec.Code)
	}

	var payload issuesResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode closed issue list: %v", err)
	}
	if len(payload.Issues) != 1 {
		t.Fatalf("expected 1 closed issue, got %d", len(payload.Issues))
	}
	if payload.Issues[0].ID != created.ID {
		t.Fatalf("expected closed issue %q, got %q", created.ID, payload.Issues[0].ID)
	}
	if payload.Issues[0].Status != issues.StatusClosed {
		t.Fatalf("expected closed list item status closed, got %q", payload.Issues[0].Status)
	}

	openMapReq := httptest.NewRequest(http.MethodGet, "/api/map?status=open", nil)
	openMapRec := httptest.NewRecorder()
	handler.ServeHTTP(openMapRec, openMapReq)

	if openMapRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for open map, got %d", openMapRec.Code)
	}

	var openMapPayload issuemap.MapResponse
	if err := json.NewDecoder(openMapRec.Body).Decode(&openMapPayload); err != nil {
		t.Fatalf("decode open map response: %v", err)
	}
	if len(openMapPayload.Issues) != 0 {
		t.Fatalf("expected closed issue to be hidden from open map, got %d issues", len(openMapPayload.Issues))
	}

	allMapReq := httptest.NewRequest(http.MethodGet, "/api/map?status=all", nil)
	allMapRec := httptest.NewRecorder()
	handler.ServeHTTP(allMapRec, allMapReq)

	if allMapRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for all-status map, got %d", allMapRec.Code)
	}

	var allMapPayload issuemap.MapResponse
	if err := json.NewDecoder(allMapRec.Body).Decode(&allMapPayload); err != nil {
		t.Fatalf("decode all-status map response: %v", err)
	}
	if len(allMapPayload.Issues) != 1 {
		t.Fatalf("expected closed issue to appear in all-status map, got %d issues", len(allMapPayload.Issues))
	}
	if allMapPayload.Issues[0].Status != issues.StatusClosed {
		t.Fatalf("expected map issue status closed, got %q", allMapPayload.Issues[0].Status)
	}
}

func TestAPIIntegrationRefineIssueUpdatesCanonicalTagsAndDiscussion(t *testing.T) {
	store := newSQLiteIssueStore(t, nil)
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		Analyzer: ai.NewAnalyzerWithCanonicalizer(
			&integrationMockTagger{},
			&integrationMockEmbedder{},
			&integrationMockCanonicalizer{},
		),
		IssueStore: store,
	})
	if err := server.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize server: %v", err)
	}

	handler := server.Handler()
	created := createIssueViaAPI(t, handler, createIssueRequest{
		Raw:       "Maybe we should improve this workflow",
		CreatedBy: "Casey",
	})
	if !slices.Equal(created.Tags, []string{"idea"}) {
		t.Fatalf("unexpected tags before refinement: %#v", created.Tags)
	}

	refined := refineIssueViaAPI(t, handler, created.ID, refineIssueRequest{
		Raw:       "Customer reports export spins forever when downloading a CSV.",
		CreatedBy: "Jordan",
	})
	if len(refined.Discussion) != 2 {
		t.Fatalf("expected 2 discussion posts, got %#v", refined.Discussion)
	}
	if !strings.Contains(strings.ToLower(refined.Raw), "export") {
		t.Fatalf("expected canonical raw to include refinement context, got %q", refined.Raw)
	}
	if !slices.Equal(refined.Tags, []string{"export", "csv", "performance"}) {
		t.Fatalf("unexpected tags after refinement: %#v", refined.Tags)
	}

	stored := getIssueViaAPI(t, handler, created.ID)
	if len(stored.Discussion) != 2 {
		t.Fatalf("expected stored discussion history, got %#v", stored.Discussion)
	}
	if stored.Discussion[1].CreatedBy != "Jordan" {
		t.Fatalf("expected refinement author Jordan, got %q", stored.Discussion[1].CreatedBy)
	}
}

func createIssueViaAPI(t *testing.T, handler http.Handler, request createIssueRequest) issues.Issue {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal issue request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/issues", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for issue create, got %d", rec.Code)
	}

	var issue issues.Issue
	if err := json.NewDecoder(rec.Body).Decode(&issue); err != nil {
		t.Fatalf("decode created issue: %v", err)
	}
	return issue
}

func getIssueViaAPI(t *testing.T, handler http.Handler, id string) issues.Issue {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/issues/"+id, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue lookup, got %d", rec.Code)
	}

	var issue issues.Issue
	if err := json.NewDecoder(rec.Body).Decode(&issue); err != nil {
		t.Fatalf("decode issue: %v", err)
	}
	return issue
}

func closeIssueViaAPI(
	t *testing.T,
	handler http.Handler,
	id string,
	request closeIssueRequest,
) issues.Issue {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal close issue request: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/issues/"+id+"/close",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue close, got %d", rec.Code)
	}

	var issue issues.Issue
	if err := json.NewDecoder(rec.Body).Decode(&issue); err != nil {
		t.Fatalf("decode closed issue: %v", err)
	}
	return issue
}

func refineIssueViaAPI(
	t *testing.T,
	handler http.Handler,
	id string,
	request refineIssueRequest,
) issues.Issue {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal refine issue request: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/issues/"+id+"/refine",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for issue refine, got %d", rec.Code)
	}

	var issue issues.Issue
	if err := json.NewDecoder(rec.Body).Decode(&issue); err != nil {
		t.Fatalf("decode refined issue: %v", err)
	}
	return issue
}

func findMapIssue(t *testing.T, mapIssues []issuemap.MapIssue, id string) issuemap.MapIssue {
	t.Helper()

	for _, issue := range mapIssues {
		if issue.ID == id {
			return issue
		}
	}

	t.Fatalf("expected map response to include issue %q", id)
	return issuemap.MapIssue{}
}
