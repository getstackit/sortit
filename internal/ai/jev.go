package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	defaultJevURL   = "https://api.typesafe.ai/v1/systemone"
	defaultJevModel = "jev-1.13.0"
)

// JevConfig configures the optional weight assessor. The wrapped Tagger remains
// responsible for discovering tags, quoting evidence, and proposing negations.
type JevConfig struct {
	APIKey     string
	URL        string
	Model      string
	HTTPClient *http.Client
}

type JevReweightedTagger struct {
	base          Tagger
	apiKey        string
	url           string
	model         string
	httpClient    *http.Client
	usageObserver func(TokenUsage)
}

func NewJevReweightedTagger(base Tagger, cfg JevConfig) (*JevReweightedTagger, error) {
	if base == nil {
		return nil, errors.New("base tagger is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("typesafe api key is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &JevReweightedTagger{
		base: base, apiKey: cfg.APIKey,
		url:        withDefault(strings.TrimSpace(cfg.URL), defaultJevURL),
		model:      withDefault(strings.TrimSpace(cfg.Model), defaultJevModel),
		httpClient: client,
	}, nil
}

func (t *JevReweightedTagger) Provider() string { return t.base.Provider() + "+typesafe" }
func (t *JevReweightedTagger) Model() string    { return t.base.Model() + "+" + t.model }

// Preserve the optional capability exposed by the LLM tagger.
func (t *JevReweightedTagger) ScoreSpecificity(ctx context.Context, tag Tag, catalog []Tag) (float64, error) {
	scorer, ok := t.base.(SpecificityScorer)
	if !ok {
		return 0, errors.New("base tagger does not support specificity scoring")
	}
	return scorer.ScoreSpecificity(ctx, tag, catalog)
}

func (t *JevReweightedTagger) SetTokenUsageObserver(observer func(TokenUsage)) {
	t.usageObserver = observer
}

type jevQuestion struct {
	Type         string   `json:"type"`
	Instructions string   `json:"instructions"`
	Criteria     []string `json:"criteria,omitempty"`
}

type jevRequest struct {
	State     any                    `json:"state"`
	Model     string                 `json:"model"`
	Questions map[string]jevQuestion `json:"questions"`
}

type jevAnswer struct {
	Type  string   `json:"type"`
	Score *float64 `json:"score"`
	Noul  *float64 `json:"noul"`
}

type jevResponse struct {
	Answers map[string]jevAnswer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

var jevRelevanceLevels = []string{
	"The tag does not apply to this issue.",
	"The tag is only incidental background context.",
	"The tag is a secondary concern of the issue.",
	"The tag is a central concern alongside other concerns.",
	"The tag is the primary subject of the issue.",
}

// Score keeps the LLM's complete tag set and grounded quotations, while Jev
// independently assesses the weights of those same tags. It never invents a
// tag or a negation without an LLM-supplied evidence quote.
func (t *JevReweightedTagger) Score(ctx context.Context, text string, tags []Tag, examples []FewShotExample, priorDecisions []PriorDecision, frame ConceptFrame) (ScoreResult, error) {
	result, err := t.base.Score(ctx, text, tags, examples, priorDecisions, frame)
	if err != nil || (len(result.Tags) == 0 && len(result.Negated) == 0) {
		return result, err
	}

	descriptions := make(map[string]string, len(tags))
	for _, tag := range tags {
		descriptions[strings.ToLower(tag.Name)] = tag.Description
	}
	questions := make(map[string]jevQuestion, len(result.Tags)+len(result.Negated))
	for i, tag := range result.Tags {
		description := tag.Description
		if description == "" {
			description = descriptions[strings.ToLower(tag.Tag)]
		}
		questions[fmt.Sprintf("positive_%d", i)] = jevQuestion{
			Type: "score", Instructions: fmt.Sprintf("How centrally does `issue` concern the tag %q (%s)? Judge the issue's meaning, not whether the word appears.", tag.Tag, description),
			Criteria: jevRelevanceLevels,
		}
	}
	for i, tag := range result.Negated {
		questions[fmt.Sprintf("negative_%d", i)] = jevQuestion{
			Type: "noul", Instructions: fmt.Sprintf("Does `issue` explicitly refute the tag %q? An absent or irrelevant tag is not a refuted tag.", tag.Tag),
		}
	}
	response, err := t.evaluate(ctx, jevRequest{State: map[string]any{"issue": text, "project_overview": frame.Overview}, Model: t.model, Questions: questions})
	if err != nil {
		return ScoreResult{}, err
	}
	for i := range result.Tags {
		id := fmt.Sprintf("positive_%d", i)
		answer, ok := response.Answers[id]
		if !ok || answer.Type != "score" || answer.Score == nil || !finiteInRange(*answer.Score, 0, 4) {
			return ScoreResult{}, fmt.Errorf("invalid Jev score answer %q", id)
		}
		result.Tags[i].Relevance = *answer.Score / 4
	}
	for i := range result.Negated {
		id := fmt.Sprintf("negative_%d", i)
		answer, ok := response.Answers[id]
		if !ok || answer.Type != "noul" || answer.Noul == nil || !finiteInRange(*answer.Noul, 0, 1) {
			return ScoreResult{}, fmt.Errorf("invalid Jev negation answer %q", id)
		}
		result.Negated[i].Confidence = *answer.Noul
	}
	if t.usageObserver != nil {
		t.usageObserver(TokenUsage{InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens})
	}
	return result, nil
}

func finiteInRange(value, low, high float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= low && value <= high
}

func (t *JevReweightedTagger) evaluate(ctx context.Context, request jevRequest) (jevResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return jevResponse{}, fmt.Errorf("encode Jev request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return jevResponse{}, fmt.Errorf("build Jev request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return jevResponse{}, fmt.Errorf("call Jev: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return jevResponse{}, fmt.Errorf("Jev API returned HTTP %d", resp.StatusCode)
	}
	var result jevResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return jevResponse{}, fmt.Errorf("decode Jev response: %w", err)
	}
	return result, nil
}
