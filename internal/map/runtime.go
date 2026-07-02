package issuemap

import (
	"cmp"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"unicode"

	"sortit/internal/domain"
	"sortit/internal/issueanalytics"
	"sortit/internal/issuemath"
	"sortit/internal/issues"
	"sortit/internal/vectors"
)

func BuildMapFromIssues(storeIssues []issues.Issue, viewport *Viewport) (MapResponse, error) {
	return BuildMapFromIssuesWithTags(storeIssues, nil, viewport)
}

func BuildMapFromIssuesWithTags(storeIssues []issues.Issue, storeTags []issues.Tag, viewport *Viewport) (MapResponse, error) {
	return BuildMapFromIssuesWithTagsAndThreshold(storeIssues, storeTags, viewport, minEdgeSimilarity)
}

func BuildMapFromIssuesWithTagsAndThreshold(storeIssues []issues.Issue, storeTags []issues.Tag, viewport *Viewport, edgeThreshold float64) (MapResponse, error) {
	base, unavailableReason, issueCount, err := buildBaseMapDataFromIssues(storeIssues, storeTags, edgeThreshold)
	if err != nil {
		return MapResponse{}, err
	}
	if unavailableReason != "" {
		return unavailableMapResponse(unavailableReason, issueCount), nil
	}

	edges := filterEdgesForViewport(base, normalizeViewport(viewport))
	if edges == nil {
		edges = []Edge{}
	}

	clusters := base.clusters
	if clusters == nil {
		clusters = []Cluster{}
	}

	return MapResponse{
		Available:         true,
		IssueCount:        len(base.mapIssues),
		MinimumIssueCount: minMapIssueCount,
		Issues:            base.mapIssues,
		Edges:             edges,
		Clusters:          clusters,
	}, nil
}

func BuildEdgeResponseFromIssues(storeIssues []issues.Issue, viewport *Viewport) (EdgeResponse, error) {
	return BuildEdgeResponseFromIssuesWithTags(storeIssues, nil, viewport)
}

func BuildEdgeResponseFromIssuesWithTags(storeIssues []issues.Issue, storeTags []issues.Tag, viewport *Viewport) (EdgeResponse, error) {
	return BuildEdgeResponseFromIssuesWithTagsAndThreshold(storeIssues, storeTags, viewport, minEdgeSimilarity)
}

func BuildEdgeResponseFromIssuesWithTagsAndThreshold(storeIssues []issues.Issue, storeTags []issues.Tag, viewport *Viewport, edgeThreshold float64) (EdgeResponse, error) {
	base, unavailableReason, _, err := buildBaseMapDataFromIssues(storeIssues, storeTags, edgeThreshold)
	if err != nil {
		return EdgeResponse{}, err
	}
	if unavailableReason != "" {
		return EdgeResponse{Edges: []Edge{}}, nil
	}

	edges := filterEdgesForViewport(base, normalizeViewport(viewport))
	if edges == nil {
		edges = []Edge{}
	}

	return EdgeResponse{Edges: edges}, nil
}

func buildBaseMapDataFromIssues(storeIssues []issues.Issue, storeTags []issues.Tag, edgeThreshold float64) (mapBaseData, string, int, error) {
	corpus := runtimeMapInputs(storeIssues, storeTags)
	mapIssuesInput := corpus.issues
	if unavailableReason := mapProjectionUnavailableReason(len(mapIssuesInput), len(corpus.tagNames)); unavailableReason != "" {
		return mapBaseData{}, unavailableReason, len(mapIssuesInput), nil
	}

	positions, err := issuemath.ComputePositions(mapIssuesInput, corpus.tagNames, corpus.tagEmbeddings)
	if err != nil {
		return mapBaseData{}, "", 0, err
	}

	mapIssues := make([]MapIssue, len(mapIssuesInput))
	roundedPositions := make(map[string]Position, len(mapIssuesInput))
	for i, issue := range mapIssuesInput {
		p, ok := positions[issue.ID]
		if !ok {
			return mapBaseData{}, "", 0, fmt.Errorf("missing position for issue %s", issue.ID)
		}

		rounded := roundPosition(p)
		roundedPositions[issue.ID] = rounded
		mapIssues[i] = MapIssue{
			ID:         issue.ID,
			Raw:        issue.Raw,
			Status:     issue.Status,
			AssignedTo: issue.AssignedTo,
			Tags:       issue.TagScores,
			X:          rounded.X,
			Y:          rounded.Y,
			Hubness:    issueanalytics.IssueHubness(storeIssues[i]),
		}
	}

	// Edges intentionally use the uncentered embeddings — see runtimeCorpus.
	edges := ComputeEdgesWithEmbeddings(mapIssuesInput, corpus.rawIssueEmbeddings, edgeThreshold)
	sortEdgesBySimilarity(edges)
	clusters := ComputeFactorClusters(mapIssuesInput, positions)

	return mapBaseData{
		mapIssues:      mapIssues,
		positions:      roundedPositions,
		candidateEdges: edges,
		clusters:       clusters,
	}, "", len(mapIssuesInput), nil
}

// runtimeCorpus is the corpus-load boundary for the map and search math:
// store data converted to runtime issues plus unit-normalized embeddings,
// with corpus-mean centering applied once so every consumer (factor
// decomposition, tag covariance, search blending) works in the same
// centered space. Persisted embeddings are never rewritten.
type runtimeCorpus struct {
	issues   []issues.Issue
	tagNames []string
	// rawIssueEmbeddings are unit-normalized but uncentered. Map edges keep
	// consuming these: edge similarity thresholds (minEdgeSimilarity) were
	// tuned against raw cosines, and centering collapses that scale.
	issueEmbeddings    map[string][]float64
	rawIssueEmbeddings map[string][]float64
	tagEmbeddings      map[string][]float64
	// means are the corpus means the embeddings were centered with. Query
	// embeddings arriving at search time must be centered with these same
	// means, never with statistics derived from the query itself.
	means issuemath.CorpusMeans
}

func runtimeMapInputs(storeIssues []issues.Issue, storeTags []issues.Tag) runtimeCorpus {
	return runtimeMapInputsWithMeans(storeIssues, storeTags, nil)
}

// runtimeMapInputsWithMeans builds the runtime corpus, centering with the
// supplied corpus means when non-nil. External means matter when storeIssues
// is a retrieved subset (e.g. semantic-search candidates): the subset's own
// mean points toward the query neighborhood, and centering with it would
// subtract exactly the signal being searched for.
func runtimeMapInputsWithMeans(storeIssues []issues.Issue, storeTags []issues.Tag, means *issuemath.CorpusMeans) runtimeCorpus {
	tagNames := runtimeTagNames(storeIssues, storeTags)
	tagEmbeddings := runtimeTagEmbeddings(tagNames, storeTags)

	prepared := make([]issues.Issue, len(storeIssues))
	embeddings := make(map[string][]float64, len(storeIssues))
	for i, storeIssue := range storeIssues {
		tagScores := runtimeStoredTagRelevances(storeIssue)
		prepared[i] = issues.Issue{
			ID:         storeIssue.ID,
			Raw:        storeIssue.Raw,
			Status:     storeIssue.Status,
			AssignedTo: storeIssue.AssignedTo,
			TagScores:  tagScores,
			Embedding:  storeIssue.Embedding,
		}
		embeddings[storeIssue.ID] = runtimeStoredEmbedding(storeIssue, prepared[i], tagEmbeddings)
	}

	// If every issue is effectively untagged, fall back to circular layout.
	hasTagSignal := false
	for _, issue := range prepared {
		if len(issue.TagScores) > 0 {
			hasTagSignal = true
			break
		}
	}
	if !hasTagSignal {
		tagNames = nil
		tagEmbeddings = nil
	}

	return centerRuntimeCorpus(prepared, tagNames, embeddings, tagEmbeddings, means)
}

// centerRuntimeCorpus applies corpus-mean centering to the runtime
// embeddings. When means is nil, the means are derived from the corpus
// itself (appropriate when the issue set is the full corpus).
func centerRuntimeCorpus(
	prepared []issues.Issue,
	tagNames []string,
	embeddings map[string][]float64,
	tagEmbeddings map[string][]float64,
	means *issuemath.CorpusMeans,
) runtimeCorpus {
	var centeredIssues, centeredTags map[string][]float64
	var usedMeans issuemath.CorpusMeans
	if means != nil {
		usedMeans = *means
		centeredIssues, centeredTags = issuemath.CenterEmbeddingsWith(usedMeans, embeddings, tagEmbeddings)
	} else {
		centeredIssues, centeredTags, usedMeans = issuemath.CenterEmbeddings(embeddings, tagEmbeddings)
	}

	return runtimeCorpus{
		issues:             prepared,
		tagNames:           tagNames,
		issueEmbeddings:    centeredIssues,
		rawIssueEmbeddings: embeddings,
		tagEmbeddings:      centeredTags,
		means:              usedMeans,
	}
}

func runtimeProjectionInputs(
	storeIssues []issues.MapProjectionIssue,
	storeTags []issues.Tag,
) runtimeCorpus {
	prepared := make([]issues.Issue, len(storeIssues))
	for i, storeIssue := range storeIssues {
		tagScores := copyProjectionTagScores(storeIssue.TagScores)
		if len(tagScores) == 0 {
			tagScores = runtimeTagRelevances(storeIssue.Tags)
		}

		prepared[i] = issues.Issue{
			ID:         storeIssue.ID,
			Raw:        storeIssue.Raw,
			Tags:       append([]string(nil), storeIssue.Tags...),
			Status:     storeIssue.Status,
			AssignedTo: storeIssue.AssignedTo,
			TagScores:  tagScores,
			Embedding:  append([]float64(nil), storeIssue.Embedding...),
		}
	}

	tagNames := runtimeTagNames(prepared, storeTags)
	tagEmbeddings := runtimeTagEmbeddings(tagNames, storeTags)
	embeddings := make(map[string][]float64, len(prepared))
	for i, storeIssue := range prepared {
		embeddings[storeIssue.ID] = runtimeStoredEmbedding(storeIssue, prepared[i], tagEmbeddings)
	}

	hasTagSignal := false
	for _, issue := range prepared {
		if len(issue.TagScores) > 0 {
			hasTagSignal = true
			break
		}
	}
	if !hasTagSignal {
		tagNames = nil
		tagEmbeddings = nil
	}

	return centerRuntimeCorpus(prepared, tagNames, embeddings, tagEmbeddings, nil)
}

func copyProjectionTagScores(input []issues.TagRelevance) []issues.TagRelevance {
	if len(input) == 0 {
		return nil
	}

	items := make([]issues.TagRelevance, len(input))
	copy(items, input)
	return items
}

func runtimeTagNames(storeIssues []issues.Issue, storeTags []issues.Tag) []string {
	seen := make(map[string]struct{}, len(tagCatalog)+len(storeTags))
	tags := make([]string, 0, len(tagCatalog)+len(storeTags))

	for _, tag := range storeTags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
		tags = append(tags, name)
	}

	if len(tags) == 0 {
		for _, spec := range tagCatalog {
			if spec.Name == "" {
				continue
			}
			seen[spec.Name] = struct{}{}
			tags = append(tags, spec.Name)
		}
	}

	for _, issue := range storeIssues {
		for _, tag := range issue.TagScores {
			name := strings.TrimSpace(tag.Tag)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			tags = append(tags, name)
		}

		for _, tag := range issue.Tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			tags = append(tags, tag)
		}
	}

	slices.Sort(tags)
	return tags
}

func runtimeTagEmbeddings(tags []string, storeTags []issues.Tag) map[string][]float64 {
	if len(tags) == 0 {
		return nil
	}

	definitions := make(map[string]string, len(tagCatalog))
	for _, spec := range tagCatalog {
		definitions[spec.Name] = spec.Description
	}

	storedEmbeddings := make(map[string][]float64, len(storeTags))
	for _, tag := range storeTags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		if description := strings.TrimSpace(tag.Description); description != "" {
			definitions[name] = description
		}
		if len(tag.Embedding) > 0 {
			storedEmbeddings[name] = append([]float64(nil), tag.Embedding...)
		}
	}

	embeddings := make(map[string][]float64, len(tags))
	for _, tag := range tags {
		if embedding := storedEmbeddings[tag]; len(embedding) > 0 {
			embeddings[tag] = embedding
			continue
		}

		recordEmbeddingFallback(EmbeddingFallbackKindTag, tag)
		descriptor := tag
		if description := definitions[tag]; description != "" {
			descriptor += " " + description
		}
		embeddings[tag] = embeddingFromText(descriptor)
	}
	return embeddings
}

func runtimeStoredTagRelevances(issue issues.Issue) []TagRelevance {
	if len(issue.TagScores) > 0 {
		relevances := make([]TagRelevance, 0, len(issue.TagScores))
		seen := make(map[string]struct{}, len(issue.TagScores))
		for _, tag := range issue.TagScores {
			name := strings.TrimSpace(tag.Tag)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			relevances = append(relevances, copyRuntimeTagRelevance(tag, name))
		}

		slices.SortStableFunc(relevances, func(a, b TagRelevance) int {
			if c := cmp.Compare(b.Relevance, a.Relevance); c != 0 {
				return c
			}
			return cmp.Compare(a.Tag, b.Tag)
		})
		return relevances
	}

	return runtimeTagRelevances(issue.Tags)
}

func copyRuntimeTagRelevance(tag TagRelevance, name string) TagRelevance {
	out := tag
	out.Tag = name
	out.CandidateSources = append([]string(nil), tag.CandidateSources...)
	out.Evidence = append([]domain.EvidenceRange(nil), tag.Evidence...)
	out.NegationEvidence = append([]domain.EvidenceRange(nil), tag.NegationEvidence...)
	out.Alignment = copyFloat64Ptr(tag.Alignment)
	out.Specificity = copyFloat64Ptr(tag.Specificity)
	out.DominanceGap = copyFloat64Ptr(tag.DominanceGap)
	out.Negation = copyFloat64Ptr(tag.Negation)
	return out
}

func copyFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func runtimeTagRelevances(tags []string) []TagRelevance {
	if len(tags) == 0 {
		return nil
	}

	relevances := make([]TagRelevance, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		relevances = append(relevances, TagRelevance{
			Tag:       tag,
			Relevance: 1,
		})
	}

	slices.SortStableFunc(relevances, func(a, b TagRelevance) int {
		return cmp.Compare(a.Tag, b.Tag)
	})
	return relevances
}

func runtimeStoredEmbedding(storeIssue issues.Issue, prepared issues.Issue, tagEmbeddings map[string][]float64) []float64 {
	if len(storeIssue.Embedding) > 0 {
		embedding := append([]float64(nil), storeIssue.Embedding...)
		if !vectors.IsZero(embedding) {
			vectors.NormalizeUnit(embedding)
			return embedding
		}
	}

	recordEmbeddingFallback(EmbeddingFallbackKindIssue, storeIssue.ID)
	return runtimeIssueEmbedding(prepared, tagEmbeddings)
}

func runtimeIssueEmbedding(issue issues.Issue, tagEmbeddings map[string][]float64) []float64 {
	vector := make([]float64, embeddingDimensions)
	textVector := embeddingFromText(issue.Raw)
	addScaled(vector, textVector, 0.7)

	for _, tag := range issue.TagScores {
		addScaled(vector, tagEmbeddings[tag.Tag], 0.9*tag.Relevance)
	}

	if vectors.IsZero(vector) {
		return embeddingFromText(issue.ID + " " + issue.Raw)
	}
	vectors.NormalizeUnit(vector)
	return vector
}

func embeddingFromText(text string) []float64 {
	vector := make([]float64, embeddingDimensions)
	tokens := tokenize(text)
	if len(tokens) == 0 {
		tokens = []string{strings.TrimSpace(text)}
	}

	for _, token := range tokens {
		if token == "" {
			continue
		}

		weight := 1.0
		for axis := range 3 {
			hash := fnvHash(token, axis)
			index := int(hash % embeddingDimensions)
			sign := 1.0
			if (hash>>8)&1 == 1 {
				sign = -1
			}
			vector[index] += sign * weight
			weight *= 0.72
		}
	}

	if vectors.IsZero(vector) {
		return vector
	}
	vectors.NormalizeUnit(vector)
	return vector
}

func tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		tokens = append(tokens, field)
	}
	return tokens
}

func fnvHash(text string, salt int) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte{byte(salt)}) //nolint:gosec
	_, _ = hasher.Write([]byte(text))
	return hasher.Sum64()
}

func addScaled(dst, src []float64, scale float64) {
	if len(dst) == 0 || len(dst) != len(src) {
		return
	}
	for i := range dst {
		dst[i] += src[i] * scale
	}
}
