package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"splat/internal/issues"
	"splat/internal/people"
)

const testUser = "Casey"
const testPerson = "Avery"

func TestPersonDetailReturnsAssignedQueueHeadAndRecommendations(t *testing.T) {
	makeIssue := func(
		id string,
		raw string,
		assignedTo string,
		status issues.IssueStatus,
		createdAt time.Time,
		tagScores []issues.TagRelevance,
		embedding []float64,
	) issues.Issue {
		issue := issues.BuildNewIssue(id, issues.CreateInput{
			Raw:       raw,
			CreatedBy: testUser,
			TagScores: tagScores,
			Embedding: embedding,
		})
		issue.AssignedTo = assignedTo
		issue.Status = status
		issue.CreatedAt = createdAt
		if status == issues.StatusClosed {
			closedAt := createdAt.Add(2 * time.Hour)
			issue.ClosedAt = &closedAt
			issue.ClosedBy = testUser
		}
		return issue
	}

	seed := []issues.Issue{
		makeIssue(
			"issue-assigned-old",
			"Fix auth redirect loop for invited users",
			testPerson,
			issues.StatusOpen,
			time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.9}, {Tag: "onboarding", Relevance: 0.6}},
			[]float64{1, 0},
		),
		makeIssue(
			"issue-assigned-new",
			"Tighten search filters on the team page",
			testPerson,
			issues.StatusOpen,
			time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "search", Relevance: 0.8}, {Tag: "ui", Relevance: 0.4}},
			[]float64{0.7, 0.3},
		),
		makeIssue(
			"issue-assigned-closed",
			"Polish invite acceptance copy",
			testPerson,
			issues.StatusClosed,
			time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "onboarding", Relevance: 0.7}, {Tag: "ui", Relevance: 0.3}},
			[]float64{0.9, 0.1},
		),
		makeIssue(
			"issue-recommended-best",
			"Invited users land on a blank auth handoff screen",
			"",
			issues.StatusOpen,
			time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.85}, {Tag: "onboarding", Relevance: 0.7}},
			[]float64{0.98, 0.02},
		),
		makeIssue(
			"issue-recommended-weaker",
			"Export CSV columns shift on Safari",
			"",
			issues.StatusOpen,
			time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "export", Relevance: 0.9}, {Tag: "safari", Relevance: 0.7}},
			[]float64{0.05, 0.95},
		),
		makeIssue(
			"issue-assigned-other",
			"Auth callback page flashes twice",
			"Jordan",
			issues.StatusOpen,
			time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.9}},
			[]float64{1, 0},
		),
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  newPostgresIssueStore(t, seed),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/people/Avery", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for person detail, got %d", rec.Code)
	}

	var payload people.PersonDetail
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode person detail response: %v", err)
	}

	if payload.Person != testPerson {
		t.Fatalf("expected person Avery, got %q", payload.Person)
	}
	if payload.IssueCount != 3 || payload.OpenIssueCount != 2 || payload.ClosedIssueCount != 1 {
		t.Fatalf("unexpected issue counts: %+v", payload)
	}
	if payload.NextIssue == nil {
		t.Fatal("expected nextIssue")
	}
	if payload.NextIssue.Source != "assigned" {
		t.Fatalf("expected assigned next issue source, got %q", payload.NextIssue.Source)
	}
	if payload.NextIssue.Issue.ID != "issue-assigned-old" {
		t.Fatalf("expected oldest assigned open issue, got %q", payload.NextIssue.Issue.ID)
	}
	if len(payload.RecommendedIssues) == 0 {
		t.Fatal("expected recommended issues")
	}
	if payload.RecommendedIssues[0].Issue.ID != "issue-recommended-best" {
		t.Fatalf("expected best recommendation first, got %q", payload.RecommendedIssues[0].Issue.ID)
	}
	if payload.RecommendedIssues[0].Source != "recommended" {
		t.Fatalf("expected recommended source, got %q", payload.RecommendedIssues[0].Source)
	}
}

func TestPersonDetailRecommendationsPreferMatureIssueWhenBaseMatchIsNearEqual(t *testing.T) {
	makeIssue := func(
		id string,
		raw string,
		assignedTo string,
		status issues.IssueStatus,
		createdAt time.Time,
		tagScores []issues.TagRelevance,
		embedding []float64,
	) issues.Issue {
		issue := issues.BuildNewIssue(id, issues.CreateInput{
			Raw:       raw,
			CreatedBy: testUser,
			TagScores: tagScores,
			Embedding: embedding,
		})
		issue.AssignedTo = assignedTo
		issue.Status = status
		issue.CreatedAt = createdAt
		if len(issue.Discussion) > 0 {
			issue.Discussion[0].CreatedAt = createdAt
		}
		return issue
	}

	store := newPostgresIssueStore(t, []issues.Issue{
		makeIssue(
			"issue-assigned",
			"Fix auth redirect loop for invited users",
			testPerson,
			issues.StatusOpen,
			time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.9}, {Tag: "onboarding", Relevance: 0.6}},
			[]float64{1, 0},
		),
		makeIssue(
			"issue-mature",
			"Invited users land on a blank auth handoff screen.\n1. Open invite link.\n2. Complete sign in.\n3. Observe blank handoff page.\nError: callback stalls in /auth/handoff.",
			"",
			issues.StatusOpen,
			time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.85}, {Tag: "onboarding", Relevance: 0.7}},
			[]float64{0.98, 0.02},
		),
		makeIssue(
			"issue-immature",
			"auth issue",
			"",
			issues.StatusOpen,
			time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.85}, {Tag: "onboarding", Relevance: 0.7}},
			[]float64{0.98, 0.02},
		),
	})

	for _, post := range []issues.IssuePost{
		{ID: "issue-mature-post-000002", IssueID: "issue-mature", Raw: "Add exact repro steps", CreatedBy: testUser, CreatedAt: time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC), Sequence: 2, Kind: "refinement"},
		{ID: "issue-mature-post-000003", IssueID: "issue-mature", Raw: "Added logging around auth handoff", CreatedBy: testUser, CreatedAt: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC), Sequence: 3, Kind: "progress"},
		{ID: "issue-immature-post-000002", IssueID: "issue-immature", Raw: "Maybe callback issue", CreatedBy: testUser, CreatedAt: time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC), Sequence: 2, Kind: "refinement"},
	} {
		if err := store.SaveIssuePost(context.Background(), post); err != nil {
			t.Fatalf("save post %s: %v", post.ID, err)
		}
	}

	raw1 := "Invited users land on a blank auth handoff screen."
	seq1 := 1
	if err := store.UpdateIssueFields(context.Background(), "issue-mature", issues.IssueFieldUpdate{
		Raw:                      &raw1,
		TagScores:                []issues.TagRelevance{{Tag: "auth", Relevance: 0.85}, {Tag: "onboarding", Relevance: 0.7}},
		Embedding:                []float64{1, 0},
		EnrichmentTargetSequence: &seq1,
	}); err != nil {
		t.Fatalf("seed mature snapshot 1: %v", err)
	}
	raw2 := "Invited users land on a blank auth handoff screen after completing sign in."
	seq2 := 2
	if err := store.UpdateIssueFields(context.Background(), "issue-mature", issues.IssueFieldUpdate{
		Raw:                      &raw2,
		TagScores:                []issues.TagRelevance{{Tag: "auth", Relevance: 0.86}, {Tag: "onboarding", Relevance: 0.72}},
		Embedding:                []float64{0.99, 0.01},
		EnrichmentTargetSequence: &seq2,
	}); err != nil {
		t.Fatalf("seed mature snapshot 2: %v", err)
	}
	raw3 := "auth issue"
	seq3 := 1
	if err := store.UpdateIssueFields(context.Background(), "issue-immature", issues.IssueFieldUpdate{
		Raw:                      &raw3,
		TagScores:                []issues.TagRelevance{{Tag: "auth", Relevance: 0.85}, {Tag: "onboarding", Relevance: 0.7}},
		Embedding:                []float64{1, 0},
		EnrichmentTargetSequence: &seq3,
	}); err != nil {
		t.Fatalf("seed immature snapshot 1: %v", err)
	}
	raw4 := "Actually maybe this is onboarding instead"
	seq4 := 2
	if err := store.UpdateIssueFields(context.Background(), "issue-immature", issues.IssueFieldUpdate{
		Raw:                      &raw4,
		TagScores:                []issues.TagRelevance{{Tag: "onboarding", Relevance: 0.9}},
		Embedding:                []float64{0, 1},
		EnrichmentTargetSequence: &seq4,
	}); err != nil {
		t.Fatalf("seed immature snapshot 2: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/people/Avery", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for person detail, got %d", rec.Code)
	}

	var payload people.PersonDetail
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode person detail response: %v", err)
	}
	if len(payload.RecommendedIssues) < 2 {
		t.Fatalf("expected at least 2 recommendations, got %#v", payload.RecommendedIssues)
	}
	if payload.RecommendedIssues[0].Issue.ID != "issue-mature" {
		t.Fatalf("expected mature issue to rank first, got %#v", payload.RecommendedIssues)
	}
}

func TestPersonDetailRecommendationsDeprioritizeHighVelocityIssue(t *testing.T) {
	makeIssue := func(
		id string,
		raw string,
		assignedTo string,
		status issues.IssueStatus,
		createdAt time.Time,
		tagScores []issues.TagRelevance,
		embedding []float64,
	) issues.Issue {
		issue := issues.BuildNewIssue(id, issues.CreateInput{
			Raw:       raw,
			CreatedBy: testUser,
			TagScores: tagScores,
			Embedding: embedding,
		})
		issue.AssignedTo = assignedTo
		issue.Status = status
		issue.CreatedAt = createdAt
		if len(issue.Discussion) > 0 {
			issue.Discussion[0].CreatedAt = createdAt
		}
		return issue
	}

	store := newPostgresIssueStore(t, []issues.Issue{
		makeIssue(
			"issue-assigned",
			"Fix auth redirect loop for invited users",
			testPerson,
			issues.StatusOpen,
			time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.9}, {Tag: "onboarding", Relevance: 0.6}},
			[]float64{1, 0},
		),
		makeIssue(
			"issue-active",
			"Invited users land on a blank auth handoff screen",
			"",
			issues.StatusOpen,
			time.Now().UTC().Add(-24*time.Hour),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.85}, {Tag: "onboarding", Relevance: 0.7}},
			[]float64{0.98, 0.02},
		),
		makeIssue(
			"issue-quiet",
			"Invited users land on a blank auth handoff screen",
			"",
			issues.StatusOpen,
			time.Now().UTC().Add(-24*time.Hour),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.85}, {Tag: "onboarding", Relevance: 0.7}},
			[]float64{0.98, 0.02},
		),
	})

	for _, post := range []issues.IssuePost{
		{ID: "issue-active-post-000002", IssueID: "issue-active", Raw: "Add exact repro steps", CreatedBy: testUser, CreatedAt: time.Now().UTC().Add(-6 * time.Hour), Sequence: 2, Kind: "refinement"},
		{ID: "issue-active-post-000003", IssueID: "issue-active", Raw: "Added more auth handoff logging", CreatedBy: testUser, CreatedAt: time.Now().UTC().Add(-3 * time.Hour), Sequence: 3, Kind: "progress"},
	} {
		if err := store.SaveIssuePost(context.Background(), post); err != nil {
			t.Fatalf("save post %s: %v", post.ID, err)
		}
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/people/Avery", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for person detail, got %d", rec.Code)
	}

	var payload people.PersonDetail
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode person detail response: %v", err)
	}
	if len(payload.RecommendedIssues) < 2 {
		t.Fatalf("expected at least 2 recommended issues, got %#v", payload.RecommendedIssues)
	}
	if payload.RecommendedIssues[0].Issue.ID != "issue-quiet" {
		t.Fatalf("expected quieter issue to rank first, got %#v", payload.RecommendedIssues)
	}
}

func TestPersonDetailRecommendationsPreferFreshIssueWhenBaseMatchIsEqual(t *testing.T) {
	now := time.Now().UTC()
	makeIssue := func(
		id string,
		raw string,
		assignedTo string,
		status issues.IssueStatus,
		createdAt time.Time,
		tagScores []issues.TagRelevance,
		embedding []float64,
	) issues.Issue {
		issue := issues.BuildNewIssue(id, issues.CreateInput{
			Raw:       raw,
			CreatedBy: testUser,
			TagScores: tagScores,
			Embedding: embedding,
		})
		issue.AssignedTo = assignedTo
		issue.Status = status
		issue.CreatedAt = createdAt
		if len(issue.Discussion) > 0 {
			issue.Discussion[0].CreatedAt = createdAt
		}
		return issue
	}

	store := newPostgresIssueStore(t, []issues.Issue{
		makeIssue(
			"issue-assigned",
			"Fix auth redirect loop for invited users",
			testPerson,
			issues.StatusOpen,
			now.Add(-10*24*time.Hour),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.9}, {Tag: "onboarding", Relevance: 0.6}},
			[]float64{1, 0},
		),
		makeIssue(
			"issue-stale",
			"Invited users land on a blank auth handoff screen",
			"",
			issues.StatusOpen,
			now.Add(-320*24*time.Hour),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.85}, {Tag: "onboarding", Relevance: 0.7}},
			[]float64{0.98, 0.02},
		),
		makeIssue(
			"issue-fresh",
			"Invited users land on a blank auth handoff screen",
			"",
			issues.StatusOpen,
			now.Add(-24*time.Hour),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.85}, {Tag: "onboarding", Relevance: 0.7}},
			[]float64{0.98, 0.02},
		),
	})

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/people/Avery", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for person detail, got %d", rec.Code)
	}

	var payload people.PersonDetail
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode person detail response: %v", err)
	}
	if len(payload.RecommendedIssues) < 2 {
		t.Fatalf("expected at least 2 recommended issues, got %#v", payload.RecommendedIssues)
	}
	if payload.RecommendedIssues[0].Issue.ID != "issue-fresh" {
		t.Fatalf("expected fresher issue first, got %#v", payload.RecommendedIssues)
	}
}
