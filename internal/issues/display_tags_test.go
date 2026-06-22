package issues

import "testing"

func TestDisplayTagsUsesEffectiveRelevance(t *testing.T) {
	negated := 0.5
	tags := DisplayTags(nil, []TagRelevance{
		{Tag: "backend", Relevance: 0.45, Negation: &negated},
		{Tag: "signed-relevance", Relevance: 0.4},
	})

	if len(tags) != 1 || tags[0] != "signed-relevance" {
		t.Fatalf("expected negated tag to be skipped, got %#v", tags)
	}
}

func TestDisplayTagsWithSpecificityRanksByEffectiveRelevance(t *testing.T) {
	negated := 0.85
	backendSpecificity := 1.0
	signedSpecificity := 0.0
	tags := DisplayTagsWithSpecificity(nil, []TagRelevance{
		{Tag: "backend", Relevance: 0.9, Negation: &negated},
		{Tag: "signed-relevance", Relevance: 0.65},
	}, map[string]*float64{
		"backend":          &backendSpecificity,
		"signed-relevance": &signedSpecificity,
	})

	if len(tags) == 0 || tags[0] != "signed-relevance" {
		t.Fatalf("expected display ranking to use net signed relevance, got %#v", tags)
	}
}

func TestDisplayTagsReturnsEmptyForFullyNegatedScores(t *testing.T) {
	negated := 0.6
	tags := DisplayTags(nil, []TagRelevance{
		{Tag: "backend", Relevance: 0.4, Negation: &negated},
	})

	if len(tags) != 0 {
		t.Fatalf("expected fully negated tag set to produce no display tags, got %#v", tags)
	}
}
