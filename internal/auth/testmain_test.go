package auth

import (
	"context"
	"os"
	"testing"
)

// TestMain terminates the shared ParadeDB harness after the package's tests run.
// This is the clean-exit cleanup path: testpostgres.Start leaves Ryuk (the
// testcontainers reaper) enabled as a backstop for interrupted runs, but on a
// normal exit the container is removed by this explicit Terminate rather than
// waiting on the reaper. Without it, every `go test ./internal/auth/...` run
// would leak a container. Mirrors internal/api, internal/issues, and
// internal/issues/appendonly.
func TestMain(m *testing.M) {
	code := m.Run()
	if authPostgresHarness.harness != nil {
		_ = authPostgresHarness.harness.Terminate(context.Background())
	}
	os.Exit(code)
}
