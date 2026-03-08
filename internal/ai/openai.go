package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultOpenAIBaseURL        = "https://api.openai.com/v1"
	defaultOpenAITagModel       = "gpt-4.1-mini"
	defaultOpenAIEmbeddingModel = "text-embedding-3-small"
)

type OpenAIConfig struct {
	APIKey         string
	BaseURL        string
	TagModel       string
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
	Tags []TagScore `json:"tags"`
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
		model:  withDefault(strings.TrimSpace(cfg.TagModel), defaultOpenAITagModel),
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

func (t *OpenAITagger) Score(ctx context.Context, text string, tags []Tag) ([]TagScore, error) {
	request := openAIChatCompletionRequest{
		Model:       t.model,
		Temperature: 0,
		ResponseFormat: &openAIResponseFormat{
			Type: "json_object",
		},
		Messages: []openAIChatMessageRequest{
			{
				Role:    "system",
				Content: buildOpenAITaggingSystemPrompt(),
			},
			{
				Role:    "user",
				Content: buildOpenAITaggingPrompt(text, tags),
			},
		},
	}

	var response openAIChatCompletionResponse
	if err := t.client.doJSON(ctx, http.MethodPost, "/chat/completions", request, &response); err != nil {
		return nil, err
	}
	if len(response.Choices) == 0 {
		return nil, errors.New("openai returned no completion choices")
	}

	var payload openAITagScoresResponse
	if err := json.Unmarshal([]byte(response.Choices[0].Message.Content), &payload); err != nil {
		return nil, fmt.Errorf("decode tag response: %w", err)
	}
	return payload.Tags, nil
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
				Role:    "system",
				Content: buildOpenAICanonicalizationSystemPrompt(),
			},
			{
				Role:    "user",
				Content: buildOpenAICanonicalizationPrompt(posts),
			},
		},
	}

	var response openAIChatCompletionResponse
	if err := c.client.doJSON(ctx, http.MethodPost, "/chat/completions", request, &response); err != nil {
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
	if err := e.client.doJSON(ctx, http.MethodPost, "/embeddings", request, &response); err != nil {
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

func (c *openAIClient) doJSON(ctx context.Context, method string, requestPath string, requestBody any, responseBody any) error {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

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

func buildOpenAITaggingPrompt(text string, tags []Tag) string {
	var builder strings.Builder
	builder.WriteString("Classify the issue against the supplied taxonomy.\n")
	builder.WriteString("Consider issue type, failure mode, affected surface, platform, and user workflow.\n")
	builder.WriteString("Reuse supplied tags when they fit. Suggest at most 2 new tags only if an important concept is missing from the taxonomy.\n\n")
	builder.WriteString("Issue text:\n")
	builder.WriteString(text)
	builder.WriteString("\n\nSupplied taxonomy (reuse these exact labels when applicable):\n")
	for _, tag := range tags {
		builder.WriteString("- ")
		builder.WriteString(tag.Name)
		if tag.Description != "" {
			builder.WriteString(": ")
			builder.WriteString(tag.Description)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func buildOpenAITaggingSystemPrompt() string {
	return "Classify issue text against the supplied taxonomy and return a JSON object with a single key named tags. " +
		"tags must be an array of objects sorted by descending relevance. " +
		"Each object must include tag and relevance. " +
		"Suggested tags may also include suggested=true and description. " +
		"Relevance must be between 0 and 1. Omit tags below 0.05 relevance. " +
		"Prefer supplied taxonomy tags whenever they reasonably fit. " +
		"Use multiple existing tags before inventing a new one. " +
		"Suggest a new tag only when a materially important concept cannot be expressed well by the supplied taxonomy. " +
		"Do not suggest synonyms, spelling variants, or narrower/broader restatements of existing tags. " +
		"Suggested tags must be short, lowercase, reusable across many issues, and not product-specific. " +
		"Return at most 2 suggested tags, and every suggested tag must include a short description."
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
		builder.WriteString(fmt.Sprintf("Post %d:\n", index+1))
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

func withDefault(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
