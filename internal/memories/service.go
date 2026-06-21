// Package memories provides permanent, tag/region-anchored knowledge artifacts.
// Memories reuse the issue math primitives (embedding + tag-relevance profile)
// via the enrichment pipeline, but live in their own store with a permanent,
// reinforced lifecycle so they never pollute the issue corpus they derive from.
package memories

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"sortit/internal/domain"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issues"
	"sortit/internal/tags"
)

// ErrProposalNotPending is returned when accepting or rejecting a proposal that
// has already been resolved.
var ErrProposalNotPending = errors.New("memory proposal is not pending")

// anchorTagRelevanceFloor is the minimum relevance for a scored tag to be
// auto-promoted into a memory's anchor set when the author supplied none.
const anchorTagRelevanceFloor = 0.4

// maxDerivedAnchorTags caps how many tags are auto-derived as anchors.
const maxDerivedAnchorTags = 3

// TextEnricher turns raw text into a tag-relevance profile and embedding using
// the existing issue enrichment pipeline. *issueenrichment.IssueEnricher
// satisfies this; it may be nil when no analyzer is configured.
type TextEnricher interface {
	AnalyzeText(ctx context.Context, raw string, opts issueenrichment.AnalyzeTextOptions) (issueenrichment.AnalyzeTextResult, error)
	// EmbedText returns the embedding for raw text without a full tag analysis,
	// powering query-time memory recall.
	EmbedText(ctx context.Context, raw string) ([]float64, error)
}

// CorpusReader exposes the issue corpus that synthesis reads from.
type CorpusReader interface {
	List(ctx context.Context) ([]issues.Issue, error)
}

// ConceptProfiler generates the prose body for a concept memory from its subject
// tag and a sample of issues that reference it. *ai.Analyzer satisfies it. It is
// optional: without one, concept proposals keep their deterministic placeholder
// body (the pure synthesizer stays pure; this only enriches concept drafts).
type ConceptProfiler interface {
	GenerateConceptProfile(ctx context.Context, tag string, issueSummaries []string) (string, error)
}

// TagCatalogReader exposes tag specificity so concept synthesis can skip generic
// bucket tags. The issue store satisfies it. Optional: without it, concept
// synthesis falls back to frequency only.
type TagCatalogReader interface {
	ListTags(ctx context.Context) ([]issues.Tag, error)
}

// ConceptTagSeeder ensures a concept's subject tag exists as a first-class
// catalog tag, so authoring concepts grows the project's tagging vocabulary —
// the curated nouns then enter the enrichment candidate set. *tags.CatalogService
// satisfies it (EnsureStoredTags embeds and persists). Optional.
type ConceptTagSeeder interface {
	EnsureStoredTags(ctx context.Context, tags []issues.Tag) error
}

// Service is the application layer for memories.
type Service struct {
	store      issues.MemoryStore
	enricher   TextEnricher
	logger     *slog.Logger
	proposals  issues.MemoryProposalStore
	corpus     CorpusReader
	profiler   ConceptProfiler
	tagCatalog TagCatalogReader
	tagSeeder  ConceptTagSeeder
}

// NewService constructs a memory service. enricher may be nil (memories are
// then persisted without tag scores/embeddings until re-enriched).
func NewService(store issues.MemoryStore, enricher TextEnricher, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, enricher: enricher, logger: logger.With("component", "memories")}
}

// UseSynthesis enables the synthesis proposal queue (hybrid creation): the
// system drafts candidate memories from the corpus for human review. Pass nil
// stores to leave synthesis disabled.
func (s *Service) UseSynthesis(proposals issues.MemoryProposalStore, corpus CorpusReader) {
	s.proposals = proposals
	s.corpus = corpus
}

// UseConceptProfiler enables LLM-generated bodies for synthesized concept
// proposals. Optional: without it, concept proposals keep the deterministic
// placeholder body the pure synthesizer drafts.
func (s *Service) UseConceptProfiler(profiler ConceptProfiler) {
	s.profiler = profiler
}

// UseTagCatalog enables the specificity gate for concept synthesis, so generic
// bucket tags don't become concepts. Optional: without it, concept synthesis
// gates on frequency only.
func (s *Service) UseTagCatalog(reader TagCatalogReader) {
	s.tagCatalog = reader
}

// UseConceptTagSeeder makes authoring a concept register its subject tag in the
// catalog, growing the tagging vocabulary. Optional: without it, concepts are
// created but their subject tags are not seeded into the catalog.
func (s *Service) UseConceptTagSeeder(seeder ConceptTagSeeder) {
	s.tagSeeder = seeder
}

// CreateMemoryInput describes a new memory.
type CreateMemoryInput struct {
	Title      string
	Body       string
	Kind       domain.MemoryKind
	AnchorTags []string
	// SubjectTag is required when Kind is concept: the single tag the concept
	// profiles. Ignored for every other kind.
	SubjectTag     string
	AnchorRegion   string
	CreatedBy      string
	Source         domain.MemorySource
	SourceIssueIDs []string
	Confidence     float64
}

// CreateMemory validates, enriches, and persists a new memory.
func (s *Service) CreateMemory(ctx context.Context, input CreateMemoryInput) (domain.Memory, error) {
	body, err := issues.ValidateRaw(input.Body, "body")
	if err != nil {
		return domain.Memory{}, err
	}

	confidence := input.Confidence
	if confidence <= 0 {
		confidence = 1
	}

	kind := domain.NormalizeMemoryKind(input.Kind)
	subjectTag := domain.NormalizeSubjectTag(kind, input.SubjectTag)
	if kind == domain.MemoryKindConcept && subjectTag == "" {
		return domain.Memory{}, fmt.Errorf("a concept memory requires a subject tag")
	}

	anchorTags := normalizeTagList(input.AnchorTags)
	if kind == domain.MemoryKindConcept {
		// A concept is anchored on the tag it profiles, so recall and enrichment
		// see it sitting on its subject node. The subject leads the anchor set.
		anchorTags = normalizeTagList(append([]string{subjectTag}, anchorTags...))
	}

	memory := domain.Memory{
		ID:             issues.NewMemoryID(),
		Title:          strings.TrimSpace(input.Title),
		Body:           body,
		Kind:           kind,
		SubjectTag:     subjectTag,
		AnchorTags:     anchorTags,
		AnchorRegion:   strings.TrimSpace(input.AnchorRegion),
		Status:         domain.MemoryStatusActive,
		Source:         domain.NormalizeMemorySource(input.Source),
		SourceIssueIDs: normalizeIDList(input.SourceIssueIDs),
		Confidence:     confidence,
		CreatedBy:      strings.TrimSpace(input.CreatedBy),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	s.enrich(ctx, &memory)

	// A concept is bound 1:1 to its tag: supersede any existing active concept
	// for the same subject before inserting, or the unique index would reject it.
	if err := s.supersedePriorConcept(ctx, memory); err != nil {
		return domain.Memory{}, err
	}

	if err := s.store.UpsertMemory(ctx, memory); err != nil {
		return domain.Memory{}, fmt.Errorf("save memory: %w", err)
	}
	s.logger.InfoContext(ctx, "memory created",
		"memory_id", memory.ID,
		"kind", memory.Kind,
		"subject_tag", memory.SubjectTag,
		"anchor_tags", len(memory.AnchorTags),
		"scored_tags", len(memory.TagScores),
		"has_embedding", len(memory.Embedding) > 0,
	)
	s.seedConceptTag(ctx, memory)
	return memory, nil
}

// conceptTagDescriptionMaxChars bounds how much of a concept body seeds the
// catalog tag description (which is embedded for retrieval).
const conceptTagDescriptionMaxChars = 280

// seedConceptTag registers a concept's subject tag in the catalog so the curated
// noun becomes part of the tagging vocabulary — and thus an enrichment candidate
// that future issues can be tagged with. Best-effort: a failure is logged, never
// fatal to memory creation.
func (s *Service) seedConceptTag(ctx context.Context, memory domain.Memory) {
	if s.tagSeeder == nil || memory.Kind != domain.MemoryKindConcept || memory.SubjectTag == "" {
		return
	}
	description := strings.TrimSpace(memory.Title)
	if body := strings.TrimSpace(memory.Body); body != "" {
		summary := truncate(body, conceptTagDescriptionMaxChars)
		if description == "" {
			description = summary
		} else {
			description += ". " + summary
		}
	}
	if err := s.tagSeeder.EnsureStoredTags(ctx, []issues.Tag{{Name: memory.SubjectTag, Description: description}}); err != nil {
		s.logger.WarnContext(ctx, "failed to seed concept subject tag into catalog",
			"subject_tag", memory.SubjectTag, "error", err)
		return
	}
	s.logger.InfoContext(ctx, "seeded concept subject tag into catalog", "subject_tag", memory.SubjectTag)
}

// supersedePriorConcept retires the active concept (if any) already bound to the
// new memory's subject tag, pointing it at the replacement. No-op for non-concept
// memories. This keeps the 1:1 concept↔tag binding the unique index enforces.
func (s *Service) supersedePriorConcept(ctx context.Context, replacement domain.Memory) error {
	if replacement.Kind != domain.MemoryKindConcept {
		return nil
	}
	prior, err := s.store.GetActiveConceptBySubjectTag(ctx, replacement.SubjectTag)
	if errors.Is(err, issues.ErrMemoryNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look up prior concept: %w", err)
	}
	if prior.ID == replacement.ID {
		return nil
	}
	prior.Status = domain.MemoryStatusSuperseded
	prior.SupersededBy = replacement.ID
	prior.UpdatedAt = time.Now().UTC()
	if err := s.store.UpsertMemory(ctx, prior); err != nil {
		return fmt.Errorf("supersede prior concept %q: %w", prior.ID, err)
	}
	s.logger.InfoContext(ctx, "superseded prior concept",
		"subject_tag", replacement.SubjectTag,
		"prior_id", prior.ID,
		"replacement_id", replacement.ID,
	)
	return nil
}

// GetMemory returns one memory or issues.ErrMemoryNotFound.
func (s *Service) GetMemory(ctx context.Context, id string) (domain.Memory, error) {
	return s.store.GetMemory(ctx, id)
}

// ListMemories returns memories matching opts, most-recent first.
func (s *Service) ListMemories(ctx context.Context, opts issues.MemoryListOptions) ([]domain.Memory, error) {
	return s.store.ListMemories(ctx, opts)
}

// ConceptForTag returns the active concept memory bound to subjectTag, or
// issues.ErrMemoryNotFound. It powers the tag↔concept cross-link.
func (s *Service) ConceptForTag(ctx context.Context, subjectTag string) (domain.Memory, error) {
	return s.store.GetActiveConceptBySubjectTag(ctx, subjectTag)
}

// ReinforceConceptsForTags strengthens the active concept bound to each given
// tag — bumping ReinforcementCount and stamping LastReinforcedAt — so concepts
// whose subject tags keep recurring in the corpus grow more prominent. This is
// the reinforced-permanence model: related activity strengthens a memory rather
// than letting it decay. Tags without an active concept are skipped; duplicates
// are deduped so one enrichment reinforces a concept at most once. Returns the
// number of concepts reinforced.
func (s *Service) ReinforceConceptsForTags(ctx context.Context, tagNames []string) (int, error) {
	seen := make(map[string]bool, len(tagNames))
	now := time.Now().UTC()
	reinforced := 0
	for _, raw := range tagNames {
		tag := domain.NormalizeTagName(raw)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true

		concept, err := s.store.GetActiveConceptBySubjectTag(ctx, tag)
		if errors.Is(err, issues.ErrMemoryNotFound) {
			continue
		}
		if err != nil {
			return reinforced, fmt.Errorf("look up concept for %q: %w", tag, err)
		}
		concept.ReinforcementCount++
		concept.LastReinforcedAt = &now
		concept.UpdatedAt = now
		if err := s.store.UpsertMemory(ctx, concept); err != nil {
			return reinforced, fmt.Errorf("reinforce concept %q: %w", concept.ID, err)
		}
		reinforced++
	}
	return reinforced, nil
}

// SupersedeMemory marks a memory as superseded — the "system evolves" path.
// supersededBy optionally records the memory that replaces it.
func (s *Service) SupersedeMemory(ctx context.Context, id, supersededBy string) (domain.Memory, error) {
	return s.transition(ctx, id, domain.MemoryStatusSuperseded, strings.TrimSpace(supersededBy))
}

// ArchiveMemory retires a memory without pointing at a replacement.
func (s *Service) ArchiveMemory(ctx context.Context, id string) (domain.Memory, error) {
	return s.transition(ctx, id, domain.MemoryStatusArchived, "")
}

func (s *Service) transition(ctx context.Context, id string, status domain.MemoryStatus, supersededBy string) (domain.Memory, error) {
	memory, err := s.store.GetMemory(ctx, id)
	if err != nil {
		return domain.Memory{}, err
	}
	if supersededBy != "" && supersededBy == memory.ID {
		return domain.Memory{}, fmt.Errorf("a memory cannot supersede itself")
	}
	memory.Status = status
	memory.SupersededBy = supersededBy
	memory.UpdatedAt = time.Now().UTC()
	if err := s.store.UpsertMemory(ctx, memory); err != nil {
		return domain.Memory{}, fmt.Errorf("save memory transition: %w", err)
	}
	s.logger.InfoContext(ctx, "memory transitioned",
		"memory_id", memory.ID,
		"status", status,
		"superseded_by", supersededBy,
	)
	return memory, nil
}

// SynthesizeProposals scans the corpus for clusters of closed decisions and
// drafts memory proposals for human review. Existing pending proposals and
// active memories are respected so reruns don't duplicate. Returns the newly
// created proposals.
func (s *Service) SynthesizeProposals(ctx context.Context) ([]domain.MemoryProposal, error) {
	if s.proposals == nil || s.corpus == nil {
		return nil, fmt.Errorf("memory synthesis is not configured")
	}
	corpusIssues, err := s.corpus.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load corpus for synthesis: %w", err)
	}
	pending, err := s.proposals.ListMemoryProposals(ctx, domain.MemoryProposalStatusPending)
	if err != nil {
		return nil, fmt.Errorf("load pending proposals: %w", err)
	}
	active, err := s.store.ListMemories(ctx, issues.MemoryListOptions{Status: domain.MemoryStatusActive})
	if err != nil {
		return nil, fmt.Errorf("load active memories: %w", err)
	}

	drafts := SynthesizeMemoryProposals(corpusIssues, pending, active)
	// Concepts profile load-bearing tags; decisions consolidate closed-issue
	// choices. They draft independently and dedup on different keys (subject_tag
	// vs anchor tags), so a tag can earn both. Concept synthesis is also gated by
	// tag specificity so generic bucket tags don't become concepts.
	drafts = append(drafts, SynthesizeConceptProposals(corpusIssues, pending, active, s.tagSpecificities(ctx))...)
	// Replace each concept draft's deterministic placeholder body with an
	// LLM-generated profile, when a profiler is configured. Best-effort: a
	// generation failure leaves the placeholder in place.
	s.fillConceptBodies(ctx, drafts, corpusIssues)
	created := make([]domain.MemoryProposal, 0, len(drafts))
	now := time.Now().UTC()
	for _, draft := range drafts {
		draft.ID = issues.NewMemoryProposalID()
		draft.CreatedAt = now
		draft.UpdatedAt = now
		if err := s.proposals.UpsertMemoryProposal(ctx, draft); err != nil {
			return nil, fmt.Errorf("save proposal: %w", err)
		}
		created = append(created, draft)
	}
	s.logger.InfoContext(ctx, "synthesized memory proposals", "count", len(created))
	return created, nil
}

// tagSpecificities loads the catalog's per-tag specificity scores for the
// concept specificity gate. Returns nil when no catalog reader is configured or
// the load fails — concept synthesis then gates on frequency alone.
func (s *Service) tagSpecificities(ctx context.Context) map[string]float64 {
	if s.tagCatalog == nil {
		return nil
	}
	tags, err := s.tagCatalog.ListTags(ctx)
	if err != nil {
		s.logger.WarnContext(ctx, "load tag specificities for concept synthesis failed; gating on frequency only", "error", err)
		return nil
	}
	out := make(map[string]float64, len(tags))
	for _, tag := range tags {
		if tag.Specificity != nil {
			out[domain.NormalizeTagName(tag.Name)] = *tag.Specificity
		}
	}
	return out
}

// fillConceptBodies replaces each concept draft's placeholder body with an
// LLM-generated profile built from the tag and a sample of the issues that
// reference it. No-op without a profiler; a per-draft failure keeps the
// placeholder so synthesis never fails on generation.
func (s *Service) fillConceptBodies(ctx context.Context, drafts []domain.MemoryProposal, corpus []issues.Issue) {
	if s.profiler == nil {
		return
	}
	byID := make(map[string]issues.Issue, len(corpus))
	for _, issue := range corpus {
		byID[issue.ID] = issue
	}
	for i := range drafts {
		if drafts[i].Kind != domain.MemoryKindConcept {
			continue
		}
		summaries := make([]string, 0, len(drafts[i].SourceIssueIDs))
		for _, id := range drafts[i].SourceIssueIDs {
			issue, ok := byID[id]
			if !ok {
				continue
			}
			if raw := strings.TrimSpace(issue.Raw); raw != "" {
				summaries = append(summaries, raw)
			}
			if len(summaries) >= maxSourceIssuesPerProposal {
				break
			}
		}
		body, err := s.profiler.GenerateConceptProfile(ctx, drafts[i].SubjectTag, summaries)
		if err != nil {
			s.logger.WarnContext(ctx, "concept profile generation failed; keeping placeholder",
				"subject_tag", drafts[i].SubjectTag, "error", err)
			continue
		}
		if body = strings.TrimSpace(body); body != "" {
			drafts[i].Body = body
		}
	}
}

// ListProposals returns synthesis proposals, optionally filtered by status.
func (s *Service) ListProposals(ctx context.Context, status domain.MemoryProposalStatus) ([]domain.MemoryProposal, error) {
	if s.proposals == nil {
		return nil, fmt.Errorf("memory synthesis is not configured")
	}
	return s.proposals.ListMemoryProposals(ctx, status)
}

// AcceptProposal turns a pending proposal into a permanent memory (enriched and
// persisted), then marks the proposal accepted.
func (s *Service) AcceptProposal(ctx context.Context, id, acceptedBy string) (domain.Memory, error) {
	if s.proposals == nil {
		return domain.Memory{}, fmt.Errorf("memory synthesis is not configured")
	}
	proposal, err := s.proposals.GetMemoryProposal(ctx, id)
	if err != nil {
		return domain.Memory{}, err
	}
	if proposal.Status != domain.MemoryProposalStatusPending {
		return domain.Memory{}, ErrProposalNotPending
	}

	memory, err := s.CreateMemory(ctx, CreateMemoryInput{
		Title:          proposal.Title,
		Body:           proposal.Body,
		Kind:           proposal.Kind,
		AnchorTags:     proposal.AnchorTags,
		SubjectTag:     proposal.SubjectTag,
		AnchorRegion:   proposal.AnchorRegion,
		CreatedBy:      strings.TrimSpace(acceptedBy),
		Source:         domain.MemorySourceSynthesized,
		SourceIssueIDs: proposal.SourceIssueIDs,
		Confidence:     proposal.Confidence,
	})
	if err != nil {
		return domain.Memory{}, err
	}

	proposal.Status = domain.MemoryProposalStatusAccepted
	proposal.AcceptedMemoryID = memory.ID
	proposal.UpdatedAt = time.Now().UTC()
	if err := s.proposals.UpsertMemoryProposal(ctx, proposal); err != nil {
		return domain.Memory{}, fmt.Errorf("mark proposal accepted: %w", err)
	}
	s.logger.InfoContext(ctx, "memory proposal accepted", "proposal_id", proposal.ID, "memory_id", memory.ID)
	return memory, nil
}

// RejectProposal marks a pending proposal as rejected.
func (s *Service) RejectProposal(ctx context.Context, id string) (domain.MemoryProposal, error) {
	if s.proposals == nil {
		return domain.MemoryProposal{}, fmt.Errorf("memory synthesis is not configured")
	}
	proposal, err := s.proposals.GetMemoryProposal(ctx, id)
	if err != nil {
		return domain.MemoryProposal{}, err
	}
	if proposal.Status != domain.MemoryProposalStatusPending {
		return domain.MemoryProposal{}, ErrProposalNotPending
	}
	proposal.Status = domain.MemoryProposalStatusRejected
	proposal.UpdatedAt = time.Now().UTC()
	if err := s.proposals.UpsertMemoryProposal(ctx, proposal); err != nil {
		return domain.MemoryProposal{}, fmt.Errorf("mark proposal rejected: %w", err)
	}
	return proposal, nil
}

// enrich runs the memory body through the issue enrichment pipeline to attach a
// tag-relevance profile and embedding. Failures are logged but non-fatal: a
// memory always persists and can be re-enriched later.
func (s *Service) enrich(ctx context.Context, memory *domain.Memory) {
	if s.enricher == nil {
		return
	}
	raw := strings.TrimSpace(strings.TrimSpace(memory.Title) + "\n\n" + memory.Body)
	result, err := s.enricher.AnalyzeText(ctx, raw, issueenrichment.AnalyzeTextOptions{
		PreferredTags: memory.AnchorTags,
		CandidateMode: tags.CandidateModeRetrievalShortlist,
		Verify:        true,
		// A memory's own tagging should not be influenced by other memories.
		SkipPriorDecisions: true,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "memory enrichment failed; persisting without scores", "error", err)
		return
	}
	memory.TagScores = result.TagScores
	memory.Embedding = issueenrichment.Float32VectorToFloat64(result.Analyzed.Embedding.Vector)
	if len(memory.AnchorTags) == 0 {
		memory.AnchorTags = deriveAnchorTags(result.TagScores)
	}
}

// deriveAnchorTags picks the highest-relevance tags as a memory's anchors when
// the author supplied none — the high-value tags the memory centers on.
func deriveAnchorTags(scores []domain.TagRelevance) []string {
	ranked := append([]domain.TagRelevance(nil), scores...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Relevance > ranked[j].Relevance
	})
	out := make([]string, 0, maxDerivedAnchorTags)
	for _, score := range ranked {
		if score.Relevance < anchorTagRelevanceFloor {
			break
		}
		out = append(out, score.Tag)
		if len(out) >= maxDerivedAnchorTags {
			break
		}
	}
	return out
}

func normalizeTagList(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, tag := range in {
		tag = domain.NormalizeTagName(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func normalizeIDList(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, id := range in {
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
