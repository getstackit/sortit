package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"sortit/internal/diagnostics"
	"sortit/internal/issues"
	"sortit/internal/themes"
)

// themesFixtureIssues builds n issues split between two anchored, embedded
// tag clusters — enough (with n >= themes.minThemeParticipants, currently 16)
// to clear the theme participation floor end to end through the real server.
func themesFixtureIssues(n int) []issues.Issue {
	base := issues.FixtureIssues()
	items := make([]issues.Issue, 0, n)
	for i := range n {
		tag := "alpha"
		emb := []float64{1, 0.05 * float64(i%3)}
		if i%2 == 1 {
			tag = "beta"
			emb = []float64{0.05 * float64(i%3), 1}
		}
		items = append(items, issues.Issue{
			ID:        fmt.Sprintf("theme-issue-%02d", i),
			Raw:       fmt.Sprintf("issue %d about %s", i, tag),
			Status:    issues.StatusOpen,
			CreatedBy: "Casey",
			CreatedAt: base[i%len(base)].CreatedAt,
			TagScores: []issues.TagRelevance{{Tag: tag, Relevance: 0.9}},
			Embedding: emb,
		})
	}
	return items
}

// TestDebugThemesEndpointsEndToEnd exercises the three WP-204 endpoints
// through the real chi router (registered in both the /api/ui and the
// dedicated /api/{prefix} route groups), not just the handler in isolation —
// confirming the route registration in internal/api/server.go actually wires
// up.
func TestDebugThemesEndpointsEndToEnd(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	if err := store.Replace(context.Background(), themesFixtureIssues(24)); err != nil {
		t.Fatalf("replace issues: %v", err)
	}
	if err := store.UpsertTags(context.Background(), []issues.Tag{
		{Name: "alpha", Embedding: []float64{1, 0}},
		{Name: "beta", Embedding: []float64{0, 1}},
	}); err != nil {
		t.Fatalf("upsert tags: %v", err)
	}

	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})
	handler := server.Handler()

	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/ui/debug/themes", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var list diagnostics.DebugThemesListResult
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if !list.Computed || len(list.Themes) == 0 {
		t.Fatalf("expected computed themes, got %+v", list)
	}
	stableID := list.Themes[0].StableID

	// Also reachable via the dedicated API prefix, not just /api/ui.
	dedicatedListRec := httptest.NewRecorder()
	handler.ServeHTTP(dedicatedListRec, httptest.NewRequest(http.MethodGet, "/api/debug/themes", nil))
	if dedicatedListRec.Code != http.StatusOK {
		t.Fatalf("dedicated list: expected 200, got %d: %s", dedicatedListRec.Code, dedicatedListRec.Body.String())
	}

	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, httptest.NewRequest(http.MethodGet, "/api/ui/debug/themes/"+stableID, nil))
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail: expected 200, got %d: %s", detailRec.Code, detailRec.Body.String())
	}
	var detail diagnostics.DebugThemeDetailResult
	if err := json.NewDecoder(detailRec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.StableID != stableID {
		t.Fatalf("expected stable id %q, got %q", stableID, detail.StableID)
	}
	if len(detail.CentroidTopIssues) == 0 {
		t.Fatal("expected centroid-top issues in the detail response")
	}
	if len(detail.TopIssuesByWeight) == 0 {
		t.Fatal("expected top-issues-by-weight in the detail response")
	}

	missingDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(missingDetailRec, httptest.NewRequest(http.MethodGet, "/api/ui/debug/themes/does-not-exist", nil))
	if missingDetailRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown theme, got %d", missingDetailRec.Code)
	}

	issueRec := httptest.NewRecorder()
	handler.ServeHTTP(issueRec, httptest.NewRequest(http.MethodGet, "/api/ui/debug/issues/theme-issue-00/themes", nil))
	if issueRec.Code != http.StatusOK {
		t.Fatalf("issue-themes: expected 200, got %d: %s", issueRec.Code, issueRec.Body.String())
	}
	var issueThemes diagnostics.DebugIssueThemesResult
	if err := json.NewDecoder(issueRec.Body).Decode(&issueThemes); err != nil {
		t.Fatalf("decode issue-themes: %v", err)
	}
	if issueThemes.Participation != string(themes.ParticipationParticipating) {
		t.Fatalf("expected participating, got %q", issueThemes.Participation)
	}
	if len(issueThemes.Themes) == 0 {
		t.Fatal("expected a non-empty theme distribution")
	}

	missingIssueRec := httptest.NewRecorder()
	handler.ServeHTTP(missingIssueRec, httptest.NewRequest(http.MethodGet, "/api/ui/debug/issues/does-not-exist/themes", nil))
	if missingIssueRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown issue, got %d", missingIssueRec.Code)
	}
}
