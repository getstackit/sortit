// tageval measures analyzer tagging fidelity against the committed matheval
// fixture. It makes live calls only with -live; ordinary tests never invoke it.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"sortit/internal/ai"
	"sortit/internal/matheval"
)

const defaultModels = "gpt-5.4-nano,gpt-5.6-luna,gpt-5.4-mini"

func main() {
	var (
		live       = flag.Bool("live", false, "allow live OpenAI tagging calls")
		models     = flag.String("models", defaultModels, "comma-separated analyzer models")
		runs       = flag.Int("runs", 3, "serial runs per model")
		date       = flag.String("date", time.Now().UTC().Format("2006-01-02"), "artifact date (YYYY-MM-DD)")
		outDir     = flag.String("out-dir", "internal/matheval/testdata", "directory for committed result tables")
		fixtureDir = flag.String("fixture-dir", "", "fixture input directory (defaults to -out-dir)")
		combine    = flag.String("combine", "", "comma-separated one-run table JSON files to combine without live calls")
	)
	flag.Parse()
	if strings.TrimSpace(*fixtureDir) == "" {
		*fixtureDir = *outDir
	}
	if *combine != "" {
		combineArtifacts(*combine, *date, *outDir)
		return
	}
	if !*live {
		fatalf("refusing live model calls without -live (ordinary go test never calls this command)")
	}
	if *runs < 1 {
		fatalf("-runs must be positive")
	}
	if _, err := time.Parse("2006-01-02", *date); err != nil {
		fatalf("-date must be YYYY-MM-DD: %v", err)
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		fatalf("OPENAI_API_KEY is required with -live")
	}

	corpus, err := matheval.LoadCorpus(filepath.Join(*fixtureDir, "corpus.json"))
	if err != nil {
		fatalf("load corpus: %v", err)
	}
	embeddings, err := matheval.LoadRealEmbeddings(filepath.Join(*fixtureDir, "real_embeddings.json"))
	if err != nil {
		fatalf("load real embeddings: %v", err)
	}
	realCorpus, err := corpus.RealCorpus(embeddings)
	if err != nil {
		fatalf("build real corpus: %v", err)
	}
	judgments, err := matheval.LoadJudgments(filepath.Join(*fixtureDir, "judgments.json"), realCorpus)
	if err != nil {
		fatalf("load judgments: %v", err)
	}

	modelList := matheval.CanonicalModelList(strings.Split(*models, ","))
	if len(modelList) == 0 {
		fatalf("-models must include at least one model")
	}
	command := generatingCommand(modelList, *runs, *date, *outDir)
	artifact := matheval.TagEvalArtifact{
		GeneratedAt: *date,
		Command:     command,
		Fixture:     "matheval real corpus (48 issues, 16 tags; committed text-embedding-3-small vectors)",
		Reports:     make([]matheval.TagEvalReport, 0, len(modelList)),
	}

	for _, model := range modelList {
		price, ok := modelPrice(model)
		if !ok {
			fatalf("no standard API price configured for %q", model)
		}
		counter := &ai.TokenUsageCounter{}
		tagger, err := ai.NewOpenAITagger(ai.OpenAIConfig{APIKey: apiKey, TagModel: model})
		if err != nil {
			fatalf("configure %s: %v", model, err)
		}
		tagger.SetTokenUsageObserver(counter.Add)
		fmt.Fprintf(os.Stderr, "tageval: %s (%d runs × 48 issues)\n", model, *runs)
		report, err := matheval.RunTaggingFidelity(context.Background(), matheval.TagEvalConfig{
			Model: model, Runs: *runs, Corpus: realCorpus, Judgments: judgments,
			Tagger: tagger, Usage: counter.Snapshot, Price: price,
		})
		if err != nil {
			fatalf("evaluate %s: %v", model, err)
		}
		artifact.Reports = append(artifact.Reports, report)
	}

	if err := os.MkdirAll(*outDir, 0o750); err != nil {
		fatalf("create output directory: %v", err)
	}
	for _, report := range artifact.Reports {
		path := filepath.Join(*outDir, "tagging_fidelity_"+modelSlug(report.Model)+".json")
		if err := writeJSON(path, matheval.TagEvalArtifact{
			GeneratedAt: artifact.GeneratedAt, Command: artifact.Command, Fixture: artifact.Fixture, Reports: []matheval.TagEvalReport{report},
		}); err != nil {
			fatalf("write %s: %v", path, err)
		}
	}
	comparison := filepath.Join(*outDir, "tagging_fidelity_comparison.md")
	if err := os.WriteFile(comparison, []byte(renderComparison(artifact)), 0o600); err != nil {
		fatalf("write %s: %v", comparison, err)
	}
	fmt.Printf("wrote %d per-model tables and %s\n", len(artifact.Reports), comparison)
}

func combineArtifacts(raw, date, outDir string) {
	paths := strings.Split(raw, ",")
	byModel := make(map[string][]matheval.TagEvalReport)
	for _, path := range paths {
		data, err := os.ReadFile(strings.TrimSpace(path))
		if err != nil {
			fatalf("read %s: %v", path, err)
		}
		var artifact matheval.TagEvalArtifact
		if err := json.Unmarshal(data, &artifact); err != nil {
			fatalf("parse %s: %v", path, err)
		}
		if len(artifact.Reports) != 1 {
			fatalf("%s must contain exactly one report", path)
		}
		report := artifact.Reports[0]
		byModel[report.Model] = append(byModel[report.Model], report)
	}
	models := make([]string, 0, len(byModel))
	for model := range byModel {
		models = append(models, model)
	}
	models = matheval.CanonicalModelList(models)
	artifact := matheval.TagEvalArtifact{GeneratedAt: date, Command: "go run ./internal/matheval/cmd/tageval -live -models " + strings.Join(models, ",") + " -runs 3 -date " + date + " -out-dir " + outDir, Fixture: "matheval real corpus (48 issues, 16 tags; committed text-embedding-3-small vectors)"}
	for _, model := range models {
		report, err := matheval.CombineTagEvalReports(byModel[model])
		if err != nil {
			fatalf("combine %s: %v", model, err)
		}
		artifact.Reports = append(artifact.Reports, report)
		path := filepath.Join(outDir, "tagging_fidelity_"+modelSlug(model)+".json")
		if err := writeJSON(path, matheval.TagEvalArtifact{GeneratedAt: artifact.GeneratedAt, Command: artifact.Command, Fixture: artifact.Fixture, Reports: []matheval.TagEvalReport{report}}); err != nil {
			fatalf("write %s: %v", path, err)
		}
	}
	comparison := filepath.Join(outDir, "tagging_fidelity_comparison.md")
	if err := os.WriteFile(comparison, []byte(renderComparison(artifact)), 0o600); err != nil {
		fatalf("write %s: %v", comparison, err)
	}
	fmt.Printf("wrote %d combined per-model tables and %s\n", len(artifact.Reports), comparison)
}

func modelPrice(model string) (matheval.TagEvalPrice, bool) {
	// Standard API prices checked 2026-08-08. See openai.com/api/pricing.
	switch model {
	case "gpt-5.4-nano":
		return matheval.TagEvalPrice{InputPerMillion: 0.20, OutputPerMillion: 1.25}, true
	case "gpt-5.6-luna":
		return matheval.TagEvalPrice{InputPerMillion: 1.00, OutputPerMillion: 6.00}, true
	case "gpt-5.4-mini":
		return matheval.TagEvalPrice{InputPerMillion: 0.75, OutputPerMillion: 4.50}, true
	default:
		return matheval.TagEvalPrice{}, false
	}
}

func generatingCommand(models []string, runs int, date, outDir string) string {
	return fmt.Sprintf("go run ./internal/matheval/cmd/tageval -live -models %s -runs %d -date %s -out-dir %s", strings.Join(models, ","), runs, date, outDir)
}

func renderComparison(artifact matheval.TagEvalArtifact) string {
	var out strings.Builder
	fmt.Fprintln(&out, "# WP-702 tagging-fidelity comparison")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "Generated: %s  \n", artifact.GeneratedAt)
	fmt.Fprintf(&out, "Fixture: %s  \n", artifact.Fixture)
	fmt.Fprintf(&out, "Command: `%s`\n", artifact.Command)
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "Fidelity uses `AnalysisTrace.ModelOutput` after the production relevance floor (0.08), before verifier/post-processing. Ranking substitutes the verifier's final tag scores into the full uncentered ridge path. Ranges are min–max across three serial model runs.")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "| Model | Micro F1 | Macro F1 | Correct / incorrect relevance | Negation FP | NDCG@8 Δ | Recall@8 Δ | Tokens in / out per issue | $ / 1k enrichments |")
	fmt.Fprintln(&out, "|---|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, report := range artifact.Reports {
		s := report.Summary
		rank := averageRank(report.RunData)
		fmt.Fprintf(&out, "| %s | %.3f [%.3f–%.3f] | %.3f [%.3f–%.3f] | %.3f / %.3f | %d/%d (%.1f%%) | %+.3f | %+.3f | %.1f / %.1f | $%.3f |\n",
			report.Model, s.MicroF1, s.Spread.MicroF1Min, s.Spread.MicroF1Max,
			s.MacroF1, s.Spread.MacroF1Min, s.Spread.MacroF1Max,
			s.CorrectRelevanceMean, s.IncorrectRelevanceMean,
			s.NegationFalsePositives, s.NegationCount, 100*s.NegationFalsePositiveRate,
			rank.DeltaNDCG, rank.DeltaRecall, s.InputTokensPerIssue, s.OutputTokensPerIssue, s.CostPer1KEnrichments)
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "The JSON table for each model includes all 48 per-issue precision/recall/F1 rows, pre-verifier assignments and negations, final verified scores, and per-issue provider token usage.")
	return out.String()
}

func averageRank(runs []matheval.TagEvalRun) matheval.TagEvalRank {
	var rank matheval.TagEvalRank
	for _, run := range runs {
		rank.NDCG += run.Ranking.NDCG
		rank.Recall += run.Ranking.Recall
		rank.DeltaNDCG += run.Ranking.DeltaNDCG
		rank.DeltaRecall += run.Ranking.DeltaRecall
	}
	if len(runs) > 0 {
		n := float64(len(runs))
		rank.NDCG /= n
		rank.Recall /= n
		rank.DeltaNDCG /= n
		rank.DeltaRecall /= n
	}
	return rank
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
func modelSlug(model string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			return r
		}
		return '-'
	}, model)
}
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "tageval: "+format+"\n", args...)
	os.Exit(2)
}
