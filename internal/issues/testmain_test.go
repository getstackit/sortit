package issues

import (
	"context"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if issuesPostgresHarness.harness != nil {
		_ = issuesPostgresHarness.harness.Terminate(context.Background())
	}
	os.Exit(code)
}
