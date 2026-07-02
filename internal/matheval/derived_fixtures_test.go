package matheval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"

	"sortit/internal/scoring"
)

// This file holds the committed fixtures and mechanical judgment derivation for
// the explore and person-recommendation evals (WP-302). Both surfaces share the
// blend code that search exercises, but not the query shape — explore is seeded
// by an issue, person fit by a profile — so each gets its own judged fixture.
//
// Judgments are derived MECHANICALLY from the corpus generator's ground truth:
// every fixture tag carries a `domain` (generate.go tagSpecs), which is the only
// relatedness signal the generator encodes. An issue's region is the domain of
// its highest-relevance non-generic tag; two issues are "related" when they share
// a region (grade 3) or any non-generic domain (grade 2). This is circular in the
// same way the synthetic embeddings are — it grades "same region", not "actually
// useful" — which is acceptable for a REGRESSION guard (it detects change) but is
// not a calibration of usefulness. The real fixture (WP-301) pairs with these to
// show whether the same mechanical relatedness survives real geometry.

const (
	exploreJudgmentsPath = "testdata/explore_judgments.json"
	peoplePath           = "testdata/people.json"
)

// exploreSeeds are the ~12 issues explore is judged from. Chosen for spread
// across the ground-truth regions, including a few cross-domain issues
// (pay-02, srch-03, perf-03, ios-01) whose secondary domains exercise grade-2
// adjacency.
var exploreSeeds = []string{
	"bill-01", "pay-02", "auth-02", "login-01",
	"exp-01", "pdf-02", "srch-03", "perf-03",
	"mob-01", "ios-01", "notif-01", "db-02",
}

// personHistories assign ~6 synthetic people a set of resolved issues drawn from
// the ground-truth regions (5–15 each). At eval time each person's history is
// marked closed + assigned to them; every other issue stays open and unassigned
// as the candidate pool. WP-401 (profiles on f_i) reuses this fixture.
var personHistories = []struct {
	person  string
	history []string
}{
	{"p-money", []string{"bill-01", "bill-02", "bill-03", "pay-01", "pay-02"}},
	{"p-identity", []string{"auth-01", "auth-02", "auth-03", "auth-04", "login-01", "login-02"}},
	{"p-docs", []string{"exp-01", "exp-02", "exp-03", "pdf-01", "pdf-02"}},
	{"p-mobile", []string{"mob-01", "mob-02", "mob-03", "ios-01", "ios-02"}},
	{"p-data", []string{"db-01", "db-02", "db-03", "mig-01", "perf-01", "perf-03"}},
	{"p-comms", []string{"notif-01", "notif-02", "notif-03", "email-01", "email-02", "email-03"}},
}

const exploreJudgmentsComment = "MECHANICALLY DERIVED from the generator's tag-domain ground truth " +
	"(deriveExploreJudgments, pinned by TestExplorePersonFixturesMatchDerivation). " +
	"For each seed, every other issue is graded by region overlap: same primary domain = 3, " +
	"any shared non-generic domain = 2, otherwise omitted (0). CIRCULARITY CAVEAT: this grades " +
	"'same region', not 'actually useful' — same failure mode as the synthetic embeddings; " +
	"it is a regression guard, not a usefulness calibration (WP-302 Risks). Regenerate with " +
	"`go test ./internal/matheval -update`."

const peopleComment = "MECHANICALLY DERIVED from the generator's tag-domain ground truth " +
	"(derivePersonJudgments, pinned by TestExplorePersonFixturesMatchDerivation). " +
	"`history` is the person's resolved-issue set (hand-chosen from ground-truth regions); " +
	"`expected` grades each OPEN candidate (non-history issue) by overlap with the person's " +
	"history regions: candidate primary domain in the history's primary domains = 3, any shared " +
	"non-generic domain = 2, otherwise omitted (0). CIRCULARITY CAVEAT: grades 'same region', " +
	"not 'actually useful' — a regression guard, not a usefulness calibration (WP-302 Risks). " +
	"Regenerate with `go test ./internal/matheval -update`."

// ExploreSeedJudgment grades a single explore seed against the other corpus
// issues, same grade convention as search (see Judgment).
type ExploreSeedJudgment struct {
	Seed     string         `json:"seed"`
	Expected map[string]int `json:"expected"`
}

// ExploreJudgmentFile is the committed explore judgment set.
type ExploreJudgmentFile struct {
	Comment string                `json:"_comment"`
	Seeds   []ExploreSeedJudgment `json:"seeds"`
}

// PersonFixture is one synthetic person: their resolved-issue history and the
// mechanical relevance grades over the open candidate pool.
type PersonFixture struct {
	Person   string         `json:"person"`
	History  []string       `json:"history"`
	Expected map[string]int `json:"expected"`
}

// PeopleFile is the committed person-recommendation fixture.
type PeopleFile struct {
	Comment string          `json:"_comment"`
	People  []PersonFixture `json:"people"`
}

// tagDomainOf maps each fixture tag to its generator domain (generate.go tagSpecs).
func tagDomainOf() map[string]string {
	m := make(map[string]string, len(tagSpecs))
	for _, s := range tagSpecs {
		m[s.name] = s.domain
	}
	return m
}

// tagIsGeneric marks tags whose specificity is below the generic threshold
// (ui, backend). Generic tags do not define a region.
func tagIsGeneric() map[string]bool {
	m := make(map[string]bool, len(tagSpecs))
	for _, s := range tagSpecs {
		m[s.name] = s.specificity < scoring.GenericTagThreshold
	}
	return m
}

// issueDomainWeights sums an issue's tag relevance by non-generic domain.
func issueDomainWeights(issue CorpusIssue, domainOf map[string]string, generic map[string]bool) map[string]float64 {
	weights := map[string]float64{}
	for _, ts := range issue.TagScores {
		if generic[ts.Tag] {
			continue
		}
		weights[domainOf[ts.Tag]] += ts.Relevance
	}
	return weights
}

// primaryDomain returns the highest-weight domain, breaking ties by domain name
// (alphabetical) so the derivation is deterministic. Empty when the issue has no
// non-generic tag.
func primaryDomain(weights map[string]float64) string {
	best, bestW := "", -1.0
	for _, d := range sortedFloatKeys(weights) {
		if weights[d] > bestW {
			best, bestW = d, weights[d]
		}
	}
	return best
}

func sortedFloatKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// gradeRelated grades a candidate against a seed's region signature: same primary
// domain = 3, any shared non-generic domain = 2, otherwise 0.
func gradeRelated(seedPrimary string, seedDomains map[string]float64, candPrimary string, candDomains map[string]float64) int {
	if seedPrimary == "" || candPrimary == "" {
		return 0
	}
	if candPrimary == seedPrimary {
		return 3
	}
	for _, d := range sortedFloatKeys(candDomains) {
		if _, ok := seedDomains[d]; ok {
			return 2
		}
	}
	return 0
}

// deriveExploreJudgments builds the mechanical explore judgment set from the
// corpus ground truth.
func deriveExploreJudgments(corpus Corpus) ExploreJudgmentFile {
	domainOf, generic := tagDomainOf(), tagIsGeneric()
	byID := make(map[string]CorpusIssue, len(corpus.Issues))
	for _, issue := range corpus.Issues {
		byID[issue.ID] = issue
	}

	seeds := make([]ExploreSeedJudgment, 0, len(exploreSeeds))
	for _, seedID := range exploreSeeds {
		seed := byID[seedID]
		seedDomains := issueDomainWeights(seed, domainOf, generic)
		seedPrimary := primaryDomain(seedDomains)

		expected := map[string]int{}
		for _, cand := range corpus.Issues {
			if cand.ID == seedID {
				continue
			}
			candDomains := issueDomainWeights(cand, domainOf, generic)
			if grade := gradeRelated(seedPrimary, seedDomains, primaryDomain(candDomains), candDomains); grade > 0 {
				expected[cand.ID] = grade
			}
		}
		seeds = append(seeds, ExploreSeedJudgment{Seed: seedID, Expected: expected})
	}
	return ExploreJudgmentFile{Comment: exploreJudgmentsComment, Seeds: seeds}
}

// derivePersonJudgments builds the mechanical person fixture: each person's
// history plus relevance grades over the open (non-history) candidate pool.
func derivePersonJudgments(corpus Corpus) PeopleFile {
	domainOf, generic := tagDomainOf(), tagIsGeneric()
	byID := make(map[string]CorpusIssue, len(corpus.Issues))
	for _, issue := range corpus.Issues {
		byID[issue.ID] = issue
	}

	people := make([]PersonFixture, 0, len(personHistories))
	for _, ph := range personHistories {
		history := set(ph.history)
		personPrimaries := map[string]struct{}{}
		personDomains := map[string]float64{}
		for _, id := range ph.history {
			weights := issueDomainWeights(byID[id], domainOf, generic)
			if p := primaryDomain(weights); p != "" {
				personPrimaries[p] = struct{}{}
			}
			for d, w := range weights {
				personDomains[d] += w
			}
		}

		expected := map[string]int{}
		for _, cand := range corpus.Issues {
			if _, inHistory := history[cand.ID]; inHistory {
				continue
			}
			candDomains := issueDomainWeights(cand, domainOf, generic)
			candPrimary := primaryDomain(candDomains)
			if candPrimary == "" {
				continue
			}
			if _, ok := personPrimaries[candPrimary]; ok {
				expected[cand.ID] = 3
				continue
			}
			// Adjacency: candidate shares a non-generic domain with the union of
			// the person's history domains.
			for _, d := range sortedFloatKeys(candDomains) {
				if _, ok := personDomains[d]; ok {
					expected[cand.ID] = 2
					break
				}
			}
		}
		people = append(people, PersonFixture{Person: ph.person, History: ph.history, Expected: expected})
	}
	return PeopleFile{Comment: peopleComment, People: people}
}

func set(ids []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

// loadExploreJudgments reads and validates the committed explore judgment set
// against the corpus.
func loadExploreJudgments(corpus Corpus) (ExploreJudgmentFile, error) {
	data, err := os.ReadFile(exploreJudgmentsPath)
	if err != nil {
		return ExploreJudgmentFile{}, err
	}
	var file ExploreJudgmentFile
	if err := json.Unmarshal(data, &file); err != nil {
		return ExploreJudgmentFile{}, fmt.Errorf("parse %s: %w", exploreJudgmentsPath, err)
	}
	issueIDs := corpusIssueIDs(corpus)
	for _, seed := range file.Seeds {
		if _, ok := issueIDs[seed.Seed]; !ok {
			return ExploreJudgmentFile{}, fmt.Errorf("explore judgment references unknown seed %q", seed.Seed)
		}
		if err := validateGrades(seed.Seed, seed.Expected, issueIDs); err != nil {
			return ExploreJudgmentFile{}, err
		}
	}
	return file, nil
}

// loadPeople reads and validates the committed person fixture against the corpus.
func loadPeople(corpus Corpus) (PeopleFile, error) {
	data, err := os.ReadFile(peoplePath)
	if err != nil {
		return PeopleFile{}, err
	}
	var file PeopleFile
	if err := json.Unmarshal(data, &file); err != nil {
		return PeopleFile{}, fmt.Errorf("parse %s: %w", peoplePath, err)
	}
	issueIDs := corpusIssueIDs(corpus)
	for _, person := range file.People {
		for _, id := range person.History {
			if _, ok := issueIDs[id]; !ok {
				return PeopleFile{}, fmt.Errorf("person %q history references unknown issue %q", person.Person, id)
			}
		}
		if err := validateGrades(person.Person, person.Expected, issueIDs); err != nil {
			return PeopleFile{}, err
		}
	}
	return file, nil
}

func corpusIssueIDs(corpus Corpus) map[string]struct{} {
	ids := make(map[string]struct{}, len(corpus.Issues))
	for _, issue := range corpus.Issues {
		ids[issue.ID] = struct{}{}
	}
	return ids
}

func validateGrades(owner string, expected map[string]int, issueIDs map[string]struct{}) error {
	hasPositive := false
	for id, grade := range expected {
		if _, ok := issueIDs[id]; !ok {
			return fmt.Errorf("%q grades unknown issue %q", owner, id)
		}
		if grade < 0 || grade > 3 {
			return fmt.Errorf("%q grades issue %q as %d, want 0..3", owner, id, grade)
		}
		if grade > 0 {
			hasPositive = true
		}
	}
	if !hasPositive {
		return fmt.Errorf("%q has no positively graded issues", owner)
	}
	return nil
}

// TestExplorePersonFixturesMatchDerivation pins the committed explore/person
// fixtures to their mechanical derivation over the synthetic corpus, the same
// way TestCorpusMatchesGenerator pins corpus.json. `-update` rewrites them. The
// derivation reads only tag scores and the generator's domains — both fixtures
// share these — so the synthetic corpus is the canonical source.
func TestExplorePersonFixturesMatchDerivation(t *testing.T) {
	corpus, err := LoadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}

	explore := mustMarshal(t, deriveExploreJudgments(corpus))
	people := mustMarshal(t, derivePersonJudgments(corpus))

	if *update {
		if err := os.WriteFile(exploreJudgmentsPath, explore, 0o644); err != nil {
			t.Fatalf("write %s: %v", exploreJudgmentsPath, err)
		}
		if err := os.WriteFile(peoplePath, people, 0o644); err != nil {
			t.Fatalf("write %s: %v", peoplePath, err)
		}
		t.Logf("regenerated %s and %s", exploreJudgmentsPath, peoplePath)
		return
	}

	assertFileMatches(t, exploreJudgmentsPath, explore)
	assertFileMatches(t, peoplePath, people)
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return append(data, '\n')
}

func assertFileMatches(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s (run `go test ./internal/matheval -update` to generate): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is out of sync with its derivation; run `go test ./internal/matheval -update`", path)
	}
}
