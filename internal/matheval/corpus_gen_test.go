package matheval

import (
	"bytes"
	"encoding/json"
	"flag"
	"math"
	"os"
	"slices"
	"testing"

	"sortit/internal/issuemath"
	"sortit/internal/vectors"
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

// TestGeneratedCorpusIsAnisotropic pins the geometric property the fixtures
// exist to reproduce: like real text embeddings over a homogeneous corpus,
// raw vectors share a large common direction (unrelated pairs still cosine
// high), and corpus-mean centering removes it. If this breaks, the harness
// is no longer exercising the runtime's centering path and metric movements
// stop being trustworthy evidence.
func TestGeneratedCorpusIsAnisotropic(t *testing.T) {
	corpus := GenerateCorpus()

	raw := corpus.IssueEmbeddings()
	centered, _, _ := issuemath.CenterEmbeddings(raw, corpus.TagEmbeddings())

	rawMean := meanPairwiseCosine(raw)
	centeredMean := meanPairwiseCosine(centered)

	if rawMean < 0.4 {
		t.Errorf("mean raw pairwise cosine = %.3f, want >= 0.4 (anisotropic common direction missing)", rawMean)
	}
	if math.Abs(centeredMean) > 0.15 {
		t.Errorf("mean centered pairwise cosine = %.3f, want ~0 (centering should remove the common direction)", centeredMean)
	}
	t.Logf("mean pairwise cosine: raw=%.3f centered=%.3f", rawMean, centeredMean)
}

func meanPairwiseCosine(embeddings map[string][]float64) float64 {
	ids := make([]string, 0, len(embeddings))
	for id := range embeddings {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	var sum float64
	var count int
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			sum += vectors.CosineSimilarity(embeddings[ids[i]], embeddings[ids[j]])
			count++
		}
	}
	return sum / float64(count)
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
