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

func TestCosineSimilarityHandlesMismatchedEmbeddings(t *testing.T) {
	if got := cosineSimilarity([]float64{1, 0}, []float64{1}); got != 0 {
		t.Fatalf("expected mismatched embeddings to return 0, got %f", got)
	}
}
