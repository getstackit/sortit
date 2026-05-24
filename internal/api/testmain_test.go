package api

import (
	"context"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if apiPostgresHarness.harness != nil {
		_ = apiPostgresHarness.harness.Terminate(context.Background())
	}
	os.Exit(code)
}
