package appendonly

import (
	"context"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if appendonlyPostgresHarness.harness != nil {
		_ = appendonlyPostgresHarness.harness.Terminate(context.Background())
	}
	os.Exit(code)
}
