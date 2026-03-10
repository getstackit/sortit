package issues

import (
	"fmt"
	"strings"
	"time"
)

// ValidateID trims and validates an issue ID is not empty.
func ValidateID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ErrNotFound
	}
	return id, nil
}

// ValidateRaw trims and validates raw content is not empty.
func ValidateRaw(raw string, fieldName string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	return raw, nil
}

// ValidateRefineInput validates and trims both PostRaw and CanonicalRaw.
func ValidateRefineInput(input RefineInput) (postRaw, canonicalRaw string, err error) {
	postRaw, err = ValidateRaw(input.PostRaw, "post raw")
	if err != nil {
		return "", "", err
	}
	canonicalRaw, err = ValidateRaw(input.CanonicalRaw, "canonical raw")
	if err != nil {
		return "", "", err
	}
	return postRaw, canonicalRaw, nil
}

// EnsureMutable returns ErrIssueClosed if the issue is closed.
func EnsureMutable(issue Issue) error {
	if issue.Status == StatusClosed {
		return ErrIssueClosed
	}
	return nil
}

// NewDiscussionPost creates a new discussion post with the correct sequence.
func NewDiscussionPost(issueID string, discussion []IssuePost, raw, createdBy, kind string) IssuePost {
	return IssuePost{
		ID:        issuePostID(issueID, len(discussion)+1),
		IssueID:   issueID,
		Raw:       raw,
		CreatedBy: defaultActor(createdBy),
		CreatedAt: time.Now().UTC(),
		Sequence:  len(discussion) + 1,
		Kind:      kind,
	}
}

// ApplyRefinement updates an issue's canonical fields from a RefineInput.
func ApplyRefinement(issue *Issue, canonicalRaw string, input RefineInput) {
	issue.Raw = canonicalRaw
	issue.Tags = displayTags(input.Tags, input.TagScores)
	issue.TagScores = copyTagScores(input.TagScores)
	issue.Embedding = copyEmbedding(input.Embedding)
}

// BuildNewIssue creates a new Issue from CreateInput with proper defaults.
func BuildNewIssue(id string, input CreateInput) Issue {
	issue := Issue{
		ID:        id,
		Raw:       strings.TrimSpace(input.Raw),
		Tags:      displayTags(input.Tags, input.TagScores),
		CreatedBy: defaultActor(input.CreatedBy),
		CreatedAt: time.Now().UTC(),
		Status:    StatusOpen,
		TagScores: copyTagScores(input.TagScores),
		Embedding: copyEmbedding(input.Embedding),
	}
	issue.Discussion = initialDiscussion(issue)
	return issue
}
