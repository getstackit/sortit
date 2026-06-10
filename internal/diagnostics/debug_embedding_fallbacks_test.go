package diagnostics

import (
	"context"
	"testing"
)

func TestDebugEmbeddingFallbacksTotalsConsistent(t *testing.T) {
	result, err := DebugEmbeddingFallbacksHandler{}.Handle(context.Background())
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Total != result.Issue+result.Tag {
		t.Fatalf("total %d != issue %d + tag %d", result.Total, result.Issue, result.Tag)
	}
}
