package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"splat/internal/schemaexport"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "generate schema.sql failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	baseURL := strings.TrimSpace(firstNonEmpty(
		os.Getenv("SPLAT_TEST_DATABASE_URL"),
		os.Getenv("SPLAT_DATABASE_URL"),
	))
	if baseURL == "" {
		return fmt.Errorf("set SPLAT_TEST_DATABASE_URL or SPLAT_DATABASE_URL")
	}

	if err := schemaexport.WriteCanonicalSchema(ctx, baseURL); err != nil {
		return err
	}

	fmt.Printf("wrote %s\n", schemaexport.CanonicalSchemaPath())
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
