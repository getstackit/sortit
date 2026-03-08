package issues

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrNotFound = errors.New("issue not found")

type IssueStatus string

const (
	StatusOpen   IssueStatus = "open"
	StatusClosed IssueStatus = "closed"
)

type Issue struct {
	ID        string         `json:"id"`
	Raw       string         `json:"raw"`
	Tags      []string       `json:"tags"`
	CreatedBy string         `json:"createdBy"`
	CreatedAt time.Time      `json:"createdAt"`
	Status    IssueStatus    `json:"status"`
	ClosedAt  *time.Time     `json:"closedAt"`
	ClosedBy  string         `json:"closedBy,omitempty"`
	TagScores []TagRelevance `json:"-"`
	Embedding []float64      `json:"-"`
}

type Tag struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Embedding   []float64 `json:"-"`
}

type TagRelevance struct {
	Tag       string  `json:"tag"`
	Relevance float64 `json:"relevance"`
}

type CreateInput struct {
	Raw       string
	Tags      []string
	CreatedBy string
	TagScores []TagRelevance
	Embedding []float64
}

type Store interface {
	List(context.Context) ([]Issue, error)
	Get(context.Context, string) (Issue, error)
	Create(context.Context, CreateInput) (Issue, error)
	CloseIssue(context.Context, string, string) (Issue, error)
	ReopenIssue(context.Context, string) (Issue, error)
}

func DefaultTags() []Tag {
	return []Tag{
		{Name: "bug", Description: "software defect or incorrect behavior"},
		{Name: "crash", Description: "hard failure, freeze, or abrupt termination"},
		{Name: "feature", Description: "request for a new capability"},
		{Name: "idea", Description: "early concept, exploration, or brainstorming"},
		{Name: "improvement", Description: "refinement to an existing workflow or capability"},
		{Name: "ui", Description: "visual interface, layout, or component presentation"},
		{Name: "ux", Description: "usability, clarity, friction, or flow quality"},
		{Name: "frontend", Description: "client-side app behavior in the browser"},
		{Name: "performance", Description: "speed, latency, efficiency, or scaling concerns"},
		{Name: "safari", Description: "Safari or WebKit-specific behavior"},
		{Name: "onboarding", Description: "first-run setup, signup, invite, or initial activation flow"},
		{Name: "search", Description: "querying, filtering, ranking, or finding content"},
		{Name: "export", Description: "download, file generation, sharing, or data extraction"},
	}
}

type InMemoryStore struct {
	mu      sync.RWMutex
	issues  []Issue
	nextSeq atomic.Uint64
}

func NewInMemoryStore(seed []Issue) *InMemoryStore {
	store := &InMemoryStore{
		issues: cloneIssues(seed),
	}

	slices.SortStableFunc(store.issues, compareIssueOrder)
	store.nextSeq.Store(uint64(len(seed)))
	return store
}

func (s *InMemoryStore) List(_ context.Context) ([]Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return cloneIssues(s.issues), nil
}

func (s *InMemoryStore) Get(_ context.Context, id string) (Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id = strings.TrimSpace(id)
	for _, issue := range s.issues {
		if issue.ID == id {
			return cloneIssues([]Issue{issue})[0], nil
		}
	}

	return Issue{}, ErrNotFound
}

func (s *InMemoryStore) Create(_ context.Context, input CreateInput) (Issue, error) {
	raw := strings.TrimSpace(input.Raw)
	if raw == "" {
		return Issue{}, fmt.Errorf("raw is required")
	}

	createdBy := strings.TrimSpace(input.CreatedBy)
	if createdBy == "" {
		createdBy = "You"
	}

	issue := Issue{
		ID:        fmt.Sprintf("issue-%06d", s.nextSeq.Add(1)),
		Raw:       raw,
		Tags:      displayTags(input.Tags, input.TagScores),
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
		Status:    StatusOpen,
		TagScores: copyTagScores(input.TagScores),
		Embedding: copyEmbedding(input.Embedding),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.issues = append([]Issue{issue}, s.issues...)
	return issue, nil
}

func (s *InMemoryStore) CloseIssue(_ context.Context, id string, closedBy string) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	for index, issue := range s.issues {
		if issue.ID != id {
			continue
		}

		if issue.Status == StatusClosed && issue.ClosedAt != nil {
			return cloneIssues([]Issue{issue})[0], nil
		}

		closedAt := time.Now().UTC()
		issue.Status = StatusClosed
		issue.ClosedAt = &closedAt
		issue.ClosedBy = defaultActor(closedBy)
		s.issues[index] = issue
		return cloneIssues([]Issue{issue})[0], nil
	}

	return Issue{}, ErrNotFound
}

func (s *InMemoryStore) ReopenIssue(_ context.Context, id string) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	for index, issue := range s.issues {
		if issue.ID != id {
			continue
		}

		if issue.Status == StatusOpen {
			return cloneIssues([]Issue{issue})[0], nil
		}

		issue.Status = StatusOpen
		issue.ClosedAt = nil
		issue.ClosedBy = ""
		s.issues[index] = issue
		return cloneIssues([]Issue{issue})[0], nil
	}

	return Issue{}, ErrNotFound
}

func (s *InMemoryStore) Replace(_ context.Context, next []Issue) error {
	items := cloneIssues(next)
	slices.SortStableFunc(items, compareIssueOrder)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.issues = items
	return nil
}

func sanitizeTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}

	if len(out) == 0 {
		return []string{}
	}
	return out
}

func displayTags(explicitTags []string, scores []TagRelevance) []string {
	if tags := sanitizeTags(explicitTags); len(tags) > 0 {
		return tags
	}
	if len(scores) == 0 {
		return []string{}
	}

	normalized := copyTagScores(scores)
	slices.SortStableFunc(normalized, func(a, b TagRelevance) int {
		if a.Relevance > b.Relevance {
			return -1
		}
		if a.Relevance < b.Relevance {
			return 1
		}
		if a.Tag < b.Tag {
			return -1
		}
		if a.Tag > b.Tag {
			return 1
		}
		return 0
	})

	out := make([]string, 0, minInt(3, len(normalized)))
	for _, score := range normalized {
		if score.Tag == "" {
			continue
		}
		if len(out) > 0 && score.Relevance < 0.2 {
			break
		}
		out = append(out, score.Tag)
		if len(out) == 3 {
			break
		}
	}
	if len(out) == 0 && normalized[0].Tag != "" {
		return []string{normalized[0].Tag}
	}
	return out
}

func cloneIssues(input []Issue) []Issue {
	items := make([]Issue, len(input))
	for i, issue := range input {
		items[i] = Issue{
			ID:        issue.ID,
			Raw:       issue.Raw,
			Tags:      append([]string(nil), issue.Tags...),
			CreatedBy: issue.CreatedBy,
			CreatedAt: issue.CreatedAt,
			Status:    normalizeIssueStatus(issue.Status),
			ClosedAt:  cloneTimePtr(issue.ClosedAt),
			ClosedBy:  issue.ClosedBy,
			TagScores: copyTagScores(issue.TagScores),
			Embedding: copyEmbedding(issue.Embedding),
		}
	}
	return items
}

func cloneTags(input []Tag) []Tag {
	items := make([]Tag, len(input))
	for i, tag := range input {
		items[i] = Tag{
			Name:        tag.Name,
			Description: tag.Description,
			CreatedAt:   tag.CreatedAt,
			Embedding:   copyEmbedding(tag.Embedding),
		}
	}
	return items
}

func sanitizeTagName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}

func copyTagScores(input []TagRelevance) []TagRelevance {
	if len(input) == 0 {
		return nil
	}
	out := make([]TagRelevance, len(input))
	copy(out, input)
	return out
}

func copyEmbedding(input []float64) []float64 {
	if len(input) == 0 {
		return nil
	}
	out := make([]float64, len(input))
	copy(out, input)
	return out
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	cloned := value.UTC()
	return &cloned
}

func normalizeIssueStatus(status IssueStatus) IssueStatus {
	if strings.EqualFold(strings.TrimSpace(string(status)), string(StatusClosed)) {
		return StatusClosed
	}
	return StatusOpen
}

func defaultActor(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "You"
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func compareIssueOrder(a, b Issue) int {
	if result := b.CreatedAt.Compare(a.CreatedAt); result != 0 {
		return result
	}
	if a.ID < b.ID {
		return -1
	}
	if a.ID > b.ID {
		return 1
	}
	return 0
}
