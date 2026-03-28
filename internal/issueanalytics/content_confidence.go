package issueanalytics

import (
	"math"
	"regexp"
	"strings"
)

var (
	contentConfidenceTokenPattern    = regexp.MustCompile(`[A-Za-z0-9_./:-]+`)
	contentConfidenceSentencePattern = regexp.MustCompile(`[.!?]+(?:\s|$)`)
	contentConfidenceListPattern     = regexp.MustCompile(`^\s*(?:[-*]\s+|\d+[.)]\s+)`)
	contentConfidenceErrorPattern    = regexp.MustCompile(`\b(error|exception|trace|stack)\b`)
	contentConfidenceNumberPattern   = regexp.MustCompile(`\b(?:v?\d+(?:\.\d+){1,}|\d{2,})\b`)
)

type ContentConfidenceFeatures struct {
	TokenCount        int
	UniqueRatio       float64
	StructureSignal   float64
	RepetitionPenalty float64
	Confidence        float64
}

func ComputeContentConfidence(raw string) float64 {
	return computeContentConfidenceFeatures(raw).Confidence
}

func computeContentConfidenceFeatures(raw string) ContentConfidenceFeatures {
	normalized := normalizeContentConfidenceText(raw)
	if normalized == "" {
		return ContentConfidenceFeatures{}
	}

	lower := strings.ToLower(normalized)
	tokens := contentConfidenceTokenPattern.FindAllString(lower, -1)
	if len(tokens) == 0 {
		return ContentConfidenceFeatures{}
	}

	contentTokens := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if len(token) >= 3 {
			contentTokens = append(contentTokens, token)
		}
	}

	uniqueRatio := 0.0
	if len(contentTokens) > 0 {
		seen := make(map[string]struct{}, len(contentTokens))
		for _, token := range contentTokens {
			seen[token] = struct{}{}
		}
		uniqueRatio = float64(len(seen)) / float64(len(contentTokens))
	}

	lengthSignal := 1 - math.Exp(-float64(len(tokens))/80.0)
	diversitySignal := clamp01((uniqueRatio - 0.25) / 0.45)
	if len(contentTokens) < 12 {
		diversitySignal *= clamp01((float64(len(contentTokens)) - 4) / 8.0)
	}

	structureSignal := contentConfidenceStructureSignal(normalized, lower, tokens)
	repetitionPenalty := contentConfidenceRepetitionPenalty(normalized, len(tokens), uniqueRatio)

	confidence := clamp01(
		0.25 +
			0.45*lengthSignal +
			0.15*diversitySignal +
			0.20*structureSignal -
			0.25*repetitionPenalty,
	)

	return ContentConfidenceFeatures{
		TokenCount:        len(tokens),
		UniqueRatio:       uniqueRatio,
		StructureSignal:   structureSignal,
		RepetitionPenalty: repetitionPenalty,
		Confidence:        confidence,
	}
}

func normalizeContentConfidenceText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	normalized := strings.ReplaceAll(trimmed, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	lines := strings.Split(normalized, "\n")
	collapsed := make([]string, 0, len(lines))
	lastBlank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
			collapsed = append(collapsed, "")
			continue
		}
		lastBlank = false
		collapsed = append(collapsed, line)
	}
	return strings.TrimSpace(strings.Join(collapsed, "\n"))
}

func contentConfidenceStructureSignal(normalized string, lower string, tokens []string) float64 {
	matched := 0

	if len(contentConfidenceSentencePattern.FindAllString(normalized, -1)) >= 2 {
		matched++
	}

	nonEmptyLines := 0
	listLike := false
	for _, line := range strings.Split(normalized, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		nonEmptyLines++
		if contentConfidenceListPattern.MatchString(line) {
			listLike = true
		}
	}
	if nonEmptyLines >= 2 {
		matched++
	}
	if listLike {
		matched++
	}
	if contentConfidenceHasTechnicalDetail(normalized, lower, tokens) {
		matched++
	}

	return float64(matched) / 4.0
}

func contentConfidenceHasTechnicalDetail(normalized string, lower string, tokens []string) bool {
	if strings.Contains(normalized, "`") || contentConfidenceErrorPattern.MatchString(lower) || contentConfidenceNumberPattern.MatchString(lower) {
		return true
	}
	for _, token := range tokens {
		if strings.Contains(token, "/") || strings.Contains(token, ":") {
			return true
		}
	}
	return false
}

func contentConfidenceRepetitionPenalty(normalized string, tokenCount int, uniqueRatio float64) float64 {
	lines := strings.Split(normalized, "\n")
	seen := make(map[string]int, len(lines))
	nonEmpty := 0
	duplicateLines := 0
	for _, line := range lines {
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if trimmed == "" {
			continue
		}
		nonEmpty++
		if seen[trimmed] > 0 {
			duplicateLines++
		}
		seen[trimmed]++
	}

	duplicateLineSignal := 0.0
	if nonEmpty > 0 {
		duplicateLineSignal = float64(duplicateLines) / float64(nonEmpty)
	}

	lowDiversitySignal := 0.0
	if tokenCount >= 12 && uniqueRatio < 0.35 {
		lowDiversitySignal = 1
	}

	return clamp01(max(duplicateLineSignal, lowDiversitySignal))
}
