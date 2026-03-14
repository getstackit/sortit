package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"splat/internal/issues"
	"splat/internal/queries"
)

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
			CreatedBy: "Casey",
			TagScores: tagScores,
			Embedding: embedding,
		})
		issue.AssignedTo = assignedTo
		issue.Status = status
		issue.CreatedAt = createdAt
		if status == issues.StatusClosed {
			closedAt := createdAt.Add(2 * time.Hour)
			issue.ClosedAt = &closedAt
			issue.ClosedBy = "Casey"
		}
		return issue
	}

	seed := []issues.Issue{
		makeIssue(
			"issue-assigned-old",
			"Fix auth redirect loop for invited users",
			"Avery",
			issues.StatusOpen,
			time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "auth", Relevance: 0.9}, {Tag: "onboarding", Relevance: 0.6}},
			[]float64{1, 0},
		),
		makeIssue(
			"issue-assigned-new",
			"Tighten search filters on the team page",
			"Avery",
			issues.StatusOpen,
			time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
			[]issues.TagRelevance{{Tag: "search", Relevance: 0.8}, {Tag: "ui", Relevance: 0.4}},
			[]float64{0.7, 0.3},
		),
		makeIssue(
			"issue-assigned-closed",
			"Polish invite acceptance copy",
			"Avery",
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

	var payload queries.PersonDetail
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode person detail response: %v", err)
	}

	if payload.Person != "Avery" {
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
