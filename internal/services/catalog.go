package services

import (
	"cmp"
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

var retrievalAnchorTagNames = []string{
	"bug",
	"crash",
	"feature",
	"idea",
	"improvement",
	"ui",
	"ux",
	"frontend",
	"performance",
	"safari",
	"onboarding",
	"search",
	"export",
}

const retrievalSimilarityThreshold = 0.2

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
	tags = activeCatalogTags(tags)
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
	stored = activeCatalogTags(stored)
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

// IssueTaxonomyShortlist builds a retrieval-first candidate set from the
// nearest embedded tags plus a small stable anchor set for broad recall.
func (s *CatalogService) IssueTaxonomyShortlist(ctx context.Context, issueEmbedding []float64, preferred []string, retrievedLimit int) ([]ai.Tag, error) {
	if len(issueEmbedding) == 0 {
		return s.IssueTaxonomy(ctx, preferred)
	}

	stored, err := s.StoredTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list stored tags: %w", err)
	}
	stored = activeCatalogTags(stored)
	if len(stored) == 0 {
		stored = issues.DefaultTags()
	}

	catalogByName := make(map[string]issues.Tag, len(stored))
	for _, tag := range stored {
		name := normalizeCatalogTagName(tag.Name)
		if name == "" {
			continue
		}
		catalogByName[name] = issues.Tag{
			Name:        name,
			Description: strings.TrimSpace(tag.Description),
			Embedding:   append([]float64(nil), tag.Embedding...),
		}
	}
	if len(catalogByName) == 0 {
		return s.IssueTaxonomy(ctx, preferred)
	}

	type scoredTag struct {
		tag        issues.Tag
		similarity float64
	}
	scored := make([]scoredTag, 0, len(catalogByName))
	for _, tag := range catalogByName {
		if len(tag.Embedding) == 0 {
			continue
		}
		similarity := vectors.CosineSimilarity(issueEmbedding, tag.Embedding)
		if similarity < retrievalSimilarityThreshold {
			continue
		}
		scored = append(scored, scoredTag{
			tag:        tag,
			similarity: similarity,
		})
	}
	slices.SortFunc(scored, func(a, b scoredTag) int {
		if c := cmp.Compare(b.similarity, a.similarity); c != 0 {
			return c
		}
		return cmp.Compare(a.tag.Name, b.tag.Name)
	})

	selected := make([]ai.Tag, 0, len(preferred)+len(retrievalAnchorTagNames)+retrievedLimit)
	seen := make(map[string]struct{}, len(preferred)+len(retrievalAnchorTagNames)+retrievedLimit)
	add := func(tag issues.Tag) {
		name := normalizeCatalogTagName(tag.Name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		selected = append(selected, ai.Tag{
			Name:        name,
			Description: strings.TrimSpace(tag.Description),
		})
	}

	for _, raw := range preferred {
		name := normalizeCatalogTagName(raw)
		if tag, ok := catalogByName[name]; ok {
			add(tag)
			continue
		}
		add(issues.Tag{Name: name})
	}

	if retrievedLimit <= 0 {
		retrievedLimit = 15
	}
	for _, item := range scored {
		add(item.tag)
		if len(selected) >= len(preferred)+retrievedLimit {
			break
		}
	}

	for _, name := range retrievalAnchorTagNames {
		if tag, ok := catalogByName[name]; ok {
			add(tag)
		}
	}

	if len(selected) == 0 {
		return s.IssueTaxonomy(ctx, preferred)
	}
	return selected, nil
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

func activeCatalogTags(tags []issues.Tag) []issues.Tag {
	if len(tags) == 0 {
		return nil
	}
	filtered := make([]issues.Tag, 0, len(tags))
	for _, tag := range tags {
		if !isActiveCatalogTag(tag.Name) {
			continue
		}
		filtered = append(filtered, tag)
	}
	return filtered
}

func isActiveCatalogTag(name string) bool {
	name = normalizeCatalogTagName(name)
	if name == "" {
		return false
	}
	return !strings.HasPrefix(name, "suggested-")
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

const minSpecificityCatalogSize = 4

// ScoreTagSpecificity computes the canonical deterministic specificity score
// for a single tag and persists it. The stored LLM sub-score is cleared so the
// canonical field remains reproducible and embedding-derived.
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

	found := false
	for _, tag := range tags {
		if normalizeCatalogTagName(tag.Name) == tagName {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tag %q not found in catalog", tagName)
	}

	score := cloneFloat64Pointer(computeEmbeddingSpecificity(tags)[tagName])
	now := time.Now().UTC()
	if err := s.store.UpdateTagSpecificity(ctx, tagName, score, nil, cloneFloat64Pointer(score), &now); err != nil {
		return fmt.Errorf("persist specificity for %q: %w", tagName, err)
	}

	s.logger.InfoContext(ctx, "tag specificity scored",
		"tag", tagName,
		"specificity", float64LogValue(score),
		"scored", score != nil,
		"duration", time.Since(start).Round(time.Millisecond),
	)
	return nil
}

// ScoreAllTagsSpecificity computes the canonical deterministic specificity score
// for every tag in the catalog. This is a bulk operation for initial scoring or
// re-scoring.
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

	orderedNames := sortedCatalogTagNames(tags)
	embeddingScores := computeEmbeddingSpecificity(tags)
	now := time.Now().UTC()

	s.logger.InfoContext(ctx, "computed embedding specificity",
		"tag_count", len(orderedNames),
		"scored_count", countComputedScores(embeddingScores),
	)

	for i, tagName := range orderedNames {
		tagStart := time.Now()
		score := cloneFloat64Pointer(embeddingScores[tagName])
		if err := s.store.UpdateTagSpecificity(ctx, tagName, score, nil, cloneFloat64Pointer(score), &now); err != nil {
			return fmt.Errorf("persist specificity for %q: %w", tagName, err)
		}

		s.logger.InfoContext(ctx, "tag scored",
			"tag", tagName,
			"specificity", float64LogValue(score),
			"scored", score != nil,
			"progress", fmt.Sprintf("%d/%d", i+1, len(orderedNames)),
			"duration", time.Since(tagStart).Round(time.Millisecond),
		)
	}

	s.logger.InfoContext(ctx, "all tags rescored",
		"tag_count", len(orderedNames),
		"duration", time.Since(start).Round(time.Millisecond),
	)
	return nil
}

func countComputedScores(scores map[string]*float64) int {
	count := 0
	for _, score := range scores {
		if score != nil {
			count++
		}
	}
	return count
}

// computeEmbeddingSpecificity computes canonical embedding-based specificity
// scores by tag name. Tags without embeddings, or catalogs too small to support
// a meaningful relative measure, receive nil.
func computeEmbeddingSpecificity(tags []issues.Tag) map[string]*float64 {
	type scoredTag struct {
		name      string
		embedding []float64
	}

	scored := make([]scoredTag, 0, len(tags))
	for _, tag := range tags {
		name := normalizeCatalogTagName(tag.Name)
		if name == "" || len(tag.Embedding) == 0 {
			continue
		}
		scored = append(scored, scoredTag{
			name:      name,
			embedding: append([]float64(nil), tag.Embedding...),
		})
	}
	slices.SortStableFunc(scored, func(a, b scoredTag) int {
		return cmp.Compare(a.name, b.name)
	})

	scores := make(map[string]*float64, len(scored))
	if len(scored) < minSpecificityCatalogSize {
		return scores
	}

	embeddings := make([][]float64, len(scored))
	for i, tag := range scored {
		embeddings[i] = tag.embedding
	}

	results := vectors.NeighborhoodSpecificity(embeddings)
	for i, result := range results {
		score := result.Specificity
		scores[scored[i].name] = &score
	}
	return scores
}

func sortedCatalogTagNames(tags []issues.Tag) []string {
	seen := make(map[string]struct{}, len(tags))
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		name := normalizeCatalogTagName(tag.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func float64LogValue(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

// AnnotateHints marks up to limit tags in the taxonomy as hints based on
// embedding similarity to the given issue embedding. This draws the AI's
// attention to tags that are semantically close to the issue but might
// otherwise be overlooked in a large taxonomy.
func (s *CatalogService) AnnotateHints(ctx context.Context, taxonomy []ai.Tag, issueEmbedding []float64, limit int) []ai.Tag {
	if s == nil || s.store == nil || len(issueEmbedding) == 0 || len(taxonomy) == 0 {
		return taxonomy
	}

	stored, err := s.store.ListTags(ctx)
	if err != nil {
		return taxonomy
	}

	embeddingByName := make(map[string][]float64, len(stored))
	for _, tag := range stored {
		if len(tag.Embedding) > 0 {
			embeddingByName[normalizeCatalogTagName(tag.Name)] = tag.Embedding
		}
	}

	type scored struct {
		index      int
		similarity float64
	}
	candidates := make([]scored, 0, len(taxonomy))
	for i, tag := range taxonomy {
		emb, ok := embeddingByName[normalizeCatalogTagName(tag.Name)]
		if !ok {
			continue
		}
		sim := vectors.CosineSimilarity(issueEmbedding, emb)
		if sim > 0.1 {
			candidates = append(candidates, scored{index: i, similarity: sim})
		}
	}

	slices.SortFunc(candidates, func(a, b scored) int {
		return cmp.Compare(b.similarity, a.similarity)
	})

	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	// Clone the taxonomy so we don't mutate the caller's slice.
	result := make([]ai.Tag, len(taxonomy))
	copy(result, taxonomy)
	for _, c := range candidates {
		result[c.index].Hint = true
	}
	return result
}

func normalizeCatalogTagName(name string) string {
	return domain.NormalizeTagName(name)
}
