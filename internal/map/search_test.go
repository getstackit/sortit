package issuemap

import (
	"testing"

	"splat/internal/issues"
)

func TestSearchTagsDeprioritizesGenericBucketTags(t *testing.T) {
	tags := []issues.Tag{
		{Name: "backend", Embedding: []float64{1, 0}},
		{Name: "billing", Embedding: []float64{0.98, 0.02}},
		{Name: "payments", Embedding: []float64{0.96, 0.04}},
	}

	results := SearchTags(tags, []float64{1, 0}, 3)
	if len(results) != 3 {
		t.Fatalf("expected 3 related tags, got %d", len(results))
	}

	if results[0].Name != "billing" {
		t.Fatalf("expected specific tag to rank first, got %q", results[0].Name)
	}
	if results[2].Name != "backend" {
		t.Fatalf("expected generic bucket tag to be deprioritized, got ordering %+v", results)
	}
}
