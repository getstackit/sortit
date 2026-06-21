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
	defaultOpenAIBaseURL        = "https://api.openai.com/v1"
	defaultOpenAITagModel       = "gpt-5.4-nano"
	defaultOpenAICanonicalModel = "gpt-5.4-mini"
	defaultOpenAIEmbeddingModel = "text-embedding-3-small"

	// Chat message roles.
	chatRoleSystem = "system"
	chatRoleUser   = "user"
)

type OpenAIConfig struct {
	APIKey         string
	BaseURL        string
	TagModel       string
	CanonicalModel string
	EmbeddingModel string
	HTTPClient     *http.Client
}

type openAIClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type OpenAITagger struct {
	client *openAIClient
	model  string
}

type OpenAICanonicalizer struct {
	client *openAIClient
	model  string
}

type OpenAIConceptProfiler struct {
	client *openAIClient
	model  string
}

type OpenAIEmbedder struct {
	client         *openAIClient
	model          string
	maxChunkTokens int
	previewSize    int
}

type openAIChatCompletionRequest struct {
	Model          string                     `json:"model"`
	Temperature    float64                    `json:"temperature"`
	ResponseFormat *openAIResponseFormat      `json:"response_format,omitempty"`
	Messages       []openAIChatMessageRequest `json:"messages"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatMessageRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type openAITagScoresResponse struct {
	Tags        []TagScore   `json:"tags"`
	NegatedTags []NegatedTag `json:"negated_tags,omitempty"`
}

type openAISpecificityResponse struct {
	Specificity float64 `json:"specificity"`
}

type openAIEmbeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func NewOpenAITagger(cfg OpenAIConfig) (*OpenAITagger, error) {
	client, err := newOpenAIClient(cfg)
	if err != nil {
		return nil, err
	}
	return &OpenAITagger{
		client: client,
		model:  withDefault(strings.TrimSpace(cfg.TagModel), defaultOpenAITagModel),
	}, nil
}

func NewOpenAICanonicalizer(cfg OpenAIConfig) (*OpenAICanonicalizer, error) {
	client, err := newOpenAIClient(cfg)
	if err != nil {
		return nil, err
	}
	return &OpenAICanonicalizer{
		client: client,
		model:  withDefault(strings.TrimSpace(cfg.CanonicalModel), defaultOpenAICanonicalModel),
	}, nil
}

func NewOpenAIConceptProfiler(cfg OpenAIConfig) (*OpenAIConceptProfiler, error) {
	client, err := newOpenAIClient(cfg)
	if err != nil {
		return nil, err
	}
	return &OpenAIConceptProfiler{
		client: client,
		model:  withDefault(strings.TrimSpace(cfg.CanonicalModel), defaultOpenAICanonicalModel),
	}, nil
}

func NewOpenAIEmbedder(cfg OpenAIConfig) (*OpenAIEmbedder, error) {
	client, err := newOpenAIClient(cfg)
	if err != nil {
		return nil, err
	}
	return &OpenAIEmbedder{
		client:         client,
		model:          withDefault(strings.TrimSpace(cfg.EmbeddingModel), defaultOpenAIEmbeddingModel),
		maxChunkTokens: defaultEmbeddingChunkTokens,
		previewSize:    defaultEmbeddingPreviewSize,
	}, nil
}

func (t *OpenAITagger) Score(ctx context.Context, text string, tags []Tag, examples []FewShotExample, priorDecisions []PriorDecision, frame ConceptFrame) (ScoreResult, error) {
	request := openAIChatCompletionRequest{
		Model:       t.model,
		Temperature: 0,
		ResponseFormat: &openAIResponseFormat{
			Type: "json_object",
		},
		Messages: []openAIChatMessageRequest{
			{
				Role:    chatRoleSystem,
				Content: buildOpenAITaggingSystemPrompt(frame),
			},
			{
				Role:    chatRoleUser,
				Content: buildOpenAITaggingPrompt(text, tags, examples, priorDecisions...),
			},
		},
	}

	var response openAIChatCompletionResponse
	if err := t.client.doJSON(ctx, "/chat/completions", request, &response); err != nil {
		return ScoreResult{}, err
	}
	if len(response.Choices) == 0 {
		return ScoreResult{}, errors.New("openai returned no completion choices")
	}

	var payload openAITagScoresResponse
	if err := json.Unmarshal([]byte(response.Choices[0].Message.Content), &payload); err != nil {
		return ScoreResult{}, fmt.Errorf("decode tag response: %w", err)
	}
	return ScoreResult{Tags: payload.Tags, Negated: payload.NegatedTags}, nil
}

func (t *OpenAITagger) Provider() string {
	return "openai"
}

func (t *OpenAITagger) Model() string {
	return t.model
}

func (c *OpenAICanonicalizer) CanonicalizeDiscussion(ctx context.Context, posts []string) (string, error) {
	request := openAIChatCompletionRequest{
		Model:       c.model,
		Temperature: 0,
		Messages: []openAIChatMessageRequest{
			{
				Role:    chatRoleSystem,
				Content: buildOpenAICanonicalizationSystemPrompt(),
			},
			{
				Role:    chatRoleUser,
				Content: buildOpenAICanonicalizationPrompt(posts),
			},
		},
	}

	var response openAIChatCompletionResponse
	if err := c.client.doJSON(ctx, "/chat/completions", request, &response); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", errors.New("openai returned no completion choices")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

func (e *OpenAIEmbedder) EmbedText(ctx context.Context, text string) (EmbeddingResult, error) {
	return embedChunkedText(ctx, text, e.maxChunkTokens, e.previewSize, e.embedTexts)
}

func (e *OpenAIEmbedder) embedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	request := openAIEmbeddingRequest{
		Model:          e.model,
		Input:          texts,
		EncodingFormat: "float",
	}

	var response openAIEmbeddingResponse
	if err := e.client.doJSON(ctx, "/embeddings", request, &response); err != nil {
		return nil, err
	}

	embeddings := make([][]float32, len(response.Data))
	for _, item := range response.Data {
		if item.Index < 0 || item.Index >= len(embeddings) {
			return nil, fmt.Errorf("openai returned embedding index %d out of bounds", item.Index)
		}
		embeddings[item.Index] = item.Embedding
	}
	return embeddings, nil
}

func (e *OpenAIEmbedder) Provider() string {
	return "openai"
}

func (e *OpenAIEmbedder) Model() string {
	return e.model
}

func newOpenAIClient(cfg OpenAIConfig) (*openAIClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("openai api key is required")
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &openAIClient{
		baseURL:    strings.TrimRight(withDefault(strings.TrimSpace(cfg.BaseURL), defaultOpenAIBaseURL), "/"),
		apiKey:     cfg.APIKey,
		httpClient: client,
	}, nil
}

func (c *openAIClient) doJSON(ctx context.Context, requestPath string, requestBody any, responseBody any) error {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+requestPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiError openAIErrorResponse
		if json.Unmarshal(payload, &apiError) == nil && apiError.Error.Message != "" {
			return fmt.Errorf("openai api error (%d): %s", resp.StatusCode, apiError.Error.Message)
		}
		return fmt.Errorf("openai api error (%d)", resp.StatusCode)
	}

	if err := json.Unmarshal(payload, responseBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func buildOpenAITaggingPrompt(text string, tags []Tag, examples []FewShotExample, priorDecisions ...PriorDecision) string {
	var builder strings.Builder
	builder.WriteString("Classify the issue against the supplied taxonomy.\n")
	builder.WriteString("Consider issue type, failure mode, affected surface, platform, and user workflow.\n")
	builder.WriteString("The taxonomy spans multiple axes such as kind, failure mode, surface, platform, experience, and implementation layer. Use the best matching tags across those axes when the issue supports them.\n")
	builder.WriteString("Reuse supplied tags when they fit. Prefer specific tags over generic ones.\n")
	builder.WriteString("Only suggest a new tag if an important residual concept remains after choosing the best existing tags.\n")
	builder.WriteString("Prefer a concrete reusable surface, subsystem, workflow, or artifact tag over broad buckets like backend, frontend, or ui.\n")
	builder.WriteString("Do not suggest a tag that is already implied by a combination of existing tags.\n")
	builder.WriteString("Examples are reference patterns, not templates. Do not copy example-only tags unless the same surface, subsystem, workflow, or artifact is clearly supported by the current issue text.\n")
	builder.WriteString("Prefer a compact tag set. Add another tag only when it contributes distinct evidence-backed information rather than a near-synonym or predictable co-occurrence.\n")
	builder.WriteString("When you can identify supporting text, include an evidence array of 1 to 3 short verbatim quotes copied from the issue text that justify the tag. Quotes must appear character-for-character in the issue text — do not paraphrase, translate, summarize, or normalize. Each quote should be 3 to 15 words. Tags with evidence citations are treated as higher confidence. It is acceptable to assign a tag without evidence when the issue clearly warrants it, but grounded tags are preferred.\n")
	builder.WriteString("Also identify tags from the supplied taxonomy that the issue text EXPLICITLY REFUTES, and return them in a negated_tags array. The taxonomy below is the candidate set for negation as well; never invent a negated tag name.\n")

	if len(examples) > 0 {
		builder.WriteString("\nHere are examples of well-tagged issues for reference:\n")
		for i, ex := range examples {
			fmt.Fprintf(&builder, "\nExample %d:\n", i+1)
			fmt.Fprintf(&builder, "Issue: %s\n", truncateExampleText(ex.Text, 200))
			builder.WriteString("Tags: ")
			for j, tag := range ex.Tags {
				if j > 0 {
					builder.WriteString(", ")
				}
				fmt.Fprintf(&builder, "%s (%.2f)", tag.Name, tag.Relevance)
			}
			builder.WriteString("\n")
		}
	}

	if len(priorDecisions) > 0 {
		builder.WriteString("\nDocumented prior decisions (permanent memories) relevant to this area. ")
		builder.WriteString("If the issue restates one of these, lean on the tags it already uses for consistency; ")
		builder.WriteString("do not invent new tags for a question already settled here:\n")
		for i, decision := range priorDecisions {
			title := strings.TrimSpace(decision.Title)
			if title == "" {
				title = fmt.Sprintf("Decision %d", i+1)
			}
			fmt.Fprintf(&builder, "- %s", title)
			if len(decision.Tags) > 0 {
				fmt.Fprintf(&builder, " [%s]", strings.Join(decision.Tags, ", "))
			}
			if summary := strings.TrimSpace(decision.Summary); summary != "" {
				fmt.Fprintf(&builder, ": %s", truncateExampleText(summary, 200))
			}
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\nIssue text:\n")
	builder.WriteString(text)

	// Separate hint tags from regular taxonomy tags.
	var hintTags, regularTags []Tag
	for _, tag := range tags {
		if tag.Hint {
			hintTags = append(hintTags, tag)
		} else {
			regularTags = append(regularTags, tag)
		}
	}

	builder.WriteString("\n\nSupplied taxonomy (reuse these exact labels when applicable):\n")
	for _, tag := range regularTags {
		builder.WriteString("- ")
		builder.WriteString(tag.Name)
		if tag.Description != "" {
			builder.WriteString(": ")
			builder.WriteString(tag.Description)
		}
		builder.WriteString("\n")
	}

	if len(hintTags) > 0 {
		builder.WriteString("\nHigh-affinity tags (embedding similarity suggests these may be relevant — assign them if they fit):\n")
		for _, tag := range hintTags {
			builder.WriteString("- ")
			builder.WriteString(tag.Name)
			if tag.Description != "" {
				builder.WriteString(": ")
				builder.WriteString(tag.Description)
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// truncateExampleText truncates text to roughly maxWords words to limit prompt
// token growth from few-shot examples.
func truncateExampleText(text string, maxWords int) string {
	words := strings.Fields(text)
	if len(words) <= maxWords {
		return text
	}
	return strings.Join(words[:maxWords], " ") + "…"
}

// conceptFramePreamble renders the project's identity frame as grounding that
// prefixes the tagging system prompt. It is the stable, cacheable prefix: the
// overview and concept set change rarely, so a re-enrich batch reuses it. The
// wording is adoption-first — it tells the model to prefer the project's own
// concept names over generic substitutes when an issue genuinely concerns a
// concept, with a lighter evidence-and-no-unrelated-nouns guard (the verifier and
// the corpus show over-application is the rarer failure than under-adoption). An
// empty frame renders nothing, so the prompt is byte-identical to the pre-frame
// prompt on a fresh install.
func conceptFramePreamble(frame ConceptFrame) string {
	if frame.IsEmpty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("You are tagging issues for the following project. ")
	if overview := strings.TrimSpace(frame.Overview); overview != "" {
		b.WriteString("Project: ")
		b.WriteString(overview)
		if !strings.HasSuffix(overview, ".") {
			b.WriteString(".")
		}
		b.WriteString(" ")
	}
	if len(frame.Concepts) > 0 {
		b.WriteString("This project's core concepts (its own vocabulary): ")
		for _, c := range frame.Concepts {
			tag := strings.TrimSpace(c.SubjectTag)
			if tag == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(tag)
			if profile := strings.TrimSpace(c.Profile); profile != "" {
				b.WriteString(" — ")
				b.WriteString(profile)
			}
			b.WriteString(" ")
		}
	}
	b.WriteString("Use this to place each issue within the project and to prefer the project's own concept names: when an issue genuinely concerns one of these concepts, assign that concept's tag rather than a generic substitute. ")
	b.WriteString("They are context, not a checklist — assign a concept only on evidence in the issue text, and don't attach an unrelated project noun. ")
	return b.String()
}

func buildOpenAITaggingSystemPrompt(frame ConceptFrame) string {
	return conceptFramePreamble(frame) +
		"Classify issue text against the supplied taxonomy and return a JSON object with two keys: tags and negated_tags. " +
		"tags must be an array of objects sorted by descending relevance. " +
		"Each object must include tag and relevance. " +
		"Each object may also include an evidence array of 1 to 3 short verbatim quotes copied character-for-character from the issue text that justify the tag. " +
		"Do not paraphrase, translate, summarize, fix typos, or normalize whitespace inside evidence quotes. " +
		"Each evidence quote should be 3 to 15 words. " +
		"Tags with evidence citations are treated as higher confidence. It is acceptable to omit evidence when the issue clearly warrants the tag. " +
		"Suggested tags may also include suggested=true and description. " +
		"Relevance must be between 0 and 1 inclusive, using up to 2 decimal places. " +
		"Omit tags below 0.08 relevance. " +
		"Use supplied taxonomy tags by default. " +
		"The taxonomy may include multiple axes such as issue kind, failure mode, affected surface, platform, experience, and implementation layer. " +
		"Use the best supported tags across those axes instead of inventing cross-product tags. " +
		"Prefer specific tags over generic ones — a tag that names a concrete subsystem or artifact is better than a broad category. " +
		"When high-affinity tags are listed, evaluate each one carefully and assign it if it genuinely fits the issue; these tags were selected because their embeddings are close to the issue text. " +
		"Treat suggested tags as rare taxonomy expansions, not labels for one issue. " +
		"First assign the best existing tags, aiming for the most specific and descriptive set. " +
		"Then identify whether one materially important residual concept is not already expressed by any combination of 1 to 3 existing tags. " +
		"If an important concept remains unexplained by any existing tag, suggest a new tag. " +
		"Only suggest a new tag when that residual concept is central to the issue, likely to recur across future issues, specific enough to name a concrete surface, subsystem, workflow, or artifact, and orthogonal to the existing taxonomy rather than a predictable co-occurrence. " +
		"Do not suggest synonyms, spelling variants, parent or child categories, narrower or broader restatements, platform-specific variants, or tags that merely restate issue type, browser, platform, frontend, backend, ui, ux, performance, or crash when those dimensions are already covered by existing tags. " +
		"Prefer combinations of existing tags over inventing a new one. " +
		"Prefer one strong suggested tag over two weak ones. " +
		"Suggested tags must be short, lowercase, reusable, and corpus-shaping. " +
		"Every suggested tag must include a description that states what it captures and what it excludes. " +
		"Return at most 1 suggested tag unless 2 independent missing concepts are both central. " +
		"negated_tags must be an array (use [] when nothing is refuted) of objects describing tags from the supplied taxonomy that the issue text EXPLICITLY REFUTES. " +
		"Each object must include tag (verbatim taxonomy label), confidence in [0, 1], and an evidence array of 1 to 3 short verbatim quotes from the issue text containing the refutation. " +
		"A tag is refuted only when the text contains direct negation: phrases like \"not a regression\", \"doesn't affect Safari\", \"working as designed, not a bug\", or \"customer confirmed this is intentional\". " +
		"Do not mark a tag as negated merely because the text doesn't mention it, because you suspect it might not apply, or because another tag fits better. Negation requires textual evidence. " +
		"Never invent tag names in negated_tags; only refute tags that appear in the supplied taxonomy verbatim. " +
		"Evidence for negated tags is required, not optional, and will be cross-verified against the source text; entries without evidence are discarded."
}

func (t *OpenAITagger) ScoreSpecificity(ctx context.Context, tag Tag, catalog []Tag) (float64, error) {
	request := openAIChatCompletionRequest{
		Model:       t.model,
		Temperature: 0,
		ResponseFormat: &openAIResponseFormat{
			Type: "json_object",
		},
		Messages: []openAIChatMessageRequest{
			{
				Role:    chatRoleSystem,
				Content: buildOpenAISpecificitySystemPrompt(),
			},
			{
				Role:    chatRoleUser,
				Content: buildOpenAISpecificityPrompt(tag, catalog),
			},
		},
	}

	var response openAIChatCompletionResponse
	if err := t.client.doJSON(ctx, "/chat/completions", request, &response); err != nil {
		return 0, err
	}
	if len(response.Choices) == 0 {
		return 0, errors.New("openai returned no completion choices")
	}

	var payload openAISpecificityResponse
	if err := json.Unmarshal([]byte(response.Choices[0].Message.Content), &payload); err != nil {
		return 0, fmt.Errorf("decode specificity response: %w", err)
	}

	return math.Max(0, math.Min(1, math.Round(payload.Specificity*100)/100)), nil
}

func buildOpenAISpecificitySystemPrompt() string {
	return "Score how semantically specific a tag is within its tag catalog. " +
		"Return a JSON object with a single key named specificity. " +
		"specificity must be a float between 0 and 1 inclusive, using up to 2 decimal places. " +
		"A score of 0 means the tag is extremely broad and generic (e.g. 'backend', 'frontend', 'ui'). " +
		"A score of 1 means the tag is maximally narrow and precise (e.g. 'safari-flexbox-gap', 'stripe-webhook-retry'). " +
		"Consider the full catalog for relative reasoning: a tag is more specific if more general alternatives exist in the catalog. " +
		"A tag is less specific if it could apply to nearly any issue without discriminating. " +
		"Focus on how narrowly the tag scopes a problem domain, subsystem, or feature area."
}

func buildOpenAISpecificityPrompt(tag Tag, catalog []Tag) string {
	var builder strings.Builder
	builder.WriteString("Score the specificity of this tag:\n\n")
	builder.WriteString("Tag: ")
	builder.WriteString(tag.Name)
	if tag.Description != "" {
		builder.WriteString("\nDescription: ")
		builder.WriteString(tag.Description)
	}
	builder.WriteString("\n\nFull tag catalog for context:\n")
	for _, t := range catalog {
		builder.WriteString("- ")
		builder.WriteString(t.Name)
		if t.Description != "" {
			builder.WriteString(": ")
			builder.WriteString(t.Description)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func buildOpenAICanonicalizationPrompt(posts []string) string {
	var builder strings.Builder
	builder.WriteString("Rewrite this issue discussion into one concise canonical issue description.\n")
	builder.WriteString("Use the combined discussion, but prefer the latest corrected understanding when details conflict.\n")
	builder.WriteString("Return plain text only.\n\n")

	for index, post := range posts {
		post = strings.TrimSpace(post)
		if post == "" {
			continue
		}
		fmt.Fprintf(&builder, "Post %d:\n", index+1)
		builder.WriteString(post)
		builder.WriteString("\n\n")
	}

	return strings.TrimSpace(builder.String())
}

func buildOpenAICanonicalizationSystemPrompt() string {
	return "You turn an issue discussion into a single canonical issue description for triage, tagging, and semantic search. " +
		"Preserve the latest understanding, concrete failures, requested changes, affected surfaces, and important constraints. " +
		"Do not mention discussion mechanics, authors, or timestamps."
}

func (p *OpenAIConceptProfiler) GenerateConceptProfile(ctx context.Context, tag string, issueSummaries []string) (string, error) {
	request := openAIChatCompletionRequest{
		Model:       p.model,
		Temperature: 0,
		Messages: []openAIChatMessageRequest{
			{
				Role:    chatRoleSystem,
				Content: buildOpenAIConceptProfileSystemPrompt(),
			},
			{
				Role:    chatRoleUser,
				Content: buildOpenAIConceptProfilePrompt(tag, issueSummaries),
			},
		},
	}

	var response openAIChatCompletionResponse
	if err := p.client.doJSON(ctx, "/chat/completions", request, &response); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", errors.New("openai returned no completion choices")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

type openAIClusterConceptResponse struct {
	Name    string `json:"name"`
	Profile string `json:"profile"`
}

// ProposeConceptFromCluster names and profiles a new concept from a cluster of
// issues sharing an unexplained residual. It returns the proposed subject tag and
// its profile as a JSON object, primed by the project frame so the invented name
// fits the project's vocabulary.
func (p *OpenAIConceptProfiler) ProposeConceptFromCluster(ctx context.Context, issueSummaries []string, frame ConceptFrame) (string, string, error) {
	request := openAIChatCompletionRequest{
		Model:       p.model,
		Temperature: 0,
		ResponseFormat: &openAIResponseFormat{
			Type: "json_object",
		},
		Messages: []openAIChatMessageRequest{
			{
				Role:    chatRoleSystem,
				Content: buildOpenAIClusterConceptSystemPrompt(frame),
			},
			{
				Role:    chatRoleUser,
				Content: buildOpenAIClusterConceptPrompt(issueSummaries),
			},
		},
	}

	var response openAIChatCompletionResponse
	if err := p.client.doJSON(ctx, "/chat/completions", request, &response); err != nil {
		return "", "", err
	}
	if len(response.Choices) == 0 {
		return "", "", errors.New("openai returned no completion choices")
	}

	var payload openAIClusterConceptResponse
	if err := json.Unmarshal([]byte(response.Choices[0].Message.Content), &payload); err != nil {
		return "", "", fmt.Errorf("decode cluster concept response: %w", err)
	}
	return strings.TrimSpace(payload.Name), strings.TrimSpace(payload.Profile), nil
}

func buildOpenAIClusterConceptSystemPrompt(frame ConceptFrame) string {
	return conceptFramePreamble(frame) +
		"A set of issues shares a concept the existing tags fail to capture — they cluster together in the unexplained residual of the tag model. " +
		"Name that shared concept and profile it. " +
		"Return a JSON object with two keys: name and profile. " +
		"name is a short, lowercase, reusable subject tag (1 to 3 words) naming the concrete subsystem, workflow, surface, or artifact the issues share — fit the project's existing vocabulary and naming style. " +
		"Do not invent a synonym for an existing tag, a generic bucket (improvement, backend, ui, performance), or a restatement of issue type; name the specific missing noun. " +
		"profile is concise plain prose: what the concept is, why it exists, and how it behaves, grounded in the example issues. No headings, no issue IDs."
}

func buildOpenAIClusterConceptPrompt(issueSummaries []string) string {
	var builder strings.Builder
	builder.WriteString("Issues that share an unexplained concept:\n")
	for index, summary := range issueSummaries {
		summary = strings.TrimSpace(summary)
		if summary == "" {
			continue
		}
		fmt.Fprintf(&builder, "Issue %d:\n", index+1)
		builder.WriteString(summary)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Name and profile the concept these issues share.")
	return strings.TrimSpace(builder.String())
}

func buildOpenAIConceptProfilePrompt(tag string, issueSummaries []string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Write a canonical profile for the concept tagged %q.\n", strings.TrimSpace(tag))
	builder.WriteString("Cover what it is, why it exists, and how it behaves. Return plain text only.\n\n")

	if len(issueSummaries) > 0 {
		builder.WriteString("Issues that reference this concept:\n")
		for index, summary := range issueSummaries {
			summary = strings.TrimSpace(summary)
			if summary == "" {
				continue
			}
			fmt.Fprintf(&builder, "Issue %d:\n", index+1)
			builder.WriteString(summary)
			builder.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(builder.String())
}

func buildOpenAIConceptProfileSystemPrompt() string {
	return "You write the canonical profile of a single engineering concept — a subsystem, component, or domain concept — for a durable knowledge base. " +
		"Explain what it is, why it exists, and how it behaves, grounded in the example issues. Be concise and concrete. " +
		"Return plain prose only; no preamble, no headings, and no mention of issue IDs, authors, or timestamps."
}

func withDefault(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
