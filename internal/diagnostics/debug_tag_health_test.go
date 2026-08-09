package diagnostics

import (
	"context"
	"testing"

	"sortit/internal/domain"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

func ptrFloat(v float64) *float64 { return &v }

// splitDriftTags ranks by |zDelta| where present and gates on both the raw
// floor and the z-threshold, falling back to the raw-delta rule for rows with no
// valid z-score. This exercises: (a) the z-rule flipping raw order, (b) a
// z-scored row below the threshold being dropped even though it clears the raw
// floor, (c) a raw-only row surviving and ranking behind z-scored rows, and (d)
// the payload carrying both delta and zDelta.
func TestSplitDriftTagsCombinedRule(t *testing.T) {
	rows := []issuemath.TagDrift{
		// Spurious (anchored, Δ<0). "highvar" has the larger raw |Δ| but the
		// smaller |z|; "genuine" wins on the z-rule → must rank first.
		{Tag: "highvar", Anchored: true, Delta: -0.9, ZDelta: ptrFloat(-2.5)},
		{Tag: "genuine", Anchored: true, Delta: -0.5, ZDelta: ptrFloat(-5.0)},
		// z present but |z| < 2.0 → dropped despite clearing the raw floor.
		{Tag: "lowz", Anchored: true, Delta: -0.4, ZDelta: ptrFloat(-1.0)},
		// No valid z-score → raw-delta fallback → survives, ranks after z rows.
		{Tag: "rawonly", Anchored: true, Delta: -0.6, ZDelta: nil},
		// Below the raw floor → dropped regardless of z.
		{Tag: "tiny", Anchored: true, Delta: -0.1, ZDelta: ptrFloat(-9.0)},
		// Missing (unanchored, Δ>0), z-scored above threshold.
		{Tag: "missing", Anchored: false, Delta: 0.7, ZDelta: ptrFloat(3.1)},
	}

	spurious, missing := splitDriftTags(rows)

	gotOrder := make([]string, len(spurious))
	for i, s := range spurious {
		gotOrder[i] = s.Tag
	}
	wantOrder := []string{"genuine", "highvar", "rawonly"}
	if len(gotOrder) != len(wantOrder) {
		t.Fatalf("spurious tags = %v, want %v", gotOrder, wantOrder)
	}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("spurious order = %v, want %v (z-rule must flip raw order and rank raw-only last)", gotOrder, wantOrder)
		}
	}

	// Payload carries both delta and the standardized zDelta.
	if spurious[0].ZDelta == nil || *spurious[0].ZDelta != -5.0 {
		t.Errorf("genuine zDelta = %v, want -5.0 in payload", spurious[0].ZDelta)
	}
	if spurious[0].Delta != -0.5 {
		t.Errorf("genuine delta = %f, want -0.5 in payload", spurious[0].Delta)
	}
	// Raw-only candidate keeps a nil zDelta.
	if spurious[2].Tag != "rawonly" || spurious[2].ZDelta != nil {
		t.Errorf("rawonly should carry nil zDelta, got %+v", spurious[2])
	}

	if len(missing) != 1 || missing[0].Tag != "missing" {
		t.Fatalf("missing tags = %+v, want [missing]", missing)
	}
	if missing[0].ZDelta == nil || *missing[0].ZDelta != 3.1 {
		t.Errorf("missing zDelta = %v, want 3.1", missing[0].ZDelta)
	}
}

// With a sub-MinCenteringVectors corpus (2 tags, 3 embeddings) centering is a
// no-op, so the orthonormal alpha/beta geometry yields the closed-form drift
// asserted here. Centering/RidgeLambda are nil, so the handler uses the fixed
// fallback penalties (λ_scored=0.5, λ_unscored=0.05).
func TestDebugTagHealthFlagsMisTagsOpenOnly(t *testing.T) {
	ctx := context.Background()
	store := issues.NewInMemoryStore([]issues.Issue{
		// Tagged alpha, embedding points at beta → alpha spurious, beta missing.
		{ID: "mistagged", Raw: "mis", Status: issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
			Embedding: unitVec64([]float64{0, 1, 0, 0})},
		// Tagged alpha, embedding agrees → low drift, not flagged.
		{ID: "welltagged", Raw: "well", Status: issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
			Embedding: unitVec64([]float64{1, 0, 0, 0})},
		// Same mis-tag as "mistagged" but closed → must be excluded.
		{ID: "mistagged-closed", Raw: "closed", Status: issues.StatusClosed,
			TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
			Embedding: unitVec64([]float64{0, 1, 0, 0})},
	})
	tagStore := &debugTagStore{}
	catalog := tags.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, []issues.Tag{
		{Name: "alpha", Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unitVec64([]float64{0, 1, 0, 0})},
	}); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	result, err := (DebugTagHealthHandler{Store: store, Catalog: catalog}).Handle(ctx)
	if err != nil {
		t.Fatalf("handle tag health: %v", err)
	}
	if !result.Computed {
		t.Fatal("expected drift to be computed")
	}

	byID := make(map[string]DebugDriftIssue, len(result.HighDriftIssues))
	for _, d := range result.HighDriftIssues {
		byID[d.ID] = d
	}
	if _, ok := byID["welltagged"]; ok {
		t.Error("welltagged (DriftCosine ≈ 1.0) must not be flagged")
	}
	if _, ok := byID["mistagged-closed"]; ok {
		t.Error("closed issues must be excluded from the drift list")
	}

	mis, ok := byID["mistagged"]
	if !ok {
		t.Fatalf("expected mistagged issue flagged, got %+v", result.HighDriftIssues)
	}
	if mis.DriftCosine >= DefaultDriftCosineThreshold {
		t.Errorf("mistagged DriftCosine = %f, want < %f", mis.DriftCosine, DefaultDriftCosineThreshold)
	}
	if len(mis.SpuriousTags) == 0 || mis.SpuriousTags[0].Tag != "alpha" || mis.SpuriousTags[0].Delta >= 0 {
		t.Errorf("expected alpha as spurious (Δ<0), got %+v", mis.SpuriousTags)
	}
	if len(mis.MissingTags) == 0 || mis.MissingTags[0].Tag != "beta" || mis.MissingTags[0].Delta <= 0 {
		t.Errorf("expected beta as missing (Δ>0), got %+v", mis.MissingTags)
	}
}

func TestDebugTagHealthDeterministicOrder(t *testing.T) {
	ctx := context.Background()
	// Several open issues with identical mis-tag geometry tie at the same
	// DriftCosine; the result must be ID-ordered and identical across calls.
	ids := []string{"mis-3", "mis-1", "mis-4", "mis-2"}
	seed := make([]issues.Issue, 0, len(ids))
	for _, id := range ids {
		seed = append(seed, issues.Issue{
			ID: id, Raw: id, Status: issues.StatusOpen,
			TagScores: []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}},
			Embedding: unitVec64([]float64{0, 1, 0, 0}),
		})
	}
	store := issues.NewInMemoryStore(seed)
	tagStore := &debugTagStore{}
	catalog := tags.NewCatalogService(tagStore, nil, nil)
	if err := catalog.EnsureStoredTags(ctx, []issues.Tag{
		{Name: "alpha", Embedding: unitVec64([]float64{1, 0, 0, 0})},
		{Name: "beta", Embedding: unitVec64([]float64{0, 1, 0, 0})},
	}); err != nil {
		t.Fatalf("ensure stored tags: %v", err)
	}

	handler := DebugTagHealthHandler{Store: store, Catalog: catalog}
	want := []string{"mis-1", "mis-2", "mis-3", "mis-4"}
	var first []string
	for call := range 4 {
		result, err := handler.Handle(ctx)
		if err != nil {
			t.Fatalf("handle tag health: %v", err)
		}
		got := make([]string, len(result.HighDriftIssues))
		for i, d := range result.HighDriftIssues {
			got[i] = d.ID
		}
		if call == 0 {
			first = got
			if len(got) != len(want) {
				t.Fatalf("drift IDs = %v, want %v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("drift order = %v, want %v", got, want)
				}
			}
			continue
		}
		if len(got) != len(first) {
			t.Fatalf("call %d IDs = %v, first = %v", call, got, first)
		}
		for i := range first {
			if got[i] != first[i] {
				t.Fatalf("call %d order = %v, first = %v", call, got, first)
			}
		}
	}
}
