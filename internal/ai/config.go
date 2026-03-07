package ai

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func NewAnalyzerFromEnv() (*Analyzer, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER")))
	if runningUnderGoTest() && provider != "" && provider != "stub" {
		return nil, fmt.Errorf("AI_PROVIDER %q is disabled during go test; use stub or inject fakes", provider)
	}

	switch provider {
	case "", "stub":
		return NewAnalyzer(NewStubTagger(), NewStubEmbedder()), nil
	case "openai":
		cfg := OpenAIConfig{
			APIKey:         os.Getenv("OPENAI_API_KEY"),
			BaseURL:        os.Getenv("OPENAI_BASE_URL"),
			TagModel:       os.Getenv("OPENAI_TAG_MODEL"),
			EmbeddingModel: os.Getenv("OPENAI_EMBED_MODEL"),
		}

		tagger, err := NewOpenAITagger(cfg)
		if err != nil {
			return nil, err
		}
		embedder, err := NewOpenAIEmbedder(cfg)
		if err != nil {
			return nil, err
		}
		return NewAnalyzer(tagger, embedder), nil
	default:
		return nil, fmt.Errorf("unsupported AI_PROVIDER %q", provider)
	}
}

func runningUnderGoTest() bool {
	return flag.Lookup("test.v") != nil
}
