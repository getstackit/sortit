package issuemap

import (
	"math"
	"testing"
)

func TestGeneratedDatasetShape(t *testing.T) {
	issues := AllIssues()
	if len(issues) != generatedIssueCount {
		t.Fatalf("expected %d generated issues, got %d", generatedIssueCount, len(issues))
	}

	if len(AllTags()) != len(tagCatalog) {
		t.Fatalf("expected %d tags, got %d", len(tagCatalog), len(AllTags()))
	}

	embeddings := AllEmbeddings()
	if len(embeddings) != generatedIssueCount {
		t.Fatalf("expected %d embeddings, got %d", generatedIssueCount, len(embeddings))
	}

	for _, issue := range issues[:10] {
		if issue.Raw == "" {
			t.Fatalf("issue %s has empty raw text", issue.ID)
		}
		if len(issue.Tags) < 2 {
			t.Fatalf("issue %s should have at least two tags, got %d", issue.ID, len(issue.Tags))
		}

		embedding := Embedding(issue.ID)
		if len(embedding) != embeddingDimensions {
			t.Fatalf("issue %s embedding has %d dims, expected %d", issue.ID, len(embedding), embeddingDimensions)
		}
	}
}

func TestBuildMapReturnsPositionForEveryIssue(t *testing.T) {
	result, err := BuildMap(nil)
	if err != nil {
		t.Fatalf("BuildMap returned error: %v", err)
	}

	if len(result.Issues) != len(AllIssues()) {
		t.Fatalf("expected %d issues, got %d", len(AllIssues()), len(result.Issues))
	}

	for _, issue := range result.Issues {
		if issue.X < 0 || issue.X > 1 || issue.Y < 0 || issue.Y > 1 {
			t.Fatalf("issue %s has out-of-range coordinates: (%f, %f)", issue.ID, issue.X, issue.Y)
		}
	}

	if len(result.Edges) == 0 {
		t.Fatal("expected generated map to include similarity edges")
	}

	maxEdges := maxInt(
		minVisibleEdgeCount,
		int(math.Ceil(float64(len(result.Issues))*maxVisibleEdgeRatio)),
	)
	maxEdges = minInt(maxEdges, maxVisibleEdges)
	if len(result.Edges) > maxEdges {
		t.Fatalf("expected at most %d edges, got %d", maxEdges, len(result.Edges))
	}

	for i := 1; i < len(result.Edges); i++ {
		prev := result.Edges[i-1]
		curr := result.Edges[i]
		if prev.Similarity < curr.Similarity {
			t.Fatalf("expected edges sorted by similarity: %+v before %+v", prev, curr)
		}
	}
}

func TestBaseMapUsesRenderedCoordinatesForViewportFiltering(t *testing.T) {
	base, err := loadBaseMapData()
	if err != nil {
		t.Fatalf("loadBaseMapData returned error: %v", err)
	}

	if len(base.mapIssues) == 0 {
		t.Fatal("expected map issues")
	}

	for _, issue := range base.mapIssues {
		position, ok := base.positions[issue.ID]
		if !ok {
			t.Fatalf("missing stored position for issue %s", issue.ID)
		}
		if position.X != issue.X || position.Y != issue.Y {
			t.Fatalf(
				"expected backend viewport filtering to use rendered coordinates for issue %s: position=%+v issue=(%f,%f)",
				issue.ID,
				position,
				issue.X,
				issue.Y,
			)
		}
	}
}

func TestBuildEdgeResponseRespectsViewport(t *testing.T) {
	base, err := loadBaseMapData()
	if err != nil {
		t.Fatalf("loadBaseMapData returned error: %v", err)
	}

	anchor := base.mapIssues[0]
	viewport := &Viewport{
		XMin: anchor.X - 0.1,
		XMax: anchor.X + 0.1,
		YMin: anchor.Y - 0.1,
		YMax: anchor.Y + 0.1,
	}

	result, err := BuildEdgeResponse(viewport)
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}

	for _, edge := range result.Edges {
		source := base.positions[edge.Source]
		target := base.positions[edge.Target]
		sourceVisible :=
			source.X >= viewport.XMin &&
				source.X <= viewport.XMax &&
				source.Y >= viewport.YMin &&
				source.Y <= viewport.YMax
		targetVisible :=
			target.X >= viewport.XMin &&
				target.X <= viewport.XMax &&
				target.Y >= viewport.YMin &&
				target.Y <= viewport.YMax

		if !sourceVisible && !targetVisible {
			t.Fatalf(
				"edge %s-%s should have at least one visible endpoint: source=%+v target=%+v",
				edge.Source,
				edge.Target,
				source,
				target,
			)
		}
	}
}

func TestFilterEdgesForViewportPrioritizesFullyVisibleEdges(t *testing.T) {
	base := mapBaseData{
		positions: map[string]Position{
			"a": {X: 0.2, Y: 0.2},
			"b": {X: 0.3, Y: 0.3},
		},
	}

	base.candidateEdges = append(base.candidateEdges, Edge{
		Source:     "a",
		Target:     "b",
		Similarity: 0.75,
	})

	for i := 0; i < minVisibleEdgeCount+4; i++ {
		targetID := string(rune('c' + i))
		base.positions[targetID] = Position{X: 1.4, Y: 1.4}
		base.candidateEdges = append(base.candidateEdges, Edge{
			Source:     "a",
			Target:     targetID,
			Similarity: 0.99 - float64(i)*0.01,
		})
	}

	sortEdgesBySimilarity(base.candidateEdges)

	result := filterEdgesForViewport(base, Viewport{
		XMin: 0.1,
		XMax: 0.4,
		YMin: 0.1,
		YMax: 0.4,
	})

	if len(result) != minVisibleEdgeCount {
		t.Fatalf("expected %d edges, got %d", minVisibleEdgeCount, len(result))
	}

	first := result[0]
	if first.Source != "a" || first.Target != "b" {
		t.Fatalf("expected fully visible edge to be prioritized first, got %+v", first)
	}

	foundFullyVisible := false
	for _, edge := range result {
		if edge.Source == "a" && edge.Target == "b" {
			foundFullyVisible = true
			break
		}
	}
	if !foundFullyVisible {
		t.Fatal("expected fully visible edge to survive backend truncation")
	}
}

func TestBuildEdgeResponseSingleVisibleIssueReturnsConnectedEdges(t *testing.T) {
	base, err := loadBaseMapData()
	if err != nil {
		t.Fatalf("loadBaseMapData returned error: %v", err)
	}
	if len(base.candidateEdges) == 0 {
		t.Fatal("expected candidate edges in base map data")
	}

	anchorID := base.candidateEdges[0].Source
	anchor := base.positions[anchorID]
	viewport := &Viewport{
		XMin: anchor.X,
		XMax: anchor.X,
		YMin: anchor.Y,
		YMax: anchor.Y,
	}

	result, err := BuildEdgeResponse(viewport)
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}
	if len(result.Edges) == 0 {
		t.Fatal("expected exact-point viewport to retain edges connected to the visible issue")
	}

	full, err := BuildEdgeResponse(nil)
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}
	if len(result.Edges) >= len(full.Edges) {
		t.Fatalf("expected exact-point viewport to return fewer than %d full-map edges, got %d", len(full.Edges), len(result.Edges))
	}

	for _, edge := range result.Edges {
		if edge.Source != anchorID && edge.Target != anchorID {
			t.Fatalf("edge %+v should be connected to the only visible issue %s", edge, anchorID)
		}
	}
}

func TestCosineSimilarityHandlesMismatchedEmbeddings(t *testing.T) {
	if got := cosineSimilarity([]float64{1, 0}, []float64{1}); got != 0 {
		t.Fatalf("expected mismatched embeddings to return 0, got %f", got)
	}
}

func TestUnitCosineSimilarityMatchesNormalizedCosine(t *testing.T) {
	a := []float64{0.6, 0.8}
	b := []float64{0.8, 0.6}

	got := unitCosineSimilarity(a, b)
	want := cosineSimilarity(a, b)
	if got != want {
		t.Fatalf("expected unit cosine similarity %f to match cosine similarity %f", got, want)
	}
}
