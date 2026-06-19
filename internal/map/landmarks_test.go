package issuemap

import (
	"testing"

	"sortit/internal/domain"
)

func TestComputeMemoryLandmarksAnchorRegion(t *testing.T) {
	clusters := []Cluster{{ID: "c-1", CenterX: 0.7, CenterY: 0.2}}
	memories := []domain.Memory{{
		ID:           "m1",
		Title:        "Ridge default",
		Kind:         domain.MemoryKindDecision,
		Status:       domain.MemoryStatusActive,
		AnchorRegion: "c-1",
		AnchorTags:   []string{"search"},
		Embedding:    []float64{1, 0, 0},
	}}
	issuePositions := map[string]Position{"i1": {X: 0.1, Y: 0.1}}
	issueEmbeddings := map[string][]float64{"i1": {1, 0, 0}}

	landmarks := ComputeMemoryLandmarks(memories, issuePositions, issueEmbeddings, clusters)
	if len(landmarks) != 1 {
		t.Fatalf("expected 1 landmark, got %d", len(landmarks))
	}
	got := landmarks[0]
	if got.X != 0.7 || got.Y != 0.2 {
		t.Fatalf("anchored landmark should sit at cluster centroid, got (%v,%v)", got.X, got.Y)
	}
	if got.Title != "Ridge default" || got.Kind != "decision" {
		t.Fatalf("unexpected landmark metadata: %+v", got)
	}
	if got.Reinforcement <= 0 {
		t.Fatalf("expected positive reinforcement from a related issue, got %v", got.Reinforcement)
	}
}

func TestComputeMemoryLandmarksWeightedCentroid(t *testing.T) {
	memories := []domain.Memory{{
		ID:        "m1",
		Status:    domain.MemoryStatusActive,
		Embedding: []float64{1, 0, 0},
	}}
	issuePositions := map[string]Position{
		"near": {X: 0.1, Y: 0.1},
		"far":  {X: 0.9, Y: 0.9},
	}
	issueEmbeddings := map[string][]float64{
		"near": {1, 0, 0}, // sim 1.0 -> contributes
		"far":  {0, 1, 0}, // sim 0.0 -> below placement floor, excluded
	}

	landmarks := ComputeMemoryLandmarks(memories, issuePositions, issueEmbeddings, nil)
	if len(landmarks) != 1 {
		t.Fatalf("expected 1 landmark, got %d", len(landmarks))
	}
	if landmarks[0].X != 0.1 || landmarks[0].Y != 0.1 {
		t.Fatalf("landmark should land near the related issue, got (%v,%v)", landmarks[0].X, landmarks[0].Y)
	}
}

func TestBuildMapFromProjectionCarriesLandmarks(t *testing.T) {
	mapIssues := make([]MapIssue, 0, minMapIssueCount)
	visible := make(map[string]struct{}, minMapIssueCount)
	for i := range minMapIssueCount {
		id := string(rune('a' + i))
		mapIssues = append(mapIssues, MapIssue{ID: id, X: 0.5, Y: 0.5})
		visible[id] = struct{}{}
	}

	projection := MapProjection{
		Available:       true,
		IssueCount:      len(mapIssues),
		MapIssues:       mapIssues,
		Clusters:        []Cluster{},
		Landmarks:       []Landmark{{ID: "m1", Title: "Ridge default", X: 0.7, Y: 0.2, Reinforcement: 0.5}},
		VisibleIssueIDs: visible,
	}

	response, err := BuildMapFromProjection(projection, nil, 0.4)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(response.Landmarks) != 1 || response.Landmarks[0].ID != "m1" {
		t.Fatalf("expected landmark carried into response, got %+v", response.Landmarks)
	}
}

func TestComputeMemoryLandmarksSkips(t *testing.T) {
	issuePositions := map[string]Position{"i1": {X: 0.1, Y: 0.1}}
	issueEmbeddings := map[string][]float64{"i1": {1, 0, 0}}

	memories := []domain.Memory{
		{ID: "no-embedding", Status: domain.MemoryStatusActive},
		{ID: "superseded", Status: domain.MemoryStatusSuperseded, Embedding: []float64{1, 0, 0}},
		{ID: "no-neighbor", Status: domain.MemoryStatusActive, Embedding: []float64{0, 1, 0}}, // orthogonal to all issues
	}

	landmarks := ComputeMemoryLandmarks(memories, issuePositions, issueEmbeddings, nil)
	if len(landmarks) != 0 {
		t.Fatalf("expected all memories skipped, got %d landmarks: %+v", len(landmarks), landmarks)
	}
}
