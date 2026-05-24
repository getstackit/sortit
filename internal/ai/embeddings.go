package ai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	defaultEmbeddingChunkTokens = 2000
	defaultEmbeddingPreviewSize = 8
)

type textChunk struct {
	Text            string
	EstimatedTokens int
}

func embedChunkedText(
	ctx context.Context,
	text string,
	maxChunkTokens int,
	previewSize int,
	embedFn func(context.Context, []string) ([][]float32, error),
) (EmbeddingResult, error) {
	if maxChunkTokens <= 0 {
		maxChunkTokens = defaultEmbeddingChunkTokens
	}
	if previewSize <= 0 {
		previewSize = defaultEmbeddingPreviewSize
	}

	chunks := splitTextIntoChunks(text, maxChunkTokens)
	inputs := make([]string, len(chunks))
	for i, chunk := range chunks {
		inputs[i] = chunk.Text
	}

	vectors, err := embedFn(ctx, inputs)
	if err != nil {
		return EmbeddingResult{}, err
	}
	if len(vectors) != len(chunks) {
		return EmbeddingResult{}, fmt.Errorf("expected %d chunk embeddings, got %d", len(chunks), len(vectors))
	}

	pooled, err := poolVectors(vectors, chunks)
	if err != nil {
		return EmbeddingResult{}, err
	}

	return EmbeddingResult{
		Vector: pooled,
		Info: EmbeddingInfo{
			Dimensions:          len(pooled),
			Preview:             previewVector(pooled, previewSize),
			ChunkCount:          len(chunks),
			EstimatedTokenCount: totalEstimatedTokens(chunks),
			PooledFromChunks:    len(chunks) > 1,
		},
	}, nil
}

func splitTextIntoChunks(text string, maxChunkTokens int) []textChunk {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return []textChunk{{Text: "", EstimatedTokens: 0}}
	}

	estimatedTokens := estimateTokens(trimmed)
	if estimatedTokens <= maxChunkTokens {
		return []textChunk{{Text: trimmed, EstimatedTokens: estimatedTokens}}
	}

	words := strings.Fields(trimmed)
	chunks := make([]textChunk, 0, maxInt(1, len(words)/64))
	currentWords := make([]string, 0, 128)
	currentTokens := 0

	flush := func() {
		if len(currentWords) == 0 {
			return
		}
		chunks = append(chunks, textChunk{
			Text:            strings.Join(currentWords, " "),
			EstimatedTokens: currentTokens,
		})
		currentWords = currentWords[:0]
		currentTokens = 0
	}

	for _, word := range words {
		wordTokens := estimateTokens(word)
		if len(currentWords) > 0 && currentTokens+wordTokens > maxChunkTokens {
			flush()
		}

		currentWords = append(currentWords, word)
		currentTokens += wordTokens

		if currentTokens >= maxChunkTokens {
			flush()
		}
	}

	flush()
	return chunks
}

func estimateTokens(text string) int {
	length := len([]rune(text))
	if length == 0 {
		return 0
	}

	estimated := int(math.Ceil(float64(length) / 4))
	if estimated < 1 {
		return 1
	}
	return estimated
}

func poolVectors(vectors [][]float32, chunks []textChunk) ([]float32, error) {
	if len(vectors) == 0 {
		return nil, errors.New("no embeddings returned")
	}

	dimensions := len(vectors[0])
	if dimensions == 0 {
		return []float32{}, nil
	}

	pooled := make([]float64, dimensions)
	totalWeight := 0.0
	for i, vector := range vectors {
		if len(vector) != dimensions {
			return nil, fmt.Errorf("embedding dimension mismatch at chunk %d", i)
		}

		weight := float64(chunks[i].EstimatedTokens)
		if weight <= 0 {
			weight = 1
		}

		totalWeight += weight
		for j, value := range vector {
			pooled[j] += float64(value) * weight
		}
	}

	if totalWeight == 0 {
		totalWeight = 1
	}

	for i := range pooled {
		pooled[i] /= totalWeight
	}

	norm := 0.0
	for _, value := range pooled {
		norm += value * value
	}
	norm = math.Sqrt(norm)

	result := make([]float32, dimensions)
	if norm == 0 {
		return result, nil
	}

	for i, value := range pooled {
		result[i] = float32(value / norm)
	}
	return result, nil
}

func previewVector(vector []float32, previewSize int) []float32 {
	size := minInt(previewSize, len(vector))
	preview := make([]float32, size)
	for i := range size {
		preview[i] = float32(math.Round(float64(vector[i])*1_000_000) / 1_000_000)
	}
	return preview
}

func totalEstimatedTokens(chunks []textChunk) int {
	total := 0
	for _, chunk := range chunks {
		total += chunk.EstimatedTokens
	}
	return total
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
