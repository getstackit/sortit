package themes

import (
	"context"
	"testing"

	"sortit/internal/issuethemes"
)

func TestBuildTelemetry(t *testing.T) {
	res := &Result{
		Result: issuethemes.Result{Iterations: 37, ReconstructionError: 0.42},
		Identities: []ThemeIdentity{
			{StableID: "theme-1", MatchScore: 0.9},
			{StableID: "theme-2", MatchScore: 0.7},
			{StableID: "theme-3", MatchScore: 0}, // minted this refresh
		},
		MintedIDs:  []string{"theme-3"},
		RetiredIDs: []string{"theme-0"},
	}

	got := buildTelemetry(res)
	if got.Iterations != 37 || got.ReconstructionError != 0.42 {
		t.Fatalf("convergence fields not carried: %+v", got)
	}
	if got.Minted != 1 || got.Retired != 1 {
		t.Fatalf("churn counts wrong: %+v", got)
	}
	// Mean over inherited themes only: (0.9 + 0.7) / 2.
	if diff := got.MeanMatchScore - 0.8; diff > 1e-12 || diff < -1e-12 {
		t.Fatalf("mean match score = %v, want 0.8", got.MeanMatchScore)
	}
}

func TestBuildTelemetryAllMinted(t *testing.T) {
	res := &Result{
		Identities: []ThemeIdentity{{StableID: "theme-1"}, {StableID: "theme-2"}},
		MintedIDs:  []string{"theme-1", "theme-2"},
	}
	if got := buildTelemetry(res); got.MeanMatchScore != 0 {
		t.Fatalf("all-minted refresh must report zero mean match score, got %v", got.MeanMatchScore)
	}
}

func TestTelemetryOnComputeAndRefresh(t *testing.T) {
	const groups, perGroup = 6, 8
	c, store, _, rev := mutableCache(groupedIssues(groups, perGroup), groupTags(groups))

	res, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("initial compute: ok=%v err=%v", ok, err)
	}
	tel := res.Telemetry
	if tel.Iterations <= 0 {
		t.Fatalf("iterations not populated: %+v", tel)
	}
	if tel.ReconstructionError <= 0 || tel.ReconstructionError >= 1 {
		t.Fatalf("reconstruction error outside (0,1): %+v", tel)
	}
	if tel.Minted != groups || tel.Retired != 0 || tel.MeanMatchScore != 0 {
		t.Fatalf("first refresh mints everything with no inherited scores: %+v", tel)
	}

	tweakGroupIssue(store, 0)
	rev.rev.Add(1)
	res2, ok, err := c.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("refresh: ok=%v err=%v", ok, err)
	}
	tel2 := res2.Telemetry
	if tel2.Minted != 0 || tel2.Retired != 0 {
		t.Fatalf("small mutation churned identity: %+v", tel2)
	}
	if tel2.MeanMatchScore < 0.9 {
		t.Fatalf("inherited themes on a near-identical corpus should match ~1.0, got %v", tel2.MeanMatchScore)
	}
}
