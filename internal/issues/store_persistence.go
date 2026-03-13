package issues

import (
	"context"
	"strings"
	"time"
)

func (s *InMemoryStore) SaveIssue(_ context.Context, issue Issue) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	issue = normalizeIssueEnrichment(issue)

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
		if fields.EnrichmentStatus != nil {
			issue.EnrichmentStatus = *fields.EnrichmentStatus
		}
		if fields.EnrichmentError != nil {
			issue.EnrichmentError = *fields.EnrichmentError
		}
		if fields.EnrichmentTargetSequence != nil {
			issue.EnrichmentTargetSequence = *fields.EnrichmentTargetSequence
		}

		s.issues[index] = normalizeIssueEnrichment(issue)
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

	sourceID := strings.TrimSpace(link.SourceIssueID)
	targetID := strings.TrimSpace(link.TargetIssueID)
	if sourceID == "" || targetID == "" || normalizeIssueLinkType(link.Type) == "" {
		return ErrInvalidIssueLink
	}
	if sourceID == targetID {
		return ErrInvalidIssueLink
	}
	for _, existing := range s.links {
		if existing.SourceIssueID == sourceID &&
			existing.TargetIssueID == targetID &&
			existing.Type == normalizeIssueLinkType(link.Type) {
			return ErrDuplicateIssueLink
		}
	}

	s.links = append(s.links, link)
	return nil
}

func (s *InMemoryStore) EnqueueIssueEnrichment(_ context.Context, issueID string, targetSequence int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.enrichmentJobs[issueID]
	if current.TargetSequence > targetSequence {
		targetSequence = current.TargetSequence
	}
	current.IssueID = strings.TrimSpace(issueID)
	current.TargetSequence = targetSequence
	current.AvailableAt = time.Time{}
	s.enrichmentJobs[issueID] = current
	return nil
}

func (s *InMemoryStore) ClaimNextIssueEnrichment(_ context.Context, leaseDuration time.Duration) (IssueEnrichmentJob, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	var (
		selected inMemoryEnrichmentJob
		found    bool
	)
	for _, job := range s.enrichmentJobs {
		if !job.AvailableAt.IsZero() && job.AvailableAt.After(now) {
			continue
		}
		if !found || job.AvailableAt.Before(selected.AvailableAt) || selected.AvailableAt.IsZero() {
			selected = job
			found = true
		}
	}
	if !found {
		return IssueEnrichmentJob{}, false, nil
	}

	selected.AttemptCount++
	selected.AvailableAt = now.Add(leaseDuration)
	s.enrichmentJobs[selected.IssueID] = selected
	return selected.IssueEnrichmentJob, true, nil
}

func (s *InMemoryStore) CompleteIssueEnrichment(_ context.Context, issueID string, targetSequence int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.enrichmentJobs[issueID]
	if ok && job.TargetSequence == targetSequence {
		delete(s.enrichmentJobs, issueID)
	}
	return nil
}

func (s *InMemoryStore) RetryIssueEnrichment(
	_ context.Context,
	issueID string,
	targetSequence int,
	nextAttemptAt time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.enrichmentJobs[issueID]
	if !ok || job.TargetSequence != targetSequence {
		return nil
	}
	job.AvailableAt = nextAttemptAt.UTC()
	s.enrichmentJobs[issueID] = job
	return nil
}
