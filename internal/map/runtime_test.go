package issuemap

import "testing"

func TestEmbeddingFromTextUsesWiderHashSpace(t *testing.T) {
	vector := embeddingFromText("search ranking drift")
	if len(vector) != embeddingDimensions {
		t.Fatalf("expected vector length %d, got %d", embeddingDimensions, len(vector))
	}
	if embeddingDimensions < 64 {
		t.Fatalf("expected at least 64 embedding dimensions, got %d", embeddingDimensions)
	}
}
