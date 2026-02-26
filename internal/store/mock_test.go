package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errMock is a sentinel error used in mock expectations.
var errMock = errors.New("mock error")

// mockDB creates a go-sqlmock database and wraps it in a store.DB for repository tests.
func mockDB(t *testing.T) (*DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		// Allow close to be called or not.
		mock.ExpectClose()
		sqlDB.Close()
	})
	return &DB{db: sqlDB}, mock
}

// --- db.go error paths ---

// TestConfigureDB_busyTimeoutError verifies configureDB returns DATABASE_ERROR when busy_timeout fails.
func TestConfigureDB_busyTimeoutError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// WAL succeeds, busy_timeout fails.
	mock.ExpectExec("PRAGMA journal_mode=WAL").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("PRAGMA busy_timeout=5000").WillReturnError(errMock)

	err = configureDB(db)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestConfigureDB_foreignKeysError verifies configureDB returns DATABASE_ERROR when foreign_keys fails.
func TestConfigureDB_foreignKeysError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// WAL and busy_timeout succeed, foreign_keys fails.
	mock.ExpectExec("PRAGMA journal_mode=WAL").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("PRAGMA busy_timeout=5000").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("PRAGMA foreign_keys=ON").WillReturnError(errMock)

	err = configureDB(db)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestClose_error verifies Close wraps and returns the underlying error as DATABASE_ERROR.
func TestClose_error(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	mock.ExpectClose().WillReturnError(errMock)

	d := &DB{db: sqlDB}
	err = d.Close()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// --- sqlite_file_repository.go error paths ---

// TestMock_Insert_commitError verifies Insert returns DATABASE_ERROR when tx.Commit fails (FR-3.2.1).
func TestMock_Insert_commitError(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO files").
		WithArgs("test.md", "agent", "summary", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errMock)
	mock.ExpectRollback()

	meta := FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "agent",
		Summary:     "summary",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err := repo.Insert(ctx, meta)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_Update_rowsAffectedError verifies Update returns DATABASE_ERROR when RowsAffected fails (FR-3.2.2).
func TestMock_Update_rowsAffectedError(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE files SET").
		WithArgs("agent", "summary", sqlmock.AnyArg(), "test.md").
		WillReturnResult(sqlmock.NewErrorResult(errMock))
	mock.ExpectRollback()

	meta := FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "agent",
		Summary:     "summary",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err := repo.Update(ctx, meta)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_Update_commitError verifies Update returns DATABASE_ERROR when tx.Commit fails (FR-3.2.2).
func TestMock_Update_commitError(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE files SET").
		WithArgs("agent", "summary", sqlmock.AnyArg(), "test.md").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// DELETE old tags.
	mock.ExpectExec("DELETE FROM file_tags").
		WithArgs("test.md").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit().WillReturnError(errMock)
	mock.ExpectRollback()

	meta := FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "agent",
		Summary:     "summary",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err := repo.Update(ctx, meta)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_Delete_rowsAffectedError verifies Delete returns DATABASE_ERROR when RowsAffected fails (FR-3.2.4).
func TestMock_Delete_rowsAffectedError(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM files").
		WithArgs("test.md").
		WillReturnResult(sqlmock.NewErrorResult(errMock))

	err := repo.Delete(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_Search_rowsErr verifies Search returns DATABASE_ERROR when rows.Err reports an error (FR-3.3.1).
func TestMock_Search_rowsErr(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()
	// Must add a row so the driver reaches the RowError check (without rows, Next returns EOF).
	rows := sqlmock.NewRows([]string{"filepath", "source_agent", "summary", "created_at", "updated_at"}).
		AddRow("test.md", "agent", "summary", now, now).
		RowError(0, errMock)

	mock.ExpectQuery("SELECT .+ FROM files").WillReturnRows(rows)

	_, err := repo.Search(ctx, SearchFilters{})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_ListTags_scanError verifies ListTags returns DATABASE_ERROR when row scanning fails (FR-3.3.3).
func TestMock_ListTags_scanError(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	// Return a single column; ListTags scans into (string, int), so column count mismatch causes Scan error.
	rows := sqlmock.NewRows([]string{"tag"}).AddRow("tag1")
	mock.ExpectQuery("SELECT tag, COUNT").WillReturnRows(rows)

	_, err := repo.ListTags(ctx, "name")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_ListTags_rowsErr verifies ListTags returns DATABASE_ERROR when rows.Err reports an error (FR-3.3.3).
func TestMock_ListTags_rowsErr(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	// Must add a row so the driver reaches the RowError check.
	rows := sqlmock.NewRows([]string{"tag", "count"}).
		AddRow("tag1", int64(1)).
		RowError(0, errMock)
	mock.ExpectQuery("SELECT tag, COUNT").WillReturnRows(rows)

	_, err := repo.ListTags(ctx, "name")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_AllFilepaths_scanError verifies AllFilepaths returns DATABASE_ERROR when row scanning fails (FR-3.4.3).
func TestMock_AllFilepaths_scanError(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	// Return two columns; AllFilepaths scans into a single string, so column count mismatch causes Scan error.
	rows := sqlmock.NewRows([]string{"filepath", "extra"}).AddRow("a.md", "extra")
	mock.ExpectQuery("SELECT filepath FROM files").WillReturnRows(rows)

	_, err := repo.AllFilepaths(ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_AllFilepaths_rowsErr verifies AllFilepaths returns DATABASE_ERROR when rows.Err reports an error (FR-3.4.3).
func TestMock_AllFilepaths_rowsErr(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	// Must add a row so the driver reaches the RowError check.
	rows := sqlmock.NewRows([]string{"filepath"}).
		AddRow("test.md").
		RowError(0, errMock)
	mock.ExpectQuery("SELECT filepath FROM files").WillReturnRows(rows)

	_, err := repo.AllFilepaths(ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_getTags_scanError verifies getTags returns DATABASE_ERROR when row scanning fails.
// Tested indirectly through Get, which calls getTags after the file row query succeeds.
func TestMock_getTags_scanError(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()

	// Get first does a QueryRow for the file — return a valid row.
	fileRow := sqlmock.NewRows([]string{"filepath", "source_agent", "summary", "created_at", "updated_at"}).
		AddRow("test.md", "agent", "summary", now, now)
	mock.ExpectQuery("SELECT filepath, source_agent, summary, created_at, updated_at FROM files").
		WithArgs("test.md").
		WillReturnRows(fileRow)

	// getTags query — return two columns but Scan expects one string, causing Scan error.
	tagRows := sqlmock.NewRows([]string{"tag", "extra"}).AddRow("tag1", "extra")
	mock.ExpectQuery("SELECT tag FROM file_tags").
		WithArgs("test.md").
		WillReturnRows(tagRows)

	_, err := repo.Get(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMock_getTags_rowsErr verifies getTags returns DATABASE_ERROR when rows.Err reports an error.
// Tested indirectly through Get, which calls getTags after the file row query succeeds.
func TestMock_getTags_rowsErr(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteFileRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()

	// Get first does a QueryRow for the file — return a valid row.
	fileRow := sqlmock.NewRows([]string{"filepath", "source_agent", "summary", "created_at", "updated_at"}).
		AddRow("test.md", "agent", "summary", now, now)
	mock.ExpectQuery("SELECT filepath, source_agent, summary, created_at, updated_at FROM files").
		WithArgs("test.md").
		WillReturnRows(fileRow)

	// getTags query — must add a row so the driver reaches the RowError check.
	tagRows := sqlmock.NewRows([]string{"tag"}).
		AddRow("tag1").
		RowError(0, errMock)
	mock.ExpectQuery("SELECT tag FROM file_tags").
		WithArgs("test.md").
		WillReturnRows(tagRows)

	_, err := repo.Get(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// --- sqlite_embedding_repository.go error paths ---

// TestMock_Embedding_GetAll_rowsErr verifies GetAll returns DATABASE_ERROR when rows.Err reports an error (FR-3.3.2).
func TestMock_Embedding_GetAll_rowsErr(t *testing.T) {
	db, mock := mockDB(t)
	repo := NewSQLiteEmbeddingRepository(db)
	ctx := context.Background()

	// Must add a row so the driver reaches the RowError check.
	rows := sqlmock.NewRows([]string{"filepath", "chunk_index", "vector", "model_id", "generated_at", "chunk_start", "chunk_end", "is_summary"}).
		AddRow("test.md", 0, []byte{0, 0, 0, 0}, "model", time.Now().UTC(), 0, 0, false).
		RowError(0, errMock)
	mock.ExpectQuery("SELECT filepath, chunk_index, vector, model_id, generated_at, chunk_start, chunk_end, is_summary FROM embeddings").WillReturnRows(rows)

	_, err := repo.GetAll(ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}
