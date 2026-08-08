package issues

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/viant/sqlite-vec/vec"
	_ "modernc.org/sqlite"
)

const sqliteDriverName = "sqlite"

var sqliteMemoryDatabaseSequence uint64

//go:embed sqlitemigrations/*.up.sql
var sqliteMigrationsFS embed.FS

// SQLiteDatabase owns a locally persisted SQLite database and its migration
// lifecycle. It is intentionally separate from PostgresStore while the SQLite
// issue-store implementation is built out; callers can use it to establish a
// correctly configured, forward-migrated local database.
//
// SQLite supports many concurrent readers but only one writer. Application
// work uses one connection; the pure-Go vector virtual table opens its own
// handle for durable-index reads. WAL still lets readers run while a writer is
// active.
type SQLiteDatabase struct {
	db               *sql.DB
	vectorDataSource string
}

// OpenSQLiteDatabase opens a SQLite file (or SQLite URI), enables the
// invariants the application relies on, and applies all embedded migrations.
// The special :memory: value is useful for tests.
func OpenSQLiteDatabase(ctx context.Context, dataSource string) (*SQLiteDatabase, error) {
	dataSource = strings.TrimSpace(dataSource)
	if dataSource == "" {
		return nil, fmt.Errorf("sqlite data source is required")
	}

	resolvedDataSource := sqliteDataSource(dataSource)
	db, err := sql.Open(sqliteDriverName, resolvedDataSource)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	// Registering the virtual-table module and creating its schema must happen
	// on the same connection. The virtual table opens its own handle for index
	// work, so application writes remain serialized through this pool.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := vec.Register(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("register SQLite vector index: %w", err)
	}

	store := &SQLiteDatabase{db: db, vectorDataSource: resolvedDataSource}
	if err := store.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (d *SQLiteDatabase) Close() error { return d.db.Close() }

// DB exposes the connection for the SQLite store implementation and its
// focused integration tests. Application code should prefer store methods.
func (d *SQLiteDatabase) DB() *sql.DB { return d.db }

func (d *SQLiteDatabase) initialize(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}
	if _, err := d.db.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("enable sqlite WAL mode: %w", err)
	}
	if _, err := d.db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("set sqlite busy timeout: %w", err)
	}
	if err := d.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite database: %w", err)
	}
	if err := d.runMigrations(ctx); err != nil {
		return err
	}
	return d.ensureVectorVirtualTable(ctx)
}

func (d *SQLiteDatabase) ensureVectorVirtualTable(ctx context.Context) error {
	var exists bool
	if err := d.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'issue_embeddings'
	)`).Scan(&exists); err != nil {
		return fmt.Errorf("check SQLite vector table: %w", err)
	}
	if exists {
		return nil
	}
	// sqlite-vec keeps connection-scoped state while constructing the virtual
	// table, so create it outside the migration transaction.
	vectorDataSource := strings.ReplaceAll(d.vectorDataSource, "'", "''")
	statement := fmt.Sprintf("CREATE VIRTUAL TABLE issue_embeddings USING vec(issue_id, dbpath='%s')", vectorDataSource)
	if _, err := d.db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("create SQLite vector table: %w", err)
	}
	return nil
}

func (d *SQLiteDatabase) runMigrations(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at_unix_nano INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create sqlite migration table: %w", err)
	}

	migrations, err := readSQLiteMigrations()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		var applied bool
		if err := d.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", migration.version).Scan(&applied); err != nil {
			return fmt.Errorf("check sqlite migration %d: %w", migration.version, err)
		}
		if applied {
			continue
		}

		tx, err := d.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin sqlite migration %d: %w", migration.version, err)
		}
		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply sqlite migration %d: %w", migration.version, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, applied_at_unix_nano) VALUES (?, ?)", migration.version, time.Now().UTC().UnixNano()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record sqlite migration %d: %w", migration.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit sqlite migration %d: %w", migration.version, err)
		}
	}
	return nil
}

type sqliteMigration struct {
	version int
	name    string
	sql     string
}

func readSQLiteMigrations() ([]sqliteMigration, error) {
	entries, err := fs.ReadDir(sqliteMigrationsFS, "sqlitemigrations")
	if err != nil {
		return nil, fmt.Errorf("read sqlite migrations: %w", err)
	}

	migrations := make([]sqliteMigration, 0, len(entries))
	seen := make(map[int]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("invalid sqlite migration name %q", entry.Name())
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid sqlite migration version in %q", entry.Name())
		}
		if _, duplicate := seen[version]; duplicate {
			return nil, fmt.Errorf("duplicate sqlite migration version %d", version)
		}
		seen[version] = struct{}{}

		name := path.Join("sqlitemigrations", entry.Name())
		body, err := sqliteMigrationsFS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read sqlite migration %q: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("sqlite migration %q is empty", entry.Name())
		}
		migrations = append(migrations, sqliteMigration{version: version, name: entry.Name(), sql: string(body)})
	}
	if len(migrations) == 0 {
		return nil, errors.New("no sqlite migrations found")
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}

func sqliteDataSource(dataSource string) string {
	if dataSource == ":memory:" {
		// Multiple connections need to see the same in-memory database. Give
		// each OpenSQLiteDatabase call its own named shared-cache database so
		// independent tests and callers remain isolated.
		return fmt.Sprintf("file:sortit-%d?mode=memory&cache=shared", atomic.AddUint64(&sqliteMemoryDatabaseSequence, 1))
	}
	if strings.HasPrefix(dataSource, "file:") {
		return dataSource
	}
	return dataSource
}
