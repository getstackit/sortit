package issues

import (
	"context"
	"strings"
)

func (s *InMemoryStore) SaveIssue(_ context.Context, issue Issue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if issue already exists (update)
	for index, existing := range s.issues {
		if existing.ID == issue.ID {
			s.issues[index] = cloneIssues([]Issue{issue})[0]
			s.discussion[issue.ID] = cloneIssuePosts(issue.Discussion)
			return nil
		}
	}

	// New issue — prepend
	cloned := cloneIssues([]Issue{issue})[0]
	s.issues = append([]Issue{cloned}, s.issues...)
	s.discussion[issue.ID] = cloneIssuePosts(issue.Discussion)
	return nil
}

func (s *InMemoryStore) SaveIssuePost(_ context.Context, post IssuePost) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	discussion := cloneIssuePosts(s.discussion[post.IssueID])
	discussion = append(discussion, post)
	s.discussion[post.IssueID] = cloneIssuePosts(discussion)

	// Also update the issue's Discussion field if the issue exists
	for index, issue := range s.issues {
		if issue.ID == post.IssueID {
			s.issues[index].Discussion = cloneIssuePosts(discussion)
			break
		}
	}
	return nil
}

func (s *InMemoryStore) UpdateIssueFields(_ context.Context, id string, fields IssueFieldUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	for index, issue := range s.issues {
		if issue.ID != id {
			continue
		}

		if fields.Raw != nil {
			issue.Raw = *fields.Raw
		}
		if fields.Tags != nil {
			issue.Tags = append([]string(nil), fields.Tags...)
		}
		if fields.TagScores != nil {
			issue.TagScores = copyTagScores(fields.TagScores)
		}
		if fields.Embedding != nil {
			issue.Embedding = copyEmbedding(fields.Embedding)
		}
		if fields.Status != nil {
			issue.Status = *fields.Status
		}
		if fields.ClosedAt != nil {
			t := fields.ClosedAt.UTC()
			issue.ClosedAt = &t
		}
		if fields.ClosedBy != nil {
			issue.ClosedBy = *fields.ClosedBy
		}
		if fields.AssignedTo != nil {
			issue.AssignedTo = *fields.AssignedTo
		}

		s.issues[index] = issue
		return nil
	}

	return ErrNotFound
}

func (s *InMemoryStore) SaveOperation(_ context.Context, op IssueOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.operations = append([]IssueOperation{op}, s.operations...)
	return nil
}

func (s *InMemoryStore) SaveLink(_ context.Context, link IssueLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.links = append(s.links, link)
	return nil
}

