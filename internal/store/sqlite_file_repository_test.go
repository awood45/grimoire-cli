package store

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates an in-memory DB with schema applied and returns
// a SQLiteFileRepository ready for testing.
func setupTestRepo(t *testing.T) *SQLiteFileRepository {
	t.Helper()
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	err = db.EnsureSchema()
	require.NoError(t, err)

	return NewSQLiteFileRepository(db)
}

// sampleMeta returns a FileMetadata with sensible defaults for testing.
func sampleMeta(filepath, agent string, tags []string) FileMetadata {
	now := time.Now().UTC().Truncate(time.Second)
	return FileMetadata{
		Filepath:    filepath,
		SourceAgent: agent,
		Tags:        tags,
		Summary:     "A test summary for " + filepath,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// TestInsert_success verifies that Insert stores a file and its tags, retrievable via Get (FR-3.2.1).
func TestInsert_success(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	meta := sampleMeta("docs/hello.md", "claude", []string{"go", "testing"})

	err := repo.Insert(ctx, meta)
	require.NoError(t, err)

	got, err := repo.Get(ctx, "docs/hello.md")
	require.NoError(t, err)

	assert.Equal(t, meta.Filepath, got.Filepath)
	assert.Equal(t, meta.SourceAgent, got.SourceAgent)
	assert.Equal(t, meta.Summary, got.Summary)
	assert.Equal(t, meta.CreatedAt.Unix(), got.CreatedAt.Unix())
	assert.Equal(t, meta.UpdatedAt.Unix(), got.UpdatedAt.Unix())

	sort.Strings(got.Tags)
	sort.Strings(meta.Tags)
	assert.Equal(t, meta.Tags, got.Tags)
}

// TestInsert_duplicate verifies that inserting a duplicate filepath returns METADATA_EXISTS (FR-3.2.1).
func TestInsert_duplicate(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	meta := sampleMeta("docs/dup.md", "claude", []string{"tag1"})

	err := repo.Insert(ctx, meta)
	require.NoError(t, err)

	err = repo.Insert(ctx, meta)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataExists),
		"expected METADATA_EXISTS error, got: %v", err)
}

// TestUpdate_success verifies that Update modifies fields and replaces tags (FR-3.2.2).
func TestUpdate_success(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	meta := sampleMeta("docs/update.md", "claude", []string{"old-tag"})
	err := repo.Insert(ctx, meta)
	require.NoError(t, err)

	// Update with new values.
	meta.SourceAgent = "gpt"
	meta.Summary = "Updated summary"
	meta.Tags = []string{"new-tag1", "new-tag2"}
	meta.UpdatedAt = meta.UpdatedAt.Add(time.Hour)

	err = repo.Update(ctx, meta)
	require.NoError(t, err)

	got, err := repo.Get(ctx, "docs/update.md")
	require.NoError(t, err)

	assert.Equal(t, "gpt", got.SourceAgent)
	assert.Equal(t, "Updated summary", got.Summary)
	assert.Equal(t, meta.UpdatedAt.Unix(), got.UpdatedAt.Unix())

	sort.Strings(got.Tags)
	assert.Equal(t, []string{"new-tag1", "new-tag2"}, got.Tags)
}

// TestUpdate_notfound verifies that updating a nonexistent file returns METADATA_NOT_FOUND (FR-3.2.2).
func TestUpdate_notfound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	meta := sampleMeta("docs/missing.md", "claude", nil)

	err := repo.Update(ctx, meta)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound),
		"expected METADATA_NOT_FOUND error, got: %v", err)
}

// TestFileGet_success verifies that Get returns full metadata with tags (FR-3.2.3).
func TestFileGet_success(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	meta := sampleMeta("docs/get.md", "agent-x", []string{"alpha", "beta", "gamma"})
	err := repo.Insert(ctx, meta)
	require.NoError(t, err)

	got, err := repo.Get(ctx, "docs/get.md")
	require.NoError(t, err)

	assert.Equal(t, "docs/get.md", got.Filepath)
	assert.Equal(t, "agent-x", got.SourceAgent)

	sort.Strings(got.Tags)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, got.Tags)
}

// TestFileGet_notfound verifies that Get returns METADATA_NOT_FOUND for missing files (FR-3.2.3).
func TestFileGet_notfound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, "docs/nonexistent.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound),
		"expected METADATA_NOT_FOUND error, got: %v", err)
}

// TestDelete_success verifies that Delete removes a record and cascades to tags (FR-3.2.4).
func TestDelete_success(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	meta := sampleMeta("docs/delete.md", "claude", []string{"tag1", "tag2"})
	err := repo.Insert(ctx, meta)
	require.NoError(t, err)

	err = repo.Delete(ctx, "docs/delete.md")
	require.NoError(t, err)

	_, err = repo.Get(ctx, "docs/delete.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound))
}

// TestDelete_notfound verifies that deleting a nonexistent file returns METADATA_NOT_FOUND (FR-3.2.4).
func TestDelete_notfound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	err := repo.Delete(ctx, "docs/missing.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound),
		"expected METADATA_NOT_FOUND error, got: %v", err)
}

// TestSearch_byTagsAND verifies that Search returns files having ALL specified tags (FR-3.3.1).
func TestSearch_byTagsAND(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	// File with both tags.
	err := repo.Insert(ctx, sampleMeta("docs/both.md", "claude", []string{"go", "testing"}))
	require.NoError(t, err)

	// File with only one tag.
	err = repo.Insert(ctx, sampleMeta("docs/one.md", "claude", []string{"go"}))
	require.NoError(t, err)

	// File with neither tag.
	err = repo.Insert(ctx, sampleMeta("docs/none.md", "claude", []string{"python"}))
	require.NoError(t, err)

	results, err := repo.Search(ctx, SearchFilters{Tags: []string{"go", "testing"}})
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "docs/both.md", results[0].Filepath)
}

// TestSearch_byTagsOR verifies that Search returns files having ANY specified tag (FR-3.3.1).
func TestSearch_byTagsOR(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	err := repo.Insert(ctx, sampleMeta("docs/go.md", "claude", []string{"go"}))
	require.NoError(t, err)

	err = repo.Insert(ctx, sampleMeta("docs/python.md", "claude", []string{"python"}))
	require.NoError(t, err)

	err = repo.Insert(ctx, sampleMeta("docs/rust.md", "claude", []string{"rust"}))
	require.NoError(t, err)

	results, err := repo.Search(ctx, SearchFilters{AnyTags: []string{"go", "python"}})
	require.NoError(t, err)

	require.Len(t, results, 2)

	paths := []string{results[0].Filepath, results[1].Filepath}
	sort.Strings(paths)
	assert.Equal(t, []string{"docs/go.md", "docs/python.md"}, paths)
}

// TestSearch_bySourceAgent verifies that Search filters by source agent (FR-3.3.1).
func TestSearch_bySourceAgent(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	err := repo.Insert(ctx, sampleMeta("docs/claude.md", "claude", nil))
	require.NoError(t, err)

	err = repo.Insert(ctx, sampleMeta("docs/gpt.md", "gpt", nil))
	require.NoError(t, err)

	results, err := repo.Search(ctx, SearchFilters{SourceAgent: "claude"})
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "docs/claude.md", results[0].Filepath)
}

// TestSearch_byDateRange verifies that Search filters by after/before dates (FR-3.3.1).
func TestSearch_byDateRange(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	old := sampleMeta("docs/old.md", "claude", nil)
	old.UpdatedAt = base
	old.CreatedAt = base
	err := repo.Insert(ctx, old)
	require.NoError(t, err)

	mid := sampleMeta("docs/mid.md", "claude", nil)
	mid.UpdatedAt = base.Add(24 * time.Hour)
	mid.CreatedAt = base
	err = repo.Insert(ctx, mid)
	require.NoError(t, err)

	recent := sampleMeta("docs/recent.md", "claude", nil)
	recent.UpdatedAt = base.Add(48 * time.Hour)
	recent.CreatedAt = base
	err = repo.Insert(ctx, recent)
	require.NoError(t, err)

	after := base.Add(12 * time.Hour)
	before := base.Add(36 * time.Hour)

	results, err := repo.Search(ctx, SearchFilters{After: &after, Before: &before})
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "docs/mid.md", results[0].Filepath)
}

// TestSearch_bySummaryContains verifies substring matching on summary (FR-3.3.1).
func TestSearch_bySummaryContains(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	m1 := sampleMeta("docs/a.md", "claude", nil)
	m1.Summary = "This is about Kubernetes deployment"
	err := repo.Insert(ctx, m1)
	require.NoError(t, err)

	m2 := sampleMeta("docs/b.md", "claude", nil)
	m2.Summary = "Docker container basics"
	err = repo.Insert(ctx, m2)
	require.NoError(t, err)

	results, err := repo.Search(ctx, SearchFilters{SummaryContains: "Kubernetes"})
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "docs/a.md", results[0].Filepath)
}

// TestSearch_combinedFilters verifies multiple filters combined (FR-3.3.1).
func TestSearch_combinedFilters(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	// Matches all filters.
	m1 := sampleMeta("docs/match.md", "claude", []string{"go"})
	m1.Summary = "Go testing patterns"
	m1.UpdatedAt = base
	m1.CreatedAt = base
	err := repo.Insert(ctx, m1)
	require.NoError(t, err)

	// Wrong agent.
	m2 := sampleMeta("docs/wrong-agent.md", "gpt", []string{"go"})
	m2.Summary = "Go testing patterns"
	m2.UpdatedAt = base
	m2.CreatedAt = base
	err = repo.Insert(ctx, m2)
	require.NoError(t, err)

	// Wrong tag.
	m3 := sampleMeta("docs/wrong-tag.md", "claude", []string{"python"})
	m3.Summary = "Go testing patterns"
	m3.UpdatedAt = base
	m3.CreatedAt = base
	err = repo.Insert(ctx, m3)
	require.NoError(t, err)

	after := base.Add(-time.Hour)
	results, err := repo.Search(ctx, SearchFilters{
		Tags:            []string{"go"},
		SourceAgent:     "claude",
		SummaryContains: "testing",
		After:           &after,
	})
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "docs/match.md", results[0].Filepath)
}

// TestSearch_limit verifies that Search respects the limit parameter (FR-3.3.1).
func TestSearch_limit(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		meta := sampleMeta("docs/file"+string(rune('a'+i))+".md", "claude", nil)
		err := repo.Insert(ctx, meta)
		require.NoError(t, err)
	}

	results, err := repo.Search(ctx, SearchFilters{Limit: 3})
	require.NoError(t, err)

	assert.Len(t, results, 3)
}

// TestSearch_sort verifies that Search orders by the specified field (FR-3.3.1).
func TestSearch_sort(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	m1 := sampleMeta("docs/a.md", "claude", nil)
	m1.CreatedAt = base
	m1.UpdatedAt = base.Add(2 * time.Hour)
	err := repo.Insert(ctx, m1)
	require.NoError(t, err)

	m2 := sampleMeta("docs/b.md", "claude", nil)
	m2.CreatedAt = base
	m2.UpdatedAt = base.Add(1 * time.Hour)
	err = repo.Insert(ctx, m2)
	require.NoError(t, err)

	m3 := sampleMeta("docs/c.md", "claude", nil)
	m3.CreatedAt = base
	m3.UpdatedAt = base.Add(3 * time.Hour)
	err = repo.Insert(ctx, m3)
	require.NoError(t, err)

	// Sort by filepath ascending.
	results, err := repo.Search(ctx, SearchFilters{Sort: "filepath"})
	require.NoError(t, err)

	require.Len(t, results, 3)
	assert.Equal(t, "docs/a.md", results[0].Filepath)
	assert.Equal(t, "docs/b.md", results[1].Filepath)
	assert.Equal(t, "docs/c.md", results[2].Filepath)
}

// TestListTags_byName verifies ListTags sorts alphabetically with counts (FR-3.3.3).
func TestListTags_byName(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	err := repo.Insert(ctx, sampleMeta("docs/a.md", "claude", []string{"beta", "alpha"}))
	require.NoError(t, err)
	err = repo.Insert(ctx, sampleMeta("docs/b.md", "claude", []string{"alpha", "gamma"}))
	require.NoError(t, err)

	tags, err := repo.ListTags(ctx, "name")
	require.NoError(t, err)

	require.Len(t, tags, 3)
	assert.Equal(t, "alpha", tags[0].Name)
	assert.Equal(t, 2, tags[0].Count)
	assert.Equal(t, "beta", tags[1].Name)
	assert.Equal(t, 1, tags[1].Count)
	assert.Equal(t, "gamma", tags[2].Name)
	assert.Equal(t, 1, tags[2].Count)
}

// TestListTags_byCount verifies ListTags sorts by count descending (FR-3.3.3).
func TestListTags_byCount(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	err := repo.Insert(ctx, sampleMeta("docs/a.md", "claude", []string{"rare", "common"}))
	require.NoError(t, err)
	err = repo.Insert(ctx, sampleMeta("docs/b.md", "claude", []string{"common"}))
	require.NoError(t, err)
	err = repo.Insert(ctx, sampleMeta("docs/c.md", "claude", []string{"common", "rare"}))
	require.NoError(t, err)
	err = repo.Insert(ctx, sampleMeta("docs/d.md", "claude", []string{"common", "mid"}))
	require.NoError(t, err)

	tags, err := repo.ListTags(ctx, "count")
	require.NoError(t, err)

	require.Len(t, tags, 3)
	assert.Equal(t, "common", tags[0].Name)
	assert.Equal(t, 4, tags[0].Count)
	// Second and third depend on ties, but both should have correct counts.
	assert.Equal(t, 2, tags[1].Count)
	assert.Equal(t, 1, tags[2].Count)
}

// TestAllFilepaths verifies that AllFilepaths returns all tracked paths (FR-3.4.3).
func TestAllFilepaths(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	err := repo.Insert(ctx, sampleMeta("docs/a.md", "claude", nil))
	require.NoError(t, err)
	err = repo.Insert(ctx, sampleMeta("docs/b.md", "claude", nil))
	require.NoError(t, err)
	err = repo.Insert(ctx, sampleMeta("docs/c.md", "claude", nil))
	require.NoError(t, err)

	paths, err := repo.AllFilepaths(ctx)
	require.NoError(t, err)

	sort.Strings(paths)
	assert.Equal(t, []string{"docs/a.md", "docs/b.md", "docs/c.md"}, paths)
}

// TestCount verifies that Count returns the total number of tracked files (FR-3.4.1).
func TestCount(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	err = repo.Insert(ctx, sampleMeta("docs/a.md", "claude", nil))
	require.NoError(t, err)
	err = repo.Insert(ctx, sampleMeta("docs/b.md", "claude", nil))
	require.NoError(t, err)

	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// closedRepo returns a SQLiteFileRepository backed by a closed database for error-path testing.
func closedRepo(t *testing.T) (*SQLiteFileRepository, context.Context) {
	t.Helper()
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.EnsureSchema())
	repo := NewSQLiteFileRepository(db)
	require.NoError(t, db.Close())
	return repo, context.Background()
}

// TestInsert_closedDB verifies Insert returns DATABASE_ERROR when the database is closed.
func TestInsert_closedDB(t *testing.T) {
	repo, ctx := closedRepo(t)
	err := repo.Insert(ctx, sampleMeta("test.md", "claude", nil))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestUpdate_closedDB verifies Update returns DATABASE_ERROR when the database is closed.
func TestUpdate_closedDB(t *testing.T) {
	repo, ctx := closedRepo(t)
	err := repo.Update(ctx, sampleMeta("test.md", "claude", nil))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestGet_closedDB verifies Get returns DATABASE_ERROR when the database is closed.
func TestGet_closedDB(t *testing.T) {
	repo, ctx := closedRepo(t)
	_, err := repo.Get(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestDelete_closedDB verifies Delete returns DATABASE_ERROR when the database is closed.
func TestDelete_closedDB(t *testing.T) {
	repo, ctx := closedRepo(t)
	err := repo.Delete(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestSearch_closedDB verifies Search returns DATABASE_ERROR when the database is closed.
func TestSearch_closedDB(t *testing.T) {
	repo, ctx := closedRepo(t)
	_, err := repo.Search(ctx, SearchFilters{})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestListTags_closedDB verifies ListTags returns DATABASE_ERROR when the database is closed.
func TestListTags_closedDB(t *testing.T) {
	repo, ctx := closedRepo(t)
	_, err := repo.ListTags(ctx, "name")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestAllFilepaths_closedDB verifies AllFilepaths returns DATABASE_ERROR when the database is closed.
func TestAllFilepaths_closedDB(t *testing.T) {
	repo, ctx := closedRepo(t)
	_, err := repo.AllFilepaths(ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestCount_closedDB verifies Count returns DATABASE_ERROR when the database is closed.
func TestCount_closedDB(t *testing.T) {
	repo, ctx := closedRepo(t)
	_, err := repo.Count(ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestMakePlaceholders_zero verifies makePlaceholders returns empty for zero input.
func TestMakePlaceholders_zero(t *testing.T) {
	assert.Equal(t, "", makePlaceholders(0))
	assert.Equal(t, "", makePlaceholders(-1))
}

// setupTestRepoWithDB returns both DB and repo so tests can manipulate the schema directly.
func setupTestRepoWithDB(t *testing.T) (*DB, *SQLiteFileRepository) {
	t.Helper()
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	err = db.EnsureSchema()
	require.NoError(t, err)

	return db, NewSQLiteFileRepository(db)
}

// TestInsert_nonUniqueDBError verifies Insert returns DATABASE_ERROR for non-unique DB failures.
func TestInsert_nonUniqueDBError(t *testing.T) {
	db, repo := setupTestRepoWithDB(t)
	ctx := context.Background()

	// Drop tables so INSERT INTO files fails with "no such table" (not a unique constraint error).
	_, err := db.SQLDB().Exec("DROP TABLE file_tags")
	require.NoError(t, err)
	_, err = db.SQLDB().Exec("DROP TABLE embeddings")
	require.NoError(t, err)
	_, err = db.SQLDB().Exec("DROP TABLE files")
	require.NoError(t, err)

	err = repo.Insert(ctx, sampleMeta("test.md", "claude", nil))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestInsert_insertTagsError verifies Insert returns DATABASE_ERROR when tag insertion fails.
func TestInsert_insertTagsError(t *testing.T) {
	db, repo := setupTestRepoWithDB(t)
	ctx := context.Background()

	// Drop only file_tags so INSERT INTO files succeeds but insertTags fails.
	_, err := db.SQLDB().Exec("DROP TABLE file_tags")
	require.NoError(t, err)

	err = repo.Insert(ctx, sampleMeta("test.md", "claude", []string{"tag1"}))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestUpdate_execError verifies Update returns DATABASE_ERROR when the UPDATE statement fails.
func TestUpdate_execError(t *testing.T) {
	db, repo := setupTestRepoWithDB(t)
	ctx := context.Background()

	// Drop tables so UPDATE files fails with "no such table".
	_, err := db.SQLDB().Exec("DROP TABLE file_tags")
	require.NoError(t, err)
	_, err = db.SQLDB().Exec("DROP TABLE embeddings")
	require.NoError(t, err)
	_, err = db.SQLDB().Exec("DROP TABLE files")
	require.NoError(t, err)

	err = repo.Update(ctx, sampleMeta("test.md", "claude", nil))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestUpdate_deleteTagsError verifies Update returns DATABASE_ERROR when deleting old tags fails.
func TestUpdate_deleteTagsError(t *testing.T) {
	db, repo := setupTestRepoWithDB(t)
	ctx := context.Background()

	// Insert a file first so the UPDATE succeeds.
	err := repo.Insert(ctx, sampleMeta("test.md", "claude", nil))
	require.NoError(t, err)

	// Drop file_tags so DELETE FROM file_tags fails.
	_, err = db.SQLDB().Exec("DROP TABLE file_tags")
	require.NoError(t, err)

	err = repo.Update(ctx, sampleMeta("test.md", "claude", []string{"tag"}))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestUpdate_insertTagsError verifies Update returns DATABASE_ERROR when inserting new tags fails.
func TestUpdate_insertTagsError(t *testing.T) {
	db, repo := setupTestRepoWithDB(t)
	ctx := context.Background()

	// Insert a file with no tags.
	err := repo.Insert(ctx, sampleMeta("test.md", "claude", nil))
	require.NoError(t, err)

	// Replace file_tags with a table that has filepath (so DELETE succeeds)
	// but no tag column (so INSERT INTO file_tags (filepath, tag) fails).
	_, err = db.SQLDB().Exec("DROP TABLE file_tags")
	require.NoError(t, err)
	_, err = db.SQLDB().Exec("CREATE TABLE file_tags (filepath TEXT)")
	require.NoError(t, err)

	err = repo.Update(ctx, sampleMeta("test.md", "claude", []string{"tag"}))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestGet_getTagsError verifies Get returns DATABASE_ERROR when fetching tags fails.
func TestGet_getTagsError(t *testing.T) {
	db, repo := setupTestRepoWithDB(t)
	ctx := context.Background()

	// Insert a file so the file query succeeds.
	err := repo.Insert(ctx, sampleMeta("test.md", "claude", nil))
	require.NoError(t, err)

	// Drop file_tags so getTags query fails.
	_, err = db.SQLDB().Exec("DROP TABLE file_tags")
	require.NoError(t, err)

	_, err = repo.Get(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestSearch_getTagsError verifies Search returns DATABASE_ERROR when fetching tags fails.
func TestSearch_getTagsError(t *testing.T) {
	db, repo := setupTestRepoWithDB(t)
	ctx := context.Background()

	// Insert a file so the main query returns results.
	err := repo.Insert(ctx, sampleMeta("test.md", "claude", nil))
	require.NoError(t, err)

	// Drop file_tags so getTags fails after the main query succeeds.
	_, err = db.SQLDB().Exec("DROP TABLE file_tags")
	require.NoError(t, err)

	_, err = repo.Search(ctx, SearchFilters{})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestSearch_scanError verifies Search returns DATABASE_ERROR when row scanning fails.
func TestSearch_scanError(t *testing.T) {
	db, repo := setupTestRepoWithDB(t)
	ctx := context.Background()

	// Insert a row with corrupt time data that can't be scanned into time.Time.
	_, err := db.SQLDB().Exec(
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"bad.md", "agent", "", "not-a-time", "also-not-a-time",
	)
	require.NoError(t, err)

	_, err = repo.Search(ctx, SearchFilters{})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}
