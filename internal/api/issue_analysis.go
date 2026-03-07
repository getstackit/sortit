package api

import (
	"context"
	"strings"

	"bored/internal/ai"
	"bored/internal/issues"
	issuemap "bored/internal/map"
)

func defaultIssueAnalyzer() *ai.Analyzer {
	return ai.NewAnalyzer(ai.NewStubTagger(), ai.NewStubEmbedder())
}

func (s *Server) analyzeIssueInput(ctx context.Context, input issues.CreateInput) (issues.CreateInput, error) {
	taxonomy := issueTaxonomy(input.Tags)
	analyzed, err := s.issueAnalyzer.AnalyzeIssueData(ctx, input.Raw, taxonomy)
	if err != nil {
		return issues.CreateInput{}, err
	}

	input.TagScores = issueTagScoresFromAnalysis(analyzed.Tags)
	input.Embedding = float32VectorToFloat64(analyzed.Embedding.Vector)
	if len(input.Tags) == 0 {
		input.Tags = nil
	}
	return input, nil
}

func (s *Server) analyzeSeedIssues(ctx context.Context, seeds []issues.Issue) ([]issues.Issue, error) {
	enriched := make([]issues.Issue, len(seeds))
	for i, seed := range seeds {
		analyzed, err := s.analyzeIssueInput(ctx, issues.CreateInput{
			Raw:  seed.Raw,
			Tags: seed.Tags,
		})
		if err != nil {
			return nil, err
		}

		enriched[i] = issues.Issue{
			ID:        seed.ID,
			Raw:       seed.Raw,
			Tags:      append([]string(nil), seed.Tags...),
			CreatedBy: seed.CreatedBy,
			CreatedAt: seed.CreatedAt,
			TagScores: analyzed.TagScores,
			Embedding: analyzed.Embedding,
		}
	}
	return enriched, nil
}

func issueTaxonomy(preferred []string) []ai.Tag {
	if len(preferred) > 0 {
		tags := make([]ai.Tag, 0, len(preferred))
		seen := make(map[string]struct{}, len(preferred))
		for _, raw := range preferred {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			tags = append(tags, ai.Tag{Name: name})
		}
		if len(tags) > 0 {
			return tags
		}
	}

	definitions := issuemap.AllTagDefinitions()
	tags := make([]ai.Tag, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Name == "" {
			continue
		}
		tags = append(tags, ai.Tag{
			Name:        definition.Name,
			Description: definition.Description,
		})
	}
	return tags
}

func issueTagScoresFromAnalysis(scores []ai.TagScore) []issues.TagRelevance {
	if len(scores) == 0 {
		return nil
	}

	tagScores := make([]issues.TagRelevance, 0, len(scores))
	for _, score := range scores {
		name := strings.TrimSpace(score.Tag)
		if name == "" {
			continue
		}
		tagScores = append(tagScores, issues.TagRelevance{
			Tag:       name,
			Relevance: score.Relevance,
		})
	}
	return tagScores
}

func float32VectorToFloat64(values []float32) []float64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = float64(value)
	}
	return out
}
