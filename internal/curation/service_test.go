package curation

import (
	"context"
	"errors"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
	"sortit/internal/issues/commands"
)

// fakeDispatcher records the inputs it receives and returns canned results so
// the service's routing, validation, and lifecycle logic can be tested without
// the full handler/UnitOfWork wiring.
type fakeDispatcher struct {
	combineIn  *commands.CombineIssues
	closeIn    *commands.CloseIssue
	reenrichIn *commands.ReEnrichIssue
	supersede  [2]string
	archiveID  string

	combineResult  issues.IssueOperationResult
	closeResult    issues.Issue
	reenrichResult issues.Issue
	memResult      domain.Memory
	err            error
}

func (d *fakeDispatcher) Combine(_ context.Context, in commands.CombineIssues) (issues.IssueOperationResult, error) {
	d.combineIn = &in
	return d.combineResult, d.err
}

func (d *fakeDispatcher) Close(_ context.Context, in commands.CloseIssue) (issues.Issue, error) {
	d.closeIn = &in
	return d.closeResult, d.err
}

func (d *fakeDispatcher) Reenrich(_ context.Context, in commands.ReEnrichIssue) (issues.Issue, error) {
	d.reenrichIn = &in
	return d.reenrichResult, d.err
}

func (d *fakeDispatcher) SupersedeMemory(_ context.Context, id, supersededBy string) (domain.Memory, error) {
	d.supersede = [2]string{id, supersededBy}
	return d.memResult, d.err
}

func (d *fakeDispatcher) ArchiveMemory(_ context.Context, id string) (domain.Memory, error) {
	d.archiveID = id
	return d.memResult, d.err
}

func newTestService(d Dispatcher) *Service {
	return NewService(issues.NewInMemoryStore(nil), d, nil)
}

func TestCreateProposalValidatesPayload(t *testing.T) {
	t.Parallel()
	svc := newTestService(&fakeDispatcher{})
	ctx := context.Background()

	cases := []struct {
		name    string
		input   CreateProposalInput
		wantErr error
	}{
		{
			name:    "unknown kind",
			input:   CreateProposalInput{Kind: "frobnicate", Payload: domain.CurationPayload{IssueID: "a"}},
			wantErr: ErrUnknownKind,
		},
		{
			name:    "combine needs two issues",
			input:   CreateProposalInput{Kind: domain.CurationKindCombineIssues, Payload: domain.CurationPayload{IssueIDs: []string{"a"}}},
			wantErr: ErrInvalidPayload,
		},
		{
			name:    "close needs issue",
			input:   CreateProposalInput{Kind: domain.CurationKindCloseStale, Payload: domain.CurationPayload{}},
			wantErr: ErrInvalidPayload,
		},
		{
			name:    "archive needs memory",
			input:   CreateProposalInput{Kind: domain.CurationKindArchiveMemory, Payload: domain.CurationPayload{}},
			wantErr: ErrInvalidPayload,
		},
		{
			name:    "supersede cannot self-reference",
			input:   CreateProposalInput{Kind: domain.CurationKindSupersedeMemory, Payload: domain.CurationPayload{MemoryID: "m1", ReplacementID: "m1"}},
			wantErr: ErrInvalidPayload,
		},
		{
			name:  "valid combine",
			input: CreateProposalInput{Kind: domain.CurationKindCombineIssues, Payload: domain.CurationPayload{IssueIDs: []string{"a", "b"}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			created, err := svc.CreateProposal(ctx, tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if created.Status != domain.CurationProposalStatusPending {
				t.Fatalf("expected pending status, got %q", created.Status)
			}
			if created.ID == "" {
				t.Fatal("expected an id to be assigned")
			}
		})
	}
}

func TestAcceptDispatchesByKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("combine_issues", func(t *testing.T) {
		t.Parallel()
		d := &fakeDispatcher{combineResult: issues.IssueOperationResult{CreatedIssues: []issues.Issue{{ID: "combined-1"}}}}
		svc := newTestService(d)
		p, err := svc.CreateProposal(ctx, CreateProposalInput{
			Kind:    domain.CurationKindCombineIssues,
			Payload: domain.CurationPayload{IssueIDs: []string{"a", "b"}},
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		accepted, err := svc.AcceptProposal(ctx, p.ID, "reviewer")
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		if d.combineIn == nil || len(d.combineIn.SourceIDs) != 2 {
			t.Fatalf("expected combine dispatched with 2 sources, got %+v", d.combineIn)
		}
		if accepted.ResultRef != "combined-1" {
			t.Fatalf("expected result ref combined-1, got %q", accepted.ResultRef)
		}
		if accepted.Status != domain.CurationProposalStatusAccepted || accepted.ReviewedBy != "reviewer" {
			t.Fatalf("unexpected accepted state: %+v", accepted)
		}
	})

	t.Run("close_stale defaults reason", func(t *testing.T) {
		t.Parallel()
		d := &fakeDispatcher{closeResult: issues.Issue{ID: "issue-9"}}
		svc := newTestService(d)
		p, err := svc.CreateProposal(ctx, CreateProposalInput{
			Kind:    domain.CurationKindCloseStale,
			Payload: domain.CurationPayload{IssueID: "issue-9"},
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := svc.AcceptProposal(ctx, p.ID, "reviewer"); err != nil {
			t.Fatalf("accept: %v", err)
		}
		if d.closeIn == nil || d.closeIn.Reason != defaultCloseReason || d.closeIn.ID != "issue-9" {
			t.Fatalf("expected close dispatched with reason stale, got %+v", d.closeIn)
		}
	})

	t.Run("reenrich_issue", func(t *testing.T) {
		t.Parallel()
		d := &fakeDispatcher{reenrichResult: issues.Issue{ID: "issue-3"}}
		svc := newTestService(d)
		p, _ := svc.CreateProposal(ctx, CreateProposalInput{
			Kind:    domain.CurationKindReenrichIssue,
			Payload: domain.CurationPayload{IssueID: "issue-3"},
		})
		accepted, err := svc.AcceptProposal(ctx, p.ID, "reviewer")
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		if d.reenrichIn == nil || d.reenrichIn.ID != "issue-3" {
			t.Fatalf("expected reenrich dispatched for issue-3, got %+v", d.reenrichIn)
		}
		if accepted.ResultRef != "issue-3" {
			t.Fatalf("expected result ref issue-3, got %q", accepted.ResultRef)
		}
	})

	t.Run("archive_memory", func(t *testing.T) {
		t.Parallel()
		d := &fakeDispatcher{memResult: domain.Memory{ID: "mem-1"}}
		svc := newTestService(d)
		p, _ := svc.CreateProposal(ctx, CreateProposalInput{
			Kind:    domain.CurationKindArchiveMemory,
			Payload: domain.CurationPayload{MemoryID: "mem-1"},
		})
		if _, err := svc.AcceptProposal(ctx, p.ID, "reviewer"); err != nil {
			t.Fatalf("accept: %v", err)
		}
		if d.archiveID != "mem-1" {
			t.Fatalf("expected archive of mem-1, got %q", d.archiveID)
		}
	})

	t.Run("supersede_memory", func(t *testing.T) {
		t.Parallel()
		d := &fakeDispatcher{memResult: domain.Memory{ID: "mem-old"}}
		svc := newTestService(d)
		p, _ := svc.CreateProposal(ctx, CreateProposalInput{
			Kind:    domain.CurationKindSupersedeMemory,
			Payload: domain.CurationPayload{MemoryID: "mem-old", ReplacementID: "mem-new"},
		})
		if _, err := svc.AcceptProposal(ctx, p.ID, "reviewer"); err != nil {
			t.Fatalf("accept: %v", err)
		}
		if d.supersede != [2]string{"mem-old", "mem-new"} {
			t.Fatalf("expected supersede(mem-old, mem-new), got %v", d.supersede)
		}
	})
}

func TestAcceptLeavesProposalPendingOnDispatchError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := &fakeDispatcher{err: errors.New("boom")}
	svc := newTestService(d)
	p, _ := svc.CreateProposal(ctx, CreateProposalInput{
		Kind:    domain.CurationKindReenrichIssue,
		Payload: domain.CurationPayload{IssueID: "issue-3"},
	})
	if _, err := svc.AcceptProposal(ctx, p.ID, "reviewer"); err == nil {
		t.Fatal("expected accept to fail when dispatch errors")
	}
	got, err := svc.GetProposal(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != domain.CurationProposalStatusPending {
		t.Fatalf("expected proposal to remain pending after dispatch error, got %q", got.Status)
	}
}

func TestRejectAndNotPendingGuards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := &fakeDispatcher{reenrichResult: issues.Issue{ID: "issue-3"}}
	svc := newTestService(d)
	p, _ := svc.CreateProposal(ctx, CreateProposalInput{
		Kind:    domain.CurationKindReenrichIssue,
		Payload: domain.CurationPayload{IssueID: "issue-3"},
	})

	rejected, err := svc.RejectProposal(ctx, p.ID, "reviewer")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Status != domain.CurationProposalStatusRejected || rejected.ReviewedBy != "reviewer" {
		t.Fatalf("unexpected rejected state: %+v", rejected)
	}

	if _, err := svc.AcceptProposal(ctx, p.ID, "reviewer"); !errors.Is(err, ErrProposalNotPending) {
		t.Fatalf("expected ErrProposalNotPending accepting a rejected proposal, got %v", err)
	}
	if _, err := svc.RejectProposal(ctx, p.ID, "reviewer"); !errors.Is(err, ErrProposalNotPending) {
		t.Fatalf("expected ErrProposalNotPending re-rejecting, got %v", err)
	}
}
