package queries

import (
	"context"

	"splat/internal/issues"
	"splat/internal/services"
)

type ListTagsHandler struct {
	Catalog *services.CatalogService
}

func (h ListTagsHandler) Handle(ctx context.Context) ([]issues.Tag, error) {
	return h.Catalog.AvailableTags(ctx)
}
