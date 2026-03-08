package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
)

func defaultIssueAnalyzer() *ai.Analyzer {
	return ai.NewAnalyzer(ai.NewStubTagger(), ai.NewStubEmbedder())
}

func (s *Server) analyzeIssueInput(ctx context.Context, input issues.CreateInput) (issues.CreateInput, error) {
	taxonomy, err := s.issueTaxonomy(ctx, input.Tags)
	if err != nil {
		return issues.CreateInput{}, err
	}
	analyzed, err := s.issueAnalyzer.AnalyzeIssueData(ctx, input.Raw, taxonomy)
	if err != nil {
		return issues.CreateInput{}, err
	}

	if err := s.ensureStoredTags(ctx, catalogTagsFromAnalysis(taxonomy, input.Tags, analyzed.Tags)); err != nil {
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

func (s *Server) issueTaxonomy(ctx context.Context, preferred []string) ([]ai.Tag, error) {
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
			return tags, nil
		}
	}

	if s.tagStore != nil {
		stored, err := s.tagStore.ListTags(ctx)
		if err != nil {
			return nil, fmt.Errorf("list stored tags: %w", err)
		}
		if len(stored) > 0 {
			return aiTagsFromCatalog(stored), nil
		}
	}

	definitions := issues.DefaultTags()
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
	return tags, nil
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

func (s *Server) ensureStoredTags(ctx context.Context, tags []issues.Tag) error {
	if s.tagStore == nil || len(tags) == 0 {
		return nil
	}

	existing, err := s.tagStore.ListTags(ctx)
	if err != nil {
		return fmt.Errorf("list tags for sync: %w", err)
	}

	existingByName := make(map[string]issues.Tag, len(existing))
	for _, tag := range existing {
		existingByName[normalizeCatalogTagName(tag.Name)] = tag
	}

	toPersist := make([]issues.Tag, 0, len(tags))
	for _, raw := range tags {
		name := normalizeCatalogTagName(raw.Name)
		if name == "" {
			continue
		}

		tag := issues.Tag{
			Name:        name,
			Description: strings.TrimSpace(raw.Description),
			CreatedAt:   raw.CreatedAt,
			Embedding:   append([]float64(nil), raw.Embedding...),
		}

		existingTag, exists := existingByName[name]
		if exists {
			if tag.Description == "" {
				tag.Description = existingTag.Description
			}
			if len(tag.Embedding) == 0 {
				tag.Embedding = append([]float64(nil), existingTag.Embedding...)
			}
			if tag.CreatedAt.IsZero() {
				tag.CreatedAt = existingTag.CreatedAt
			}
		}

		if tag.CreatedAt.IsZero() {
			tag.CreatedAt = time.Now().UTC()
		}
		if len(tag.Embedding) == 0 {
			embedding, err := s.embedTag(ctx, tag)
			if err != nil {
				return err
			}
			tag.Embedding = embedding
		}

		toPersist = append(toPersist, tag)
	}

	if len(toPersist) == 0 {
		return nil
	}
	if err := s.tagStore.UpsertTags(ctx, toPersist); err != nil {
		return fmt.Errorf("upsert stored tags: %w", err)
	}
	return nil
}

func (s *Server) embedTag(ctx context.Context, tag issues.Tag) ([]float64, error) {
	descriptor := tag.Name
	if description := strings.TrimSpace(tag.Description); description != "" {
		descriptor += " - " + description
	}

	result, err := s.issueAnalyzer.EmbedText(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("embed tag %q: %w", tag.Name, err)
	}
	return float32VectorToFloat64(result.Vector), nil
}

func aiTagsFromCatalog(tags []issues.Tag) []ai.Tag {
	out := make([]ai.Tag, 0, len(tags))
	for _, tag := range tags {
		name := normalizeCatalogTagName(tag.Name)
		if name == "" {
			continue
		}
		out = append(out, ai.Tag{
			Name:        name,
			Description: strings.TrimSpace(tag.Description),
		})
	}
	return out
}

func catalogTagsFromAnalysis(taxonomy []ai.Tag, explicit []string, scores []ai.TagScore) []issues.Tag {
	definitions := make(map[string]string, len(taxonomy))
	for _, tag := range taxonomy {
		name := normalizeCatalogTagName(tag.Name)
		if name == "" {
			continue
		}
		definitions[name] = strings.TrimSpace(tag.Description)
	}

	catalog := make([]issues.Tag, 0, len(explicit)+len(scores))
	for _, tag := range explicit {
		name := normalizeCatalogTagName(tag)
		if name == "" {
			continue
		}
		catalog = append(catalog, issues.Tag{
			Name:        name,
			Description: definitions[name],
		})
	}

	for _, score := range scores {
		name := normalizeCatalogTagName(score.Tag)
		if name == "" {
			continue
		}

		description := definitions[name]
		if description == "" {
			description = strings.TrimSpace(score.Description)
		}

		catalog = append(catalog, issues.Tag{
			Name:        name,
			Description: description,
		})
	}

	return catalog
}

func normalizeCatalogTagName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}
