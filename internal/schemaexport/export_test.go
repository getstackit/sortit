package schemaexport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalSchemaPathPointsToCheckedInSchema(t *testing.T) {
	path := CanonicalSchemaPath()

	if got, want := filepath.ToSlash(path), "internal/issues/sqlc/schema.sql"; !filepath.IsAbs(path) || filepath.Base(path) != "schema.sql" || !hasPathSuffix(got, want) {
		t.Fatalf("CanonicalSchemaPath() = %q, want absolute path ending with %q", path, want)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("CanonicalSchemaPath() should exist: %v", err)
	}
}

func hasPathSuffix(path, suffix string) bool {
	if len(path) < len(suffix) {
		return false
	}
	return path[len(path)-len(suffix):] == suffix
}
