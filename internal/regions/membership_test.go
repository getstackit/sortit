package regions

import (
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

func TestBelongsToTagRegion(t *testing.T) {
	tests := []struct {
		name   string
		issue  issues.Issue
		key    domain.RegionKey
		expect bool
	}{
		{
			name: "relevance at floor is member",
			issue: issues.Issue{
				TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: MembershipFloor}},
			},
			key:    domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"},
			expect: true,
		},
		{
			name: "relevance below floor is not member",
			issue: issues.Issue{
				TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: MembershipFloor - 0.01}},
			},
			key:    domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"},
			expect: false,
		},
		{
			name: "negation-only synthetic row excluded",
			issue: issues.Issue{
				TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0, Negation: new(0.6)}},
			},
			key:    domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"},
			expect: false,
		},
		{
			name: "contested but above floor is member",
			issue: issues.Issue{
				TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.7, Negation: new(0.5)}},
			},
			key:    domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"},
			expect: true,
		},
		{
			name: "different tag",
			issue: issues.Issue{
				TagScores: []domain.TagRelevance{{Tag: "billing", Relevance: 0.9}},
			},
			key:    domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"},
			expect: false,
		},
		{
			name: "tag name normalized",
			issue: issues.Issue{
				TagScores: []domain.TagRelevance{{Tag: "Auth", Relevance: 0.8}},
			},
			key:    domain.RegionKey{Kind: domain.RegionKindTag, ID: "AUTH"},
			expect: true,
		},
		{
			name: "empty issue",
			issue: issues.Issue{
				TagScores: nil,
			},
			key:    domain.RegionKey{Kind: domain.RegionKindTag, ID: "auth"},
			expect: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BelongsTo(tc.issue, tc.key)
			if got != tc.expect {
				t.Fatalf("BelongsTo = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestBelongsToUnsupportedKinds(t *testing.T) {
	issue := issues.Issue{
		TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}},
	}
	for _, kind := range []domain.RegionKind{domain.RegionKindCluster, domain.RegionKindCustom, ""} {
		t.Run(string(kind), func(t *testing.T) {
			if BelongsTo(issue, domain.RegionKey{Kind: kind, ID: "auth"}) {
				t.Fatalf("BelongsTo returned true for unsupported kind %q", kind)
			}
		})
	}
}

func TestBelongsToEmptyTagID(t *testing.T) {
	issue := issues.Issue{
		TagScores: []domain.TagRelevance{{Tag: "auth", Relevance: 0.9}},
	}
	if BelongsTo(issue, domain.RegionKey{Kind: domain.RegionKindTag, ID: ""}) {
		t.Fatal("BelongsTo with empty ID should return false")
	}
}
