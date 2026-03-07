package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"bored/internal/issues"
)

func TestIssuesEndpointListsSeededIssues(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  newSQLiteIssueStore(t, issues.SeedIssues()),
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

	if len(payload.Issues) != len(issues.SeedIssues()) {
		t.Fatalf("expected %d seeded issues, got %d", len(issues.SeedIssues()), len(payload.Issues))
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
		IssueStore:  newSQLiteIssueStore(t, issues.SeedIssues()),
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
}

func TestIssuesEndpointReturnsNotFoundForMissingIssue(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  newSQLiteIssueStore(t, issues.SeedIssues()),
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
	store := newSQLiteIssueStore(t, nil)
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
	if len(created.Tags) != 1 || created.Tags[0] != "bug" {
		t.Fatalf("expected sanitized tags, got %#v", created.Tags)
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
