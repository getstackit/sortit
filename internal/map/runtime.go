package issuemap

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"unicode"

	"splat/internal/issues"
)

func BuildMapFromIssues(storeIssues []issues.Issue, viewport *Viewport) (MapResponse, error) {
	return BuildMapFromIssuesWithTags(storeIssues, nil, viewport)
}

func BuildMapFromIssuesWithTags(storeIssues []issues.Issue, storeTags []issues.Tag, viewport *Viewport) (MapResponse, error) {
	return BuildMapFromIssuesWithTagsAndThreshold(storeIssues, storeTags, viewport, minEdgeSimilarity)
}

func BuildMapFromIssuesWithTagsAndThreshold(storeIssues []issues.Issue, storeTags []issues.Tag, viewport *Viewport, edgeThreshold float64) (MapResponse, error) {
	base, err := buildBaseMapDataFromIssues(storeIssues, storeTags, edgeThreshold)
	if err != nil {
		return MapResponse{}, err
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
		Issues:   base.mapIssues,
		Edges:    edges,
		Clusters: clusters,
	}, nil
}

func BuildEdgeResponseFromIssues(storeIssues []issues.Issue, viewport *Viewport) (EdgeResponse, error) {
	return BuildEdgeResponseFromIssuesWithTags(storeIssues, nil, viewport)
}

func BuildEdgeResponseFromIssuesWithTags(storeIssues []issues.Issue, storeTags []issues.Tag, viewport *Viewport) (EdgeResponse, error) {
	return BuildEdgeResponseFromIssuesWithTagsAndThreshold(storeIssues, storeTags, viewport, minEdgeSimilarity)
}

func BuildEdgeResponseFromIssuesWithTagsAndThreshold(storeIssues []issues.Issue, storeTags []issues.Tag, viewport *Viewport, edgeThreshold float64) (EdgeResponse, error) {
	base, err := buildBaseMapDataFromIssues(storeIssues, storeTags, edgeThreshold)
	if err != nil {
		return EdgeResponse{}, err
	}

	edges := filterEdgesForViewport(base, normalizeViewport(viewport))
	if edges == nil {
		edges = []Edge{}
	}

	return EdgeResponse{Edges: edges}, nil
}

func buildBaseMapDataFromIssues(storeIssues []issues.Issue, storeTags []issues.Tag, edgeThreshold float64) (mapBaseData, error) {
	mapIssuesInput, tags, issueEmbeddings, tagEmbeddings := runtimeMapInputs(storeIssues, storeTags)

	positions, err := ComputePositions(mapIssuesInput, tags, tagEmbeddings)
	if err != nil {
		return mapBaseData{}, err
	}

	mapIssues := make([]MapIssue, len(mapIssuesInput))
	roundedPositions := make(map[string]Position, len(mapIssuesInput))
	for i, issue := range mapIssuesInput {
		p, ok := positions[issue.ID]
		if !ok {
			return mapBaseData{}, fmt.Errorf("missing position for issue %s", issue.ID)
		}

		rounded := roundPosition(p)
		roundedPositions[issue.ID] = rounded
		mapIssues[i] = MapIssue{
			ID:     issue.ID,
			Raw:    issue.Raw,
			Status: storeIssues[i].Status,
			Tags:   issue.Tags,
			X:      rounded.X,
			Y:      rounded.Y,
		}
	}

	edges := ComputeEdgesWithEmbeddings(mapIssuesInput, issueEmbeddings, edgeThreshold)
	sortEdgesBySimilarity(edges)
	clusters := ComputeFactorClusters(mapIssuesInput, positions)

	return mapBaseData{
		mapIssues:      mapIssues,
		positions:      roundedPositions,
		candidateEdges: edges,
		clusters:       clusters,
	}, nil
}

func runtimeMapInputs(storeIssues []issues.Issue, storeTags []issues.Tag) ([]Issue, []string, map[string][]float64, map[string][]float64) {
	tagNames := runtimeTagNames(storeIssues, storeTags)
	tagEmbeddings := runtimeTagEmbeddings(tagNames, storeTags)

	mapIssues := make([]Issue, len(storeIssues))
	embeddings := make(map[string][]float64, len(storeIssues))
	for i, storeIssue := range storeIssues {
		tagScores := runtimeStoredTagRelevances(storeIssue)
		mapIssue := Issue{
			ID:   storeIssue.ID,
			Raw:  storeIssue.Raw,
			Tags: tagScores,
		}
		mapIssues[i] = mapIssue
		embeddings[storeIssue.ID] = runtimeStoredEmbedding(storeIssue, mapIssue, tagEmbeddings)
	}

	// If every issue is effectively untagged, fall back to circular layout.
	hasTagSignal := false
	for _, issue := range mapIssues {
		if len(issue.Tags) > 0 {
			hasTagSignal = true
			break
		}
	}
	if !hasTagSignal {
		tagNames = nil
		tagEmbeddings = nil
	}

	return mapIssues, tagNames, embeddings, tagEmbeddings
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

	sort.Strings(tags)
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
			relevances = append(relevances, TagRelevance{
				Tag:       name,
				Relevance: tag.Relevance,
			})
		}

		sort.Slice(relevances, func(i, j int) bool {
			if relevances[i].Relevance == relevances[j].Relevance {
				return relevances[i].Tag < relevances[j].Tag
			}
			return relevances[i].Relevance > relevances[j].Relevance
		})
		return relevances
	}

	return runtimeTagRelevances(issue.Tags)
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

	sort.Slice(relevances, func(i, j int) bool {
		return relevances[i].Tag < relevances[j].Tag
	})
	return relevances
}

func runtimeStoredEmbedding(storeIssue issues.Issue, issue Issue, tagEmbeddings map[string][]float64) []float64 {
	if len(storeIssue.Embedding) > 0 {
		embedding := append([]float64(nil), storeIssue.Embedding...)
		if !isZeroVector(embedding) {
			normalizeVector(embedding)
			return embedding
		}
	}

	return runtimeIssueEmbedding(issue, tagEmbeddings)
}

func runtimeIssueEmbedding(issue Issue, tagEmbeddings map[string][]float64) []float64 {
	vector := make([]float64, embeddingDimensions)
	textVector := embeddingFromText(issue.Raw)
	addScaled(vector, textVector, 0.7)

	for _, tag := range issue.Tags {
		addScaled(vector, tagEmbeddings[tag.Tag], 0.9*tag.Relevance)
	}

	if isZeroVector(vector) {
		return embeddingFromText(issue.ID + " " + issue.Raw)
	}
	normalizeVector(vector)
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
		for axis := 0; axis < 3; axis++ {
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

	if isZeroVector(vector) {
		return vector
	}
	normalizeVector(vector)
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
	_, _ = hasher.Write([]byte{byte(salt)})
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

func isZeroVector(values []float64) bool {
	for _, value := range values {
		if value != 0 {
			return false
		}
	}
	return true
}
