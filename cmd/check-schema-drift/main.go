package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"splat/internal/issues/schemaexport"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "schema drift check failed: %v\n", err)
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

	generated, err := schemaexport.Generate(ctx, baseURL)
	if err != nil {
		return err
	}

	current, err := os.ReadFile(schemaexport.CanonicalSchemaPath())
	if err != nil {
		return fmt.Errorf("read schema.sql: %w", err)
	}

	if !bytes.Equal([]byte(generated), current) {
		fmt.Fprintln(os.Stderr, "--- committed schema.sql")
		fmt.Fprintln(os.Stderr, "+++ generated from migrations")
		showDiff(string(current), generated)
		return fmt.Errorf(
			"internal/issues/sqlc/schema.sql is out of date; run `go run ./cmd/generate-schema-sql`",
		)
	}

	fmt.Println("schema drift check passed")
	return nil
}

func showDiff(a, b string) {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")

	maxLines := len(aLines)
	if len(bLines) > maxLines {
		maxLines = len(bLines)
	}

	for i := range maxLines {
		var aLine, bLine string
		if i < len(aLines) {
			aLine = aLines[i]
		}
		if i < len(bLines) {
			bLine = bLines[i]
		}
		if aLine != bLine {
			if aLine != "" {
				fmt.Fprintf(os.Stderr, "- %s\n", aLine)
			}
			if bLine != "" {
				fmt.Fprintf(os.Stderr, "+ %s\n", bLine)
			}
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
