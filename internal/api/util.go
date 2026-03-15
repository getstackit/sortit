package api

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	issuemap "splat/internal/map"
	"splat/internal/queries"
)

// decodeJSON reads the JSON body of an HTTP request into a value of type T.
// It limits the body size to 1 MB, disallows unknown fields, and closes the body.
func decodeJSON[T any](r *http.Request) (T, error) {
	defer r.Body.Close() //nolint:errcheck

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	var v T
	if err := decoder.Decode(&v); err != nil {
		var zero T
		if errors.Is(err, io.EOF) {
			return zero, io.EOF
		}
		return zero, errors.New("invalid request body")
	}
	return v, nil
}

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

func ParseIssueStatusFilter(values url.Values) (queries.IssueStatusFilter, error) {
	raw := strings.TrimSpace(values.Get("status"))
	if raw == "" {
		return queries.IssueStatusFilterOpen, nil
	}

	switch strings.ToLower(raw) {
	case string(queries.IssueStatusFilterOpen):
		return queries.IssueStatusFilterOpen, nil
	case string(queries.IssueStatusFilterClosed):
		return queries.IssueStatusFilterClosed, nil
	case string(queries.IssueStatusFilterAll):
		return queries.IssueStatusFilterAll, nil
	default:
		return "", strconv.ErrSyntax
	}
}

func ParsePositiveIntQuery(values url.Values, key string) (*int, error) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return nil, nil
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return nil, strconv.ErrSyntax
	}
	return &parsed, nil
}

func ParseNonNegativeIntQuery(values url.Values, key string) (*int, error) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return nil, nil
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return nil, strconv.ErrSyntax
	}
	return &parsed, nil
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
