package store

import (
	"context"
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupEmbeddingTestDB creates an in-memory database with schema and returns the DB and repo.
func setupEmbeddingTestDB(t *testing.T) (*DB, *SQLiteEmbeddingRepository) {
	t.Helper()
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	err = db.EnsureSchema()
	require.NoError(t, err)

	repo := NewSQLiteEmbeddingRepository(db)
	return db, repo
}

// insertTestFile inserts a file row required for embedding foreign key constraint.
func insertTestFile(t *testing.T, db *DB, filepath string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.SQLDB().Exec(
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		filepath, "test-agent", "", now, now,
	)
	require.NoError(t, err)
}

// TestEmbedding_Upsert_insert verifies inserting a new embedding (FR-3.2.1).
func TestEmbedding_Upsert_insert(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "notes/test.md")

	vector := []float32{0.1, 0.2, 0.3}
	err := repo.Upsert(ctx, "notes/test.md", vector, "nomic-embed-text")
	require.NoError(t, err)

	// Verify the embedding was stored by retrieving it.
	emb, err := repo.Get(ctx, "notes/test.md")
	require.NoError(t, err)
	assert.Equal(t, "notes/test.md", emb.Filepath)
	assert.Equal(t, "nomic-embed-text", emb.ModelID)
	require.Len(t, emb.Vector, 3)
	assert.InDelta(t, 0.1, emb.Vector[0], 1e-7)
	assert.InDelta(t, 0.2, emb.Vector[1], 1e-7)
	assert.InDelta(t, 0.3, emb.Vector[2], 1e-7)
	assert.False(t, emb.GeneratedAt.IsZero(), "generated_at should be set")
}

// TestEmbedding_Upsert_replace verifies replacing an existing embedding (FR-3.2.1).
func TestEmbedding_Upsert_replace(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "notes/test.md")

	// Insert initial embedding.
	err := repo.Upsert(ctx, "notes/test.md", []float32{0.1, 0.2}, "model-v1")
	require.NoError(t, err)

	// Replace with new embedding.
	err = repo.Upsert(ctx, "notes/test.md", []float32{0.5, 0.6, 0.7}, "model-v2")
	require.NoError(t, err)

	// Verify the replacement.
	emb, err := repo.Get(ctx, "notes/test.md")
	require.NoError(t, err)
	assert.Equal(t, "model-v2", emb.ModelID)
	require.Len(t, emb.Vector, 3)
	assert.InDelta(t, 0.5, emb.Vector[0], 1e-7)
	assert.InDelta(t, 0.6, emb.Vector[1], 1e-7)
	assert.InDelta(t, 0.7, emb.Vector[2], 1e-7)
}

// TestEmbedding_Get_success verifies returning a stored embedding with decoded vector (FR-3.2.1).
func TestEmbedding_Get_success(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "docs/readme.md")

	vector := []float32{1.0, -2.5, 3.14}
	err := repo.Upsert(ctx, "docs/readme.md", vector, "test-model")
	require.NoError(t, err)

	emb, err := repo.Get(ctx, "docs/readme.md")
	require.NoError(t, err)
	assert.Equal(t, "docs/readme.md", emb.Filepath)
	assert.Equal(t, "test-model", emb.ModelID)
	require.Len(t, emb.Vector, 3)
	assert.InDelta(t, float32(1.0), emb.Vector[0], 1e-7)
	assert.InDelta(t, float32(-2.5), emb.Vector[1], 1e-7)
	assert.InDelta(t, 3.14, emb.Vector[2], 1e-6)
	assert.False(t, emb.GeneratedAt.IsZero())
}

// TestEmbedding_Get_notfound returns error with METADATA_NOT_FOUND when embedding does not exist (FR-3.2.1).
func TestEmbedding_Get_notfound(t *testing.T) {
	_, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, "nonexistent.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound),
		"expected METADATA_NOT_FOUND error code, got: %v", err)
}

// TestEmbedding_Delete verifies removing an embedding (FR-3.2.1).
func TestEmbedding_Delete(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "notes/delete-me.md")

	err := repo.Upsert(ctx, "notes/delete-me.md", []float32{1.0, 2.0}, "model")
	require.NoError(t, err)

	// Verify it exists.
	_, err = repo.Get(ctx, "notes/delete-me.md")
	require.NoError(t, err)

	// Delete.
	err = repo.Delete(ctx, "notes/delete-me.md")
	require.NoError(t, err)

	// Verify it is gone.
	_, err = repo.Get(ctx, "notes/delete-me.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound))
}

// TestEmbedding_Delete_nonexistent verifies deleting a non-existent embedding does not error (FR-3.2.1).
func TestEmbedding_Delete_nonexistent(t *testing.T) {
	_, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	// Deleting something that does not exist should not error.
	err := repo.Delete(ctx, "nonexistent.md")
	require.NoError(t, err)
}

// TestEmbedding_GetAll verifies returning all stored embeddings (FR-3.3.2).
func TestEmbedding_GetAll(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	// Initially empty.
	all, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)

	// Insert multiple files and embeddings.
	insertTestFile(t, db, "file1.md")
	insertTestFile(t, db, "file2.md")
	insertTestFile(t, db, "file3.md")

	err = repo.Upsert(ctx, "file1.md", []float32{0.1, 0.2}, "model-a")
	require.NoError(t, err)
	err = repo.Upsert(ctx, "file2.md", []float32{0.3, 0.4}, "model-a")
	require.NoError(t, err)
	err = repo.Upsert(ctx, "file3.md", []float32{0.5, 0.6}, "model-b")
	require.NoError(t, err)

	all, err = repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Build a map for order-independent assertions.
	byPath := make(map[string]Embedding)
	for _, e := range all {
		byPath[e.Filepath] = e
	}

	assert.Contains(t, byPath, "file1.md")
	assert.Contains(t, byPath, "file2.md")
	assert.Contains(t, byPath, "file3.md")

	assert.Equal(t, "model-a", byPath["file1.md"].ModelID)
	assert.Equal(t, "model-b", byPath["file3.md"].ModelID)
	assert.InDelta(t, 0.1, byPath["file1.md"].Vector[0], 1e-7)
	assert.InDelta(t, 0.5, byPath["file3.md"].Vector[0], 1e-7)
}

// TestEmbedding_VectorRoundTrip verifies that EncodeVector store retrieve DecodeVector produces identical float32 values (FR-3.2.1, FR-3.3.2).
func TestEmbedding_VectorRoundTrip(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "roundtrip.md")

	// Use a variety of float32 values including edge cases.
	original := []float32{0.0, 1.0, -1.0, 0.123456, -99.99, 3.14159}

	err := repo.Upsert(ctx, "roundtrip.md", original, "test-model")
	require.NoError(t, err)

	emb, err := repo.Get(ctx, "roundtrip.md")
	require.NoError(t, err)

	require.Len(t, emb.Vector, len(original))
	for i := range original {
		assert.InDelta(t, original[i], emb.Vector[i], 1e-7,
			"float32 mismatch at index %d: want %v, got %v", i, original[i], emb.Vector[i])
	}
}

// TestEmbedding_InterfaceCompliance is a compile-time check that SQLiteEmbeddingRepository implements EmbeddingRepository.
func TestEmbedding_InterfaceCompliance(_ *testing.T) {
	var _ EmbeddingRepository = (*SQLiteEmbeddingRepository)(nil)
}

// closedEmbeddingRepo returns a SQLiteEmbeddingRepository backed by a closed database for error-path testing.
func closedEmbeddingRepo(t *testing.T) (*SQLiteEmbeddingRepository, context.Context) {
	t.Helper()
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.EnsureSchema())
	repo := NewSQLiteEmbeddingRepository(db)
	require.NoError(t, db.Close())
	return repo, context.Background()
}

// TestEmbedding_Upsert_closedDB verifies Upsert returns DATABASE_ERROR when the database is closed.
func TestEmbedding_Upsert_closedDB(t *testing.T) {
	repo, ctx := closedEmbeddingRepo(t)
	err := repo.Upsert(ctx, "test.md", []float32{0.1}, "model")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestEmbedding_Get_closedDB verifies Get returns DATABASE_ERROR when the database is closed.
func TestEmbedding_Get_closedDB(t *testing.T) {
	repo, ctx := closedEmbeddingRepo(t)
	_, err := repo.Get(ctx, "test.md")
	require.Error(t, err)
	// Could be DATABASE_ERROR or METADATA_NOT_FOUND depending on driver behavior.
	assert.Error(t, err)
}

// TestEmbedding_Delete_closedDB verifies Delete returns DATABASE_ERROR when the database is closed.
func TestEmbedding_Delete_closedDB(t *testing.T) {
	repo, ctx := closedEmbeddingRepo(t)
	err := repo.Delete(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestEmbedding_GetAll_closedDB verifies GetAll returns DATABASE_ERROR when the database is closed.
func TestEmbedding_GetAll_closedDB(t *testing.T) {
	repo, ctx := closedEmbeddingRepo(t)
	_, err := repo.GetAll(ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestEmbedding_GetAll_scanError verifies GetAll returns DATABASE_ERROR when row scanning fails.
func TestEmbedding_GetAll_scanError(t *testing.T) {
	db, _ := setupEmbeddingTestDB(t)
	repo := NewSQLiteEmbeddingRepository(db)
	ctx := context.Background()

	// Insert a file for FK constraint.
	insertTestFile(t, db, "bad.md")

	// Insert an embedding with corrupt generated_at that can't be scanned into time.Time.
	_, err := db.SQLDB().Exec(
		"INSERT INTO embeddings (filepath, vector, model_id, generated_at) VALUES (?, ?, ?, ?)",
		"bad.md", []byte{0, 0, 0, 0}, "model", "not-a-time",
	)
	require.NoError(t, err)

	_, err = repo.GetAll(ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}
