package api

import (
	"net/url"
	"strconv"
	"strings"

	issuemap "bored/internal/map"
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

func parseFloatQuery(values url.Values, key string) (float64, error) {
	value := strings.TrimSpace(values.Get(key))
	if value == "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseFloat(value, 64)
}
