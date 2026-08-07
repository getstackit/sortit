package matheval

import (
	"flag"
	"testing"
	"time"

	"sortit/internal/centering"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/people"
	"sortit/internal/ridgedecomp"
	"sortit/internal/ridgelambda"
	"sortit/internal/scoring"
	"sortit/internal/tags"
	"sortit/internal/vectors"
)

var configMatrix = flag.Bool("configmatrix", false, "run the WP-304 ridge config matrix (log-only report)")

// TestRidgeConfigMatrix is the WP-304 evidence harness: the full config matrix
// behind the ridge-default re-examination, preserved as the reproducible
// record of the verdict. Under the pre-committed rule (a config qualifies iff
// it does not lose to rank-1 on real beyond ±0.01 NDCG@8 and does not lose
// > 0.02 NDCG@8 on synthetic vs the best there), exactly one config qualified:
// ridge tag-space UNCENTERED at the uncentered-selected GCV λ — which is now
// the shipped ranking default, guarded by TestMathEval's `ridge` entries. It
// measures six similarity configs on both fixtures —
//
//  1. rank-1 centered (the fallback)
//  2. ridge tag-space centered, GCV λ (the shipped default)
//  3. ridge tag-space uncentered, GCV λ re-selected on uncentered inputs
//  4. ridge recon-space centered, GCV λ (shares the solve/λ with 2)
//  5. ridge recon-space uncentered, GCV λ (shares the solve/λ with 3)
//  6. pure semantic cosine (no decomposition; the null model)
//
// Full-ranking-path rows drive the production SearchFromQueryWithTags with all
// modifiers ON, per the plan doc "full-path where possible":
//
//   - Config 3 full-path uses WithCorpusMeans with zero-VALUED (non-empty)
//     mean vectors: vectors.CenterUnit subtracts zero and unit-normalizes, so
//     the entire pipeline (corpus, query, decomposition inputs) runs on
//     uncentered unit vectors through unchanged production code.
//   - Config 6 full-path uses a ridge bundle computed over ID-renamed "ghost"
//     clones of the corpus: every real candidate misses VectorsFor, so the
//     production missing-candidate rule (blended = semanticSim at full weight)
//     fires for every candidate — the pure-semantic null model through the
//     full modifier chain.
//   - Configs 4/5 (recon-space) have NO full-path seam: search/explore/person
//     hardcode RidgeTagSpace in the blend. They are measured similarity-only
//     (shadow-harness style) and compared against similarity-only rank-1.
//
// Explore and person rows run for the uncentered tag-space finalist through
// production entry points: explore via WithExploreRidgeDecomposition over an
// uncentered bundle (ComputeCorpusRidgeDecomposition with zero CorpusMeans),
// person via the production ridgedecomp/ridgelambda caches with an empty
// centering.Cache (zero means → uncentered λ selection and solve).
//
// Run with:
//
//	go test ./internal/matheval -run TestRidgeConfigMatrix -configmatrix -v
func TestRidgeConfigMatrix(t *testing.T) {
	if !*configMatrix {
		t.Skip("config matrix is opt-in: rerun with -configmatrix -v")
	}

	syntheticCorpus, realCorpus := loadFixtures(t)
	t.Log("===== fixture: synthetic (tag-sum embeddings, dim 24) =====")
	runConfigMatrix(t, syntheticCorpus)
	t.Log("===== fixture: real (text-embedding-3-small, dim 1536) =====")
	runConfigMatrix(t, realCorpus)
}

func runConfigMatrix(t *testing.T, corpus Corpus) {
	t.Helper()
	judgments, err := LoadJudgments(judgmentsPath, corpus)
	if err != nil {
		t.Fatalf("load judgments: %v", err)
	}

	now := time.Now().UTC()
	storeIssues := corpus.StoreIssues(now)
	storeTags := corpus.StoreTags()
	tagNames := corpus.TagNames()

	// Centered inputs + centered GCV λ — the shipped selection path.
	issueEmb, tagEmb, means, lambdaC := ridgeGCVFixture(t, corpus, storeIssues)

	// Uncentered inputs: unit-normalized raw vectors, exactly the space the
	// production path sees when centering is skipped (CenterUnit with an empty
	// or zero mean). λ is re-selected on these inputs per config.
	uncIssueEmb, uncTagEmb := issuemath.CenterEmbeddingsWith(issuemath.CorpusMeans{}, corpus.IssueEmbeddings(), corpus.TagEmbeddings())
	lambdaU, ok := issuemath.SelectRidgeLambdaGCV(storeIssues, tagNames, uncIssueEmb, uncTagEmb,
		scoring.RidgeAnchorLambdaScored, nil)
	if !ok {
		t.Fatal("GCV λ selection fell back on uncentered inputs")
	}
	t.Logf("GCV λ_unscored: centered=%.3f uncentered=%.3f (grid %v)", lambdaC, lambdaU, issuemath.DefaultRidgeLambdaGrid)

	// --- similarity-only table (blend ranking alone, shadow-harness style) ---
	// This is the only apples-to-apples surface for the recon-space configs,
	// which have no full-path seam.
	rank1Decomp := issuemath.ComputeFactorDecomposition(storeIssues, tagNames, issueEmb, tagEmb)
	tagCov := issuemath.BuildTagCovariance(tagNames, tagEmb)
	ridgeC := issuemath.ComputeRidgeDecomposition(storeIssues, tagNames, issueEmb, tagEmb,
		scoring.RidgeAnchorLambdaScored, lambdaC)
	ridgeU := issuemath.ComputeRidgeDecomposition(storeIssues, tagNames, uncIssueEmb, uncTagEmb,
		scoring.RidgeAnchorLambdaScored, lambdaU)
	if !rank1Decomp.Decomposed() || !ridgeC.Decomposed() || !ridgeU.Decomposed() {
		t.Fatal("a decomposition fell back; the fixture corpus should always decompose")
	}

	centeredQuery := func(q CorpusQuery) []float64 { return issuemath.CenterVector(q.Embedding, means.Issue) }
	uncenteredQuery := func(q CorpusQuery) []float64 { return issuemath.CenterVector(q.Embedding, nil) }

	simRank1 := func(q CorpusQuery) []string {
		qv := issuemath.DecomposeEmbedding(centeredQuery(q), toTagRelevances(q.Tags), tagNames, tagEmb, tagCov)
		return rankByScore(storeIssues, func(id string) (float64, bool) {
			cv, ok := rank1Decomp.DecomposedFor(id)
			if !ok {
				return 0, false
			}
			_, _, blended := issuemath.BlendFromDecomposition(rank1Decomp, qv, cv)
			return blended, true
		})
	}
	simRidge := func(d issuemath.RidgeDecomposition, tagSpace map[string][]float64, lambda float64,
		query func(CorpusQuery) []float64, mode issuemath.RidgeSimilarityMode) func(CorpusQuery) []string {
		return func(q CorpusQuery) []string {
			qv := issuemath.DecomposeRidgeEmbedding(query(q), toTagRelevances(q.Tags), tagNames, tagSpace,
				scoring.RidgeAnchorLambdaScored, lambda)
			return rankByScore(storeIssues, func(id string) (float64, bool) {
				cv, ok := d.VectorsFor(id)
				if !ok {
					return 0, false
				}
				_, _, blended := issuemath.RidgeBlend(d, qv, cv, mode)
				return blended, true
			})
		}
	}
	simSemantic := func(q CorpusQuery) []string {
		qv := centeredQuery(q)
		return rankByScore(storeIssues, func(id string) (float64, bool) {
			return vectors.CosineSimilarity(qv, issueEmb[id]), true
		})
	}

	type row struct {
		name string
		rank func(CorpusQuery) []string
	}
	simRows := []row{
		{"1 rank-1 centered", simRank1},
		{"2 tag-space centered", simRidge(ridgeC, tagEmb, lambdaC, centeredQuery, issuemath.RidgeTagSpace)},
		{"3 tag-space uncentered", simRidge(ridgeU, uncTagEmb, lambdaU, uncenteredQuery, issuemath.RidgeTagSpace)},
		{"4 recon-space centered", simRidge(ridgeC, tagEmb, lambdaC, centeredQuery, issuemath.RidgeReconSpace)},
		{"5 recon-space uncentered", simRidge(ridgeU, uncTagEmb, lambdaU, uncenteredQuery, issuemath.RidgeReconSpace)},
		{"6 pure semantic centered", simSemantic},
	}
	t.Logf("--- similarity-only (blend ranking, no modifiers) ---")
	t.Logf("%-28s  %8s  %8s", "config", "NDCG@8", "Recall@8")
	for _, r := range simRows {
		ndcg, recall := shadowMetrics(corpus, judgments, r.rank)
		t.Logf("%-28s  %8.4f  %8.4f", r.name, ndcg, recall)
	}

	// --- full ranking path (production SearchFromQueryWithTags, all modifiers) ---
	zeroMeans := issuemath.CorpusMeans{Issue: make([]float64, corpus.Dim), Tag: make([]float64, corpus.Dim)}
	ghostBundle := issuemap.ComputeCorpusRidgeDecomposition(ghostIssues(storeIssues), storeTags, means,
		scoring.RidgeAnchorLambdaScored, lambdaC)
	if !ghostBundle.Decomposed() {
		t.Fatal("ghost bundle failed to decompose")
	}

	type pathRow struct {
		name string
		opts []issuemap.SearchOption
	}
	pathRows := []pathRow{
		{"1 rank-1 centered", nil},
		{"2 tag-space centered", []issuemap.SearchOption{issuemap.WithRidgeSimilarity(lambdaC)}},
		{"3 tag-space uncentered", []issuemap.SearchOption{issuemap.WithRidgeSimilarity(lambdaU), issuemap.WithCorpusMeans(zeroMeans)}},
		{"6 pure semantic centered", []issuemap.SearchOption{issuemap.WithRidgeDecomposition(ghostBundle)}},
	}
	t.Logf("--- full ranking path (all modifiers; recon-space has no seam) ---")
	t.Logf("%-28s  %8s  %8s", "config", "NDCG@8", "Recall@8")
	for _, r := range pathRows {
		ndcg, recall := rankingMetrics(t, corpus, judgments, storeIssues, storeTags, r.opts...)
		t.Logf("%-28s  %8.4f  %8.4f", r.name, ndcg, recall)
	}

	// --- explore, full path (production ExploreFromIssuesWithTags) ---
	// The centered bundle row doubles as a seam parity check against the
	// baselined WithExploreRidgeSimilarity in-place solve.
	bundleC := issuemap.ComputeCorpusRidgeDecomposition(storeIssues, storeTags, means,
		scoring.RidgeAnchorLambdaScored, lambdaC)
	bundleU := issuemap.ComputeCorpusRidgeDecomposition(storeIssues, storeTags, issuemath.CorpusMeans{},
		scoring.RidgeAnchorLambdaScored, lambdaU)
	exploreRows := []struct {
		name string
		opts []issuemap.ExploreOption
	}{
		{"1 rank-1 centered", nil},
		{"2 tag centered (in-place)", []issuemap.ExploreOption{issuemap.WithExploreRidgeSimilarity(lambdaC)}},
		{"2 tag centered (bundle)", []issuemap.ExploreOption{issuemap.WithExploreRidgeDecomposition(bundleC)}},
		{"3 tag uncentered (bundle)", []issuemap.ExploreOption{issuemap.WithExploreRidgeDecomposition(bundleU)}},
	}
	exploreFile, err := loadExploreJudgments(corpus)
	if err != nil {
		t.Fatalf("load explore judgments: %v", err)
	}
	t.Logf("--- explore, full path ---")
	t.Logf("%-28s  %8s  %8s", "config", "NDCG@8", "Recall@8")
	for _, r := range exploreRows {
		var ndcgSum, recallSum float64
		for _, seed := range exploreFile.Seeds {
			resp, err := issuemap.ExploreFromIssuesWithTags(storeIssues, storeTags, seed.Seed, evalK, r.opts...)
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
		n := float64(len(exploreFile.Seeds))
		t.Logf("%-28s  %8.4f  %8.4f", r.name, ndcgSum/n, recallSum/n)
	}

	// --- person, full path (production GetPersonDetailHandler.Handle) ---
	// Centered and uncentered both run through the production ridgedecomp cache
	// (the preferred seam in server wiring); the WP-302 baseline's RidgeLambda
	// in-place row is kept for continuity.
	peopleFile, err := loadPeople(corpus)
	if err != nil {
		t.Fatalf("load people: %v", err)
	}
	personRun := func(wire func(store personEvalStore, catalog *tags.CatalogService, h *people.GetPersonDetailHandler)) (float64, float64) {
		var ndcgSum, recallSum float64
		for _, person := range peopleFile.People {
			store := newPersonEvalStore(storeIssues, person.Person, set(person.History))
			catalog := tags.NewCatalogService(personEvalTagStore{tags: storeTags}, nil, nil)
			handler := people.GetPersonDetailHandler{Store: store, Catalog: catalog}
			if wire != nil {
				wire(store, catalog, &handler)
			}
			detail, err := handler.Handle(t.Context(), person.Person)
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
		n := float64(len(peopleFile.People))
		return ndcgSum / n, recallSum / n
	}
	personRows := []struct {
		name string
		wire func(store personEvalStore, catalog *tags.CatalogService, h *people.GetPersonDetailHandler)
	}{
		{"1 rank-1 centered", nil},
		{"2 tag centered (in-place λ)", func(store personEvalStore, catalog *tags.CatalogService, h *people.GetPersonDetailHandler) {
			h.RidgeLambda = &ridgelambda.Cache{Store: store, Tags: catalog}
		}},
		{"2 tag centered (decomp cache)", func(store personEvalStore, catalog *tags.CatalogService, h *people.GetPersonDetailHandler) {
			ctr := &centering.Cache{Store: store, Tags: catalog}
			h.RidgeDecomp = &ridgedecomp.Cache{Store: store, Tags: catalog, Centering: ctr,
				Lambda: &ridgelambda.Cache{Store: store, Tags: catalog, Centering: ctr}}
		}},
		{"3 tag uncentered (decomp cache)", func(store personEvalStore, catalog *tags.CatalogService, h *people.GetPersonDetailHandler) {
			// The shipped WP-304 wiring: uncentered λ selection + uncentered
			// solve, zero bundle Means keeping the person profile uncentered too.
			h.RidgeDecomp = &ridgedecomp.Cache{Store: store, Tags: catalog, Uncentered: true,
				Lambda: &ridgelambda.Cache{Store: store, Tags: catalog, Uncentered: true}}
		}},
	}
	t.Logf("--- person, full path ---")
	t.Logf("%-32s  %8s  %8s", "config", "NDCG@8", "Recall@8")
	for _, r := range personRows {
		ndcg, recall := personRun(r.wire)
		t.Logf("%-32s  %8.4f  %8.4f", r.name, ndcg, recall)
	}
}

// ghostIssues clones the corpus issues under IDs no real candidate carries, so
// a ridge bundle computed over them makes every real candidate miss VectorsFor
// and score by production's pure-semantic missing-candidate rule.
func ghostIssues(storeIssues []issues.Issue) []issues.Issue {
	out := make([]issues.Issue, len(storeIssues))
	for i, issue := range storeIssues {
		clone := issue
		clone.ID = "ghost-" + issue.ID
		out[i] = clone
	}
	return out
}
