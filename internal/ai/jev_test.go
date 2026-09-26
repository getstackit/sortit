package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type jevRoundTripFunc func(*http.Request) (*http.Response, error)

func (f jevRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jevTestClient(handle func(*http.Request) string) *http.Client {
	return &http.Client{Transport: jevRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(handle(r)))}, nil
	})}
}

type jevTestTagger struct{ result ScoreResult }

func (t jevTestTagger) Score(context.Context, string, []Tag, []FewShotExample, []PriorDecision, ConceptFrame) (ScoreResult, error) {
	return t.result, nil
}
func (jevTestTagger) Provider() string { return "test" }
func (jevTestTagger) Model() string    { return "llm-test" }

func TestJevReweightsWithoutLosingTagsOrEvidence(t *testing.T) {
	client := jevTestClient(func(r *http.Request) string {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing API key")
		}
		var request jevRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(request.Questions) != 3 || request.Questions["positive_1"].Type != "score" || request.Questions["negative_0"].Type != "noul" {
			t.Errorf("unexpected questions: %+v", request.Questions)
		}
		return `{"answers":{"positive_0":{"type":"score","score":3.2},"positive_1":{"type":"score","score":2},"negative_0":{"type":"noul","noul":0.6}},"usage":{"input_tokens":100,"output_tokens":4}}`
	})

	base := jevTestTagger{ScoreResult{
		Tags: []TagScore{
			{Tag: "bug", Relevance: 0.5, Evidence: []string{"crashes"}},
			{Tag: "new-tag", Relevance: 0.4, Suggested: true, Description: "a new concern", Evidence: []string{"broken flow"}},
		},
		Negated: []NegatedTag{{Tag: "safari", Confidence: 0.8, Evidence: []string{"works in Safari"}}},
	}}
	tagger, err := NewJevReweightedTagger(base, JevConfig{APIKey: "test-key", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	var usage TokenUsage
	tagger.SetTokenUsageObserver(func(got TokenUsage) { usage = got })
	got, err := tagger.Score(context.Background(), "The new flow crashes but works in Safari", []Tag{{Name: "bug"}}, nil, nil, ConceptFrame{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Tags[0].Relevance != 0.8 || got.Tags[1].Relevance != 0.5 || got.Negated[0].Confidence != 0.6 {
		t.Fatalf("unexpected weights: %+v", got)
	}
	if !got.Tags[1].Suggested || got.Tags[1].Description != "a new concern" || got.Tags[0].Evidence[0] != "crashes" || got.Negated[0].Evidence[0] != "works in Safari" {
		t.Fatalf("lost LLM outputs: %+v", got)
	}
	if usage.InputTokens != 100 || usage.OutputTokens != 4 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestJevRejectsIncompleteResponse(t *testing.T) {
	client := jevTestClient(func(*http.Request) string { return `{"answers":{}}` })
	tagger, err := NewJevReweightedTagger(jevTestTagger{ScoreResult{Tags: []TagScore{{Tag: "bug"}}}}, JevConfig{APIKey: "key", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tagger.Score(context.Background(), "bug", nil, nil, nil, ConceptFrame{})
	if err == nil || !strings.Contains(err.Error(), "positive_0") {
		t.Fatalf("expected missing-answer error, got %v", err)
	}
}
