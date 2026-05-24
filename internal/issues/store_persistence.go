package issues

import (
	"context"
	"slices"
	"strings"
	"time"

	"sortit/internal/domain"
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
		if snapshot, ok := issueSnapshotFromFieldUpdate(id, fields); ok {
			s.snapshots[id] = appendOrReplaceIssueSnapshot(s.snapshots[id], snapshot)
		}
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

func (s *InMemoryStore) MergeTags(_ context.Context, canonical string, aliases []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	aliasSet := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		aliasSet[sanitizeTagName(alias)] = struct{}{}
	}
	canonicalNorm := sanitizeTagName(canonical)

	for i, issue := range s.issues {
		s.issues[i].Tags = mergeTagList(issue.Tags, canonicalNorm, aliasSet)
		s.issues[i].TagScores = mergeTagScores(issue.TagScores, canonicalNorm, aliasSet)
	}
	return nil
}

func (s *InMemoryStore) DismissTagMerge(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *InMemoryStore) ListDismissedTagMerges(_ context.Context) ([]DismissedTagMerge, error) {
	return nil, nil
}

func mergeTagList(tags []string, canonical string, aliases map[string]struct{}) []string {
	hasCanonical := false
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		norm := sanitizeTagName(tag)
		if _, isAlias := aliases[norm]; isAlias {
			if !hasCanonical {
				out = append(out, canonical)
				hasCanonical = true
			}
			continue
		}
		if norm == canonical {
			hasCanonical = true
		}
		out = append(out, tag)
	}
	return out
}

func mergeTagScores(scores []TagRelevance, canonical string, aliases map[string]struct{}) []TagRelevance {
	if len(scores) == 0 {
		return scores
	}

	canonicalScore := TagRelevance{Tag: canonical}
	hasCanonical := false
	out := make([]TagRelevance, 0, len(scores))
	for _, score := range scores {
		norm := sanitizeTagName(score.Tag)
		if _, isAlias := aliases[norm]; isAlias {
			if score.Relevance > canonicalScore.Relevance {
				canonicalScore = score
				canonicalScore.Tag = canonical
			}
			continue
		}
		if norm == canonical {
			hasCanonical = true
			if score.Relevance > canonicalScore.Relevance {
				canonicalScore = score
				canonicalScore.Tag = canonical
			}
			continue
		}
		out = append(out, score)
	}
	if hasCanonical || canonicalScore.Relevance > 0 {
		out = append([]TagRelevance{canonicalScore}, out...)
	}
	return out
}

func equalTagScores(left, right []TagRelevance) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !equalTagScore(left[i], right[i]) {
			return false
		}
	}
	return true
}

func equalTagScore(left, right TagRelevance) bool {
	return left.Tag == right.Tag &&
		left.Relevance == right.Relevance &&
		left.Suggested == right.Suggested &&
		left.Description == right.Description &&
		slices.Equal(left.CandidateSources, right.CandidateSources) &&
		equalFloat64Ptr(left.Alignment, right.Alignment) &&
		equalFloat64Ptr(left.Specificity, right.Specificity) &&
		left.VerificationVerdict == right.VerificationVerdict &&
		left.VerificationReason == right.VerificationReason &&
		left.DominatedBy == right.DominatedBy &&
		equalFloat64Ptr(left.DominanceGap, right.DominanceGap) &&
		equalEvidenceRanges(left.Evidence, right.Evidence)
}

func equalEvidenceRanges(left, right []domain.EvidenceRange) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalFloat64Ptr(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
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

func (s *InMemoryStore) ListIssueSnapshots(_ context.Context, id string) ([]IssueSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return cloneIssueSnapshots(s.snapshots[strings.TrimSpace(id)]), nil
}

func appendOrReplaceIssueSnapshot(existing []IssueSnapshot, snapshot IssueSnapshot) []IssueSnapshot {
	for i, item := range existing {
		if item.Sequence == snapshot.Sequence {
			out := cloneIssueSnapshots(existing)
			out[i] = snapshot
			return out
		}
	}

	out := cloneIssueSnapshots(existing)
	out = append(out, snapshot)
	return out
}
