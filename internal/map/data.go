package issuemap

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	generatedIssueCount = 1000
	embeddingDimensions = 24
	datasetSeed         = 20260306
)

type tagSpec struct {
	Name string
}

type archetype struct {
	Name       string
	CoreTags   []string
	Related    []string
	Components []string
	Actions    []string
	Context    []string
}

type dataset struct {
	issues        []Issue
	tags          []string
	embeddings    map[string][]float64
	tagEmbeddings map[string][]float64
}

var (
	datasetOnce sync.Once
	cachedData  dataset
)

var tagCatalog = []tagSpec{
	{Name: "bug"},
	{Name: "crash"},
	{Name: "feature"},
	{Name: "idea"},
	{Name: "improvement"},
	{Name: "ui"},
	{Name: "ux"},
	{Name: "frontend"},
	{Name: "performance"},
	{Name: "safari"},
	{Name: "onboarding"},
	{Name: "search"},
	{Name: "export"},
}

var archetypes = []archetype{
	{
		Name:       "ui-polish",
		CoreTags:   []string{"ui", "ux", "improvement"},
		Related:    []string{"frontend", "feature", "bug"},
		Components: []string{"sidebar", "header", "composer", "command palette", "issue card"},
		Actions:    []string{"feels inconsistent in", "needs refinement during", "shows flicker around", "drops affordance near"},
		Context:    []string{"dense workspace sessions", "high-contrast themes", "long review sweeps", "rapid triage loops"},
	},
	{
		Name:       "frontend-regression",
		CoreTags:   []string{"bug", "crash", "frontend"},
		Related:    []string{"performance", "ui", "search"},
		Components: []string{"list rendering", "client hydration", "detail panel", "activity feed", "filter drawer"},
		Actions:    []string{"throws a regression in", "breaks unexpectedly during", "stalls while mounting", "loses state around"},
		Context:    []string{"route transitions", "workspace boot", "issue expansion", "bulk edits"},
	},
	{
		Name:       "search-performance",
		CoreTags:   []string{"search", "performance", "bug"},
		Related:    []string{"frontend", "ux", "feature"},
		Components: []string{"global search", "issue ranking", "facet filters", "saved views", "workspace indexing"},
		Actions:    []string{"slows down during", "times out around", "needs caching for", "regresses under"},
		Context:    []string{"large imports", "cold starts", "dense workspaces", "multi-filter queries"},
	},
	{
		Name:       "export-reliability",
		CoreTags:   []string{"export", "bug", "safari"},
		Related:    []string{"performance", "frontend", "ux"},
		Components: []string{"csv export", "share sheet", "download flow", "pdf snapshot", "attachment bundle"},
		Actions:    []string{"fails intermittently in", "drops output during", "hangs after triggering", "misbehaves around"},
		Context:    []string{"tablet sessions", "mobile review", "cross-browser validation", "customer handoff"},
	},
	{
		Name:       "onboarding-flow",
		CoreTags:   []string{"onboarding", "ux", "improvement"},
		Related:    []string{"feature", "ui", "search"},
		Components: []string{"signup flow", "workspace invite", "empty state", "first-run checklist", "sample data wizard"},
		Actions:    []string{"creates friction during", "needs a simpler path for", "over-explains steps in", "should compress"},
		Context:    []string{"first-team setup", "self-serve trials", "new contributor sessions", "guided discovery"},
	},
	{
		Name:       "workflow-automation",
		CoreTags:   []string{"feature", "idea", "search"},
		Related:    []string{"ux", "improvement", "performance"},
		Components: []string{"auto-linking", "digest summaries", "rule engine", "smart views", "trend detection"},
		Actions:    []string{"could automate", "should synthesize", "might cluster", "would prioritize"},
		Context:    []string{"backlog grooming", "weekly planning", "incident review", "release prep"},
	},
	{
		Name:       "integration-request",
		CoreTags:   []string{"feature", "idea", "export"},
		Related:    []string{"search", "ux", "frontend"},
		Components: []string{"Slack bridge", "email sync", "webhook routing", "ticket import", "calendar digest"},
		Actions:    []string{"should connect into", "could mirror activity for", "needs richer hooks around", "would unlock workflows for"},
		Context:    []string{"ops teams", "support queues", "launch reviews", "cross-tool reporting"},
	},
}

var loremWords = []string{
	"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit", "sed", "do", "eiusmod",
	"tempor", "incididunt", "ut", "labore", "et", "dolore", "magna", "aliqua", "ut", "enim", "ad", "minim",
	"veniam", "quis", "nostrud", "exercitation", "ullamco", "laboris", "nisi", "ut", "aliquip", "ex", "ea",
	"commodo", "consequat", "duis", "aute", "irure", "in", "reprehenderit", "voluptate", "velit", "esse",
	"cillum", "dolore", "eu", "fugiat", "nulla", "pariatur", "excepteur", "sint", "occaecat", "cupidatat",
	"non", "proident", "sunt", "in", "culpa", "qui", "officia", "deserunt", "mollit", "anim", "id", "est",
	"laborum",
}

func issueDataset() dataset {
	datasetOnce.Do(func() {
		cachedData = generateDataset()
	})
	return cachedData
}

func generateDataset() dataset {
	rng := rand.New(rand.NewSource(datasetSeed))

	tags := make([]string, len(tagCatalog))
	for i, spec := range tagCatalog {
		tags[i] = spec.Name
	}

	tagBasis := make(map[string][]float64, len(tags))
	for _, tag := range tags {
		tagBasis[tag] = normalizedRandomVector(rng, embeddingDimensions)
	}

	issues := make([]Issue, generatedIssueCount)
	embeddings := make(map[string][]float64, generatedIssueCount)
	for i := range generatedIssueCount {
		id := strconv.Itoa(i + 1)
		issueRNG := rand.New(rand.NewSource(datasetSeed + int64(i)*17 + 11))
		archetype := archetypes[i%len(archetypes)]
		loadings := generateLoadings(issueRNG, archetype)
		raw := generateIssueBody(issueRNG, i+1, archetype, loadings)

		issues[i] = Issue{
			ID:   id,
			Raw:  raw,
			Tags: loadings,
		}
		embeddings[id] = generateEmbedding(issueRNG, loadings, tagBasis)
	}

	return dataset{
		issues:        issues,
		tags:          tags,
		embeddings:    embeddings,
		tagEmbeddings: tagBasis,
	}
}

func generateLoadings(rng *rand.Rand, archetype archetype) []TagRelevance {
	scores := make(map[string]float64, len(tagCatalog))
	for _, tag := range archetype.CoreTags {
		scores[tag] = maxScore(scores[tag], 0.62+rng.Float64()*0.28)
	}

	secondaryCount := 1
	if len(archetype.Related) > 1 {
		secondaryCount += rng.Intn(2)
	}
	for _, idx := range rng.Perm(len(archetype.Related))[:secondaryCount] {
		tag := archetype.Related[idx]
		scores[tag] = maxScore(scores[tag], 0.25+rng.Float64()*0.35)
	}

	if rng.Float64() < 0.35 {
		extra := tagCatalog[rng.Intn(len(tagCatalog))].Name
		scores[extra] = maxScore(scores[extra], 0.16+rng.Float64()*0.18)
	}

	loadings := make([]TagRelevance, 0, len(scores))
	for tag, score := range scores {
		if score < 0.2 {
			continue
		}
		loadings = append(loadings, TagRelevance{
			Tag:       tag,
			Relevance: round(score, 3),
		})
	}

	sort.Slice(loadings, func(i, j int) bool {
		if loadings[i].Relevance == loadings[j].Relevance {
			return loadings[i].Tag < loadings[j].Tag
		}
		return loadings[i].Relevance > loadings[j].Relevance
	})

	if len(loadings) > 4 {
		loadings = loadings[:4]
	}
	return loadings
}

func generateIssueBody(rng *rand.Rand, issueNumber int, archetype archetype, loadings []TagRelevance) string {
	component := archetype.Components[rng.Intn(len(archetype.Components))]
	action := archetype.Actions[rng.Intn(len(archetype.Actions))]
	context := archetype.Context[rng.Intn(len(archetype.Context))]

	title := fmt.Sprintf("Issue %04d: %s %s %s.", issueNumber, component, action, context)

	signalWords := make([]string, 0, len(loadings)+5)
	signalWords = append(signalWords, strings.Fields(component)...)
	signalWords = append(signalWords, strings.Fields(context)...)
	for _, loading := range loadings {
		signalWords = append(signalWords, loading.Tag)
	}

	body := []string{
		title,
		buildLoremSentence(rng, signalWords, 18, 28),
		buildLoremSentence(rng, signalWords, 14, 24),
	}
	if rng.Float64() < 0.45 {
		body = append(body, buildLoremSentence(rng, signalWords, 12, 20))
	}

	return strings.Join(body, " ")
}

func buildLoremSentence(rng *rand.Rand, signalWords []string, minWords, maxWords int) string {
	wordCount := minWords + rng.Intn(maxWords-minWords+1)
	words := make([]string, 0, wordCount)
	for i := 0; i < wordCount; i++ {
		if len(signalWords) > 0 && rng.Float64() < 0.22 {
			words = append(words, signalWords[rng.Intn(len(signalWords))])
			continue
		}
		words = append(words, loremWords[rng.Intn(len(loremWords))])
	}

	sentence := strings.Join(words, " ")
	return capitalize(sentence) + "."
}

func generateEmbedding(rng *rand.Rand, loadings []TagRelevance, tagBasis map[string][]float64) []float64 {
	embedding := make([]float64, embeddingDimensions)
	for _, loading := range loadings {
		basis := tagBasis[loading.Tag]
		for i, value := range basis {
			embedding[i] += loading.Relevance * value
		}
	}

	for i := range embedding {
		embedding[i] += (rng.Float64() - 0.5) * 0.08
	}

	normalizeVector(embedding)
	return embedding
}

func normalizedRandomVector(rng *rand.Rand, size int) []float64 {
	vector := make([]float64, size)
	for i := range vector {
		vector[i] = rng.Float64()*2 - 1
	}
	normalizeVector(vector)
	return vector
}

func normalizeVector(vector []float64) {
	var magnitude float64
	for _, value := range vector {
		magnitude += value * value
	}
	if magnitude == 0 {
		return
	}

	scale := math.Sqrt(magnitude)
	for i := range vector {
		vector[i] /= scale
	}
}

func round(value float64, digits int) float64 {
	scale := math.Pow(10, float64(digits))
	return math.Round(value*scale) / scale
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func maxScore(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
