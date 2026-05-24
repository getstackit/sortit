package schemaexport

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"sortit/internal/issues"
)

const pgDriverName = "pgx"

type snapshot struct {
	Extensions  []extension
	Sequences   []sequence
	Tables      []table
	Constraints []constraint
	Indexes     []index
}

type extension struct {
	Name   string
	Schema string
}

type sequence struct {
	Name        string
	OwnedTable  string
	OwnedColumn string
}

type table struct {
	Name    string
	Columns []column
}

type column struct {
	Name    string
	Type    string
	NotNull bool
	Default string
}

type constraint struct {
	TableName  string
	Name       string
	Definition string
}

type index struct {
	Definition string
}

func Generate(ctx context.Context, baseURL string) (string, error) {
	adminURL, err := adminConnectionString(baseURL)
	if err != nil {
		return "", err
	}

	adminDB, err := sql.Open(pgDriverName, adminURL)
	if err != nil {
		return "", fmt.Errorf("open postgres admin connection: %w", err)
	}
	defer adminDB.Close() //nolint:errcheck

	dbName := randomDatabaseName("sortit_schema_export")
	if err := createDatabase(ctx, adminDB, dbName); err != nil {
		return "", err
	}
	defer dropDatabase(ctx, adminDB, dbName)

	migratedURL, err := databaseConnectionString(baseURL, dbName)
	if err != nil {
		return "", err
	}

	store, err := issues.OpenPostgresStore(ctx, migratedURL)
	if err != nil {
		return "", fmt.Errorf("apply migrations: %w", err)
	}
	if err := store.Close(); err != nil {
		return "", fmt.Errorf("close migrated store: %w", err)
	}

	db, err := sql.Open(pgDriverName, migratedURL)
	if err != nil {
		return "", fmt.Errorf("open migrated database: %w", err)
	}
	defer db.Close() //nolint:errcheck

	s, err := loadSnapshot(ctx, db)
	if err != nil {
		return "", err
	}

	// Only keep extensions explicitly created by our migrations.
	s.Extensions = filterMigrationExtensions(s.Extensions)

	return renderSnapshot(s), nil
}

func WriteCanonicalSchema(ctx context.Context, baseURL string) error {
	schemaSQL, err := Generate(ctx, baseURL)
	if err != nil {
		return err
	}
	if err := os.WriteFile(schemaSQLPath(), []byte(schemaSQL), 0o600); err != nil {
		return fmt.Errorf("write schema.sql: %w", err)
	}
	return nil
}

func CanonicalSchemaPath() string {
	return schemaSQLPath()
}

func loadSnapshot(ctx context.Context, db *sql.DB) (snapshot, error) {
	extensions, err := loadExtensions(ctx, db)
	if err != nil {
		return snapshot{}, err
	}
	sequences, err := loadSequences(ctx, db)
	if err != nil {
		return snapshot{}, err
	}
	tables, err := loadTables(ctx, db)
	if err != nil {
		return snapshot{}, err
	}
	constraints, err := loadConstraints(ctx, db)
	if err != nil {
		return snapshot{}, err
	}
	indexes, err := loadIndexes(ctx, db)
	if err != nil {
		return snapshot{}, err
	}

	return snapshot{
		Extensions:  extensions,
		Sequences:   sequences,
		Tables:      tables,
		Constraints: constraints,
		Indexes:     indexes,
	}, nil
}

func loadExtensions(ctx context.Context, db *sql.DB) ([]extension, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT ext.extname, ns.nspname
		 FROM pg_extension ext
		 JOIN pg_namespace ns ON ns.oid = ext.extnamespace
		 WHERE ext.extname <> 'plpgsql'
		 ORDER BY ext.extname`,
	)
	if err != nil {
		return nil, fmt.Errorf("query extensions: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var extensions []extension
	for rows.Next() {
		var ext extension
		if err := rows.Scan(&ext.Name, &ext.Schema); err != nil {
			return nil, fmt.Errorf("scan extension: %w", err)
		}
		extensions = append(extensions, ext)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate extensions: %w", err)
	}
	return extensions, nil
}

func loadSequences(ctx context.Context, db *sql.DB) ([]sequence, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT seq.relname,
		        COALESCE(tbl.relname, ''),
		        COALESCE(att.attname, '')
		 FROM pg_class seq
		 JOIN pg_namespace seq_ns ON seq_ns.oid = seq.relnamespace
		 LEFT JOIN pg_depend dep
		   ON dep.classid = 'pg_class'::regclass
		  AND dep.objid = seq.oid
		  AND dep.deptype IN ('a', 'i')
		 LEFT JOIN pg_class tbl ON tbl.oid = dep.refobjid
		 LEFT JOIN pg_attribute att
		   ON att.attrelid = tbl.oid
		  AND att.attnum = dep.refobjsubid
		 WHERE seq_ns.nspname = 'public'
		   AND seq.relkind = 'S'
		   AND NOT EXISTS (
		     SELECT 1 FROM pg_depend d
		     WHERE d.objid = seq.oid
		       AND d.deptype = 'e'
		   )
		 ORDER BY seq.relname`,
	)
	if err != nil {
		return nil, fmt.Errorf("query sequences: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var sequences []sequence
	for rows.Next() {
		var seq sequence
		if err := rows.Scan(&seq.Name, &seq.OwnedTable, &seq.OwnedColumn); err != nil {
			return nil, fmt.Errorf("scan sequence: %w", err)
		}
		sequences = append(sequences, seq)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sequences: %w", err)
	}
	return sequences, nil
}

func loadTables(ctx context.Context, db *sql.DB) ([]table, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT cls.relname,
		        att.attname,
		        att.attnum,
		        pg_catalog.format_type(att.atttypid, att.atttypmod),
		        att.attnotnull,
		        COALESCE(pg_get_expr(def.adbin, def.adrelid, true), '')
		 FROM pg_attribute att
		 JOIN pg_class cls ON cls.oid = att.attrelid
		 JOIN pg_namespace ns ON ns.oid = cls.relnamespace
		 LEFT JOIN pg_attrdef def
		   ON def.adrelid = att.attrelid
		  AND def.adnum = att.attnum
		 WHERE ns.nspname = 'public'
		   AND cls.relkind = 'r'
		   AND cls.relname <> 'schema_migrations'
		   AND NOT EXISTS (
		     SELECT 1 FROM pg_depend d
		     WHERE d.objid = cls.oid
		       AND d.deptype = 'e'
		   )
		   AND att.attnum > 0
		   AND NOT att.attisdropped
		 ORDER BY cls.relname, att.attnum`,
	)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var (
		tables      []table
		current     *table
		currentName string
	)
	for rows.Next() {
		var (
			tableName string
			col       column
			position  int
		)
		if err := rows.Scan(&tableName, &col.Name, &position, &col.Type, &col.NotNull, &col.Default); err != nil {
			return nil, fmt.Errorf("scan table column: %w", err)
		}
		col.Default = normalizeSQL(col.Default)

		if current == nil || currentName != tableName {
			tables = append(tables, table{Name: tableName})
			current = &tables[len(tables)-1]
			currentName = tableName
		}
		current.Columns = append(current.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table columns: %w", err)
	}
	return tables, nil
}

func loadConstraints(ctx context.Context, db *sql.DB) ([]constraint, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT cls.relname, con.conname, pg_get_constraintdef(con.oid, true)
		 FROM pg_constraint con
		 JOIN pg_class cls ON cls.oid = con.conrelid
		 JOIN pg_namespace ns ON ns.oid = cls.relnamespace
		 WHERE ns.nspname = 'public'
		   AND cls.relname <> 'schema_migrations'
		   AND con.contype <> 'n'
		   AND NOT EXISTS (
		     SELECT 1 FROM pg_depend d
		     WHERE d.objid = cls.oid
		       AND d.deptype = 'e'
		   )
		 ORDER BY cls.relname, con.conname`,
	)
	if err != nil {
		return nil, fmt.Errorf("query constraints: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var constraints []constraint
	for rows.Next() {
		var c constraint
		if err := rows.Scan(&c.TableName, &c.Name, &c.Definition); err != nil {
			return nil, fmt.Errorf("scan constraint: %w", err)
		}
		c.Definition = normalizeSQL(c.Definition)
		constraints = append(constraints, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate constraints: %w", err)
	}
	return constraints, nil
}

func loadIndexes(ctx context.Context, db *sql.DB) ([]index, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT pg_get_indexdef(idx.oid)
		 FROM pg_class idx
		 JOIN pg_index idx_meta ON idx_meta.indexrelid = idx.oid
		 JOIN pg_class tbl ON tbl.oid = idx_meta.indrelid
		 JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
		 LEFT JOIN pg_constraint con ON con.conindid = idx.oid
		 WHERE ns.nspname = 'public'
		   AND tbl.relname <> 'schema_migrations'
		   AND NOT EXISTS (
		     SELECT 1 FROM pg_depend d
		     WHERE d.objid = tbl.oid
		       AND d.deptype = 'e'
		   )
		   AND con.oid IS NULL
		 ORDER BY tbl.relname, idx.relname`,
	)
	if err != nil {
		return nil, fmt.Errorf("query indexes: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var indexes []index
	for rows.Next() {
		var idx index
		if err := rows.Scan(&idx.Definition); err != nil {
			return nil, fmt.Errorf("scan index: %w", err)
		}
		idx.Definition = normalizeSQL(idx.Definition)
		indexes = append(indexes, idx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexes: %w", err)
	}
	return indexes, nil
}

func renderSnapshot(s snapshot) string {
	var out bytes.Buffer

	writeSection(&out, renderExtensions(s.Extensions))
	writeSection(&out, renderSequences(s.Sequences))
	writeSection(&out, renderTables(s.Tables))
	writeSection(&out, renderSequenceOwnerships(s.Sequences))
	writeSection(&out, renderDefaults(s.Tables))
	writeSection(&out, renderConstraints(s.Constraints))
	writeSection(&out, renderIndexes(s.Indexes))

	return strings.TrimSpace(out.String()) + "\n"
}

func renderExtensions(extensions []extension) string {
	var out bytes.Buffer
	for _, ext := range extensions {
		fmt.Fprintf(&out, "CREATE EXTENSION IF NOT EXISTS %s WITH SCHEMA %s;\n", quoteIdent(ext.Name), quoteIdent(ext.Schema))
	}
	return out.String()
}

func renderSequences(sequences []sequence) string {
	var out bytes.Buffer
	for _, seq := range sequences {
		fmt.Fprintf(&out, "CREATE SEQUENCE %s;\n", qualifyPublic(seq.Name))
	}
	return out.String()
}

func renderTables(tables []table) string {
	var out bytes.Buffer
	for _, table := range tables {
		fmt.Fprintf(&out, "CREATE TABLE %s (\n", qualifyPublic(table.Name))
		for i, col := range table.Columns {
			fmt.Fprintf(&out, "    %s %s", quoteIdent(col.Name), col.Type)
			if col.NotNull {
				out.WriteString(" NOT NULL")
			}
			if i < len(table.Columns)-1 {
				out.WriteString(",")
			}
			out.WriteString("\n")
		}
		out.WriteString(");\n")
	}
	return out.String()
}

func renderSequenceOwnerships(sequences []sequence) string {
	var out bytes.Buffer
	for _, seq := range sequences {
		if seq.OwnedTable == "" || seq.OwnedColumn == "" {
			continue
		}
		fmt.Fprintf(
			&out,
			"ALTER SEQUENCE %s OWNED BY %s.%s;\n",
			qualifyPublic(seq.Name),
			qualifyPublic(seq.OwnedTable),
			quoteIdent(seq.OwnedColumn),
		)
	}
	return out.String()
}

func renderDefaults(tables []table) string {
	var out bytes.Buffer
	for _, table := range tables {
		for _, col := range table.Columns {
			if col.Default == "" {
				continue
			}
			fmt.Fprintf(
				&out,
				"ALTER TABLE ONLY %s ALTER COLUMN %s SET DEFAULT %s;\n",
				qualifyPublic(table.Name),
				quoteIdent(col.Name),
				col.Default,
			)
		}
	}
	return out.String()
}

func renderConstraints(constraints []constraint) string {
	var out bytes.Buffer
	for _, c := range constraints {
		fmt.Fprintf(
			&out,
			"ALTER TABLE ONLY %s ADD CONSTRAINT %s %s;\n",
			qualifyPublic(c.TableName),
			quoteIdent(c.Name),
			c.Definition,
		)
	}
	return out.String()
}

func renderIndexes(indexes []index) string {
	var out bytes.Buffer
	for _, idx := range indexes {
		fmt.Fprintf(&out, "%s;\n", idx.Definition)
	}
	return out.String()
}

func writeSection(out *bytes.Buffer, section string) {
	section = strings.TrimSpace(section)
	if section == "" {
		return
	}
	if out.Len() > 0 {
		out.WriteString("\n\n")
	}
	out.WriteString(section)
}

func schemaSQLPath() string {
	_, currentFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(currentFile), "..", "issues", "sqlc", "schema.sql")
}

func adminConnectionString(connString string) (string, error) {
	parsed, err := url.Parse(connString)
	if err != nil {
		return "", fmt.Errorf("parse connection string: %w", err)
	}
	parsed.Path = "/postgres"
	return parsed.String(), nil
}

func databaseConnectionString(connString string, dbName string) (string, error) {
	parsed, err := url.Parse(connString)
	if err != nil {
		return "", fmt.Errorf("parse connection string: %w", err)
	}
	parsed.Path = "/" + dbName
	return parsed.String(), nil
}

func createDatabase(ctx context.Context, db *sql.DB, name string) error {
	if _, err := db.ExecContext(ctx, "CREATE DATABASE "+quoteIdent(name)); err != nil {
		return fmt.Errorf("create database %q: %w", name, err)
	}
	return nil
}

func dropDatabase(ctx context.Context, db *sql.DB, name string) {
	if err := terminateConnections(ctx, db, name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: terminate connections for %s: %v\n", name, err)
	}
	if _, err := db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quoteIdent(name)); err != nil {
		fmt.Fprintf(os.Stderr, "warning: drop database %s: %v\n", name, err)
	}
}

func terminateConnections(ctx context.Context, db *sql.DB, name string) error {
	if _, err := db.ExecContext(
		ctx,
		`SELECT pg_terminate_backend(pid)
		 FROM pg_stat_activity
		 WHERE datname = $1 AND pid <> pg_backend_pid()`,
		name,
	); err != nil {
		return fmt.Errorf("terminate active connections: %w", err)
	}
	return nil
}

func normalizeSQL(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	return strings.Join(strings.Fields(input), " ")
}

func qualifyPublic(name string) string {
	return quoteIdent("public") + "." + quoteIdent(name)
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// migrationExtensions lists extensions explicitly created by our migrations.
// Pre-installed extensions (e.g. postgis, fuzzystrmatch) vary by platform and
// must be excluded so the schema is reproducible across architectures.
var migrationExtensions = map[string]bool{
	"pg_search": true,
	"vector":    true,
}

func filterMigrationExtensions(exts []extension) []extension {
	var filtered []extension
	for _, ext := range exts {
		if migrationExtensions[ext.Name] {
			filtered = append(filtered, ext)
		}
	}
	return filtered
}

func randomDatabaseName(prefix string) string {
	var randomBytes [4]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(randomBytes[:]))
}
