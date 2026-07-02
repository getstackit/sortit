package api

import (
	"context"
	"log/slog"

	"sortit/internal/ai"
	enrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
	"sortit/internal/themes"
)

// themeLabelerAdapter bridges the themes cache's decoupled themes.Labeler to the
// ai.Analyzer's ProposeThemeLabel, assembling the project concept frame per call
// exactly as the memory synthesizer does before naming a residual cluster. It
// keeps internal/themes free of any internal/ai dependency (the cache consumes
// only the themes-local Labeler interface); the composition root owns the wiring.
type themeLabelerAdapter struct {
	analyzer *ai.Analyzer
	store    issues.MemoryStore
	logger   *slog.Logger
}

// newThemeLabeler returns a themes.Labeler backed by the analyzer, or nil when no
// analyzer is configured — a nil Labeler leaves themes on their deterministic
// top-tag fallback with no background work, which is the correct no-AI behavior.
func newThemeLabeler(analyzer *ai.Analyzer, store issues.MemoryStore, logger *slog.Logger) themes.Labeler {
	if analyzer == nil {
		return nil
	}
	return themeLabelerAdapter{analyzer: analyzer, store: store, logger: logger}
}

func (a themeLabelerAdapter) ProposeThemeLabel(ctx context.Context, topTags []themes.LabelTag, issueTitles []string) (string, string, error) {
	frame := enrichment.ProjectConceptFrame(ctx, a.store, a.logger)
	aiTags := make([]ai.ThemeLabelTag, 0, len(topTags))
	for _, tag := range topTags {
		aiTags = append(aiTags, ai.ThemeLabelTag{Tag: tag.Tag, Loading: tag.Loading})
	}
	return a.analyzer.ProposeThemeLabel(ctx, aiTags, issueTitles, frame)
}
