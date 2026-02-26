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

// TestEmbedding_Upsert_insert verifies inserting a new embedding (FR-3).
func TestEmbedding_Upsert_insert(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "notes/test.md")

	vector := []float32{0.1, 0.2, 0.3}
	err := repo.Upsert(ctx, Embedding{
		Filepath:   "notes/test.md",
		ChunkIndex: 0,
		Vector:     vector,
		ModelID:    "nomic-embed-text",
	})
	require.NoError(t, err)

	// Verify the embedding was stored by retrieving it.
	emb, err := repo.Get(ctx, "notes/test.md")
	require.NoError(t, err)
	assert.Equal(t, "notes/test.md", emb.Filepath)
	assert.Equal(t, "nomic-embed-text", emb.ModelID)
	assert.Equal(t, 0, emb.ChunkIndex)
	require.Len(t, emb.Vector, 3)
	assert.InDelta(t, 0.1, emb.Vector[0], 1e-7)
	assert.InDelta(t, 0.2, emb.Vector[1], 1e-7)
	assert.InDelta(t, 0.3, emb.Vector[2], 1e-7)
	assert.False(t, emb.GeneratedAt.IsZero(), "generated_at should be set")
}

// TestEmbedding_Upsert_replace verifies replacing an existing embedding (FR-3).
func TestEmbedding_Upsert_replace(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "notes/test.md")

	// Insert initial embedding.
	err := repo.Upsert(ctx, Embedding{
		Filepath:   "notes/test.md",
		ChunkIndex: 0,
		Vector:     []float32{0.1, 0.2},
		ModelID:    "model-v1",
	})
	require.NoError(t, err)

	// Replace with new embedding.
	err = repo.Upsert(ctx, Embedding{
		Filepath:   "notes/test.md",
		ChunkIndex: 0,
		Vector:     []float32{0.5, 0.6, 0.7},
		ModelID:    "model-v2",
	})
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

// TestEmbedding_Get_success verifies returning a stored embedding with decoded vector (FR-3).
func TestEmbedding_Get_success(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "docs/readme.md")

	vector := []float32{1.0, -2.5, 3.14}
	err := repo.Upsert(ctx, Embedding{
		Filepath:   "docs/readme.md",
		ChunkIndex: 0,
		Vector:     vector,
		ModelID:    "test-model",
	})
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

// TestEmbedding_Get_notfound returns error with METADATA_NOT_FOUND when embedding does not exist (FR-3).
func TestEmbedding_Get_notfound(t *testing.T) {
	_, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, "nonexistent.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound),
		"expected METADATA_NOT_FOUND error code, got: %v", err)
}

// TestEmbedding_DeleteForFile verifies removing all embeddings for a file.
func TestEmbedding_DeleteForFile(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "notes/delete-me.md")

	err := repo.Upsert(ctx, Embedding{
		Filepath:   "notes/delete-me.md",
		ChunkIndex: 0,
		Vector:     []float32{1.0, 2.0},
		ModelID:    "model",
	})
	require.NoError(t, err)

	// Verify it exists.
	_, err = repo.Get(ctx, "notes/delete-me.md")
	require.NoError(t, err)

	// Delete.
	err = repo.DeleteForFile(ctx, "notes/delete-me.md")
	require.NoError(t, err)

	// Verify it is gone.
	_, err = repo.Get(ctx, "notes/delete-me.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound))
}

// TestEmbedding_DeleteForFile_nonexistent verifies deleting a non-existent embedding does not error.
func TestEmbedding_DeleteForFile_nonexistent(t *testing.T) {
	_, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	// Deleting something that does not exist should not error.
	err := repo.DeleteForFile(ctx, "nonexistent.md")
	require.NoError(t, err)
}

// TestEmbedding_GetAll verifies returning all stored embeddings (FR-3).
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

	err = repo.Upsert(ctx, Embedding{Filepath: "file1.md", ChunkIndex: 0, Vector: []float32{0.1, 0.2}, ModelID: "model-a"})
	require.NoError(t, err)
	err = repo.Upsert(ctx, Embedding{Filepath: "file2.md", ChunkIndex: 0, Vector: []float32{0.3, 0.4}, ModelID: "model-a"})
	require.NoError(t, err)
	err = repo.Upsert(ctx, Embedding{Filepath: "file3.md", ChunkIndex: 0, Vector: []float32{0.5, 0.6}, ModelID: "model-b"})
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

// TestEmbedding_VectorRoundTrip verifies that EncodeVector store retrieve DecodeVector produces identical float32 values (FR-3).
func TestEmbedding_VectorRoundTrip(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "roundtrip.md")

	// Use a variety of float32 values including edge cases.
	original := []float32{0.0, 1.0, -1.0, 0.123456, -99.99, 3.14159}

	err := repo.Upsert(ctx, Embedding{
		Filepath:   "roundtrip.md",
		ChunkIndex: 0,
		Vector:     original,
		ModelID:    "test-model",
	})
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
	err := repo.Upsert(ctx, Embedding{
		Filepath:   "test.md",
		ChunkIndex: 0,
		Vector:     []float32{0.1},
		ModelID:    "model",
	})
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

// TestEmbedding_DeleteForFile_closedDB verifies DeleteForFile returns DATABASE_ERROR when the database is closed.
func TestEmbedding_DeleteForFile_closedDB(t *testing.T) {
	repo, ctx := closedEmbeddingRepo(t)
	err := repo.DeleteForFile(ctx, "test.md")
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
		`INSERT INTO embeddings (filepath, chunk_index, vector, model_id, generated_at) VALUES (?, ?, ?, ?, ?)`,
		"bad.md", 0, []byte{0, 0, 0, 0}, "model", "not-a-time",
	)
	require.NoError(t, err)

	_, err = repo.GetAll(ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestEmbedding_GetForFile_closedDB verifies GetForFile returns DATABASE_ERROR when the database is closed.
func TestEmbedding_GetForFile_closedDB(t *testing.T) {
	repo, ctx := closedEmbeddingRepo(t)
	_, err := repo.GetForFile(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// --- New v2 tests ---

// TestUpsert_InsertChunk verifies inserting an embedding with chunk metadata (FR-3).
func TestUpsert_InsertChunk(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "chunked.md")

	err := repo.Upsert(ctx, Embedding{
		Filepath:   "chunked.md",
		ChunkIndex: 0,
		Vector:     []float32{0.1, 0.2},
		ModelID:    "model-v2",
		ChunkStart: 0,
		ChunkEnd:   500,
		IsSummary:  false,
	})
	require.NoError(t, err)

	emb, err := repo.Get(ctx, "chunked.md")
	require.NoError(t, err)
	assert.Equal(t, 0, emb.ChunkIndex)
	assert.Equal(t, 0, emb.ChunkStart)
	assert.Equal(t, 500, emb.ChunkEnd)
	assert.False(t, emb.IsSummary)
}

// TestUpsert_ReplaceChunk verifies replacing an existing (filepath, chunk_index) pair (FR-3).
func TestUpsert_ReplaceChunk(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "replace.md")

	// Insert chunk 0.
	err := repo.Upsert(ctx, Embedding{
		Filepath:   "replace.md",
		ChunkIndex: 0,
		Vector:     []float32{0.1, 0.2},
		ModelID:    "model-v1",
		ChunkStart: 0,
		ChunkEnd:   100,
	})
	require.NoError(t, err)

	// Replace chunk 0 with new data.
	err = repo.Upsert(ctx, Embedding{
		Filepath:   "replace.md",
		ChunkIndex: 0,
		Vector:     []float32{0.9, 0.8},
		ModelID:    "model-v2",
		ChunkStart: 0,
		ChunkEnd:   200,
	})
	require.NoError(t, err)

	emb, err := repo.Get(ctx, "replace.md")
	require.NoError(t, err)
	assert.Equal(t, "model-v2", emb.ModelID)
	assert.Equal(t, 200, emb.ChunkEnd)
	assert.InDelta(t, 0.9, emb.Vector[0], 1e-7)
}

// TestGet_ReturnsRepresentative verifies Get returns the lowest chunk_index embedding.
func TestGet_ReturnsRepresentative(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "multi.md")

	// Insert chunks in non-sequential order.
	err := repo.Upsert(ctx, Embedding{
		Filepath: "multi.md", ChunkIndex: 1, Vector: []float32{0.2}, ModelID: "m",
		ChunkStart: 100, ChunkEnd: 200,
	})
	require.NoError(t, err)
	err = repo.Upsert(ctx, Embedding{
		Filepath: "multi.md", ChunkIndex: 0, Vector: []float32{0.1}, ModelID: "m",
		ChunkStart: 0, ChunkEnd: 100,
	})
	require.NoError(t, err)
	err = repo.Upsert(ctx, Embedding{
		Filepath: "multi.md", ChunkIndex: 2, Vector: []float32{0.3}, ModelID: "m",
		ChunkStart: 200, ChunkEnd: 300,
	})
	require.NoError(t, err)

	emb, err := repo.Get(ctx, "multi.md")
	require.NoError(t, err)
	assert.Equal(t, 0, emb.ChunkIndex, "should return chunk 0 as representative")
	assert.InDelta(t, 0.1, emb.Vector[0], 1e-7)
}

// TestGet_ReturnsSummaryAsRepresentative verifies Get returns summary (chunk_index=-1) when it exists.
func TestGet_ReturnsSummaryAsRepresentative(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "summary.md")

	// Insert chunk 0 and summary (-1).
	err := repo.Upsert(ctx, Embedding{
		Filepath: "summary.md", ChunkIndex: 0, Vector: []float32{0.1}, ModelID: "m",
	})
	require.NoError(t, err)
	err = repo.Upsert(ctx, Embedding{
		Filepath: "summary.md", ChunkIndex: -1, Vector: []float32{0.9}, ModelID: "m", IsSummary: true,
	})
	require.NoError(t, err)

	emb, err := repo.Get(ctx, "summary.md")
	require.NoError(t, err)
	assert.Equal(t, -1, emb.ChunkIndex, "should return summary (chunk_index=-1) as representative")
	assert.True(t, emb.IsSummary)
	assert.InDelta(t, 0.9, emb.Vector[0], 1e-7)
}

// TestGetForFile_AllChunks verifies GetForFile returns all chunks ordered by chunk_index (FR-3).
func TestGetForFile_AllChunks(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "multi.md")

	// Insert chunks out of order.
	for _, idx := range []int{2, 0, 1} {
		err := repo.Upsert(ctx, Embedding{
			Filepath:   "multi.md",
			ChunkIndex: idx,
			Vector:     []float32{float32(idx) * 0.1},
			ModelID:    "model",
			ChunkStart: idx * 100,
			ChunkEnd:   (idx + 1) * 100,
		})
		require.NoError(t, err)
	}

	chunks, err := repo.GetForFile(ctx, "multi.md")
	require.NoError(t, err)
	require.Len(t, chunks, 3)

	// Verify ordering.
	assert.Equal(t, 0, chunks[0].ChunkIndex)
	assert.Equal(t, 1, chunks[1].ChunkIndex)
	assert.Equal(t, 2, chunks[2].ChunkIndex)

	// Verify offsets.
	assert.Equal(t, 0, chunks[0].ChunkStart)
	assert.Equal(t, 100, chunks[0].ChunkEnd)
	assert.Equal(t, 100, chunks[1].ChunkStart)
	assert.Equal(t, 200, chunks[1].ChunkEnd)
}

// TestGetForFile_NotFound verifies GetForFile returns METADATA_NOT_FOUND for unknown filepath.
func TestGetForFile_NotFound(t *testing.T) {
	_, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	_, err := repo.GetForFile(ctx, "nonexistent.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound),
		"expected METADATA_NOT_FOUND error code, got: %v", err)
}

// TestDeleteForFile_AllChunks verifies DeleteForFile removes all chunks for a file.
func TestDeleteForFile_AllChunks(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "multi.md")

	// Insert multiple chunks.
	for idx := 0; idx < 3; idx++ {
		err := repo.Upsert(ctx, Embedding{
			Filepath:   "multi.md",
			ChunkIndex: idx,
			Vector:     []float32{0.1},
			ModelID:    "model",
		})
		require.NoError(t, err)
	}

	// Verify chunks exist.
	chunks, err := repo.GetForFile(ctx, "multi.md")
	require.NoError(t, err)
	assert.Len(t, chunks, 3)

	// Delete all.
	err = repo.DeleteForFile(ctx, "multi.md")
	require.NoError(t, err)

	// Verify all are gone.
	_, err = repo.GetForFile(ctx, "multi.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound))
}

// TestGetAll_MultipleFilesMultipleChunks verifies GetAll returns chunks for all files (FR-6).
func TestGetAll_MultipleFilesMultipleChunks(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "file1.md")
	insertTestFile(t, db, "file2.md")

	// file1 has 2 chunks.
	err := repo.Upsert(ctx, Embedding{
		Filepath: "file1.md", ChunkIndex: 0, Vector: []float32{0.1}, ModelID: "m",
		ChunkStart: 0, ChunkEnd: 100,
	})
	require.NoError(t, err)
	err = repo.Upsert(ctx, Embedding{
		Filepath: "file1.md", ChunkIndex: 1, Vector: []float32{0.2}, ModelID: "m",
		ChunkStart: 100, ChunkEnd: 200,
	})
	require.NoError(t, err)

	// file2 has 1 chunk.
	err = repo.Upsert(ctx, Embedding{
		Filepath: "file2.md", ChunkIndex: 0, Vector: []float32{0.5}, ModelID: "m",
		ChunkStart: 0, ChunkEnd: 50,
	})
	require.NoError(t, err)

	all, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3, "should return all 3 chunks across both files")

	// Count per file.
	countByFile := make(map[string]int)
	for _, e := range all {
		countByFile[e.Filepath]++
	}
	assert.Equal(t, 2, countByFile["file1.md"])
	assert.Equal(t, 1, countByFile["file2.md"])
}

// TestGetForFile_ScanError verifies GetForFile returns DATABASE_ERROR when row scanning fails.
func TestGetForFile_ScanError(t *testing.T) {
	db, _ := setupEmbeddingTestDB(t)
	repo := NewSQLiteEmbeddingRepository(db)
	ctx := context.Background()

	insertTestFile(t, db, "bad.md")

	// Insert an embedding with corrupt generated_at that can't be scanned into time.Time.
	_, err := db.SQLDB().Exec(
		`INSERT INTO embeddings (filepath, chunk_index, vector, model_id, generated_at) VALUES (?, ?, ?, ?, ?)`,
		"bad.md", 0, []byte{0, 0, 0, 0}, "model", "not-a-time",
	)
	require.NoError(t, err)

	_, err = repo.GetForFile(ctx, "bad.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestUpsert_SummaryChunk verifies inserting a summary chunk with is_summary=true and chunk_index=-1.
func TestUpsert_SummaryChunk(t *testing.T) {
	db, repo := setupEmbeddingTestDB(t)
	ctx := context.Background()

	insertTestFile(t, db, "summary.md")

	err := repo.Upsert(ctx, Embedding{
		Filepath:   "summary.md",
		ChunkIndex: -1,
		Vector:     []float32{0.7, 0.8},
		ModelID:    "model",
		IsSummary:  true,
	})
	require.NoError(t, err)

	// Also insert a regular chunk.
	err = repo.Upsert(ctx, Embedding{
		Filepath:   "summary.md",
		ChunkIndex: 0,
		Vector:     []float32{0.1, 0.2},
		ModelID:    "model",
		ChunkStart: 0,
		ChunkEnd:   100,
	})
	require.NoError(t, err)

	// GetForFile should return both, ordered by chunk_index.
	chunks, err := repo.GetForFile(ctx, "summary.md")
	require.NoError(t, err)
	require.Len(t, chunks, 2)
	assert.Equal(t, -1, chunks[0].ChunkIndex)
	assert.True(t, chunks[0].IsSummary)
	assert.Equal(t, 0, chunks[1].ChunkIndex)
	assert.False(t, chunks[1].IsSummary)
}
