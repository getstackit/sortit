package matheval

import (
	"encoding/json"
	"fmt"
	"os"
)

// Judgment labels one corpus query with graded expectations. Grades follow
// the usual relevance convention:
//
//	3 — the issue the query is about (near-duplicate of the query)
//	2 — clearly relevant, same topic cluster
//	1 — marginally related, defensible in a long result list
//	0 / absent — irrelevant
//
// Recall counts every grade > 0 as relevant; NDCG uses exponential gains so
// grade-3 results dominate the metric.
type Judgment struct {
	Query    string         `json:"query"` // corpus query ID
	Expected map[string]int `json:"expected"`
	Notes    string         `json:"notes,omitempty"`
}

// LoadJudgments reads the labeled judgment set and validates it against the
// corpus: every query and every expected issue ID must exist, and every
// corpus query must be labeled, so the two files cannot silently drift apart.
func LoadJudgments(path string, corpus Corpus) ([]Judgment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var judgments []Judgment
	if err := json.Unmarshal(data, &judgments); err != nil {
		return nil, fmt.Errorf("parse judgments %s: %w", path, err)
	}

	issueIDs := make(map[string]struct{}, len(corpus.Issues))
	for _, issue := range corpus.Issues {
		issueIDs[issue.ID] = struct{}{}
	}

	labeled := make(map[string]struct{}, len(judgments))
	for _, j := range judgments {
		if _, ok := corpus.QueryByID(j.Query); !ok {
			return nil, fmt.Errorf("judgment references unknown query %q", j.Query)
		}
		if _, dup := labeled[j.Query]; dup {
			return nil, fmt.Errorf("duplicate judgment for query %q", j.Query)
		}
		labeled[j.Query] = struct{}{}
		hasPositive := false
		for id, grade := range j.Expected {
			if _, ok := issueIDs[id]; !ok {
				return nil, fmt.Errorf("judgment %q references unknown issue %q", j.Query, id)
			}
			if grade < 0 || grade > 3 {
				return nil, fmt.Errorf("judgment %q grades issue %q as %d, want 0..3", j.Query, id, grade)
			}
			if grade > 0 {
				hasPositive = true
			}
		}
		if !hasPositive {
			return nil, fmt.Errorf("judgment %q has no positively graded issues", j.Query)
		}
	}
	for _, q := range corpus.Queries {
		if _, ok := labeled[q.ID]; !ok {
			return nil, fmt.Errorf("corpus query %q has no judgment", q.ID)
		}
	}
	return judgments, nil
}
