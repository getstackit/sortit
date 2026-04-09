package issueanalytics

import (
	"testing"

	"sortit/internal/issues"
)

func TestCanonicalLinkAuthority(t *testing.T) {
	tests := []struct {
		name    string
		count   int
		wantMin float64
		wantMax float64
	}{
		{"zero links", 0, 0, 0},
		{"one link", 1, 0.25, 0.25},
		{"two links", 2, 0.5, 0.5},
		{"three links", 3, 0.75, 0.75},
		{"four links caps at 1", 4, 1.0, 1.0},
		{"seven links caps at 1", 7, 1.0, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanonicalLinkAuthority(tt.count)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CanonicalLinkAuthority(%d) = %v, want [%v, %v]", tt.count, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestIssueAuthority(t *testing.T) {
	t.Run("no links", func(t *testing.T) {
		issue := issues.Issue{}
		if a := IssueAuthority(issue); a != 0 {
			t.Errorf("IssueAuthority(no links) = %v, want 0", a)
		}
	})

	t.Run("only non-canonical links ignored", func(t *testing.T) {
		issue := issues.Issue{
			ID: "target",
			Links: []issues.IssueLink{
				{Type: issues.IssueLinkTypeRelatedTo, SourceIssueID: "a", TargetIssueID: "target"},
				{Type: issues.IssueLinkTypeParentOf, SourceIssueID: "b", TargetIssueID: "target"},
				{Type: issues.IssueLinkTypeDerivedFrom, SourceIssueID: "c", TargetIssueID: "target"},
				{Type: issues.IssueLinkTypeChildOf, SourceIssueID: "d", TargetIssueID: "target"},
			},
		}
		if a := IssueAuthority(issue); a != 0 {
			t.Errorf("IssueAuthority(non-canonical links) = %v, want 0", a)
		}
	})

	t.Run("counts inbound duplicate_of and merged_into", func(t *testing.T) {
		issue := issues.Issue{
			ID: "target",
			Links: []issues.IssueLink{
				{Type: issues.IssueLinkTypeDuplicateOf, SourceIssueID: "a", TargetIssueID: "target"},
				{Type: issues.IssueLinkTypeMergedInto, SourceIssueID: "b", TargetIssueID: "target"},
			},
		}
		if a := IssueAuthority(issue); a != 0.5 {
			t.Errorf("IssueAuthority(2 canonical links) = %v, want 0.5", a)
		}
	})

	t.Run("outbound canonical links ignored", func(t *testing.T) {
		issue := issues.Issue{
			ID: "source",
			Links: []issues.IssueLink{
				{Type: issues.IssueLinkTypeDuplicateOf, SourceIssueID: "source", TargetIssueID: "other"},
				{Type: issues.IssueLinkTypeMergedInto, SourceIssueID: "source", TargetIssueID: "other"},
			},
		}
		if a := IssueAuthority(issue); a != 0 {
			t.Errorf("IssueAuthority(outbound links) = %v, want 0", a)
		}
	})

	t.Run("mixed links counts only inbound canonical", func(t *testing.T) {
		issue := issues.Issue{
			ID: "target",
			Links: []issues.IssueLink{
				{Type: issues.IssueLinkTypeDuplicateOf, SourceIssueID: "a", TargetIssueID: "target"},
				{Type: issues.IssueLinkTypeRelatedTo, SourceIssueID: "b", TargetIssueID: "target"},
				{Type: issues.IssueLinkTypeMergedInto, SourceIssueID: "target", TargetIssueID: "c"}, // outbound, ignored
				{Type: issues.IssueLinkTypeDuplicateOf, SourceIssueID: "d", TargetIssueID: "target"},
			},
		}
		if a := IssueAuthority(issue); a != 0.5 {
			t.Errorf("IssueAuthority(mixed links) = %v, want 0.5", a)
		}
	})
}
