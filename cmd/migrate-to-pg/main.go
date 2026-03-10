package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: migrate-to-pg <sqlite-path> <postgres-url>\n")
		os.Exit(1)
	}

	sqlitePath := os.Args[1]
	pgURL := os.Args[2]

	if err := run(sqlitePath, pgURL); err != nil {
		log.Fatal(err)
	}
}

func run(sqlitePath, pgURL string) error {
	ctx := context.Background()

	src, err := sql.Open("sqlite", sqlitePath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer src.Close()

	dst, err := sql.Open("pgx", pgURL)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer dst.Close()

	tables := []struct {
		name    string
		columns string
	}{
		{"issues", "id, raw, tags_json, created_by, created_at_unix_nano, status, closed_at_unix_nano, closed_by, tag_scores_json, embedding_json, assigned_to"},
		{"issue_posts", "id, issue_id, raw, created_by, created_at_unix_nano, sequence, kind"},
		{"issue_operations", "id, kind, created_by, created_at_unix_nano, note"},
		{"issue_operation_participants", "operation_id, issue_id, role, sequence"},
		{"issue_links", "id, source_issue_id, target_issue_id, type, created_by, created_at_unix_nano, note, operation_id"},
		{"tags", "name, description, created_at_unix_nano, embedding_json"},
		{"users", "id, login, display_name, avatar_url, email, created_at_unix_nano, updated_at_unix_nano"},
		{"auth_accounts", "id, user_id, provider, provider_user_id, created_at_unix_nano"},
		{"sessions", "id, user_id, token_hash, expires_at_unix_nano, created_at_unix_nano"},
		{"api_tokens", "id, user_id, token_hash, token_prefix, created_at_unix_nano, revoked_at_unix_nano"},
	}

	for _, t := range tables {
		count, err := copyTable(ctx, src, dst, t.name, t.columns)
		if err != nil {
			return fmt.Errorf("copy %s: %w", t.name, err)
		}
		log.Printf("copied %d rows from %s", count, t.name)
	}

	// Sync sequences from metadata table
	if err := syncSequence(ctx, src, dst, "next_issue_seq", "issue_seq"); err != nil {
		return err
	}
	if err := syncSequence(ctx, src, dst, "next_issue_operation_seq", "issue_operation_seq"); err != nil {
		return err
	}

	// Verify row counts
	log.Println("verifying row counts...")
	for _, t := range tables {
		srcCount, err := tableCount(ctx, src, t.name)
		if err != nil {
			return fmt.Errorf("count src %s: %w", t.name, err)
		}
		dstCount, err := tableCount(ctx, dst, t.name)
		if err != nil {
			return fmt.Errorf("count dst %s: %w", t.name, err)
		}
		if srcCount != dstCount {
			return fmt.Errorf("row count mismatch for %s: sqlite=%d postgres=%d", t.name, srcCount, dstCount)
		}
		log.Printf("  %s: %d rows OK", t.name, dstCount)
	}

	log.Println("migration complete")
	return nil
}

func copyTable(ctx context.Context, src, dst *sql.DB, table, columns string) (int, error) {
	rows, err := src.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s", columns, table))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}

	var placeholders strings.Builder
	for i := range cols {
		if i > 0 {
			placeholders.WriteString(", ")
		}
		placeholders.WriteString(fmt.Sprintf("$%d", i+1))
	}
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, columns, placeholders.String())

	tx, err := dst.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return 0, err
		}
		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return 0, fmt.Errorf("insert row %d: %w", count+1, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return count, tx.Commit()
}

func syncSequence(ctx context.Context, src, dst *sql.DB, metadataKey, pgSeqName string) error {
	var value string
	err := src.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key = ?", metadataKey).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("metadata key %q not found, skipping sequence sync", metadataKey)
			return nil
		}
		return fmt.Errorf("read metadata %q: %w", metadataKey, err)
	}

	_, err = dst.ExecContext(ctx, fmt.Sprintf("SELECT setval('%s', %s, true)", pgSeqName, value))
	if err != nil {
		return fmt.Errorf("setval %s to %s: %w", pgSeqName, value, err)
	}
	log.Printf("set sequence %s to %s", pgSeqName, value)
	return nil
}

func tableCount(ctx context.Context, db *sql.DB, table string) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
