package store

import (
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewDB_success verifies that NewDB opens a database without error (FR-3.1.1).
func TestNewDB_success(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	// Verify internal sql.DB is accessible.
	assert.NotNil(t, db.SQLDB())
}

// TestEnsureSchema verifies that EnsureSchema creates tables, indexes, and sets user_version (FR-3.1.1).
func TestEnsureSchema(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.EnsureSchema()
	require.NoError(t, err)

	// Verify files table exists by inserting a row.
	_, err = db.SQLDB().Exec(
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"test.md", "agent1", "a summary", time.Now().UTC(), time.Now().UTC(),
	)
	require.NoError(t, err)

	// Verify file_tags table exists by inserting a row.
	_, err = db.SQLDB().Exec(
		"INSERT INTO file_tags (filepath, tag) VALUES (?, ?)",
		"test.md", "go",
	)
	require.NoError(t, err)

	// Verify embeddings table exists by inserting a row.
	_, err = db.SQLDB().Exec(
		"INSERT INTO embeddings (filepath, vector, model_id, generated_at) VALUES (?, ?, ?, ?)",
		"test.md", []byte{1, 2, 3}, "model-1", time.Now().UTC(),
	)
	require.NoError(t, err)

	// Verify idx_file_tags_tag index exists.
	var indexName string
	err = db.SQLDB().QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_file_tags_tag'",
	).Scan(&indexName)
	require.NoError(t, err)
	assert.Equal(t, "idx_file_tags_tag", indexName)

	// Verify user_version was set.
	var version int
	err = db.SQLDB().QueryRow("PRAGMA user_version").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, version)
}

// TestEnsureSchema_idempotent verifies that calling EnsureSchema twice succeeds (FR-3.1.1).
func TestEnsureSchema_idempotent(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.EnsureSchema()
	require.NoError(t, err)

	err = db.EnsureSchema()
	require.NoError(t, err)
}

// TestCheckVersion_match verifies no error when versions match (NFR-6.1).
func TestCheckVersion_match(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.EnsureSchema()
	require.NoError(t, err)

	err = db.CheckVersion(SchemaVersion)
	require.NoError(t, err)
}

// TestCheckVersion_mismatch returns SCHEMA_VERSION_MISMATCH when versions differ (NFR-6.1).
func TestCheckVersion_mismatch(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.EnsureSchema()
	require.NoError(t, err)

	err = db.CheckVersion(SchemaVersion + 99)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeSchemaVersion),
		"expected SCHEMA_VERSION_MISMATCH error code, got: %v", err)
}

// TestCheckVersion_noSchema returns mismatch when schema has not been applied (NFR-6.1).
func TestCheckVersion_noSchema(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// user_version defaults to 0 in a fresh database.
	err = db.CheckVersion(SchemaVersion)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeSchemaVersion))
}

// TestWALMode verifies WAL mode is enabled (NFR-6.2).
func TestWALMode(t *testing.T) {
	// WAL mode requires a file-based database; :memory: does not support it.
	dir := t.TempDir()
	dbPath := dir + "/test.db"

	db, err := NewDB(dbPath)
	require.NoError(t, err)
	defer db.Close()

	var journalMode string
	err = db.SQLDB().QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", journalMode)
}

// TestForeignKeyCascade verifies that deleting a file cascades to tags and embeddings (FR-3.2.4).
func TestForeignKeyCascade(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.EnsureSchema()
	require.NoError(t, err)

	now := time.Now().UTC()

	// Insert a file.
	_, err = db.SQLDB().Exec(
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"cascade.md", "agent1", "summary", now, now,
	)
	require.NoError(t, err)

	// Insert associated tags.
	_, err = db.SQLDB().Exec("INSERT INTO file_tags (filepath, tag) VALUES (?, ?)", "cascade.md", "tag1")
	require.NoError(t, err)
	_, err = db.SQLDB().Exec("INSERT INTO file_tags (filepath, tag) VALUES (?, ?)", "cascade.md", "tag2")
	require.NoError(t, err)

	// Insert associated embedding.
	_, err = db.SQLDB().Exec(
		"INSERT INTO embeddings (filepath, vector, model_id, generated_at) VALUES (?, ?, ?, ?)",
		"cascade.md", []byte{1, 2, 3}, "model-1", now,
	)
	require.NoError(t, err)

	// Delete the file.
	_, err = db.SQLDB().Exec("DELETE FROM files WHERE filepath = ?", "cascade.md")
	require.NoError(t, err)

	// Verify tags were cascaded.
	var tagCount int
	err = db.SQLDB().QueryRow("SELECT COUNT(*) FROM file_tags WHERE filepath = ?", "cascade.md").Scan(&tagCount)
	require.NoError(t, err)
	assert.Equal(t, 0, tagCount, "file_tags should be empty after cascade delete")

	// Verify embedding was cascaded.
	var embCount int
	err = db.SQLDB().QueryRow("SELECT COUNT(*) FROM embeddings WHERE filepath = ?", "cascade.md").Scan(&embCount)
	require.NoError(t, err)
	assert.Equal(t, 0, embCount, "embeddings should be empty after cascade delete")
}

// TestDropAll verifies that DropAll drops all tables and resets user_version (FR-3.4.2).
func TestDropAll(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.EnsureSchema()
	require.NoError(t, err)

	now := time.Now().UTC()

	// Insert test data.
	_, err = db.SQLDB().Exec(
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"drop.md", "agent1", "summary", now, now,
	)
	require.NoError(t, err)

	// Drop all tables.
	err = db.DropAll()
	require.NoError(t, err)

	// Verify tables no longer exist.
	var tableCount int
	err = db.SQLDB().QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('files', 'file_tags', 'embeddings')",
	).Scan(&tableCount)
	require.NoError(t, err)
	assert.Equal(t, 0, tableCount, "all tables should be dropped")

	// Verify user_version was reset to 0.
	var version int
	err = db.SQLDB().QueryRow("PRAGMA user_version").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 0, version)
}

// TestDropAll_reensureSchema verifies that EnsureSchema works after DropAll (FR-3.4.2).
func TestDropAll_reensureSchema(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	err = db.EnsureSchema()
	require.NoError(t, err)

	err = db.DropAll()
	require.NoError(t, err)

	err = db.EnsureSchema()
	require.NoError(t, err)

	// Verify tables are back.
	var tableCount int
	err = db.SQLDB().QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('files', 'file_tags', 'embeddings')",
	).Scan(&tableCount)
	require.NoError(t, err)
	assert.Equal(t, 3, tableCount)
}

// TestClose verifies that Close closes the database without error.
func TestClose(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)

	err = db.Close()
	require.NoError(t, err)
}

// TestSQLDB verifies the SQLDB accessor returns the underlying sql.DB.
func TestSQLDB(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	sqlDB := db.SQLDB()
	require.NotNil(t, sqlDB)

	// Verify we can ping through the accessor.
	err = sqlDB.Ping()
	require.NoError(t, err)
}

// TestBusyTimeout verifies the busy_timeout pragma is set (NFR-6.2).
func TestBusyTimeout(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	var timeout int
	err = db.SQLDB().QueryRow("PRAGMA busy_timeout").Scan(&timeout)
	require.NoError(t, err)
	assert.Equal(t, 5000, timeout)
}

// TestForeignKeysEnabled verifies the foreign_keys pragma is enabled.
func TestForeignKeysEnabled(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	var fk int
	err = db.SQLDB().QueryRow("PRAGMA foreign_keys").Scan(&fk)
	require.NoError(t, err)
	assert.Equal(t, 1, fk, "foreign_keys should be enabled")
}

// TestEnsureSchema_afterClose returns an error when the database is closed.
func TestEnsureSchema_afterClose(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)

	err = db.Close()
	require.NoError(t, err)

	err = db.EnsureSchema()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestCheckVersion_afterClose returns an error when the database is closed.
func TestCheckVersion_afterClose(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)

	err = db.Close()
	require.NoError(t, err)

	err = db.CheckVersion(SchemaVersion)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestDropAll_afterClose returns an error when the database is closed.
func TestDropAll_afterClose(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)

	err = db.Close()
	require.NoError(t, err)

	err = db.DropAll()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestNewDB_invalidPath verifies NewDB returns DATABASE_ERROR for an invalid database path.
func TestNewDB_invalidPath(t *testing.T) {
	// Use a path that exists but is not a valid database file (e.g., a directory).
	_, err := NewDB("/")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}
