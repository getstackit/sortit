package regions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
)

// ErrCustomStoreUnavailable is returned when the custom-region CRUD
// endpoints are called but no store is wired (e.g., a deployment
// without persistent storage).
var ErrCustomStoreUnavailable = errors.New("custom region store unavailable")

// CustomHandler exposes CRUD over user-defined regions. List / Get
// retrieval is also reachable through the standard regions Loader,
// but CRUD lives here so the surface is decoupled from the metric
// compute path.
type CustomHandler struct {
	Store CustomRegionWriter
	Clock func() time.Time
}

// CustomRegionWriter is the read/write surface CustomHandler needs.
// issues.PostgresStore and issues.InMemoryStore both satisfy it.
type CustomRegionWriter interface {
	ListCustomRegions(ctx context.Context) ([]domain.CustomRegion, error)
	GetCustomRegion(ctx context.Context, id string) (domain.CustomRegion, error)
	UpsertCustomRegion(ctx context.Context, region domain.CustomRegion) error
	DeleteCustomRegion(ctx context.Context, id string) error
}

// CustomRegionInput is the wire shape for create/update requests.
type CustomRegionInput struct {
	ID          string                       `json:"id,omitempty"`
	Label       string                       `json:"label"`
	Description string                       `json:"description,omitempty"`
	Definition  domain.CustomRegionPredicate `json:"definition"`
}

// List returns all stored custom regions.
func (h *CustomHandler) List(ctx context.Context) ([]domain.CustomRegion, error) {
	if h == nil || h.Store == nil {
		return nil, ErrCustomStoreUnavailable
	}
	return h.Store.ListCustomRegions(ctx)
}

// Get returns one region by id.
func (h *CustomHandler) Get(ctx context.Context, id string) (domain.CustomRegion, error) {
	if h == nil || h.Store == nil {
		return domain.CustomRegion{}, ErrCustomStoreUnavailable
	}
	return h.Store.GetCustomRegion(ctx, id)
}

// Create persists a new custom region. Generates an ID if none is
// provided.
func (h *CustomHandler) Create(ctx context.Context, input CustomRegionInput, createdBy string) (domain.CustomRegion, error) {
	if h == nil || h.Store == nil {
		return domain.CustomRegion{}, ErrCustomStoreUnavailable
	}
	if strings.TrimSpace(input.Label) == "" {
		return domain.CustomRegion{}, errors.New("label is required")
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "cr-" + randomHex(8)
	}
	now := h.now()
	region := domain.CustomRegion{
		ID:          id,
		Label:       strings.TrimSpace(input.Label),
		Description: strings.TrimSpace(input.Description),
		Definition:  input.Definition,
		CreatedBy:   strings.TrimSpace(createdBy),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.Store.UpsertCustomRegion(ctx, region); err != nil {
		return domain.CustomRegion{}, err
	}
	return region, nil
}

// Update overwrites an existing region's label/description/definition.
// CreatedAt and CreatedBy are preserved.
func (h *CustomHandler) Update(ctx context.Context, id string, input CustomRegionInput) (domain.CustomRegion, error) {
	if h == nil || h.Store == nil {
		return domain.CustomRegion{}, ErrCustomStoreUnavailable
	}
	existing, err := h.Store.GetCustomRegion(ctx, id)
	if err != nil {
		return domain.CustomRegion{}, err
	}
	now := h.now()
	existing.Label = strings.TrimSpace(input.Label)
	existing.Description = strings.TrimSpace(input.Description)
	existing.Definition = input.Definition
	existing.UpdatedAt = now
	if err := h.Store.UpsertCustomRegion(ctx, existing); err != nil {
		return domain.CustomRegion{}, err
	}
	return existing, nil
}

// Delete removes a region by id.
func (h *CustomHandler) Delete(ctx context.Context, id string) error {
	if h == nil || h.Store == nil {
		return ErrCustomStoreUnavailable
	}
	return h.Store.DeleteCustomRegion(ctx, id)
}

func (h *CustomHandler) now() time.Time {
	if h.Clock != nil {
		return h.Clock()
	}
	return time.Now().UTC()
}

func randomHex(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "0"
	}
	return hex.EncodeToString(buf)
}

// Compile-time assertion that issues.PostgresStore and
// issues.InMemoryStore satisfy CustomRegionWriter.
var _ CustomRegionWriter = (*issues.PostgresStore)(nil)
var _ CustomRegionWriter = (*issues.InMemoryStore)(nil)
