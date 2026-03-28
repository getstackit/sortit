package issueanalytics

import (
	"math"
	"testing"
	"time"

	"splat/internal/issues"
)

func TestTimeDecayWeightUsesTrueHalfLife(t *testing.T) {
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)
	timestamp := now.Add(-90 * 24 * time.Hour)

	got := TimeDecayWeight(timestamp, now, 0.3, 90)
	want := 0.3 + 0.7*0.5
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("TimeDecayWeight() = %v, want %v", got, want)
	}
}

func TestIssueFreshnessWeightPrefersRecentActivityOverCreationTime(t *testing.T) {
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)

	dormant := issues.Issue{
		ID:        "issue-dormant",
		CreatedAt: now.Add(-200 * 24 * time.Hour),
	}
	revived := issues.Issue{
		ID:        "issue-revived",
		CreatedAt: now.Add(-200 * 24 * time.Hour),
		Discussion: []issues.IssuePost{
			{Kind: "progress", CreatedAt: now.Add(-24 * time.Hour)},
		},
	}

	if IssueFreshnessWeight(revived, now) <= IssueFreshnessWeight(dormant, now) {
		t.Fatalf("expected revived issue to be fresher than dormant issue")
	}
}

func TestIssueFreshnessWeightReturnsOneForUnknownTimestamp(t *testing.T) {
	if got := IssueFreshnessWeight(issues.Issue{}, time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)); got != 1 {
		t.Fatalf("IssueFreshnessWeight(zero) = %v, want 1", got)
	}
}
