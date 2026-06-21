package memories

import (
	"context"
	"reflect"
	"testing"

	"sortit/internal/ai"
	"sortit/internal/domain"
	"sortit/internal/issues"
)

// residualSynthCorpus builds a corpus where exp* issues are well explained by the
// "alpha" tag and res* issues share an unexplained residual (identical off-tag
// embedding direction), forming a residual cluster. Returns the inputs
// SynthesizeResidualConceptProposals expects.
func residualSynthCorpus(residualIDs ...string) (
	corpus []issues.Issue,
	tagNames []string,
	issueEmb map[string][]float64,
	tagEmb map[string][]float64,
) {
	tagEmb = map[string][]float64{"alpha": {1, 0, 0, 0}}
	tagNames = []string{"alpha"}
	issueEmb = map[string][]float64{}
	add := func(id string, tagged bool, emb []float64) {
		iss := issues.Issue{ID: id, Raw: id + " body text describing a widget gadget"}
		if tagged {
			iss.TagScores = []domain.TagRelevance{{Tag: "alpha", Relevance: 0.9}}
		}
		corpus = append(corpus, iss)
		issueEmb[id] = emb
	}
	add("exp1", true, []float64{1, 0, 0, 0})
	add("exp2", true, []float64{1, 0, 0, 0})
	add("exp3", true, []float64{1, 0, 0, 0})
	for _, id := range residualIDs {
		add(id, false, []float64{0, 0, 1, 0})
	}
	return
}

func TestSynthesizeResidualConceptProposals(t *testing.T) {
	corpus, tagNames, issueEmb, tagEmb := residualSynthCorpus("res1", "res2", "res3")

	drafts := SynthesizeResidualConceptProposals(corpus, tagNames, issueEmb, tagEmb, nil, nil)
	if len(drafts) != 1 {
		t.Fatalf("expected 1 residual concept proposal, got %d: %+v", len(drafts), drafts)
	}
	d := drafts[0]
	if d.Kind != domain.MemoryKindConcept {
		t.Fatalf("expected concept kind, got %s", d.Kind)
	}
	if d.SubjectTag != "" {
		t.Fatalf("pure synthesizer must leave the subject tag unnamed, got %q", d.SubjectTag)
	}
	if d.Status != domain.MemoryProposalStatusPending {
		t.Fatalf("expected pending status, got %s", d.Status)
	}
	if !reflect.DeepEqual(d.SourceIssueIDs, []string{"res1", "res2", "res3"}) {
		t.Fatalf("expected the cluster's issue IDs (sorted), got %v", d.SourceIssueIDs)
	}
	if d.Confidence <= 0 || d.Confidence > 1 {
		t.Fatalf("confidence out of range: %v", d.Confidence)
	}
}

func TestSynthesizeResidualConceptProposalsBelowFloor(t *testing.T) {
	// Only two issues share the residual — below residualClusterMinSize (3).
	corpus, tagNames, issueEmb, tagEmb := residualSynthCorpus("res1", "res2")
	drafts := SynthesizeResidualConceptProposals(corpus, tagNames, issueEmb, tagEmb, nil, nil)
	if len(drafts) != 0 {
		t.Fatalf("expected no proposal for a below-floor cluster, got %d: %+v", len(drafts), drafts)
	}
}

func TestSynthesizeResidualConceptProposalsDedupsCoveredIssues(t *testing.T) {
	corpus, tagNames, issueEmb, tagEmb := residualSynthCorpus("res1", "res2", "res3")

	// An existing concept already cites res1 as provenance, so it is excluded from
	// the seed pool — the remaining {res2, res3} falls below the cluster floor.
	pending := []domain.MemoryProposal{{
		Kind:           domain.MemoryKindConcept,
		SubjectTag:     "widget",
		SourceIssueIDs: []string{"res1"},
		Status:         domain.MemoryProposalStatusPending,
	}}

	drafts := SynthesizeResidualConceptProposals(corpus, tagNames, issueEmb, tagEmb, pending, nil)
	if len(drafts) != 0 {
		t.Fatalf("expected no proposal once a cluster member is already covered, got %d: %+v", len(drafts), drafts)
	}

	// Same exclusion via an active concept memory.
	active := []domain.Memory{{
		Kind:           domain.MemoryKindConcept,
		SubjectTag:     "widget",
		SourceIssueIDs: []string{"res1"},
		Status:         domain.MemoryStatusActive,
	}}
	if drafts := SynthesizeResidualConceptProposals(corpus, tagNames, issueEmb, tagEmb, nil, active); len(drafts) != 0 {
		t.Fatalf("expected no proposal when an active concept covers a member, got %d", len(drafts))
	}
}

type fakeConceptProposer struct {
	name    string
	profile string
	calls   int
}

func (f *fakeConceptProposer) ProposeConceptFromCluster(_ context.Context, _ []string, _ ai.ConceptFrame) (string, string, error) {
	f.calls++
	return f.name, f.profile, nil
}

type fakeTagCatalogReader struct {
	tags []issues.Tag
}

func (f *fakeTagCatalogReader) ListTags(_ context.Context) ([]issues.Tag, error) {
	return f.tags, nil
}

func TestSynthesizeProposalsMinesResidualConcepts(t *testing.T) {
	corpus, _, issueEmb, _ := residualSynthCorpus("res1", "res2", "res3")
	// Attach the embeddings to the stored issues so the service can decompose.
	for i := range corpus {
		corpus[i].Embedding = issueEmb[corpus[i].ID]
	}

	store := issues.NewInMemoryStore(corpus)
	svc := NewService(store, nil, nil)
	svc.UseSynthesis(store, store)
	svc.UseTagCatalog(&fakeTagCatalogReader{tags: []issues.Tag{{Name: "alpha", Embedding: []float64{1, 0, 0, 0}}}})
	proposer := &fakeConceptProposer{name: "Widget Gadget", profile: "The widget gadget subsystem."}
	svc.UseConceptProposer(proposer)

	created, err := svc.SynthesizeProposals(context.Background())
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if proposer.calls == 0 {
		t.Fatal("expected the proposer to be asked to name the residual cluster")
	}

	concept := findConceptProposal(t, created)
	if concept.SubjectTag != "widget gadget" {
		t.Fatalf("expected the normalized proposed subject tag, got %q", concept.SubjectTag)
	}
	if concept.Body != "The widget gadget subsystem." {
		t.Fatalf("expected the proposer's profile as the body, got %q", concept.Body)
	}
	if !reflect.DeepEqual(concept.SourceIssueIDs, []string{"res1", "res2", "res3"}) {
		t.Fatalf("expected the cluster's issue IDs, got %v", concept.SourceIssueIDs)
	}
}

func TestSynthesizeProposalsSkipsResidualMiningWithoutProposer(t *testing.T) {
	corpus, _, issueEmb, _ := residualSynthCorpus("res1", "res2", "res3")
	for i := range corpus {
		corpus[i].Embedding = issueEmb[corpus[i].ID]
	}
	store := issues.NewInMemoryStore(corpus)
	svc := NewService(store, nil, nil)
	svc.UseSynthesis(store, store)
	// Tag catalog configured but NO proposer → residual mining is skipped entirely.
	svc.UseTagCatalog(&fakeTagCatalogReader{tags: []issues.Tag{{Name: "alpha", Embedding: []float64{1, 0, 0, 0}}}})

	created, err := svc.SynthesizeProposals(context.Background())
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	for _, p := range created {
		if p.Kind == domain.MemoryKindConcept && p.SubjectTag == "" {
			t.Fatalf("expected no unnamed residual concept without a proposer, got %+v", p)
		}
	}
}
