package ai

import (
	"context"
	"strings"
	"testing"
)

func TestEmbedChunkedTextPoolsChunks(t *testing.T) {
	text := strings.Repeat("export safari crash ", 900)

	result, err := embedChunkedText(
		context.Background(),
		text,
		100,
		4,
		func(_ context.Context, inputs []string) ([][]float32, error) {
			vectors := make([][]float32, len(inputs))
			for i := range inputs {
				vectors[i] = []float32{float32(i + 1), 1}
			}
			return vectors, nil
		},
	)
	if err != nil {
		t.Fatalf("embedChunkedText returned error: %v", err)
	}

	if !result.Info.PooledFromChunks {
		t.Fatalf("expected pooled result for long input")
	}
	if result.Info.ChunkCount < 2 {
		t.Fatalf("expected multiple chunks, got %d", result.Info.ChunkCount)
	}
	if result.Info.Dimensions != 2 {
		t.Fatalf("expected 2 dimensions, got %d", result.Info.Dimensions)
	}
	if len(result.Info.Preview) != 2 {
		t.Fatalf("expected preview length 2, got %d", len(result.Info.Preview))
	}
	if len(result.Vector) != 2 {
		t.Fatalf("expected pooled vector length 2, got %d", len(result.Vector))
	}
	if result.Info.EstimatedTokenCount <= 100 {
		t.Fatalf("expected token estimate above single chunk budget, got %d", result.Info.EstimatedTokenCount)
	}
}
