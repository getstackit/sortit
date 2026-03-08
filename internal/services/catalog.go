package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"splat/internal/ai"
	"splat/internal/issues"
)

type TagStore interface {
	ListTags(context.Context) ([]issues.Tag, error)
	UpsertTags(context.Context, []issues.Tag) error
}

type CatalogService struct {
	store    TagStore
	analyzer *ai.Analyzer
}

func NewCatalogService(store TagStore, analyzer *ai.Analyzer) *CatalogService {
	return &CatalogService{
		store:    store,
		analyzer: analyzer,
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

func normalizeCatalogTagName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}
