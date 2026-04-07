package issueviews

import (
	"context"

	"splat/internal/issues"
	"splat/internal/tags"
)

type ListTagsHandler struct {
	Catalog *tags.CatalogService
}

func (h ListTagsHandler) Handle(ctx context.Context) ([]issues.Tag, error) {
	return h.Catalog.AvailableTags(ctx)
}
