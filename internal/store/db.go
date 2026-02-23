package store

import (
	"database/sql"
	"fmt"

	"github.com/awood45/grimoire-cli/internal/sberrors"

	// Register the pure-Go SQLite driver.
	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB connection to a SQLite database with schema management.
type DB struct {
	db   *sql.DB
	path string
}

// NewDB opens a SQLite database at path with WAL mode, busy timeout, and foreign keys enabled.
func NewDB(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to open database")
	}

	if err := configureDB(sqlDB); err != nil {
		sqlDB.Close()
		return nil, err
	}

	return &DB{db: sqlDB, path: path}, nil
}

// configureDB sets PRAGMA options and connection limits on an open *sql.DB.
func configureDB(sqlDB *sql.DB) error {
	// Enable WAL mode for concurrent reads.
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to set WAL mode")
	}

	// Set 5-second busy timeout for write contention.
	if _, err := sqlDB.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to set busy timeout")
	}

	// Enable foreign key enforcement.
	if _, err := sqlDB.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to enable foreign keys")
	}

	// Single connection — SQLite does not benefit from connection pooling.
	sqlDB.SetMaxOpenConns(1)

	return nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	if err := d.db.Close(); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to close database")
	}
	return nil
}

// EnsureSchema creates all tables and indexes, then sets the schema version.
func (d *DB) EnsureSchema() error {
	statements := []string{
		createFilesTable,
		createFileTagsTable,
		createFileTagsIndex,
		createEmbeddingsTable,
		fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion),
	}

	for _, stmt := range statements {
		if _, err := d.db.Exec(stmt); err != nil {
			return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to execute schema statement")
		}
	}

	return nil
}

// CheckVersion reads the database schema version and returns an error if it does not match expected.
func (d *DB) CheckVersion(expected int) error {
	var version int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to read schema version")
	}

	if version != expected {
		return sberrors.Newf(sberrors.ErrCodeSchemaVersion,
			"schema version mismatch: database has %d, expected %d", version, expected)
	}

	return nil
}

// DropAll drops all application tables and resets the schema version to 0.
func (d *DB) DropAll() error {
	statements := []string{
		"DROP TABLE IF EXISTS file_tags",
		"DROP TABLE IF EXISTS embeddings",
		"DROP TABLE IF EXISTS files",
		"PRAGMA user_version = 0",
	}

	for _, stmt := range statements {
		if _, err := d.db.Exec(stmt); err != nil {
			return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to drop tables")
		}
	}

	return nil
}

// SQLDB returns the underlying *sql.DB for direct access by repository implementations.
func (d *DB) SQLDB() *sql.DB {
	return d.db
}
