package matheval

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"sortit/internal/ai"
	"sortit/internal/centering"
	"sortit/internal/domain"
	issueenrichment "sortit/internal/issueenrichment"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
	"sortit/internal/scoring"
	"sortit/internal/tags"
)

const (
	tagEvalRelevanceFloor = 0.08
	tagEvalK              = scoring.DefaultResultLimit
)

var tagEvalNow = time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)

// TagEvalPrice is a model's standard-processing price in USD per million
// tokens. Cached-token and batch discounts do not apply to this command.
type TagEvalPrice struct {
	InputPerMillion  float64 `json:"inputPerMillionUsd"`
	OutputPerMillion float64 `json:"outputPerMillionUsd"`
}

// TagEvalConfig supplies one model and the deterministic fixture inputs for a
// tagging-fidelity run. Tagger is live in the command, but tests can supply a
// stub; the embedder is always reconstructed from committed fixture vectors.
type TagEvalConfig struct {
	Model     string
	Runs      int
	Corpus    Corpus
	Judgments []Judgment
	Tagger    ai.Tagger
	Usage     func() ai.TokenUsage
	Price     TagEvalPrice
}

type TagEvalArtifact struct {
	GeneratedAt string          `json:"generatedAt"`
	Command     string          `json:"command"`
	Fixture     string          `json:"fixture"`
	Reports     []TagEvalReport `json:"reports"`
}

type TagEvalReport struct {
	Model    string       `json:"model"`
	Runs     int          `json:"runs"`
	Price    TagEvalPrice `json:"price"`
	Baseline TagEvalRank  `json:"baselineRanking"`
	Summary  TagEvalStats `json:"summary"`
	RunData  []TagEvalRun `json:"runData"`
}

// TagEvalRun preserves every per-issue judgment. The summary aggregates this
// exact data, so the committed artifact remains auditable when a model changes.
type TagEvalRun struct {
	Run     int            `json:"run"`
	Stats   TagEvalStats   `json:"stats"`
	Issues  []TagEvalIssue `json:"issues"`
	Ranking TagEvalRank    `json:"ranking"`
}

type TagEvalIssue struct {
	ID                string                `json:"id"`
	ExpectedTags      []string              `json:"expectedTags"`
	PredictedTags     []string              `json:"predictedTags"`
	Precision         float64               `json:"precision"`
	Recall            float64               `json:"recall"`
	F1                float64               `json:"f1"`
	ModelOutput       []ai.TagScore         `json:"modelOutput"`
	ModelNegations    []ai.NegatedTag       `json:"modelNegations"`
	VerifiedTagScores []domain.TagRelevance `json:"verifiedTagScores"`
	TokenUsage        ai.TokenUsage         `json:"tokenUsage"`
}

type TagEvalStats struct {
	Issues                    int           `json:"issues"`
	MicroPrecision            float64       `json:"microPrecision"`
	MicroRecall               float64       `json:"microRecall"`
	MicroF1                   float64       `json:"microF1"`
	MacroPrecision            float64       `json:"macroPrecision"`
	MacroRecall               float64       `json:"macroRecall"`
	MacroF1                   float64       `json:"macroF1"`
	CorrectRelevanceMean      float64       `json:"correctRelevanceMean"`
	IncorrectRelevanceMean    float64       `json:"incorrectRelevanceMean"`
	CorrectAssignmentCount    int           `json:"correctAssignmentCount"`
	IncorrectAssignmentCount  int           `json:"incorrectAssignmentCount"`
	NegationFalsePositives    int           `json:"negationFalsePositives"`
	NegationCount             int           `json:"negationCount"`
	NegationFalsePositiveRate float64       `json:"negationFalsePositiveRate"`
	InputTokensPerIssue       float64       `json:"inputTokensPerIssue"`
	OutputTokensPerIssue      float64       `json:"outputTokensPerIssue"`
	CostPer1KEnrichments      float64       `json:"costPer1KEnrichmentsUsd"`
	Spread                    TagEvalSpread `json:"spread,omitempty"`
}

type TagEvalSpread struct {
	MicroF1Min float64 `json:"microF1Min"`
	MicroF1Max float64 `json:"microF1Max"`
	MacroF1Min float64 `json:"macroF1Min"`
	MacroF1Max float64 `json:"macroF1Max"`
	NDCGMin    float64 `json:"ndcgAt8Min"`
	NDCGMax    float64 `json:"ndcgAt8Max"`
	RecallMin  float64 `json:"recallAt8Min"`
	RecallMax  float64 `json:"recallAt8Max"`
}

type TagEvalRank struct {
	NDCG        float64 `json:"ndcgAt8"`
	Recall      float64 `json:"recallAt8"`
	DeltaNDCG   float64 `json:"deltaNDCGAt8,omitempty"`
	DeltaRecall float64 `json:"deltaRecallAt8,omitempty"`
}

// RunTaggingFidelity runs a tagger serially over a deterministic fixture. It
// deliberately uses AnalysisTrace.ModelOutput for fidelity metrics and final
// verifier output for ranking, separating model quality from deterministic
// post-processing while measuring the production effect of both.
func RunTaggingFidelity(ctx context.Context, cfg TagEvalConfig) (TagEvalReport, error) {
	if cfg.Tagger == nil {
		return TagEvalReport{}, fmt.Errorf("tagger is required")
	}
	if len(cfg.Corpus.Issues) == 0 || len(cfg.Corpus.Tags) == 0 {
		return TagEvalReport{}, fmt.Errorf("fixture corpus with issues and tags is required")
	}
	if cfg.Runs < 1 {
		return TagEvalReport{}, fmt.Errorf("runs must be positive")
	}
	if len(cfg.Judgments) == 0 {
		return TagEvalReport{}, fmt.Errorf("ranking judgments are required")
	}

	baseline, err := tagEvalRanking(cfg.Corpus, cfg.Judgments, nil)
	if err != nil {
		return TagEvalReport{}, fmt.Errorf("baseline ranking: %w", err)
	}
	report := TagEvalReport{
		Model:    cfg.Model,
		Runs:     cfg.Runs,
		Price:    cfg.Price,
		Baseline: baseline,
		RunData:  make([]TagEvalRun, 0, cfg.Runs),
	}

	for run := 1; run <= cfg.Runs; run++ {
		enricher, err := newTagEvalEnricher(cfg.Corpus, cfg.Tagger)
		if err != nil {
			return TagEvalReport{}, err
		}
		issues, err := tagEvalOneRun(ctx, enricher, cfg.Corpus, cfg.Usage)
		if err != nil {
			return TagEvalReport{}, fmt.Errorf("model %s run %d: %w", cfg.Model, run, err)
		}
		variantScores := make(map[string][]domain.TagRelevance, len(issues))
		for _, issue := range issues {
			variantScores[issue.ID] = issue.VerifiedTagScores
		}
		ranking, err := tagEvalRanking(cfg.Corpus, cfg.Judgments, variantScores)
		if err != nil {
			return TagEvalReport{}, fmt.Errorf("model %s run %d ranking: %w", cfg.Model, run, err)
		}
		ranking.DeltaNDCG = ranking.NDCG - baseline.NDCG
		ranking.DeltaRecall = ranking.Recall - baseline.Recall
		report.RunData = append(report.RunData, TagEvalRun{
			Run:     run,
			Issues:  issues,
			Stats:   summarizeTagEvalIssues(issues, cfg.Price),
			Ranking: ranking,
		})
	}
	report.Summary = summarizeTagEvalRuns(report.RunData)
	return report, nil
}

// CombineTagEvalReports joins one or more single-run reports for the same
// model. It is used only to recover a long live matrix from separately invoked
// one-run jobs; a normal tageval invocation produces this shape directly.
func CombineTagEvalReports(reports []TagEvalReport) (TagEvalReport, error) {
	if len(reports) == 0 {
		return TagEvalReport{}, fmt.Errorf("at least one report is required")
	}
	combined := reports[0]
	combined.RunData = nil
	for _, report := range reports {
		if report.Model != combined.Model {
			return TagEvalReport{}, fmt.Errorf("cannot combine %q with %q", combined.Model, report.Model)
		}
		for _, run := range report.RunData {
			run.Run = len(combined.RunData) + 1
			combined.RunData = append(combined.RunData, run)
		}
	}
	combined.Runs = len(combined.RunData)
	combined.Summary = summarizeTagEvalRuns(combined.RunData)
	return combined, nil
}

func newTagEvalEnricher(corpus Corpus, tagger ai.Tagger) (*issueenrichment.IssueEnricher, error) {
	embedder, err := newTagEvalEmbedder(corpus)
	if err != nil {
		return nil, err
	}
	analyzer := ai.NewAnalyzer(tagger, embedder)
	store := &tagEvalTagStore{tags: corpus.StoreTags()}
	catalog := tags.NewCatalogService(store, analyzer, nil)
	enricher := issueenrichment.NewIssueEnricher(analyzer, catalog, nil)
	enricher.SetExemplarPool(nil)
	enricher.UseCentering(&centering.Cache{
		Store: tagEvalIssueLister{items: corpus.StoreIssues(tagEvalNow)},
		Tags:  catalog,
	})
	return enricher, nil
}

func tagEvalOneRun(ctx context.Context, enricher *issueenrichment.IssueEnricher, corpus Corpus, usage func() ai.TokenUsage) ([]TagEvalIssue, error) {
	fixtures := append([]CorpusIssue(nil), corpus.Issues...)
	slices.SortFunc(fixtures, func(a, b CorpusIssue) int { return cmp.Compare(a.ID, b.ID) })
	result := make([]TagEvalIssue, 0, len(fixtures))
	for _, fixture := range fixtures {
		before := tokenUsageSnapshot(usage)
		analysis, err := enricher.AnalyzeText(ctx, fixture.Raw, issueenrichment.AnalyzeTextOptions{
			CandidateMode:      tags.CandidateModeRetrievalShortlist,
			Verify:             true,
			SkipPriorDecisions: true,
		})
		if err != nil {
			return nil, fmt.Errorf("analyze fixture issue %s: %w", fixture.ID, err)
		}
		after := tokenUsageSnapshot(usage)
		expected := tagEvalExpectedTags(fixture.TagScores)
		predicted, scored := tagEvalPredictedTags(analysis.Trace.ModelOutput.Tags)
		precision, recall, f1 := tagEvalSetMetrics(expected, predicted)
		result = append(result, TagEvalIssue{
			ID:                fixture.ID,
			ExpectedTags:      expected,
			PredictedTags:     predicted,
			Precision:         precision,
			Recall:            recall,
			F1:                f1,
			ModelOutput:       append([]ai.TagScore(nil), scored...),
			ModelNegations:    append([]ai.NegatedTag(nil), analysis.Trace.ModelOutput.Negated...),
			VerifiedTagScores: copyTagRelevances(analysis.TagScores),
			TokenUsage: ai.TokenUsage{
				InputTokens:  after.InputTokens - before.InputTokens,
				OutputTokens: after.OutputTokens - before.OutputTokens,
			},
		})
	}
	return result, nil
}

func tagEvalExpectedTags(scores []TagScore) []string {
	tags := make([]string, 0, len(scores))
	for _, score := range scores {
		if name := domain.NormalizeTagName(score.Tag); name != "" {
			tags = append(tags, name)
		}
	}
	slices.Sort(tags)
	return slices.Compact(tags)
}

func tagEvalPredictedTags(scores []ai.TagScore) ([]string, []ai.TagScore) {
	predicted := make([]string, 0, len(scores))
	kept := make([]ai.TagScore, 0, len(scores))
	for _, score := range scores {
		name := domain.NormalizeTagName(score.Tag)
		if name == "" || score.Relevance < tagEvalRelevanceFloor {
			continue
		}
		predicted = append(predicted, name)
		kept = append(kept, score)
	}
	slices.Sort(predicted)
	predicted = slices.Compact(predicted)
	slices.SortFunc(kept, func(a, b ai.TagScore) int {
		if c := cmp.Compare(a.Tag, b.Tag); c != 0 {
			return c
		}
		return cmp.Compare(b.Relevance, a.Relevance)
	})
	return predicted, kept
}

func tagEvalSetMetrics(expected, predicted []string) (precision, recall, f1 float64) {
	expectedSet := tagEvalTagSet(expected)
	truePositives := 0
	for _, name := range predicted {
		if _, ok := expectedSet[name]; ok {
			truePositives++
		}
	}
	precision = tagEvalRatio(truePositives, len(predicted))
	recall = tagEvalRatio(truePositives, len(expected))
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return precision, recall, f1
}

func summarizeTagEvalIssues(items []TagEvalIssue, price TagEvalPrice) TagEvalStats {
	stats := TagEvalStats{Issues: len(items)}
	var tp, predicted, expected int
	var correctSum, incorrectSum float64
	for _, item := range items {
		stats.MacroPrecision += item.Precision
		stats.MacroRecall += item.Recall
		stats.MacroF1 += item.F1
		expectedSet := tagEvalTagSet(item.ExpectedTags)
		predicted += len(item.PredictedTags)
		expected += len(item.ExpectedTags)
		for _, tag := range item.PredictedTags {
			if _, ok := expectedSet[tag]; ok {
				tp++
			}
		}
		for _, score := range item.ModelOutput {
			name := domain.NormalizeTagName(score.Tag)
			if _, ok := expectedSet[name]; ok {
				correctSum += score.Relevance
				stats.CorrectAssignmentCount++
			} else {
				incorrectSum += score.Relevance
				stats.IncorrectAssignmentCount++
			}
		}
		for _, negation := range item.ModelNegations {
			stats.NegationCount++
			if _, ok := expectedSet[domain.NormalizeTagName(negation.Tag)]; ok {
				stats.NegationFalsePositives++
			}
		}
		stats.InputTokensPerIssue += float64(item.TokenUsage.InputTokens)
		stats.OutputTokensPerIssue += float64(item.TokenUsage.OutputTokens)
	}
	stats.MicroPrecision = tagEvalRatio(tp, predicted)
	stats.MicroRecall = tagEvalRatio(tp, expected)
	if stats.MicroPrecision+stats.MicroRecall > 0 {
		stats.MicroF1 = 2 * stats.MicroPrecision * stats.MicroRecall / (stats.MicroPrecision + stats.MicroRecall)
	}
	if stats.Issues > 0 {
		denominator := float64(stats.Issues)
		stats.MacroPrecision /= denominator
		stats.MacroRecall /= denominator
		stats.MacroF1 /= denominator
		stats.InputTokensPerIssue /= denominator
		stats.OutputTokensPerIssue /= denominator
	}
	stats.CorrectRelevanceMean = tagEvalRatioFloat(correctSum, stats.CorrectAssignmentCount)
	stats.IncorrectRelevanceMean = tagEvalRatioFloat(incorrectSum, stats.IncorrectAssignmentCount)
	stats.NegationFalsePositiveRate = tagEvalRatio(stats.NegationFalsePositives, stats.NegationCount)
	stats.CostPer1KEnrichments = 1000 * ((stats.InputTokensPerIssue*price.InputPerMillion + stats.OutputTokensPerIssue*price.OutputPerMillion) / 1_000_000)
	return roundTagEvalStats(stats)
}

func summarizeTagEvalRuns(runs []TagEvalRun) TagEvalStats {
	if len(runs) == 0 {
		return TagEvalStats{}
	}
	combined := TagEvalStats{}
	microF1 := make([]float64, 0, len(runs))
	macroF1 := make([]float64, 0, len(runs))
	ndcg := make([]float64, 0, len(runs))
	recall := make([]float64, 0, len(runs))
	for _, run := range runs {
		stats := run.Stats
		combined.Issues = stats.Issues
		combined.MicroPrecision += stats.MicroPrecision
		combined.MicroRecall += stats.MicroRecall
		combined.MicroF1 += stats.MicroF1
		combined.MacroPrecision += stats.MacroPrecision
		combined.MacroRecall += stats.MacroRecall
		combined.MacroF1 += stats.MacroF1
		combined.CorrectRelevanceMean += stats.CorrectRelevanceMean
		combined.IncorrectRelevanceMean += stats.IncorrectRelevanceMean
		combined.NegationFalsePositiveRate += stats.NegationFalsePositiveRate
		combined.InputTokensPerIssue += stats.InputTokensPerIssue
		combined.OutputTokensPerIssue += stats.OutputTokensPerIssue
		combined.CostPer1KEnrichments += stats.CostPer1KEnrichments
		combined.CorrectAssignmentCount += stats.CorrectAssignmentCount
		combined.IncorrectAssignmentCount += stats.IncorrectAssignmentCount
		combined.NegationFalsePositives += stats.NegationFalsePositives
		combined.NegationCount += stats.NegationCount
		microF1 = append(microF1, stats.MicroF1)
		macroF1 = append(macroF1, stats.MacroF1)
		ndcg = append(ndcg, run.Ranking.NDCG)
		recall = append(recall, run.Ranking.Recall)
	}
	denominator := float64(len(runs))
	combined.MicroPrecision /= denominator
	combined.MicroRecall /= denominator
	combined.MicroF1 /= denominator
	combined.MacroPrecision /= denominator
	combined.MacroRecall /= denominator
	combined.MacroF1 /= denominator
	combined.CorrectRelevanceMean /= denominator
	combined.IncorrectRelevanceMean /= denominator
	combined.NegationFalsePositiveRate /= denominator
	combined.InputTokensPerIssue /= denominator
	combined.OutputTokensPerIssue /= denominator
	combined.CostPer1KEnrichments /= denominator
	slices.Sort(microF1)
	slices.Sort(macroF1)
	slices.Sort(ndcg)
	slices.Sort(recall)
	combined.Spread = TagEvalSpread{
		MicroF1Min: microF1[0], MicroF1Max: microF1[len(microF1)-1],
		MacroF1Min: macroF1[0], MacroF1Max: macroF1[len(macroF1)-1],
		NDCGMin: ndcg[0], NDCGMax: ndcg[len(ndcg)-1],
		RecallMin: recall[0], RecallMax: recall[len(recall)-1],
	}
	return roundTagEvalStats(combined)
}

func tagEvalRanking(corpus Corpus, judgments []Judgment, replacement map[string][]domain.TagRelevance) (TagEvalRank, error) {
	storeIssues := corpus.StoreIssues(tagEvalNow)
	if replacement != nil {
		for i := range storeIssues {
			if scores, ok := replacement[storeIssues[i].ID]; ok {
				storeIssues[i].TagScores = copyTagRelevances(scores)
			}
		}
	}
	uncIssueEmbeddings, uncTagEmbeddings := issuemath.CenterEmbeddingsWith(issuemath.CorpusMeans{}, corpus.IssueEmbeddings(), corpus.TagEmbeddings())
	lambda, ok := issuemath.SelectRidgeLambdaGCV(storeIssues, corpus.TagNames(), uncIssueEmbeddings, uncTagEmbeddings, scoring.RidgeAnchorLambdaScored, nil)
	if !ok {
		return TagEvalRank{}, fmt.Errorf("ridge GCV selection fell back")
	}
	bundle := issuemap.ComputeCorpusRidgeDecomposition(storeIssues, corpus.StoreTags(), issuemath.CorpusMeans{}, scoring.RidgeAnchorLambdaScored, lambda)
	if !bundle.Decomposed() {
		return TagEvalRank{}, fmt.Errorf("ridge decomposition fell back")
	}
	var ndcg, recall float64
	for _, judgment := range judgments {
		query, ok := corpus.QueryByID(judgment.Query)
		if !ok {
			return TagEvalRank{}, fmt.Errorf("unknown judgment query %q", judgment.Query)
		}
		response := issuemap.SearchFromQueryWithTags(storeIssues, corpus.StoreTags(), query.Raw, toTagRelevances(query.Tags), query.Embedding, tagEvalK, issuemap.WithRidgeDecomposition(bundle))
		ranked := make([]string, len(response.RelatedIssues))
		for i, issue := range response.RelatedIssues {
			ranked[i] = issue.ID
		}
		ndcg += NDCGAtK(ranked, judgment.Expected, tagEvalK)
		recall += RecallAtK(ranked, judgment.Expected, tagEvalK)
	}
	count := float64(len(judgments))
	return TagEvalRank{NDCG: roundTagEval(ndcg / count), Recall: roundTagEval(recall / count)}, nil
}

type tagEvalEmbedder struct{ vectors map[string][]float32 }

func newTagEvalEmbedder(corpus Corpus) (*tagEvalEmbedder, error) {
	vectors := make(map[string][]float32, len(corpus.Issues))
	for _, issue := range corpus.Issues {
		if _, exists := vectors[issue.Raw]; exists {
			return nil, fmt.Errorf("fixture has duplicate issue text for %q", issue.ID)
		}
		vector := make([]float32, len(issue.Embedding))
		for i, value := range issue.Embedding {
			vector[i] = float32(value)
		}
		vectors[issue.Raw] = vector
	}
	return &tagEvalEmbedder{vectors: vectors}, nil
}

func (e *tagEvalEmbedder) EmbedText(_ context.Context, text string) (ai.EmbeddingResult, error) {
	vector, ok := e.vectors[text]
	if !ok {
		return ai.EmbeddingResult{}, fmt.Errorf("no committed fixture embedding for text")
	}
	return ai.EmbeddingResult{Vector: append([]float32(nil), vector...), Info: ai.EmbeddingInfo{Dimensions: len(vector)}}, nil
}
func (*tagEvalEmbedder) Provider() string { return "fixture" }
func (*tagEvalEmbedder) Model() string    { return "text-embedding-3-small fixture" }

type tagEvalTagStore struct{ tags []issues.Tag }

func (s *tagEvalTagStore) ListTags(context.Context) ([]issues.Tag, error) {
	return copyTagStoreTags(s.tags), nil
}
func (s *tagEvalTagStore) UpsertTags(_ context.Context, items []issues.Tag) error {
	s.tags = copyTagStoreTags(items)
	return nil
}
func (s *tagEvalTagStore) UpdateTagSpecificity(_ context.Context, name string, specificity, llm, embedding *float64, computedAt *time.Time) error {
	for i := range s.tags {
		if s.tags[i].Name == name {
			s.tags[i].Specificity = specificity
			s.tags[i].SpecificityLLM = llm
			s.tags[i].SpecificityEmbedding = embedding
			s.tags[i].SpecificityComputedAt = computedAt
		}
	}
	return nil
}

type tagEvalIssueLister struct{ items []issues.Issue }

func (l tagEvalIssueLister) List(context.Context) ([]issues.Issue, error) {
	return append([]issues.Issue(nil), l.items...), nil
}

func copyTagStoreTags(items []issues.Tag) []issues.Tag {
	out := make([]issues.Tag, len(items))
	for i, item := range items {
		out[i] = item
		out[i].Embedding = append([]float64(nil), item.Embedding...)
	}
	return out
}
func copyTagRelevances(items []domain.TagRelevance) []domain.TagRelevance {
	return append([]domain.TagRelevance(nil), items...)
}
func tokenUsageSnapshot(usage func() ai.TokenUsage) ai.TokenUsage {
	if usage == nil {
		return ai.TokenUsage{}
	}
	return usage()
}
func tagEvalTagSet(tags []string) map[string]struct{} {
	out := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		out[domain.NormalizeTagName(tag)] = struct{}{}
	}
	return out
}
func tagEvalRatio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
func tagEvalRatioFloat(numerator float64, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / float64(denominator)
}
func roundTagEval(value float64) float64 { return math.Round(value*1e6) / 1e6 }
func roundTagEvalStats(stats TagEvalStats) TagEvalStats {
	stats.MicroPrecision = roundTagEval(stats.MicroPrecision)
	stats.MicroRecall = roundTagEval(stats.MicroRecall)
	stats.MicroF1 = roundTagEval(stats.MicroF1)
	stats.MacroPrecision = roundTagEval(stats.MacroPrecision)
	stats.MacroRecall = roundTagEval(stats.MacroRecall)
	stats.MacroF1 = roundTagEval(stats.MacroF1)
	stats.CorrectRelevanceMean = roundTagEval(stats.CorrectRelevanceMean)
	stats.IncorrectRelevanceMean = roundTagEval(stats.IncorrectRelevanceMean)
	stats.NegationFalsePositiveRate = roundTagEval(stats.NegationFalsePositiveRate)
	stats.InputTokensPerIssue = roundTagEval(stats.InputTokensPerIssue)
	stats.OutputTokensPerIssue = roundTagEval(stats.OutputTokensPerIssue)
	stats.CostPer1KEnrichments = roundTagEval(stats.CostPer1KEnrichments)
	stats.Spread.MicroF1Min = roundTagEval(stats.Spread.MicroF1Min)
	stats.Spread.MicroF1Max = roundTagEval(stats.Spread.MicroF1Max)
	stats.Spread.MacroF1Min = roundTagEval(stats.Spread.MacroF1Min)
	stats.Spread.MacroF1Max = roundTagEval(stats.Spread.MacroF1Max)
	stats.Spread.NDCGMin = roundTagEval(stats.Spread.NDCGMin)
	stats.Spread.NDCGMax = roundTagEval(stats.Spread.NDCGMax)
	stats.Spread.RecallMin = roundTagEval(stats.Spread.RecallMin)
	stats.Spread.RecallMax = roundTagEval(stats.Spread.RecallMax)
	return stats
}

// CanonicalModelList normalizes a command list so output order is stable.
func CanonicalModelList(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	models := make([]string, 0, len(raw))
	for _, model := range raw {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	slices.Sort(models)
	return models
}
