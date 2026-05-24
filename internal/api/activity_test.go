package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	issueviews "sortit/internal/issues/views"
)

func TestRevisionEndpointAdvancesAfterMutation(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})
	handler := server.Handler()

	beforeReq := httptest.NewRequest(http.MethodGet, "/api/ui/revision", nil)
	beforeRec := httptest.NewRecorder()
	handler.ServeHTTP(beforeRec, beforeReq)

	if beforeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for revision, got %d", beforeRec.Code)
	}

	var before revisionResponse
	if err := json.NewDecoder(beforeRec.Body).Decode(&before); err != nil {
		t.Fatalf("decode revision response: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/issues", bytes.NewBufferString(`{"raw":"new issue"}`))
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for create, got %d", createRec.Code)
	}

	afterReq := httptest.NewRequest(http.MethodGet, "/api/ui/revision", nil)
	afterRec := httptest.NewRecorder()
	handler.ServeHTTP(afterRec, afterReq)

	if afterRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for revision after create, got %d", afterRec.Code)
	}

	var after revisionResponse
	if err := json.NewDecoder(afterRec.Body).Decode(&after); err != nil {
		t.Fatalf("decode revision response after create: %v", err)
	}

	if after.Revision <= before.Revision {
		t.Fatalf("expected revision to advance, before=%d after=%d", before.Revision, after.Revision)
	}
}

func TestActivityEndpointIncludesOperationalEvents(t *testing.T) {
	store := newPostgresIssueStore(t, nil)
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  store,
	})
	handler := server.Handler()

	createReq := httptest.NewRequest(http.MethodPost, "/api/issues", bytes.NewBufferString(`{"raw":"new issue"}`))
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for create, got %d", createRec.Code)
	}

	var created map[string]any
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created issue: %v", err)
	}
	issueID, _ := created["id"].(string)
	if issueID == "" {
		t.Fatal("expected created issue id")
	}

	closeReq := httptest.NewRequest(http.MethodPost, "/api/issues/"+issueID+"/close", bytes.NewBufferString(`{}`))
	closeRec := httptest.NewRecorder()
	handler.ServeHTTP(closeRec, closeReq)
	if closeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for close, got %d", closeRec.Code)
	}

	activityReq := httptest.NewRequest(http.MethodGet, "/api/ui/activity?limit=10", nil)
	activityRec := httptest.NewRecorder()
	handler.ServeHTTP(activityRec, activityReq)
	if activityRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for activity, got %d", activityRec.Code)
	}

	var activity issueviews.ActivityResponse
	if err := json.NewDecoder(activityRec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity response: %v", err)
	}
	if len(activity.Events) == 0 {
		t.Fatal("expected activity events")
	}

	foundClosed := false
	for _, event := range activity.Events {
		if event.Kind == "closed" && event.EntityType == "issue" && event.EntityID == issueID {
			foundClosed = true
			break
		}
	}
	if !foundClosed {
		t.Fatalf("expected closed activity event for %s", issueID)
	}
}
