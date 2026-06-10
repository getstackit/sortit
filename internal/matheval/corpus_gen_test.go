package matheval

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/ (corpus.json and baseline.json)")

const corpusPath = "testdata/corpus.json"

// TestCorpusMatchesGenerator pins testdata/corpus.json to the deterministic
// generator, so the checked-in fixtures can't drift from their documented
// provenance. It runs before the eval tests (file order), so `-update`
// regenerates the corpus before the baseline is recomputed.
func TestCorpusMatchesGenerator(t *testing.T) {
	want, err := json.MarshalIndent(GenerateCorpus(), "", "  ")
	if err != nil {
		t.Fatalf("marshal generated corpus: %v", err)
	}
	want = append(want, '\n')

	if *update {
		if err := os.WriteFile(corpusPath, want, 0o644); err != nil {
			t.Fatalf("write %s: %v", corpusPath, err)
		}
		t.Logf("regenerated %s", corpusPath)
		return
	}

	got, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read %s (run `go test ./internal/matheval -update` to generate): %v", corpusPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is out of sync with GenerateCorpus; run `go test ./internal/matheval -update`", corpusPath)
	}
}

func TestGeneratedCorpusIsValid(t *testing.T) {
	corpus := GenerateCorpus()
	if err := corpus.validate(); err != nil {
		t.Fatalf("generated corpus invalid: %v", err)
	}
	if n := len(corpus.Issues); n < 40 || n > 60 {
		t.Errorf("corpus has %d issues, want 40-60", n)
	}
	if n := len(corpus.Queries); n < 30 || n > 50 {
		t.Errorf("corpus has %d queries, want 30-50", n)
	}
}
