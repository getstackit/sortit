package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
