package issuemap

import (
	"math"
	"strings"

	"sortit/internal/domain"
	"sortit/internal/memoryanalytics"
	"sortit/internal/vectors"
)

// landmarkPlacementSimFloor is the minimum cosine similarity for an issue to
// pull a landmark toward its position. Lower than the reinforcement threshold
// so a memory still lands somewhere sensible even in a sparse neighborhood.
const landmarkPlacementSimFloor = 0.3

// ComputeMemoryLandmarks positions active memories on the issue-derived layout
// WITHOUT contributing to the PCA fit. A memory anchored to a region renders at
// that region's centroid; otherwise it lands at the similarity-weighted
// centroid of the issues it relates to. Prominence is the density of related
// issues (memoryanalytics.NeighborhoodReinforcement). Memories with no
// embedding, or no related issues to anchor against, are skipped.
func ComputeMemoryLandmarks(
	memories []domain.Memory,
	issuePositions map[string]Position,
	issueEmbeddings map[string][]float64,
	clusters []Cluster,
) []Landmark {
	clusterByID := make(map[string]Cluster, len(clusters))
	for _, cluster := range clusters {
		clusterByID[cluster.ID] = cluster
	}

	embeddingList := make([][]float64, 0, len(issueEmbeddings))
	for _, embedding := range issueEmbeddings {
		embeddingList = append(embeddingList, embedding)
	}

	landmarks := make([]Landmark, 0, len(memories))
	for _, memory := range memories {
		if memory.Status != domain.MemoryStatusActive || len(memory.Embedding) == 0 {
			continue
		}

		position, ok := landmarkPosition(memory, issuePositions, issueEmbeddings, clusterByID)
		if !ok {
			continue
		}

		reinforcement := memoryanalytics.NeighborhoodReinforcement(memory.Embedding, embeddingList)

		landmarks = append(landmarks, Landmark{
			ID:            memory.ID,
			Title:         landmarkTitle(memory),
			Kind:          string(memory.Kind),
			X:             roundCoordinate(position.X),
			Y:             roundCoordinate(position.Y),
			AnchorTags:    memory.AnchorTags,
			AnchorRegion:  memory.AnchorRegion,
			Reinforcement: math.Round(reinforcement.Score*1000) / 1000,
			Color:         landmarkColor(memory),
		})
	}
	return landmarks
}

func landmarkPosition(
	memory domain.Memory,
	issuePositions map[string]Position,
	issueEmbeddings map[string][]float64,
	clusterByID map[string]Cluster,
) (Position, bool) {
	if memory.AnchorRegion != "" {
		if cluster, ok := clusterByID[memory.AnchorRegion]; ok {
			return Position{X: cluster.CenterX, Y: cluster.CenterY}, true
		}
	}

	var sumX, sumY, sumWeight float64
	for id, embedding := range issueEmbeddings {
		position, ok := issuePositions[id]
		if !ok || len(embedding) != len(memory.Embedding) {
			continue
		}
		sim := vectors.CosineSimilarity(memory.Embedding, embedding)
		if sim < landmarkPlacementSimFloor {
			continue
		}
		sumX += position.X * sim
		sumY += position.Y * sim
		sumWeight += sim
	}
	if sumWeight == 0 {
		return Position{}, false
	}
	return Position{X: sumX / sumWeight, Y: sumY / sumWeight}, true
}

func landmarkTitle(memory domain.Memory) string {
	if title := strings.TrimSpace(memory.Title); title != "" {
		return title
	}
	if len(memory.AnchorTags) > 0 {
		return memory.AnchorTags[0]
	}
	return "memory"
}

func landmarkColor(memory domain.Memory) string {
	if len(memory.AnchorTags) > 0 {
		return TagColor(memory.AnchorTags[0])
	}
	return TagColor("")
}
