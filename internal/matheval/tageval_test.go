package matheval

import (
	"reflect"
	"testing"

	"sortit/internal/ai"
)

func TestTagEvalMetricsAndCalibration(t *testing.T) {
	items := []TagEvalIssue{
		{
			ID: "a", ExpectedTags: []string{"auth", "login"}, PredictedTags: []string{"auth", "billing"},
			Precision: 0.5, Recall: 0.5, F1: 0.5,
			ModelOutput:    []ai.TagScore{{Tag: "auth", Relevance: 0.9}, {Tag: "billing", Relevance: 0.3}},
			ModelNegations: []ai.NegatedTag{{Tag: "login", Confidence: 0.5}}, TokenUsage: ai.TokenUsage{InputTokens: 100, OutputTokens: 20},
		},
		{
			ID: "b", ExpectedTags: []string{"search"}, PredictedTags: []string{"search"},
			Precision: 1, Recall: 1, F1: 1,
			ModelOutput: []ai.TagScore{{Tag: "search", Relevance: 0.8}}, TokenUsage: ai.TokenUsage{InputTokens: 200, OutputTokens: 40},
		},
	}
	got := summarizeTagEvalIssues(items, TagEvalPrice{InputPerMillion: 1, OutputPerMillion: 10})
	if got.MicroPrecision != 0.666667 || got.MicroRecall != 0.666667 || got.MicroF1 != 0.666667 {
		t.Fatalf("micro metrics = %+v, want 0.666667 each", got)
	}
	if got.MacroF1 != 0.75 || got.CorrectRelevanceMean != 0.85 || got.IncorrectRelevanceMean != 0.3 {
		t.Fatalf("macro/calibration = %+v", got)
	}
	if got.NegationFalsePositives != 1 || got.NegationCount != 1 || got.NegationFalsePositiveRate != 1 {
		t.Fatalf("negation metrics = %+v", got)
	}
	if got.InputTokensPerIssue != 150 || got.OutputTokensPerIssue != 30 || got.CostPer1KEnrichments != 0.45 {
		t.Fatalf("cost metrics = %+v", got)
	}
}

func TestCanonicalModelList(t *testing.T) {
	got := CanonicalModelList([]string{" gpt-b ", "gpt-a", "gpt-b", ""})
	want := []string{"gpt-a", "gpt-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
}

func TestSummarizeTagEvalRunsUsesPooledNegationRate(t *testing.T) {
	got := summarizeTagEvalRuns([]TagEvalRun{
		{Stats: TagEvalStats{Issues: 1, NegationFalsePositives: 1, NegationCount: 1, NegationFalsePositiveRate: 1}},
		{Stats: TagEvalStats{Issues: 1, NegationFalsePositives: 1, NegationCount: 3, NegationFalsePositiveRate: 1.0 / 3}},
	})
	if got.NegationFalsePositiveRate != 0.5 {
		t.Fatalf("negation false-positive rate = %v, want pooled rate 0.5", got.NegationFalsePositiveRate)
	}
}
