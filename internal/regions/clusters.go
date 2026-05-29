package regions

import (
	"sort"
	"strings"
	"time"

	"sortit/internal/domain"
	"sortit/internal/issues"
	issuemap "sortit/internal/map"
)

// ListClusterRegionsWithMetrics turns the k-means clusters from a
// MapProjection into RegionWithMetrics entries. Members come from each
// cluster's IssueIDs (filtered to the items currently visible to the
// store, so closed-and-deleted issues don't appear). Sorted by mass
// descending, ties broken by cluster ID for stability.
func ListClusterRegionsWithMetrics(
	clusters []issuemap.Cluster,
	items []issues.Issue,
	events []issues.Event,
	window domain.TimeWindow,
	now time.Time,
) []RegionWithMetrics {
	if len(clusters) == 0 {
		return nil
	}

	byID := make(map[string]issues.Issue, len(items))
	for _, issue := range items {
		byID[issue.ID] = issue
	}

	out := make([]RegionWithMetrics, 0, len(clusters))
	for _, cluster := range clusters {
		members := make([]issues.Issue, 0, len(cluster.IssueIDs))
		for _, id := range cluster.IssueIDs {
			if issue, ok := byID[id]; ok {
				members = append(members, issue)
			}
		}

		key := domain.RegionKey{Kind: domain.RegionKindCluster, ID: cluster.ID}
		metrics, ok := ComputeMetricsForMembers(key, members, events, window, now)
		if !ok {
			continue
		}
		out = append(out, RegionWithMetrics{
			Region: domain.Region{
				Key:         key,
				Label:       clusterLabel(cluster),
				Description: clusterDescription(cluster),
			},
			Metrics: metrics,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Metrics.Mass != out[j].Metrics.Mass {
			return out[i].Metrics.Mass > out[j].Metrics.Mass
		}
		return out[i].Region.Key.ID < out[j].Region.Key.ID
	})
	return out
}

// clusterLabel returns a human-readable label for a cluster region. If
// the map projection already produced a Label (top tags joined), use it;
// otherwise fall back to the top tag or the cluster ID.
func clusterLabel(cluster issuemap.Cluster) string {
	if strings.TrimSpace(cluster.Label) != "" {
		return cluster.Label
	}
	if strings.TrimSpace(cluster.TopTag) != "" {
		return cluster.TopTag
	}
	return cluster.ID
}

// clusterDescription is the supplementary "what's in this cluster" text
// shown on the regions tile and detail page. We don't have richer data
// yet; the label already encodes the top tags, so this stays empty.
func clusterDescription(_ issuemap.Cluster) string {
	return ""
}
