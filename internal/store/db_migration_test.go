package store

import (
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// v1 schema DDL used to set up a v1 database for migration tests.
const v1CreateEmbeddingsTable = `CREATE TABLE IF NOT EXISTS embeddings (
    filepath TEXT PRIMARY KEY REFERENCES files(filepath) ON DELETE CASCADE,
    vector BLOB NOT NULL,
    model_id TEXT NOT NULL,
    generated_at DATETIME NOT NULL
)`

// setupV1DB creates an in-memory database with the v1 schema.
func setupV1DB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Create files table (same in v1 and v2).
	_, err = db.SQLDB().Exec(createFilesTable)
	require.NoError(t, err)
	_, err = db.SQLDB().Exec(createFileTagsTable)
	require.NoError(t, err)
	_, err = db.SQLDB().Exec(createFileTagsIndex)
	require.NoError(t, err)

	// Create v1 embeddings table.
	_, err = db.SQLDB().Exec(v1CreateEmbeddingsTable)
	require.NoError(t, err)

	// Set user_version to 1.
	_, err = db.SQLDB().Exec("PRAGMA user_version = 1")
	require.NoError(t, err)

	return db
}

// TestMigrateV1ToV2 verifies that v1 schema is migrated to v2 with composite PK (FR-10).
func TestMigrateV1ToV2(t *testing.T) {
	db := setupV1DB(t)

	// Verify we start at v1.
	ver, err := db.currentVersion()
	require.NoError(t, err)
	assert.Equal(t, 1, ver)

	// Run migration.
	err = db.MigrateIfNeeded()
	require.NoError(t, err)

	// Verify version is now 2.
	ver, err = db.currentVersion()
	require.NoError(t, err)
	assert.Equal(t, 2, ver)

	// Verify composite PK by inserting two rows for the same filepath with different chunk_index values.
	now := time.Now().UTC()
	_, err = db.SQLDB().Exec(
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"test.md", "agent", "", now, now,
	)
	require.NoError(t, err)

	_, err = db.SQLDB().Exec(
		`INSERT INTO embeddings (filepath, chunk_index, vector, model_id, generated_at, chunk_start, chunk_end, is_summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"test.md", 0, []byte{1, 2, 3, 4}, "model", now, 0, 100, false,
	)
	require.NoError(t, err)

	_, err = db.SQLDB().Exec(
		`INSERT INTO embeddings (filepath, chunk_index, vector, model_id, generated_at, chunk_start, chunk_end, is_summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"test.md", 1, []byte{5, 6, 7, 8}, "model", now, 100, 200, false,
	)
	require.NoError(t, err)

	// Verify both rows exist.
	var count int
	err = db.SQLDB().QueryRow("SELECT COUNT(*) FROM embeddings WHERE filepath = ?", "test.md").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "should have 2 chunks for same filepath")
}

// TestMigrateV1ToV2_PreservesData verifies that existing v1 embeddings survive migration with correct defaults (FR-10).
func TestMigrateV1ToV2_PreservesData(t *testing.T) {
	db := setupV1DB(t)

	// Insert test data in v1 format.
	now := time.Now().UTC()
	_, err := db.SQLDB().Exec(
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"existing.md", "agent", "summary", now, now,
	)
	require.NoError(t, err)

	vectorBlob := EncodeVector([]float32{0.1, 0.2, 0.3})
	_, err = db.SQLDB().Exec(
		"INSERT INTO embeddings (filepath, vector, model_id, generated_at) VALUES (?, ?, ?, ?)",
		"existing.md", vectorBlob, "old-model", now,
	)
	require.NoError(t, err)

	// Run migration.
	err = db.MigrateIfNeeded()
	require.NoError(t, err)

	// Verify the embedding was preserved with correct defaults.
	var (
		filepath   string
		chunkIndex int
		blob       []byte
		modelID    string
		chunkStart int
		chunkEnd   int
		isSummary  bool
	)
	err = db.SQLDB().QueryRow(
		`SELECT filepath, chunk_index, vector, model_id, chunk_start, chunk_end, is_summary
		 FROM embeddings WHERE filepath = ?`,
		"existing.md",
	).Scan(&filepath, &chunkIndex, &blob, &modelID, &chunkStart, &chunkEnd, &isSummary)
	require.NoError(t, err)

	assert.Equal(t, "existing.md", filepath)
	assert.Equal(t, 0, chunkIndex, "migrated row should have chunk_index=0")
	assert.Equal(t, "old-model", modelID)
	assert.Equal(t, 0, chunkStart, "migrated row should have chunk_start=0")
	assert.Equal(t, 0, chunkEnd, "migrated row should have chunk_end=0")
	assert.False(t, isSummary, "migrated row should have is_summary=false")

	// Verify vector data survived.
	vector := DecodeVector(blob)
	require.Len(t, vector, 3)
	assert.InDelta(t, 0.1, vector[0], 1e-7)
	assert.InDelta(t, 0.2, vector[1], 1e-7)
	assert.InDelta(t, 0.3, vector[2], 1e-7)
}

// TestMigrateIfNeeded_AlreadyV2 verifies no-op when already at v2 (FR-10).
func TestMigrateIfNeeded_AlreadyV2(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Create v2 schema directly.
	err = db.EnsureSchema()
	require.NoError(t, err)

	// Verify version is 2.
	ver, err := db.currentVersion()
	require.NoError(t, err)
	assert.Equal(t, 2, ver)

	// Insert some data.
	now := time.Now().UTC()
	_, err = db.SQLDB().Exec(
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"test.md", "agent", "", now, now,
	)
	require.NoError(t, err)

	_, err = db.SQLDB().Exec(
		`INSERT INTO embeddings (filepath, chunk_index, vector, model_id, generated_at) VALUES (?, ?, ?, ?, ?)`,
		"test.md", 0, []byte{1, 2, 3, 4}, "model", now,
	)
	require.NoError(t, err)

	// MigrateIfNeeded should be a no-op.
	err = db.MigrateIfNeeded()
	require.NoError(t, err)

	// Verify data is still there.
	var count int
	err = db.SQLDB().QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// TestMigrateIfNeeded_FreshDB verifies that a fresh DB creates v2 schema directly (FR-10).
func TestMigrateIfNeeded_FreshDB(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Fresh DB: user_version defaults to 0.
	ver, err := db.currentVersion()
	require.NoError(t, err)
	assert.Equal(t, 0, ver)

	// MigrateIfNeeded should create v2 schema from scratch.
	err = db.MigrateIfNeeded()
	require.NoError(t, err)

	// Verify version is 2.
	ver, err = db.currentVersion()
	require.NoError(t, err)
	assert.Equal(t, 2, ver)

	// Verify v2 schema features: composite PK exists.
	now := time.Now().UTC()
	_, err = db.SQLDB().Exec(
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"test.md", "agent", "", now, now,
	)
	require.NoError(t, err)

	// Insert two chunks for the same file.
	_, err = db.SQLDB().Exec(
		`INSERT INTO embeddings (filepath, chunk_index, vector, model_id, generated_at) VALUES (?, ?, ?, ?, ?)`,
		"test.md", 0, []byte{1, 2, 3, 4}, "model", now,
	)
	require.NoError(t, err)

	_, err = db.SQLDB().Exec(
		`INSERT INTO embeddings (filepath, chunk_index, vector, model_id, generated_at) VALUES (?, ?, ?, ?, ?)`,
		"test.md", 1, []byte{5, 6, 7, 8}, "model", now,
	)
	require.NoError(t, err)

	var count int
	err = db.SQLDB().QueryRow("SELECT COUNT(*) FROM embeddings WHERE filepath = ?", "test.md").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// TestMigrateIfNeeded_UnknownVersion verifies error for unknown schema version.
func TestMigrateIfNeeded_UnknownVersion(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Set an unknown version.
	_, err = db.SQLDB().Exec("PRAGMA user_version = 99")
	require.NoError(t, err)

	err = db.MigrateIfNeeded()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError),
		"expected DATABASE_ERROR, got: %v", err)
}

// TestMigrateIfNeeded_ClosedDB verifies error when database is closed.
func TestMigrateIfNeeded_ClosedDB(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)

	err = db.Close()
	require.NoError(t, err)

	err = db.MigrateIfNeeded()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestCurrentVersion verifies reading the PRAGMA user_version.
func TestCurrentVersion(t *testing.T) {
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Fresh DB should be version 0.
	ver, err := db.currentVersion()
	require.NoError(t, err)
	assert.Equal(t, 0, ver)

	// After EnsureSchema, should be SchemaVersion.
	err = db.EnsureSchema()
	require.NoError(t, err)

	ver, err = db.currentVersion()
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, ver)
}

// TestMigrateV1ToV2_MultipleEmbeddings verifies migration of multiple v1 embeddings.
func TestMigrateV1ToV2_MultipleEmbeddings(t *testing.T) {
	db := setupV1DB(t)

	now := time.Now().UTC()

	// Insert multiple files with v1 embeddings.
	for _, fp := range []string{"file1.md", "file2.md", "file3.md"} {
		_, err := db.SQLDB().Exec(
			"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			fp, "agent", "", now, now,
		)
		require.NoError(t, err)

		_, err = db.SQLDB().Exec(
			"INSERT INTO embeddings (filepath, vector, model_id, generated_at) VALUES (?, ?, ?, ?)",
			fp, EncodeVector([]float32{0.1}), "model", now,
		)
		require.NoError(t, err)
	}

	// Run migration.
	err := db.MigrateIfNeeded()
	require.NoError(t, err)

	// Verify all 3 embeddings survived.
	var count int
	err = db.SQLDB().QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Verify each has chunk_index=0.
	rows, err := db.SQLDB().Query("SELECT filepath, chunk_index FROM embeddings ORDER BY filepath")
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var fp string
		var ci int
		err = rows.Scan(&fp, &ci)
		require.NoError(t, err)
		assert.Equal(t, 0, ci, "migrated embedding for %s should have chunk_index=0", fp)
	}
	require.NoError(t, rows.Err())
}
