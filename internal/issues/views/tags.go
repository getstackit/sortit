package issueviews

import (
	"context"

	"sortit/internal/issues"
	"sortit/internal/tags"
)

type ListTagsHandler struct {
	Catalog *tags.CatalogService
}

func (h ListTagsHandler) Handle(ctx context.Context) ([]issues.Tag, error) {
	return h.Catalog.AvailableTags(ctx)
}
