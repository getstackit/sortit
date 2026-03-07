package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"bored/internal/issues"
	issuemap "bored/internal/map"
)

func TestHandlerReturnsNotFoundForUnknownPaths(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api", "/api/v1"},
	})

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown path, got %d", rec.Code)
	}
}

func TestHandlerReturnsJSONForUnknownAPIPaths(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api", "/api/v1"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown API path, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected JSON content type, got %q", got)
	}

	var payload errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if payload.Error != "route not found" {
		t.Fatalf("expected route not found error, got %q", payload.Error)
	}
}

func TestHandlerServesRootAndAPI(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api", "/api/v1"},
	})
	handler := server.Handler()

	t.Run("root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for root, got %d", rec.Code)
		}
	})

	t.Run("health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for health, got %d", rec.Code)
		}
	})

	t.Run("map", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/map", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for map, got %d", rec.Code)
		}
	})

	t.Run("issue by id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/issues/sample-1", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for unknown issue in empty store, got %d", rec.Code)
		}
	})

	t.Run("map edges", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodGet,
			"/api/map/edges?xMin=0&xMax=1&yMin=0&yMax=1",
			nil,
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for map edges, got %d", rec.Code)
		}
	})
}

func TestCORSPreflightBehavior(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})
	handler := server.Handler()

	t.Run("allowed route and origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 for valid preflight, got %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
			t.Fatalf("expected CORS allow origin header, got %q", got)
		}
	})

	t.Run("allowed nested issue route and origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/issues/sample-1", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 for nested issue preflight, got %d", rec.Code)
		}
	})

	t.Run("disallowed origin falls through to route handling", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
		req.Header.Set("Origin", "http://evil.example")
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for disallowed preflight, got %d", rec.Code)
		}
	})

	t.Run("unknown route is not treated as successful preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/missing", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for unknown preflight path, got %d", rec.Code)
		}
	})

	t.Run("invalid viewport is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/map?xMin=bad", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid viewport query, got %d", rec.Code)
		}
	})
}

func TestMapEndpointsStartEmptyByDefault(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})
	handler := server.Handler()

	mapReq := httptest.NewRequest(http.MethodGet, "/api/map", nil)
	mapRec := httptest.NewRecorder()
	handler.ServeHTTP(mapRec, mapReq)

	if mapRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for map, got %d", mapRec.Code)
	}

	var mapPayload issuemap.MapResponse
	if err := json.NewDecoder(mapRec.Body).Decode(&mapPayload); err != nil {
		t.Fatalf("failed to decode map response: %v", err)
	}
	if len(mapPayload.Issues) != 0 || len(mapPayload.Edges) != 0 || len(mapPayload.Clusters) != 0 {
		t.Fatalf(
			"expected empty map response, got %d issues, %d edges, %d clusters",
			len(mapPayload.Issues),
			len(mapPayload.Edges),
			len(mapPayload.Clusters),
		)
	}

	edgesReq := httptest.NewRequest(http.MethodGet, "/api/map/edges", nil)
	edgesRec := httptest.NewRecorder()
	handler.ServeHTTP(edgesRec, edgesReq)

	if edgesRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for map edges, got %d", edgesRec.Code)
	}

	var edgesPayload issuemap.EdgeResponse
	if err := json.NewDecoder(edgesRec.Body).Decode(&edgesPayload); err != nil {
		t.Fatalf("failed to decode map edge response: %v", err)
	}
	if len(edgesPayload.Edges) != 0 {
		t.Fatalf("expected no edges by default, got %d", len(edgesPayload.Edges))
	}
}

func TestMapEdgesEndpointReturnsEdgeResponse(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  issues.NewInMemoryStore(issues.SeedIssues()),
	})
	handler := server.Handler()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/map/edges?xMin=0.15&xMax=0.45&yMin=0.15&yMax=0.45",
		nil,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for map edges, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected JSON content type, got %q", got)
	}

	var payload issuemap.EdgeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode edge response: %v", err)
	}

	expected, err := issuemap.BuildEdgeResponseFromIssues(issues.SeedIssues(), &issuemap.Viewport{
		XMin: 0.15,
		XMax: 0.45,
		YMin: 0.15,
		YMax: 0.45,
	})
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}

	if len(payload.Edges) != len(expected.Edges) {
		t.Fatalf("expected %d edges, got %d", len(expected.Edges), len(payload.Edges))
	}
	for i := range expected.Edges {
		if payload.Edges[i] != expected.Edges[i] {
			t.Fatalf("unexpected edge at index %d: got %+v want %+v", i, payload.Edges[i], expected.Edges[i])
		}
	}
}

func TestMapEdgesEndpointUsesDefaultViewport(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  issues.NewInMemoryStore(issues.SeedIssues()),
	})
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/map/edges", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for default map edges request, got %d", rec.Code)
	}

	var payload issuemap.EdgeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode edge response: %v", err)
	}

	expected, err := issuemap.BuildEdgeResponseFromIssues(issues.SeedIssues(), nil)
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}

	if len(payload.Edges) != len(expected.Edges) {
		t.Fatalf("expected %d edges, got %d", len(expected.Edges), len(payload.Edges))
	}
}

func TestMapEdgesEndpointZoomedInViewport(t *testing.T) {
	seed := issues.SeedIssues()
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  issues.NewInMemoryStore(seed),
	})
	handler := server.Handler()

	mapResult, err := issuemap.BuildMapFromIssues(seed, nil)
	if err != nil {
		t.Fatalf("BuildMap returned error: %v", err)
	}
	anchor := mapResult.Issues[0]
	viewport := &issuemap.Viewport{
		XMin: anchor.X - 0.1,
		XMax: anchor.X + 0.1,
		YMin: anchor.Y - 0.1,
		YMax: anchor.Y + 0.1,
	}

	req := httptest.NewRequest(
		http.MethodGet,
		mapEdgesPath(viewport),
		nil,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for zoomed-in map edges request, got %d", rec.Code)
	}

	var payload issuemap.EdgeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode edge response: %v", err)
	}

	expected, err := issuemap.BuildEdgeResponseFromIssues(seed, viewport)
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}
	if len(payload.Edges) != len(expected.Edges) {
		t.Fatalf("expected %d edges, got %d", len(expected.Edges), len(payload.Edges))
	}

	positions := make(map[string]issuemap.Position, len(mapResult.Issues))
	for _, issue := range mapResult.Issues {
		positions[issue.ID] = issuemap.Position{X: issue.X, Y: issue.Y}
	}

	full, err := issuemap.BuildEdgeResponseFromIssues(seed, nil)
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}
	if len(payload.Edges) > len(full.Edges) {
		t.Fatalf("expected zoomed-in viewport to return no more than %d edges, got %d", len(full.Edges), len(payload.Edges))
	}

	for _, edge := range payload.Edges {
		source := positions[edge.Source]
		target := positions[edge.Target]
		sourceVisible := source.X >= viewport.XMin &&
			source.X <= viewport.XMax &&
			source.Y >= viewport.YMin &&
			source.Y <= viewport.YMax
		targetVisible := target.X >= viewport.XMin &&
			target.X <= viewport.XMax &&
			target.Y >= viewport.YMin &&
			target.Y <= viewport.YMax
		if !sourceVisible && !targetVisible {
			t.Fatalf("edge %s-%s should have a visible endpoint in the zoomed-in viewport", edge.Source, edge.Target)
		}
	}
}

func TestMapEdgesEndpointVeryTightViewport(t *testing.T) {
	seed := issues.SeedIssues()
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  issues.NewInMemoryStore(seed),
	})
	handler := server.Handler()

	mapResult, err := issuemap.BuildMapFromIssues(seed, nil)
	if err != nil {
		t.Fatalf("BuildMap returned error: %v", err)
	}
	anchor := mapResult.Issues[0]
	viewport := &issuemap.Viewport{
		XMin: anchor.X - 0.005,
		XMax: anchor.X + 0.005,
		YMin: anchor.Y - 0.005,
		YMax: anchor.Y + 0.005,
	}

	req := httptest.NewRequest(
		http.MethodGet,
		mapEdgesPath(viewport),
		nil,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for tight map edges request, got %d", rec.Code)
	}

	var payload issuemap.EdgeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode edge response: %v", err)
	}

	expected, err := issuemap.BuildEdgeResponseFromIssues(seed, viewport)
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}
	if len(payload.Edges) != len(expected.Edges) {
		t.Fatalf("expected %d edges, got %d", len(expected.Edges), len(payload.Edges))
	}
}

func TestMapEdgesEndpointExactPointViewport(t *testing.T) {
	seed := issues.SeedIssues()
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
		IssueStore:  issues.NewInMemoryStore(seed),
	})
	handler := server.Handler()

	mapResult, err := issuemap.BuildMapFromIssues(seed, nil)
	if err != nil {
		t.Fatalf("BuildMap returned error: %v", err)
	}
	anchor := mapResult.Issues[0]
	viewport := &issuemap.Viewport{
		XMin: anchor.X,
		XMax: anchor.X,
		YMin: anchor.Y,
		YMax: anchor.Y,
	}

	req := httptest.NewRequest(http.MethodGet, mapEdgesPath(viewport), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for exact-point map edges request, got %d", rec.Code)
	}

	var payload issuemap.EdgeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode edge response: %v", err)
	}

	expected, err := issuemap.BuildEdgeResponseFromIssues(seed, viewport)
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}
	if len(payload.Edges) != len(expected.Edges) {
		t.Fatalf("expected %d edges, got %d", len(expected.Edges), len(payload.Edges))
	}

	full, err := issuemap.BuildEdgeResponseFromIssues(seed, nil)
	if err != nil {
		t.Fatalf("BuildEdgeResponse returned error: %v", err)
	}
	if len(full.Edges) == 0 {
		if len(payload.Edges) != 0 {
			t.Fatalf("expected exact-point viewport to return 0 edges when full map has none, got %d", len(payload.Edges))
		}
		return
	}
	if len(payload.Edges) >= len(full.Edges) {
		t.Fatalf("expected exact-point viewport to return fewer than %d full-map edges, got %d", len(full.Edges), len(payload.Edges))
	}
}

func TestMapEdgesEndpointRejectsInvalidViewport(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/map/edges?xMin=bad", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid edges viewport query, got %d", rec.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if payload.Error != "invalid viewport query" {
		t.Fatalf("expected invalid viewport error, got %q", payload.Error)
	}
}

func TestMapEdgesEndpointRejectsNonFiniteViewport(t *testing.T) {
	server := NewServer(ServerConfig{
		CORSOrigins: []string{"http://localhost:3000"},
		APIPrefixes: []string{"/api"},
	})
	handler := server.Handler()

	for _, path := range []string{
		"/api/map/edges?xMin=NaN&xMax=1&yMin=0&yMax=1",
		"/api/map/edges?xMin=0&xMax=+Inf&yMin=0&yMax=1",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for non-finite viewport query %q, got %d", path, rec.Code)
		}
	}
}

func mapEdgesPath(viewport *issuemap.Viewport) string {
	return "/api/map/edges?xMin=" + formatFloat(viewport.XMin) +
		"&xMax=" + formatFloat(viewport.XMax) +
		"&yMin=" + formatFloat(viewport.YMin) +
		"&yMax=" + formatFloat(viewport.YMax)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
