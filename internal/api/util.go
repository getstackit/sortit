package api

import (
	"math"
	"net/url"
	"strconv"
	"strings"

	"splat/internal/issues"
	issuemap "splat/internal/map"
)

type issueStatusFilter string

const (
	issueStatusFilterOpen   issueStatusFilter = "open"
	issueStatusFilterClosed issueStatusFilter = "closed"
	issueStatusFilterAll    issueStatusFilter = "all"
)

func ParseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, strings.TrimRight(part, "/"))
	}
	return out
}

func ParseViewport(values url.Values) (*issuemap.Viewport, error) {
	keys := []string{"xMin", "xMax", "yMin", "yMax"}
	present := false
	for _, key := range keys {
		if values.Get(key) != "" {
			present = true
			break
		}
	}
	if !present {
		return nil, nil
	}

	xMin, err := parseFloatQuery(values, "xMin")
	if err != nil {
		return nil, err
	}
	xMax, err := parseFloatQuery(values, "xMax")
	if err != nil {
		return nil, err
	}
	yMin, err := parseFloatQuery(values, "yMin")
	if err != nil {
		return nil, err
	}
	yMax, err := parseFloatQuery(values, "yMax")
	if err != nil {
		return nil, err
	}

	return &issuemap.Viewport{
		XMin: xMin,
		XMax: xMax,
		YMin: yMin,
		YMax: yMax,
	}, nil
}

func ParseEdgeThreshold(values url.Values) (*float64, error) {
	raw := strings.TrimSpace(values.Get("edgeThreshold"))
	if raw == "" {
		return nil, nil
	}

	threshold, err := parseFloatQuery(values, "edgeThreshold")
	if err != nil {
		return nil, err
	}
	if threshold < 0 || threshold > 1 {
		return nil, strconv.ErrSyntax
	}
	return &threshold, nil
}

func ParseIssueStatusFilter(values url.Values) (issueStatusFilter, error) {
	raw := strings.TrimSpace(values.Get("status"))
	if raw == "" {
		return issueStatusFilterOpen, nil
	}

	switch strings.ToLower(raw) {
	case string(issueStatusFilterOpen):
		return issueStatusFilterOpen, nil
	case string(issueStatusFilterClosed):
		return issueStatusFilterClosed, nil
	case string(issueStatusFilterAll):
		return issueStatusFilterAll, nil
	default:
		return "", strconv.ErrSyntax
	}
}

func filterIssuesByStatus(items []issues.Issue, filter issueStatusFilter) []issues.Issue {
	if filter == issueStatusFilterAll {
		return items
	}

	filtered := make([]issues.Issue, 0, len(items))
	for _, item := range items {
		status := item.Status
		if status == "" {
			status = issues.StatusOpen
		}

		if filter == issueStatusFilterOpen && status == issues.StatusOpen {
			filtered = append(filtered, item)
		}
		if filter == issueStatusFilterClosed && status == issues.StatusClosed {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func parseFloatQuery(values url.Values, key string) (float64, error) {
	value := strings.TrimSpace(values.Get(key))
	if value == "" {
		return 0, strconv.ErrSyntax
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, strconv.ErrSyntax
	}
	return parsed, nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}
