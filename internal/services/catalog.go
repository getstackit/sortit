package services

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"splat/internal/ai"
	"splat/internal/domain"
	"splat/internal/issues"
	"splat/internal/vectors"
)

type TagStore interface {
	ListTags(context.Context) ([]issues.Tag, error)
	UpsertTags(context.Context, []issues.Tag) error
	UpdateTagSpecificity(ctx context.Context, name string, specificity, llm, embedding *float64, computedAt *time.Time) error
}

type CatalogService struct {
	store    TagStore
	analyzer *ai.Analyzer
	logger   *slog.Logger
}

func NewCatalogService(store TagStore, analyzer *ai.Analyzer, logger *slog.Logger) *CatalogService {
	return &CatalogService{
		store:    store,
		analyzer: analyzer,
		logger:   logger,
	}
}

func FallbackAnalyzer(analyzer *ai.Analyzer) *ai.Analyzer {
	if analyzer != nil {
		return analyzer
	}
	return ai.NewAnalyzer(ai.NewStubTagger(), ai.NewStubEmbedder())
}

func (s *CatalogService) StoredTags(ctx context.Context) ([]issues.Tag, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.ListTags(ctx)
}

func (s *CatalogService) AvailableTags(ctx context.Context) ([]issues.Tag, error) {
	tags, err := s.StoredTags(ctx)
	if err != nil {
		return nil, err
	}
	if len(tags) > 0 {
		return tags, nil
	}
	return issues.DefaultTags(), nil
}

func (s *CatalogService) IssueTaxonomy(ctx context.Context, preferred []string) ([]ai.Tag, error) {
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

	stored, err := s.StoredTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list stored tags: %w", err)
	}
	if len(stored) > 0 {
		return aiTagsFromCatalog(stored), nil
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

func (s *CatalogService) EnsureStoredTags(ctx context.Context, tags []issues.Tag) error {
	if s == nil || s.store == nil || len(tags) == 0 {
		return nil
	}

	existing, err := s.store.ListTags(ctx)
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
		descriptionChanged := false
		if exists {
			descriptionChanged = tag.Description != "" && tag.Description != existingTag.Description
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
		if descriptionChanged && len(raw.Embedding) == 0 {
			tag.Embedding = nil
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
	if err := s.store.UpsertTags(ctx, toPersist); err != nil {
		return fmt.Errorf("upsert stored tags: %w", err)
	}
	return nil
}

func (s *CatalogService) embedTag(ctx context.Context, tag issues.Tag) ([]float64, error) {
	descriptor := tag.Name
	if description := strings.TrimSpace(tag.Description); description != "" {
		descriptor += " - " + description
	}

	result, err := s.analyzer.EmbedText(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("embed tag %q: %w", tag.Name, err)
	}
	return Float32VectorToFloat64(result.Vector), nil
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
	// Sort by specificity descending so the AI sees specific tags first.
	specificityByName := make(map[string]float64, len(tags))
	for _, tag := range tags {
		if tag.Specificity != nil {
			specificityByName[tag.Name] = *tag.Specificity
		} else {
			specificityByName[tag.Name] = 0.5
		}
	}
	slices.SortStableFunc(out, func(a, b ai.Tag) int {
		aSpec := specificityByName[a.Name]
		bSpec := specificityByName[b.Name]
		if aSpec != bSpec {
			if aSpec > bSpec {
				return -1
			}
			return 1
		}
		return 0
	})
	return out
}

func CatalogTagsFromAnalysis(taxonomy []ai.Tag, explicit []string, scores []ai.TagScore) []issues.Tag {
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

const (
	specificityLLMWeight       = 0.7
	specificityEmbeddingWeight = 0.3
)

// ScoreTagSpecificity computes the blended specificity score for a single tag
// (LLM * 0.7 + embedding * 0.3) using the full tag catalog as context, and
// persists the result.
func (s *CatalogService) ScoreTagSpecificity(ctx context.Context, tagName string) error {
	tagName = normalizeCatalogTagName(tagName)
	if tagName == "" {
		return fmt.Errorf("tag name is required")
	}

	start := time.Now()
	s.logger.InfoContext(ctx, "scoring tag specificity", "tag", tagName)

	tags, err := s.StoredTags(ctx)
	if err != nil {
		return fmt.Errorf("list tags for specificity: %w", err)
	}

	catalog := aiTagsFromCatalog(tags)
	embeddingScores := computeEmbeddingSpecificity(tags)

	var target ai.Tag
	var embScore float64
	for i, t := range catalog {
		if t.Name == tagName {
			target = t
			if i < len(embeddingScores) {
				embScore = embeddingScores[i]
			}
			break
		}
	}
	if target.Name == "" {
		return fmt.Errorf("tag %q not found in catalog", tagName)
	}

	llmScore, err := s.analyzer.ScoreTagSpecificity(ctx, target, catalog)
	if err != nil {
		return fmt.Errorf("score specificity for %q: %w", tagName, err)
	}

	blended := llmScore*specificityLLMWeight + embScore*specificityEmbeddingWeight
	now := time.Now().UTC()
	if err := s.store.UpdateTagSpecificity(ctx, tagName, &blended, &llmScore, &embScore, &now); err != nil {
		return fmt.Errorf("persist specificity for %q: %w", tagName, err)
	}

	s.logger.InfoContext(ctx, "tag specificity scored",
		"tag", tagName,
		"blended", blended,
		"llm", llmScore,
		"embedding", embScore,
		"duration", time.Since(start).Round(time.Millisecond),
	)
	return nil
}

// ScoreAllTagsSpecificity computes the blended specificity score for every tag
// in the catalog. This is a bulk operation for initial scoring or re-scoring.
func (s *CatalogService) ScoreAllTagsSpecificity(ctx context.Context) error {
	start := time.Now()
	s.logger.InfoContext(ctx, "rescoring all tag specificity")

	tags, err := s.StoredTags(ctx)
	if err != nil {
		return fmt.Errorf("list tags for specificity: %w", err)
	}
	if len(tags) == 0 {
		s.logger.InfoContext(ctx, "no tags to rescore")
		return nil
	}

	catalog := aiTagsFromCatalog(tags)
	embeddingScores := computeEmbeddingSpecificity(tags)
	now := time.Now().UTC()

	s.logger.InfoContext(ctx, "computed embedding specificity",
		"tag_count", len(catalog),
		"embedding_count", countEmbeddings(tags),
	)

	for i, tag := range catalog {
		tagStart := time.Now()
		llmScore, err := s.analyzer.ScoreTagSpecificity(ctx, tag, catalog)
		if err != nil {
			return fmt.Errorf("score specificity for %q: %w", tag.Name, err)
		}

		var embScore float64
		if i < len(embeddingScores) {
			embScore = embeddingScores[i]
		}
		blended := llmScore*specificityLLMWeight + embScore*specificityEmbeddingWeight

		if err := s.store.UpdateTagSpecificity(ctx, tag.Name, &blended, &llmScore, &embScore, &now); err != nil {
			return fmt.Errorf("persist specificity for %q: %w", tag.Name, err)
		}

		s.logger.InfoContext(ctx, "tag scored",
			"tag", tag.Name,
			"blended", blended,
			"llm", llmScore,
			"embedding", embScore,
			"progress", fmt.Sprintf("%d/%d", i+1, len(catalog)),
			"duration", time.Since(tagStart).Round(time.Millisecond),
		)
	}

	s.logger.InfoContext(ctx, "all tags rescored",
		"tag_count", len(catalog),
		"duration", time.Since(start).Round(time.Millisecond),
	)
	return nil
}

func countEmbeddings(tags []issues.Tag) int {
	n := 0
	for _, t := range tags {
		if len(t.Embedding) > 0 {
			n++
		}
	}
	return n
}

// computeEmbeddingSpecificity computes the embedding-based specificity score
// for each tag using k-means clustering. Tags must be in the same order as
// returned by aiTagsFromCatalog. Tags without embeddings receive a score of 0.
func computeEmbeddingSpecificity(tags []issues.Tag) []float64 {
	// Build embedding matrix, tracking which tags have embeddings.
	embeddings := make([][]float64, 0, len(tags))
	indexMap := make([]int, 0, len(tags)) // maps embedding index → tag index
	for i, tag := range tags {
		if len(tag.Embedding) > 0 {
			indexMap = append(indexMap, i)
			embeddings = append(embeddings, tag.Embedding)
		}
	}

	scores := make([]float64, len(tags))
	if len(embeddings) < 2 {
		return scores
	}

	results := vectors.KMeansSpecificity(embeddings)
	for j, r := range results {
		scores[indexMap[j]] = r.Specificity
	}
	return scores
}

func normalizeCatalogTagName(name string) string {
	return domain.NormalizeTagName(name)
}
