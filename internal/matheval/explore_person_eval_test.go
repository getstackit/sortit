package matheval

import (
	"context"
	"strings"
	"testing"
	"time"

	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/people"
	"sortit/internal/ridgelambda"
	"sortit/internal/tags"
)

// SurfaceBaseline guards one derived surface (explore or person) on both
// similarity models: the rank-1 fallback (no ridge options) and the ridge
// tag-space default the handlers inject at the GCV-selected penalty. Both are
// full-path — the explore/person modifiers (relationship boost, freshness,
// maturity, velocity, authority) stay ON, matching production. Only NDCG/Recall
// are pinned; unlike search there is no factor-model summary, which is a
// corpus-level property already covered by the search entries.
type SurfaceBaseline struct {
	Rank1 SearchMetrics `json:"rank1"`
	Ridge SearchMetrics `json:"ridge"`
}

// exploreSurfaceMetrics runs the explore eval over one fixture on both models.
// It drives ExploreFromIssuesWithTags through the production option-injection
// shape: rank-1 uses no options; ridge injects WithExploreRidgeSimilarity at the
// GCV penalty, exactly as the explore handler does with ridgelambda.Cache. Every
// fixture issue is open, so each seed sees the other 47 as candidates.
func exploreSurfaceMetrics(
	t *testing.T,
	corpus Corpus,
	storeIssues []issues.Issue,
	storeTags []issues.Tag,
	gcvLambda float64,
) SurfaceBaseline {
	t.Helper()
	file, err := loadExploreJudgments(corpus)
	if err != nil {
		t.Fatalf("load explore judgments: %v", err)
	}

	run := func(opts ...issuemap.ExploreOption) SearchMetrics {
		var ndcgSum, recallSum float64
		for _, seed := range file.Seeds {
			resp, err := issuemap.ExploreFromIssuesWithTags(storeIssues, storeTags, seed.Seed, evalK, opts...)
			if err != nil {
				t.Fatalf("explore from %q: %v", seed.Seed, err)
			}
			ranked := make([]string, len(resp.RelatedIssues))
			for i, ri := range resp.RelatedIssues {
				ranked[i] = ri.ID
			}
			ndcgSum += NDCGAtK(ranked, seed.Expected, evalK)
			recallSum += RecallAtK(ranked, seed.Expected, evalK)
		}
		n := float64(len(file.Seeds))
		return searchMetrics(len(file.Seeds), ndcgSum/n, recallSum/n)
	}

	return SurfaceBaseline{
		Rank1: run(),
		Ridge: run(issuemap.WithExploreRidgeSimilarity(gcvLambda)),
	}
}

// personSurfaceMetrics runs the person-recommendation eval over one fixture on
// both models. The seam is the exported production handler
// people.GetPersonDetailHandler.Handle, driven per person with a fake store: the
// person's history is marked closed + assigned to them, every other issue stays
// open and unassigned as the candidate pool. Handle builds the profile from the
// history, then recommends open issues through the same blend + modifier chain
// production runs. The rank-1 model leaves RidgeLambda nil; the ridge model wires
// a real ridgelambda.Cache over the full corpus, which re-derives the same GCV
// penalty the search eval uses.
//
// Handle is the lowest seam that exercises the model choice honestly:
// recommendOpenIssues is unexported and pulls its tag catalog + λ cache from the
// handler, so calling it in isolation would either skip the model selection or
// re-implement the handler's wiring.
func personSurfaceMetrics(
	t *testing.T,
	corpus Corpus,
	storeIssues []issues.Issue,
	storeTags []issues.Tag,
) SurfaceBaseline {
	t.Helper()
	file, err := loadPeople(corpus)
	if err != nil {
		t.Fatalf("load people: %v", err)
	}

	run := func(ridge bool) SearchMetrics {
		var ndcgSum, recallSum float64
		for _, person := range file.People {
			store := newPersonEvalStore(storeIssues, person.Person, set(person.History))
			catalog := tags.NewCatalogService(personEvalTagStore{tags: storeTags}, nil, nil)
			handler := people.GetPersonDetailHandler{Store: store, Catalog: catalog}
			if ridge {
				handler.RidgeLambda = &ridgelambda.Cache{Store: store, Tags: catalog}
			}

			detail, err := handler.Handle(context.Background(), person.Person)
			if err != nil {
				t.Fatalf("person detail for %q: %v", person.Person, err)
			}
			ranked := make([]string, len(detail.RecommendedIssues))
			for i, rec := range detail.RecommendedIssues {
				ranked[i] = rec.Issue.ID
			}
			ndcgSum += NDCGAtK(ranked, person.Expected, evalK)
			recallSum += RecallAtK(ranked, person.Expected, evalK)
		}
		n := float64(len(file.People))
		return searchMetrics(len(file.People), ndcgSum/n, recallSum/n)
	}

	return SurfaceBaseline{
		Rank1: run(false),
		Ridge: run(true),
	}
}

// newPersonEvalStore builds one person's view of the corpus: their history
// issues closed and assigned to them (so Handle derives the profile from them),
// every other issue open and unassigned (the candidate pool).
func newPersonEvalStore(storeIssues []issues.Issue, person string, history map[string]struct{}) personEvalStore {
	all := make([]issues.Issue, len(storeIssues))
	for i, issue := range storeIssues {
		clone := issue
		if _, ok := history[issue.ID]; ok {
			clone.Status = issues.StatusClosed
			clone.AssignedTo = person
		} else {
			clone.Status = issues.StatusOpen
			clone.AssignedTo = ""
		}
		all[i] = clone
	}
	return personEvalStore{all: all}
}

// personEvalStore is a read-only in-memory issues.Store (plus FilteredIssueLister)
// over a fixed issue slice. It deliberately does not implement IssueDetailReader,
// so velocity hydration runs with a nil reader (no per-issue detail fan-out) and
// stays deterministic.
type personEvalStore struct {
	all []issues.Issue
}

func (s personEvalStore) List(context.Context) ([]issues.Issue, error) {
	return append([]issues.Issue(nil), s.all...), nil
}

func (s personEvalStore) Get(_ context.Context, id string) (issues.Issue, error) {
	for _, issue := range s.all {
		if issue.ID == id {
			return issue, nil
		}
	}
	return issues.Issue{}, issues.ErrNotFound
}

func (s personEvalStore) ListFiltered(_ context.Context, opts issues.ListOptions) ([]issues.Issue, error) {
	var out []issues.Issue
	for _, issue := range s.all {
		if opts.Status != "" && issue.Status != opts.Status {
			continue
		}
		if opts.AssignedTo != "" && !strings.EqualFold(issue.AssignedTo, opts.AssignedTo) {
			continue
		}
		out = append(out, issue)
	}
	return out, nil
}

func (s personEvalStore) SaveIssue(context.Context, issues.Issue) error         { return nil }
func (s personEvalStore) SaveIssuePost(context.Context, issues.IssuePost) error { return nil }
func (s personEvalStore) UpdateIssueFields(context.Context, string, issues.IssueFieldUpdate) error {
	return nil
}
func (s personEvalStore) SaveOperation(context.Context, issues.IssueOperation) error { return nil }
func (s personEvalStore) SaveLink(context.Context, issues.IssueLink) error           { return nil }

// personEvalTagStore is a read-only tags.TagStore over the fixture tags, so the
// catalog serves tag embeddings + specificity to the person handler.
type personEvalTagStore struct {
	tags []issues.Tag
}

func (s personEvalTagStore) ListTags(context.Context) ([]issues.Tag, error) {
	return append([]issues.Tag(nil), s.tags...), nil
}
func (s personEvalTagStore) UpsertTags(context.Context, []issues.Tag) error { return nil }
func (s personEvalTagStore) UpdateTagSpecificity(context.Context, string, *float64, *float64, *float64, *time.Time) error {
	return nil
}
