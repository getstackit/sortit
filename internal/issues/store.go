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

	"splat/internal/domain"
)

var ErrNotFound = errors.New("issue not found")
var ErrIssueClosed = errors.New("issue is closed")

type IssueStatus string

const (
	StatusOpen   IssueStatus = "open"
	StatusClosed IssueStatus = "closed"
)

type Issue struct {
	ID         string           `json:"id"`
	Raw        string           `json:"raw"`
	Tags       []string         `json:"tags"`
	CreatedBy  string           `json:"createdBy"`
	CreatedAt  time.Time        `json:"createdAt"`
	Status     IssueStatus      `json:"status"`
	ClosedAt   *time.Time       `json:"closedAt"`
	ClosedBy   string           `json:"closedBy,omitempty"`
	AssignedTo string           `json:"assignedTo,omitempty"`
	Discussion []IssuePost      `json:"discussion,omitempty"`
	Links      []IssueLink      `json:"links,omitempty"`
	Operations []IssueOperation `json:"operations,omitempty"`
	TagScores  []TagRelevance   `json:"tagScores"`
	Embedding  []float64        `json:"-"`
}

type IssuePost struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issueId,omitempty"`
	Raw       string    `json:"raw"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	Sequence  int       `json:"sequence"`
	Kind      string    `json:"kind,omitempty"`
}

type Tag struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Embedding   []float64 `json:"-"`
}

// TagRelevance is an alias for domain.TagRelevance.
// All packages should migrate to using domain.TagRelevance directly.
type TagRelevance = domain.TagRelevance

type IssueLinkType string

const (
	IssueLinkTypeParentOf    IssueLinkType = "parent_of"
	IssueLinkTypeChildOf     IssueLinkType = "child_of"
	IssueLinkTypeMergedInto  IssueLinkType = "merged_into"
	IssueLinkTypeDerivedFrom IssueLinkType = "derived_from"
	IssueLinkTypeRelatedTo   IssueLinkType = "related_to"
	IssueLinkTypeDuplicateOf IssueLinkType = "duplicate_of"
)

type IssueOperationKind string

const (
	IssueOperationKindSplit   IssueOperationKind = "split"
	IssueOperationKindCombine IssueOperationKind = "combine"
	IssueOperationKindLink    IssueOperationKind = "link"
)

type IssueReference struct {
	ID     string      `json:"id"`
	Raw    string      `json:"raw"`
	Status IssueStatus `json:"status"`
}

type IssueLink struct {
	ID            string          `json:"id"`
	Type          IssueLinkType   `json:"type"`
	SourceIssueID string          `json:"sourceIssueId"`
	TargetIssueID string          `json:"targetIssueId"`
	Direction     string          `json:"direction,omitempty"`
	CreatedBy     string          `json:"createdBy"`
	CreatedAt     time.Time       `json:"createdAt"`
	Note          string          `json:"note,omitempty"`
	OperationID   string          `json:"operationId,omitempty"`
	RelatedIssue  *IssueReference `json:"relatedIssue,omitempty"`
}

type IssueOperationParticipant struct {
	IssueID string          `json:"issueId"`
	Role    string          `json:"role"`
	Issue   *IssueReference `json:"issue,omitempty"`
}

type IssueOperation struct {
	ID           string                      `json:"id"`
	Kind         IssueOperationKind          `json:"kind"`
	CreatedBy    string                      `json:"createdBy"`
	CreatedAt    time.Time                   `json:"createdAt"`
	Note         string                      `json:"note,omitempty"`
	Participants []IssueOperationParticipant `json:"participants,omitempty"`
}

type SplitChildInput struct {
	Raw       string
	Tags      []string
	TagScores []TagRelevance
	Embedding []float64
}

type SplitInput struct {
	SourceID    string
	Children    []SplitChildInput
	CreatedBy   string
	Note        string
	CloseSource bool
}

type CombineInput struct {
	SourceIDs []string
	Raw       string
	Tags      []string
	CreatedBy string
	Note      string
	TagScores []TagRelevance
	Embedding []float64
}

type LinkInput struct {
	SourceID  string
	TargetID  string
	Type      IssueLinkType
	CreatedBy string
	Note      string
}

type IssueOperationResult struct {
	Operation     IssueOperation   `json:"operation"`
	CreatedIssues []Issue          `json:"createdIssues,omitempty"`
	TouchedIssues []IssueReference `json:"touchedIssues,omitempty"`
}

type CreateInput struct {
	Raw       string
	Tags      []string
	CreatedBy string
	TagScores []TagRelevance
	Embedding []float64
}

type RefineInput struct {
	PostRaw      string
	CanonicalRaw string
	Tags         []string
	CreatedBy    string
	TagScores    []TagRelevance
	Embedding    []float64
}

type ProgressInput struct {
	Raw       string
	CreatedBy string
}

type Store interface {
	List(context.Context) ([]Issue, error)
	Get(context.Context, string) (Issue, error)
	Create(context.Context, CreateInput) (Issue, error)
	Refine(context.Context, string, RefineInput) (Issue, error)
	ProgressPost(context.Context, string, ProgressInput) (Issue, error)
	CloseIssue(context.Context, string, string) (Issue, error)
	ReopenIssue(context.Context, string) (Issue, error)
	AssignIssue(ctx context.Context, id, assignee string) (Issue, error)
	SplitIssue(context.Context, SplitInput) (IssueOperationResult, error)
	CombineIssues(context.Context, CombineInput) (IssueOperationResult, error)
	LinkIssues(context.Context, LinkInput) (IssueOperationResult, error)
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
	mu         sync.RWMutex
	issues     []Issue
	discussion map[string][]IssuePost
	nextSeq    atomic.Uint64
	nextOpSeq  atomic.Uint64
	links      []IssueLink
	operations []IssueOperation
}

func NewInMemoryStore(seed []Issue) *InMemoryStore {
	store := &InMemoryStore{
		issues:     cloneIssues(seed),
		discussion: make(map[string][]IssuePost, len(seed)),
	}

	for _, issue := range store.issues {
		store.discussion[issue.ID] = initialDiscussion(issue)
	}

	slices.SortStableFunc(store.issues, compareIssueOrder)
	store.nextSeq.Store(uint64(len(seed)))
	return store
}

func (s *InMemoryStore) List(_ context.Context) ([]Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := cloneIssues(s.issues)
	for i := range items {
		items[i].Discussion = nil
	}
	return items, nil
}

func (s *InMemoryStore) Get(_ context.Context, id string) (Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id = strings.TrimSpace(id)
	for _, issue := range s.issues {
		if issue.ID == id {
			cloned := cloneIssues([]Issue{issue})[0]
			cloned.Discussion = cloneIssuePosts(s.discussion[id])
			s.hydrateIssueRelationships(&cloned)
			return cloned, nil
		}
	}

	return Issue{}, ErrNotFound
}

func (s *InMemoryStore) Create(_ context.Context, input CreateInput) (Issue, error) {
	if _, err := ValidateRaw(input.Raw, "raw"); err != nil {
		return Issue{}, err
	}

	issue := BuildNewIssue(fmt.Sprintf("issue-%06d", s.nextSeq.Add(1)), input)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.issues = append([]Issue{issue}, s.issues...)
	s.discussion[issue.ID] = cloneIssuePosts(issue.Discussion)
	return cloneIssues([]Issue{issue})[0], nil
}

func (s *InMemoryStore) Refine(_ context.Context, id string, input RefineInput) (Issue, error) {
	id, err := ValidateID(id)
	if err != nil {
		return Issue{}, err
	}
	postRaw, canonicalRaw, err := ValidateRefineInput(input)
	if err != nil {
		return Issue{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for index, issue := range s.issues {
		if issue.ID != id {
			continue
		}

		if err := EnsureMutable(issue); err != nil {
			return Issue{}, err
		}

		discussion := cloneIssuePosts(s.discussion[id])
		post := NewDiscussionPost(id, discussion, postRaw, input.CreatedBy, "refinement")
		discussion = append(discussion, post)

		ApplyRefinement(&issue, canonicalRaw, input)
		issue.Discussion = cloneIssuePosts(discussion)

		s.issues[index] = issue
		s.discussion[id] = cloneIssuePosts(discussion)

		return cloneIssues([]Issue{issue})[0], nil
	}

	return Issue{}, ErrNotFound
}

func (s *InMemoryStore) ProgressPost(_ context.Context, id string, input ProgressInput) (Issue, error) {
	id, err := ValidateID(id)
	if err != nil {
		return Issue{}, err
	}
	raw, err := ValidateRaw(input.Raw, "raw")
	if err != nil {
		return Issue{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for index, issue := range s.issues {
		if issue.ID != id {
			continue
		}

		if err := EnsureMutable(issue); err != nil {
			return Issue{}, err
		}

		discussion := cloneIssuePosts(s.discussion[id])
		post := NewDiscussionPost(id, discussion, raw, input.CreatedBy, "progress")
		discussion = append(discussion, post)

		issue.Discussion = cloneIssuePosts(discussion)
		s.issues[index] = issue
		s.discussion[id] = cloneIssuePosts(discussion)

		return cloneIssues([]Issue{issue})[0], nil
	}

	return Issue{}, ErrNotFound
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
			cloned := cloneIssues([]Issue{issue})[0]
			cloned.Discussion = cloneIssuePosts(s.discussion[id])
			return cloned, nil
		}

		closedAt := time.Now().UTC()
		issue.Status = StatusClosed
		issue.ClosedAt = &closedAt
		issue.ClosedBy = defaultActor(closedBy)
		discussion := appendActivityPost(
			s.discussion[id],
			id,
			closedAt,
			issue.ClosedBy,
			"closed",
			closeIssuePost(issue.ClosedBy),
		)
		issue.Discussion = cloneIssuePosts(discussion)
		s.discussion[id] = cloneIssuePosts(discussion)
		s.issues[index] = issue
		cloned := cloneIssues([]Issue{issue})[0]
		cloned.Discussion = cloneIssuePosts(discussion)
		return cloned, nil
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
			cloned := cloneIssues([]Issue{issue})[0]
			cloned.Discussion = cloneIssuePosts(s.discussion[id])
			return cloned, nil
		}

		issue.Status = StatusOpen
		issue.ClosedAt = nil
		issue.ClosedBy = ""
		reopenedAt := time.Now().UTC()
		discussion := appendActivityPost(
			s.discussion[id],
			id,
			reopenedAt,
			"",
			"reopened",
			reopenIssuePost(),
		)
		issue.Discussion = cloneIssuePosts(discussion)
		s.discussion[id] = cloneIssuePosts(discussion)
		s.issues[index] = issue
		cloned := cloneIssues([]Issue{issue})[0]
		cloned.Discussion = cloneIssuePosts(discussion)
		return cloned, nil
	}

	return Issue{}, ErrNotFound
}

func (s *InMemoryStore) AssignIssue(_ context.Context, id, assignee string) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	assignee = strings.TrimSpace(assignee)

	for index, issue := range s.issues {
		if issue.ID != id {
			continue
		}

		issue.AssignedTo = assignee
		assignedAt := time.Now().UTC()
		discussion := appendActivityPost(
			s.discussion[id],
			id,
			assignedAt,
			"",
			"assigned",
			assignIssuePost(assignee),
		)
		issue.Discussion = cloneIssuePosts(discussion)
		s.discussion[id] = cloneIssuePosts(discussion)
		s.issues[index] = issue
		cloned := cloneIssues([]Issue{issue})[0]
		cloned.Discussion = cloneIssuePosts(discussion)
		return cloned, nil
	}

	return Issue{}, ErrNotFound
}

func (s *InMemoryStore) SplitIssue(_ context.Context, input SplitInput) (IssueOperationResult, error) {
	sourceID := strings.TrimSpace(input.SourceID)
	if sourceID == "" {
		return IssueOperationResult{}, ErrNotFound
	}
	if len(input.Children) == 0 {
		return IssueOperationResult{}, fmt.Errorf("at least one child issue is required")
	}

	actor := defaultActor(input.CreatedBy)
	note := strings.TrimSpace(input.Note)

	s.mu.Lock()
	defer s.mu.Unlock()

	sourceIndex := -1
	for index, issue := range s.issues {
		if issue.ID == sourceID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex == -1 {
		return IssueOperationResult{}, ErrNotFound
	}

	source := s.issues[sourceIndex]
	createdAt := time.Now().UTC()
	operation := IssueOperation{
		ID:        fmt.Sprintf("issue-op-%06d", s.nextOpSeq.Add(1)),
		Kind:      IssueOperationKindSplit,
		CreatedBy: actor,
		CreatedAt: createdAt,
		Note:      note,
		Participants: []IssueOperationParticipant{{
			IssueID: sourceID,
			Role:    "source",
		}},
	}

	createdIssues := make([]Issue, 0, len(input.Children))
	touched := []IssueReference{issueReference(source)}
	for index, child := range input.Children {
		raw := strings.TrimSpace(child.Raw)
		if raw == "" {
			return IssueOperationResult{}, fmt.Errorf("child raw is required")
		}

		issue := Issue{
			ID:        fmt.Sprintf("issue-%06d", s.nextSeq.Add(1)),
			Raw:       raw,
			Tags:      displayTags(child.Tags, child.TagScores),
			CreatedBy: actor,
			CreatedAt: createdAt,
			Status:    StatusOpen,
			TagScores: copyTagScores(child.TagScores),
			Embedding: copyEmbedding(child.Embedding),
		}
		issue.Discussion = initialDiscussion(issue)
		s.issues = append([]Issue{issue}, s.issues...)
		s.discussion[issue.ID] = cloneIssuePosts(issue.Discussion)
		createdIssues = append(createdIssues, cloneIssues([]Issue{issue})[0])
		touched = append(touched, issueReference(issue))

		s.links = append(s.links,
			IssueLink{
				ID:            fmt.Sprintf("%s-link-%06d", operation.ID, len(createdIssues)*2-1),
				Type:          IssueLinkTypeParentOf,
				SourceIssueID: sourceID,
				TargetIssueID: issue.ID,
				CreatedBy:     actor,
				CreatedAt:     createdAt,
				Note:          note,
				OperationID:   operation.ID,
			},
			IssueLink{
				ID:            fmt.Sprintf("%s-link-%06d", operation.ID, len(createdIssues)*2),
				Type:          IssueLinkTypeChildOf,
				SourceIssueID: issue.ID,
				TargetIssueID: sourceID,
				CreatedBy:     actor,
				CreatedAt:     createdAt,
				Note:          note,
				OperationID:   operation.ID,
			},
		)
		operation.Participants = append(operation.Participants, IssueOperationParticipant{
			IssueID: issue.ID,
			Role:    fmt.Sprintf("child:%d", index+1),
		})
	}

	if input.CloseSource {
		source.Status = StatusClosed
		source.ClosedAt = &createdAt
		source.ClosedBy = actor
		for index, issue := range s.issues {
			if issue.ID == sourceID {
				s.issues[index] = source
				break
			}
		}
		touched[0] = issueReference(source)
	}

	s.operations = append([]IssueOperation{operation}, s.operations...)

	return IssueOperationResult{
		Operation:     hydrateOperation(operation, issuesByIDFromList(s.issues)),
		CreatedIssues: createdIssues,
		TouchedIssues: touched,
	}, nil
}

func (s *InMemoryStore) CombineIssues(_ context.Context, input CombineInput) (IssueOperationResult, error) {
	sourceIDs := sanitizeIssueIDs(input.SourceIDs)
	if len(sourceIDs) < 2 {
		return IssueOperationResult{}, fmt.Errorf("at least two source issues are required")
	}
	raw := strings.TrimSpace(input.Raw)
	if raw == "" {
		return IssueOperationResult{}, fmt.Errorf("raw is required")
	}

	actor := defaultActor(input.CreatedBy)
	note := strings.TrimSpace(input.Note)
	createdAt := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	indexByID := make(map[string]int, len(s.issues))
	for index, issue := range s.issues {
		indexByID[issue.ID] = index
	}
	for _, id := range sourceIDs {
		if _, ok := indexByID[id]; !ok {
			return IssueOperationResult{}, ErrNotFound
		}
	}

	createdIssue := Issue{
		ID:        fmt.Sprintf("issue-%06d", s.nextSeq.Add(1)),
		Raw:       raw,
		Tags:      displayTags(input.Tags, input.TagScores),
		CreatedBy: actor,
		CreatedAt: createdAt,
		Status:    StatusOpen,
		TagScores: copyTagScores(input.TagScores),
		Embedding: copyEmbedding(input.Embedding),
	}
	createdIssue.Discussion = initialDiscussion(createdIssue)
	s.issues = append([]Issue{createdIssue}, s.issues...)
	s.discussion[createdIssue.ID] = cloneIssuePosts(createdIssue.Discussion)

	operation := IssueOperation{
		ID:        fmt.Sprintf("issue-op-%06d", s.nextOpSeq.Add(1)),
		Kind:      IssueOperationKindCombine,
		CreatedBy: actor,
		CreatedAt: createdAt,
		Note:      note,
		Participants: []IssueOperationParticipant{{
			IssueID: createdIssue.ID,
			Role:    "result",
		}},
	}

	touched := []IssueReference{issueReference(createdIssue)}
	for index, id := range sourceIDs {
		source := s.issues[indexByID[id]+1]
		source.Status = StatusClosed
		source.ClosedAt = &createdAt
		source.ClosedBy = actor
		s.issues[indexByID[id]+1] = source
		touched = append(touched, issueReference(source))

		s.links = append(s.links, IssueLink{
			ID:            fmt.Sprintf("%s-link-%06d", operation.ID, index+1),
			Type:          IssueLinkTypeMergedInto,
			SourceIssueID: id,
			TargetIssueID: createdIssue.ID,
			CreatedBy:     actor,
			CreatedAt:     createdAt,
			Note:          note,
			OperationID:   operation.ID,
		})
		operation.Participants = append(operation.Participants, IssueOperationParticipant{
			IssueID: id,
			Role:    fmt.Sprintf("source:%d", index+1),
		})
	}

	s.operations = append([]IssueOperation{operation}, s.operations...)

	return IssueOperationResult{
		Operation:     hydrateOperation(operation, issuesByIDFromList(s.issues)),
		CreatedIssues: []Issue{cloneIssues([]Issue{createdIssue})[0]},
		TouchedIssues: touched,
	}, nil
}

func (s *InMemoryStore) LinkIssues(_ context.Context, input LinkInput) (IssueOperationResult, error) {
	sourceID := strings.TrimSpace(input.SourceID)
	targetID := strings.TrimSpace(input.TargetID)
	if sourceID == "" || targetID == "" {
		return IssueOperationResult{}, ErrNotFound
	}
	linkType := normalizeIssueLinkType(input.Type)
	if linkType == "" {
		return IssueOperationResult{}, fmt.Errorf("link type is required")
	}

	actor := defaultActor(input.CreatedBy)
	note := strings.TrimSpace(input.Note)
	createdAt := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	issuesByID := issuesByIDFromList(s.issues)
	source, ok := issuesByID[sourceID]
	if !ok {
		return IssueOperationResult{}, ErrNotFound
	}
	target, ok := issuesByID[targetID]
	if !ok {
		return IssueOperationResult{}, ErrNotFound
	}

	operation := IssueOperation{
		ID:        fmt.Sprintf("issue-op-%06d", s.nextOpSeq.Add(1)),
		Kind:      IssueOperationKindLink,
		CreatedBy: actor,
		CreatedAt: createdAt,
		Note:      note,
		Participants: []IssueOperationParticipant{
			{IssueID: sourceID, Role: "source"},
			{IssueID: targetID, Role: "target"},
		},
	}
	s.links = append(s.links, IssueLink{
		ID:            fmt.Sprintf("%s-link-000001", operation.ID),
		Type:          linkType,
		SourceIssueID: sourceID,
		TargetIssueID: targetID,
		CreatedBy:     actor,
		CreatedAt:     createdAt,
		Note:          note,
		OperationID:   operation.ID,
	})
	s.operations = append([]IssueOperation{operation}, s.operations...)

	return IssueOperationResult{
		Operation: hydrateOperation(operation, issuesByIDFromList(s.issues)),
		TouchedIssues: []IssueReference{
			issueReference(source),
			issueReference(target),
		},
	}, nil
}

func (s *InMemoryStore) Replace(_ context.Context, next []Issue) error {
	items := cloneIssues(next)
	slices.SortStableFunc(items, compareIssueOrder)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.issues = items
	s.discussion = make(map[string][]IssuePost, len(items))
	s.links = nil
	s.operations = nil
	s.nextOpSeq.Store(0)
	for _, issue := range items {
		s.discussion[issue.ID] = initialDiscussion(issue)
	}
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

	out := make([]string, 0, min(3, len(normalized)))
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
			ID:         issue.ID,
			Raw:        issue.Raw,
			Tags:       append([]string(nil), issue.Tags...),
			CreatedBy:  issue.CreatedBy,
			CreatedAt:  issue.CreatedAt,
			Status:     normalizeIssueStatus(issue.Status),
			ClosedAt:   cloneTimePtr(issue.ClosedAt),
			ClosedBy:   issue.ClosedBy,
			AssignedTo: issue.AssignedTo,
			Discussion: cloneIssuePosts(issue.Discussion),
			Links:      cloneIssueLinks(issue.Links),
			Operations: cloneIssueOperations(issue.Operations),
			TagScores:  copyTagScores(issue.TagScores),
			Embedding:  copyEmbedding(issue.Embedding),
		}
	}
	return items
}

func cloneIssuePosts(input []IssuePost) []IssuePost {
	if len(input) == 0 {
		return nil
	}

	items := make([]IssuePost, len(input))
	for i, post := range input {
		items[i] = IssuePost{
			ID:        post.ID,
			IssueID:   post.IssueID,
			Raw:       post.Raw,
			CreatedBy: post.CreatedBy,
			CreatedAt: post.CreatedAt,
			Sequence:  post.Sequence,
			Kind:      post.Kind,
		}
	}
	return items
}

func cloneIssueLinks(input []IssueLink) []IssueLink {
	if len(input) == 0 {
		return nil
	}

	items := make([]IssueLink, len(input))
	for i, link := range input {
		items[i] = IssueLink{
			ID:            link.ID,
			Type:          link.Type,
			SourceIssueID: link.SourceIssueID,
			TargetIssueID: link.TargetIssueID,
			Direction:     link.Direction,
			CreatedBy:     link.CreatedBy,
			CreatedAt:     link.CreatedAt,
			Note:          link.Note,
			OperationID:   link.OperationID,
			RelatedIssue:  cloneIssueReference(link.RelatedIssue),
		}
	}
	return items
}

func cloneIssueOperations(input []IssueOperation) []IssueOperation {
	if len(input) == 0 {
		return nil
	}

	items := make([]IssueOperation, len(input))
	for i, operation := range input {
		participants := make([]IssueOperationParticipant, len(operation.Participants))
		for j, participant := range operation.Participants {
			participants[j] = IssueOperationParticipant{
				IssueID: participant.IssueID,
				Role:    participant.Role,
				Issue:   cloneIssueReference(participant.Issue),
			}
		}
		items[i] = IssueOperation{
			ID:           operation.ID,
			Kind:         operation.Kind,
			CreatedBy:    operation.CreatedBy,
			CreatedAt:    operation.CreatedAt,
			Note:         operation.Note,
			Participants: participants,
		}
	}
	return items
}

func cloneIssueReference(input *IssueReference) *IssueReference {
	if input == nil {
		return nil
	}
	cloned := *input
	return &cloned
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
	return domain.NormalizeTagName(name)
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

func initialDiscussion(issue Issue) []IssuePost {
	if len(issue.Discussion) > 0 {
		return cloneIssuePosts(issue.Discussion)
	}

	return []IssuePost{{
		ID:        issuePostID(issue.ID, 1),
		IssueID:   issue.ID,
		Raw:       issue.Raw,
		CreatedBy: issue.CreatedBy,
		CreatedAt: issue.CreatedAt,
		Sequence:  1,
	}}
}

func issuePostID(issueID string, sequence int) string {
	return fmt.Sprintf("%s-post-%06d", strings.TrimSpace(issueID), sequence)
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

func normalizeIssueLinkType(value IssueLinkType) IssueLinkType {
	switch strings.TrimSpace(string(value)) {
	case string(IssueLinkTypeParentOf):
		return IssueLinkTypeParentOf
	case string(IssueLinkTypeChildOf):
		return IssueLinkTypeChildOf
	case string(IssueLinkTypeMergedInto):
		return IssueLinkTypeMergedInto
	case string(IssueLinkTypeDerivedFrom):
		return IssueLinkTypeDerivedFrom
	case string(IssueLinkTypeRelatedTo):
		return IssueLinkTypeRelatedTo
	case string(IssueLinkTypeDuplicateOf):
		return IssueLinkTypeDuplicateOf
	default:
		return ""
	}
}

func sanitizeIssueIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func issueReference(issue Issue) IssueReference {
	return IssueReference{
		ID:     issue.ID,
		Raw:    issue.Raw,
		Status: normalizeIssueStatus(issue.Status),
	}
}

func issuesByIDFromList(items []Issue) map[string]Issue {
	index := make(map[string]Issue, len(items))
	for _, issue := range items {
		index[issue.ID] = issue
	}
	return index
}

func hydrateOperation(operation IssueOperation, issuesByID map[string]Issue) IssueOperation {
	hydrated := operation
	hydrated.Participants = make([]IssueOperationParticipant, len(operation.Participants))
	for i, participant := range operation.Participants {
		hydrated.Participants[i] = participant
		if issue, ok := issuesByID[participant.IssueID]; ok {
			ref := issueReference(issue)
			hydrated.Participants[i].Issue = &ref
		}
	}
	return hydrated
}

func (s *InMemoryStore) hydrateIssueRelationships(issue *Issue) {
	issuesByID := issuesByIDFromList(s.issues)
	links := make([]IssueLink, 0)
	for _, link := range s.links {
		if link.SourceIssueID != issue.ID && link.TargetIssueID != issue.ID {
			continue
		}
		hydrated := link
		if hydrated.SourceIssueID == issue.ID {
			hydrated.Direction = "outgoing"
			if related, ok := issuesByID[hydrated.TargetIssueID]; ok {
				ref := issueReference(related)
				hydrated.RelatedIssue = &ref
			}
		} else {
			hydrated.Direction = "incoming"
			if related, ok := issuesByID[hydrated.SourceIssueID]; ok {
				ref := issueReference(related)
				hydrated.RelatedIssue = &ref
			}
		}
		links = append(links, hydrated)
	}
	operations := make([]IssueOperation, 0)
	for _, operation := range s.operations {
		for _, participant := range operation.Participants {
			if participant.IssueID == issue.ID {
				operations = append(operations, hydrateOperation(operation, issuesByID))
				break
			}
		}
	}
	issue.Links = cloneIssueLinks(links)
	issue.Operations = cloneIssueOperations(operations)
}
