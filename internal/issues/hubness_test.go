package issues

import "testing"

func TestLinkCountHubness(t *testing.T) {
	tests := []struct {
		name      string
		linkCount int
		wantMin   float64
		wantMax   float64
	}{
		{"zero links", 0, 0, 0},
		{"one link", 1, 0.15, 0.15},
		{"three links", 3, 0.44, 0.46},
		{"five links", 5, 0.75, 0.75},
		{"seven links caps at 1", 7, 1.0, 1.0},
		{"ten links caps at 1", 10, 1.0, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinkCountHubness(tt.linkCount)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("LinkCountHubness(%d) = %v, want [%v, %v]", tt.linkCount, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestIssueHubness(t *testing.T) {
	issue := Issue{}
	if h := IssueHubness(issue); h != 0 {
		t.Errorf("IssueHubness(no links) = %v, want 0", h)
	}

	issue.Links = make([]IssueLink, 4)
	if h := IssueHubness(issue); h != 0.6 {
		t.Errorf("IssueHubness(4 links) = %v, want 0.6", h)
	}
}
