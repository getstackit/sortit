package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sortit/internal/issues"
)

func TestMineCommandListsIssuesAssignedToCurrentUser(t *testing.T) {
	t.Parallel()

	var gotAssignedTo string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/session":
			if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
				t.Fatalf("authorization header = %q", auth)
			}
			_ = json.NewEncoder(w).Encode(authSessionResponse{
				User: SessionUser{
					Login:       "testuser",
					DisplayName: "Test User",
				},
			})
		case "/issues":
			gotAssignedTo = r.URL.Query().Get("assignedTo")
			if status := r.URL.Query().Get("status"); status != "open" {
				t.Fatalf("status query = %q", status)
			}
			_ = json.NewEncoder(w).Encode(issuesListResponse{
				Issues: []issues.Issue{{
					ID:         "issue-123",
					Raw:        "Fix auth redirect loop",
					AssignedTo: "Test User",
					Status:     issues.StatusOpen,
					CreatedAt:  time.Unix(123, 0).UTC(),
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := NewRootCmd("dev", "test", "today")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--api-url", server.URL, "--token", "test-token", "mine"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotAssignedTo != "Test User" {
		t.Fatalf("assignedTo query = %q", gotAssignedTo)
	}

	var response issuesListResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	if len(response.Issues) != 1 || response.Issues[0].ID != "issue-123" {
		t.Fatalf("unexpected issues response: %+v", response.Issues)
	}
}

func TestMineCommandFallsBackToLoginWhenDisplayNameMissing(t *testing.T) {
	t.Parallel()

	var gotAssignedTo string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/session":
			_ = json.NewEncoder(w).Encode(authSessionResponse{
				User: SessionUser{Login: "testuser"},
			})
		case "/issues":
			gotAssignedTo = r.URL.Query().Get("assignedTo")
			_ = json.NewEncoder(w).Encode(issuesListResponse{Issues: []issues.Issue{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := NewRootCmd("dev", "test", "today")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--api-url", server.URL, "--token", "test-token", "mine"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotAssignedTo != "testuser" {
		t.Fatalf("assignedTo query = %q", gotAssignedTo)
	}
}
