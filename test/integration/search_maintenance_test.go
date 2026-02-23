// Package integration_test contains integration tests for search, maintenance,
// and durability operations using real SQLite and real file system.
package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/docgen"
	"github.com/awood45/grimoire-cli/internal/embedding"
	embtest "github.com/awood45/grimoire-cli/internal/embedding/testing"
	"github.com/awood45/grimoire-cli/internal/filelock"
	"github.com/awood45/grimoire-cli/internal/frontmatter"
	"github.com/awood45/grimoire-cli/internal/maintenance"
	"github.com/awood45/grimoire-cli/internal/metadata"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/search"
	"github.com/awood45/grimoire-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullStack extends testStack with search engine and maintenance service.
type fullStack struct {
	testStack
	engine   *search.Engine
	svc      *maintenance.Service
	fileRepo store.FileRepository
	embRepo  store.EmbeddingRepository
}

// setupFullStack creates a complete stack with search engine and maintenance service.
// The frontmatter service is always included so maintenance operations work.
func setupFullStack(t *testing.T, embProvider embedding.Provider) fullStack {
	t.Helper()

	ts := setupTestStack(t, embProvider)

	// Default to NoopProvider for engine/svc if nil was passed
	// (setupTestStack already does this for the manager).
	effectiveEmb := embProvider
	if effectiveEmb == nil {
		effectiveEmb = &embedding.NoopProvider{}
	}

	fileRepo := store.NewSQLiteFileRepository(ts.db)
	embRepo := store.NewSQLiteEmbeddingRepository(ts.db)

	engine := search.NewEngine(fileRepo, embRepo, effectiveEmb)

	docGen, err := docgen.NewTemplateGenerator()
	require.NoError(t, err, "failed to create doc generator")

	locker, err := filelock.NewFlockLocker(ts.b.LockPath())
	require.NoError(t, err, "failed to create locker for maintenance")
	t.Cleanup(func() {
		locker.Close()
	})

	fm := frontmatter.NewFileService()

	svc := maintenance.NewService(
		ts.b, fileRepo, embRepo, ts.led,
		fm, effectiveEmb, locker, docGen, ts.db,
	)

	return fullStack{
		testStack: ts,
		engine:    engine,
		svc:       svc,
		fileRepo:  fileRepo,
		embRepo:   embRepo,
	}
}

// createTrackedFile is a helper that creates a file and tracks it via Manager.Create.
func createTrackedFile(ctx context.Context, t *testing.T, ts testStack, relPath, content, agent string, tags []string, summary string) store.FileMetadata {
	t.Helper()
	createTestFile(t, ts.b, relPath, content)
	meta, err := ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    relPath,
		SourceAgent: agent,
		Tags:        tags,
		Summary:     summary,
	})
	require.NoError(t, err, "failed to create tracked file %s", relPath)
	return meta
}

// TestSearchByTags exercises FR-3.3.1: search by tags with AND/OR logic.
func TestSearchByTags(t *testing.T) {
	ctx := context.Background()
	fs := setupFullStack(t, nil)

	// Create 5 files with various tag combinations.
	createTrackedFile(ctx, t, fs.testStack, "notes/meeting.md", "# Meeting\n", "agent-a", []string{"meeting", "weekly", "team-alpha"}, "Weekly meeting notes")
	createTrackedFile(ctx, t, fs.testStack, "notes/standup.md", "# Standup\n", "agent-a", []string{"meeting", "daily"}, "Daily standup")
	createTrackedFile(ctx, t, fs.testStack, "docs/api.md", "# API\n", "agent-b", []string{"docs", "api", "reference"}, "API documentation")
	createTrackedFile(ctx, t, fs.testStack, "docs/guide.md", "# Guide\n", "agent-b", []string{"docs", "guide"}, "Getting started guide")
	createTrackedFile(ctx, t, fs.testStack, "projects/alpha.md", "# Alpha\n", "agent-c", []string{"project", "team-alpha"}, "Alpha project")

	// AND search: files having ALL specified tags.
	results, err := fs.engine.Search(ctx, store.SearchFilters{Tags: []string{"meeting", "weekly"}})
	require.NoError(t, err)
	assert.Len(t, results, 1, "AND search for meeting+weekly should return 1 file")
	assert.Equal(t, "notes/meeting.md", results[0].Filepath)

	// OR search: files having ANY specified tag.
	results, err = fs.engine.Search(ctx, store.SearchFilters{AnyTags: []string{"daily", "guide"}})
	require.NoError(t, err)
	assert.Len(t, results, 2, "OR search for daily|guide should return 2 files")
	paths := []string{results[0].Filepath, results[1].Filepath}
	assert.Contains(t, paths, "notes/standup.md")
	assert.Contains(t, paths, "docs/guide.md")

	// Combined filter: tags AND source agent.
	results, err = fs.engine.Search(ctx, store.SearchFilters{
		AnyTags:     []string{"meeting", "docs"},
		SourceAgent: "agent-a",
	})
	require.NoError(t, err)
	assert.Len(t, results, 2, "OR(meeting, docs) + agent-a should return 2 files")
	for _, r := range results {
		assert.Equal(t, "agent-a", r.SourceAgent)
	}
}

// TestSearchByDateRange exercises FR-3.3.1: date range filtering.
func TestSearchByDateRange(t *testing.T) {
	ctx := context.Background()
	fs := setupFullStack(t, nil)

	// Create files with small time gaps so we can filter between them.
	createTrackedFile(ctx, t, fs.testStack, "file1.md", "# File1\n", "agent", []string{"a"}, "first")
	time.Sleep(50 * time.Millisecond)

	midpoint := time.Now().UTC()
	time.Sleep(50 * time.Millisecond)

	createTrackedFile(ctx, t, fs.testStack, "file2.md", "# File2\n", "agent", []string{"b"}, "second")
	time.Sleep(50 * time.Millisecond)

	createTrackedFile(ctx, t, fs.testStack, "file3.md", "# File3\n", "agent", []string{"c"}, "third")

	// Search after midpoint: should return file2 and file3.
	results, err := fs.engine.Search(ctx, store.SearchFilters{After: &midpoint})
	require.NoError(t, err)
	assert.Len(t, results, 2, "after midpoint should return 2 files")

	// Search before midpoint: should return file1.
	results, err = fs.engine.Search(ctx, store.SearchFilters{Before: &midpoint})
	require.NoError(t, err)
	assert.Len(t, results, 1, "before midpoint should return 1 file")
	assert.Equal(t, "file1.md", results[0].Filepath)
}

// TestSearchLimit exercises FR-3.3.1: limit parameter.
func TestSearchLimit(t *testing.T) {
	ctx := context.Background()
	fs := setupFullStack(t, nil)

	// Create 10 files.
	for i := 0; i < 10; i++ {
		name := filepath.Join("notes", filepath.Base(time.Now().Format("20060102-150405.000000000"))+".md")
		createTrackedFile(ctx, t, fs.testStack, name, "# Note\n", "agent", []string{"bulk"}, "bulk note")
		time.Sleep(1 * time.Millisecond) // Ensure unique filenames.
	}

	results, err := fs.engine.Search(ctx, store.SearchFilters{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, results, 3, "limit 3 should return exactly 3 results")
}

// TestSearchSort exercises FR-3.3.1: sort parameter.
func TestSearchSort(t *testing.T) {
	ctx := context.Background()
	fs := setupFullStack(t, nil)

	createTrackedFile(ctx, t, fs.testStack, "aaa.md", "# AAA\n", "agent", []string{"x"}, "first file")
	time.Sleep(20 * time.Millisecond)
	createTrackedFile(ctx, t, fs.testStack, "bbb.md", "# BBB\n", "agent", []string{"x"}, "second file")
	time.Sleep(20 * time.Millisecond)
	createTrackedFile(ctx, t, fs.testStack, "ccc.md", "# CCC\n", "agent", []string{"x"}, "third file")

	// Default sort is updated_at DESC (newest first).
	results, err := fs.engine.Search(ctx, store.SearchFilters{})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "ccc.md", results[0].Filepath, "newest file should be first (default sort)")
	assert.Equal(t, "aaa.md", results[2].Filepath, "oldest file should be last (default sort)")

	// Sort by filepath (ascending by default in SQL).
	results, err = fs.engine.Search(ctx, store.SearchFilters{Sort: "filepath"})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "aaa.md", results[0].Filepath)
	assert.Equal(t, "bbb.md", results[1].Filepath)
	assert.Equal(t, "ccc.md", results[2].Filepath)
}

// TestSimilarSearch exercises FR-3.3.2: similarity search by text.
func TestSimilarSearch(t *testing.T) {
	ctx := context.Background()

	// Create a FakeProvider that returns different vectors for different files.
	fakeEmb := embtest.NewFakeProvider()
	fs := setupFullStack(t, fakeEmb)

	// Create 3 files. The FakeProvider returns the same vector for all,
	// but we can manually upsert distinct vectors for each file.
	createTrackedFile(ctx, t, fs.testStack, "a.md", "# A\nAlpha content.\n", "agent", []string{"a"}, "alpha")
	createTrackedFile(ctx, t, fs.testStack, "b.md", "# B\nBeta content.\n", "agent", []string{"b"}, "beta")
	createTrackedFile(ctx, t, fs.testStack, "c.md", "# C\nGamma content.\n", "agent", []string{"c"}, "gamma")

	// Override embeddings with deterministic vectors for scoring.
	// Query will be [1, 0, 0]. a=[1,0,0] (identical), b=[0.5,0.5,0] (partial), c=[0,1,0] (orthogonal).
	require.NoError(t, fs.embRepo.Upsert(ctx, "a.md", []float32{1, 0, 0}, "fake-model"))
	require.NoError(t, fs.embRepo.Upsert(ctx, "b.md", []float32{0.5, 0.5, 0}, "fake-model"))
	require.NoError(t, fs.embRepo.Upsert(ctx, "c.md", []float32{0, 1, 0}, "fake-model"))

	// Override the fake provider to return our query vector for the text search.
	fakeEmb.FixedVector = []float32{1, 0, 0}

	results, err := fs.engine.Similar(ctx, search.SimilarInput{Text: "query text", Limit: 10})
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Results should be ranked: a (score=1.0), b (partial), c (score=0.0).
	assert.Equal(t, "a.md", results[0].Filepath, "most similar should be first")
	assert.InDelta(t, 1.0, results[0].Score, 0.01, "identical vectors should score ~1.0")
	assert.Equal(t, "b.md", results[1].Filepath, "partially similar should be second")
	assert.Greater(t, results[1].Score, results[2].Score, "b should score higher than c")
	assert.Equal(t, "c.md", results[2].Filepath, "least similar should be last")
	assert.InDelta(t, 0.0, results[2].Score, 0.01, "orthogonal vectors should score ~0.0")
}

// TestSimilarByFile exercises FR-3.3.2: similarity search using an existing file's embedding.
func TestSimilarByFile(t *testing.T) {
	ctx := context.Background()
	fakeEmb := embtest.NewFakeProvider()
	fs := setupFullStack(t, fakeEmb)

	// Create 3 files.
	createTrackedFile(ctx, t, fs.testStack, "query.md", "# Query\n", "agent", []string{"q"}, "query file")
	createTrackedFile(ctx, t, fs.testStack, "similar.md", "# Similar\n", "agent", []string{"s"}, "similar file")
	createTrackedFile(ctx, t, fs.testStack, "different.md", "# Different\n", "agent", []string{"d"}, "different file")

	// Upsert specific vectors.
	require.NoError(t, fs.embRepo.Upsert(ctx, "query.md", []float32{1, 0, 0}, "fake-model"))
	require.NoError(t, fs.embRepo.Upsert(ctx, "similar.md", []float32{0.9, 0.1, 0}, "fake-model"))
	require.NoError(t, fs.embRepo.Upsert(ctx, "different.md", []float32{0, 0, 1}, "fake-model"))

	// Search by file: use query.md's embedding.
	results, err := fs.engine.Similar(ctx, search.SimilarInput{FilePath: "query.md"})
	require.NoError(t, err)
	require.Len(t, results, 2, "query file should be excluded from results")

	// similar.md should rank higher than different.md.
	assert.Equal(t, "similar.md", results[0].Filepath)
	assert.Equal(t, "different.md", results[1].Filepath)
	assert.Greater(t, results[0].Score, results[1].Score)
}

// TestListTags exercises FR-3.3.3: listing tags with counts.
func TestListTags(t *testing.T) {
	ctx := context.Background()
	fs := setupFullStack(t, nil)

	createTrackedFile(ctx, t, fs.testStack, "a.md", "# A\n", "agent", []string{"go", "backend"}, "a")
	createTrackedFile(ctx, t, fs.testStack, "b.md", "# B\n", "agent", []string{"go", "frontend"}, "b")
	createTrackedFile(ctx, t, fs.testStack, "c.md", "# C\n", "agent", []string{"go", "backend", "api"}, "c")

	tags, err := fs.engine.ListTags(ctx, "count")
	require.NoError(t, err)

	// go:3, backend:2, frontend:1, api:1.
	tagMap := make(map[string]int)
	for _, tc := range tags {
		tagMap[tc.Name] = tc.Count
	}

	assert.Equal(t, 3, tagMap["go"], "go should have count 3")
	assert.Equal(t, 2, tagMap["backend"], "backend should have count 2")
	assert.Equal(t, 1, tagMap["frontend"], "frontend should have count 1")
	assert.Equal(t, 1, tagMap["api"], "api should have count 1")

	// Sort by name.
	tags, err = fs.engine.ListTags(ctx, "name")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(tags), 4, "should have at least 4 tags")

	// Verify alphabetical ordering.
	for i := 1; i < len(tags); i++ {
		assert.LessOrEqual(t, tags[i-1].Name, tags[i].Name,
			"tags should be alphabetical: %q should come before %q", tags[i-1].Name, tags[i].Name)
	}
}

// TestStatus exercises FR-3.4.1 and FR-3.5.1.
func TestStatus(t *testing.T) {
	ctx := context.Background()
	fs := setupFullStack(t, nil)

	// Create grimoire.md so it can be refreshed.
	docPath := fs.b.DocPath()
	require.NoError(t, os.WriteFile(docPath, []byte("# placeholder\n"), 0o600))

	// Create 3 tracked files.
	createTrackedFile(ctx, t, fs.testStack, "tracked1.md", "# T1\n", "agent-a", []string{"tag1"}, "tracked one")
	createTrackedFile(ctx, t, fs.testStack, "tracked2.md", "# T2\n", "agent-a", []string{"tag1"}, "tracked two")
	createTrackedFile(ctx, t, fs.testStack, "tracked3.md", "# T3\n", "agent-b", []string{"tag2"}, "tracked three")

	// Create 1 untracked file (on disk but not in DB).
	createTestFile(t, fs.b, "untracked.md", "# Untracked\n")

	// Create 1 orphaned record (in DB but file deleted from disk).
	createTrackedFile(ctx, t, fs.testStack, "orphaned.md", "# Orphaned\n", "agent-c", []string{"orphan"}, "will be orphaned")
	orphanedPath := filepath.Join(fs.b.FilesDir(), "orphaned.md")
	require.NoError(t, os.Remove(orphanedPath))

	report, err := fs.svc.Status(ctx)
	require.NoError(t, err)

	// Total files on disk: tracked1 + tracked2 + tracked3 + untracked = 4.
	assert.Equal(t, 4, report.TotalFiles, "total files on disk")
	// Tracked files in DB: tracked1 + tracked2 + tracked3 + orphaned = 4.
	assert.Equal(t, 4, report.TrackedFiles, "tracked files in DB")
	// Orphaned: orphaned.md (in DB, not on disk).
	assert.Equal(t, 1, report.OrphanedCount, "orphaned count")
	// Untracked: untracked.md (on disk, not in DB).
	assert.Equal(t, 1, report.UntrackedCount, "untracked count")

	// Verify grimoire.md was refreshed (no longer placeholder).
	docContent, err := os.ReadFile(docPath)
	require.NoError(t, err)
	assert.NotEqual(t, "# placeholder\n", string(docContent), "grimoire.md should be refreshed")
	assert.Contains(t, string(docContent), "Grimoire", "doc should contain Grimoire heading")
}

// TestRebuild exercises FR-3.4.2: rebuild DB from ledger.
func TestRebuild(t *testing.T) {
	ctx := context.Background()
	fs := setupFullStack(t, nil)

	// Create files with metadata (populates ledger + DB).
	createTrackedFile(ctx, t, fs.testStack, "file1.md", "# F1\n", "agent-a", []string{"tag1", "tag2"}, "file one")
	createTrackedFile(ctx, t, fs.testStack, "file2.md", "# F2\n", "agent-b", []string{"tag3"}, "file two")

	// Verify DB has 2 records before corruption.
	count, err := fs.fileRepo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// "Corrupt" DB by dropping all tables.
	require.NoError(t, fs.db.DropAll())
	require.NoError(t, fs.db.EnsureSchema())

	// Verify DB is empty after corruption.
	count, err = fs.fileRepo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Rebuild from ledger.
	report, err := fs.svc.Rebuild(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, report.EntriesReplayed, "should replay 2 create entries")
	assert.Equal(t, 2, report.FinalRecordCount, "should have 2 records after rebuild")

	// Verify records are restored.
	meta1, err := fs.fileRepo.Get(ctx, "file1.md")
	require.NoError(t, err)
	assert.Equal(t, "agent-a", meta1.SourceAgent)
	assert.ElementsMatch(t, []string{"tag1", "tag2"}, meta1.Tags)

	meta2, err := fs.fileRepo.Get(ctx, "file2.md")
	require.NoError(t, err)
	assert.Equal(t, "agent-b", meta2.SourceAgent)
}

// TestHardRebuild_fullReconciliation exercises FR-3.4.3.
func TestHardRebuild_fullReconciliation(t *testing.T) {
	ctx := context.Background()
	// Use FakeProvider because HardRebuild generates embeddings for created/updated files.
	fakeEmb := embtest.NewFakeProvider()
	fs := setupFullStack(t, fakeEmb)

	// Create a tracked file with metadata.
	createTrackedFile(ctx, t, fs.testStack, "existing.md", "# Existing\n", "agent-a", []string{"tag1"}, "existing file")

	// Scenario 1: File with frontmatter on disk but no DB row (untracked).
	// Manually write a file with frontmatter and do NOT register it in DB.
	untrackedContent := "---\nsource_agent: agent-manual\ntags:\n  - manual\nsummary: manually added\ncreated_at: \"2025-01-01T00:00:00Z\"\nupdated_at: \"2025-01-01T00:00:00Z\"\n---\n# Manual File\n"
	createTestFile(t, fs.b, "manual.md", untrackedContent)

	// Scenario 2: DB row with no file on disk (orphaned).
	createTrackedFile(ctx, t, fs.testStack, "ghost.md", "# Ghost\n", "agent-a", []string{"ghost"}, "ghost file")
	ghostPath := filepath.Join(fs.b.FilesDir(), "ghost.md")
	require.NoError(t, os.Remove(ghostPath))

	// Scenario 3: Stale tags. Modify existing.md's frontmatter to differ from DB.
	// Update the existing file's frontmatter on disk to have different tags.
	staleContent := "---\nsource_agent: agent-a\ntags:\n  - tag1\n  - newtag\nsummary: updated summary\ncreated_at: \"" + time.Now().UTC().Format(time.RFC3339) + "\"\nupdated_at: \"" + time.Now().UTC().Format(time.RFC3339) + "\"\n---\n# Existing\n"
	existingPath := filepath.Join(fs.b.FilesDir(), "existing.md")
	require.NoError(t, os.WriteFile(existingPath, []byte(staleContent), 0o600))

	// Get ledger entry count before hard rebuild.
	entriesBefore, err := fs.led.ReadAll()
	require.NoError(t, err)
	countBefore := len(entriesBefore)

	report, err := fs.svc.HardRebuild(ctx)
	require.NoError(t, err)

	// Should have: 1 create (manual.md), 1 update (existing.md stale), 1 delete (ghost.md).
	assert.Equal(t, 1, report.Creates, "should create 1 untracked file")
	assert.Equal(t, 1, report.Updates, "should update 1 stale file")
	assert.Equal(t, 1, report.Deletes, "should delete 1 orphaned record")

	// Verify manual.md is now in DB.
	manualMeta, err := fs.fileRepo.Get(ctx, "manual.md")
	require.NoError(t, err)
	assert.Equal(t, "hard-rebuild", manualMeta.SourceAgent, "untracked file should get hard-rebuild as source agent")
	assert.Contains(t, manualMeta.Tags, "manual")

	// Verify ghost.md is no longer in DB.
	_, err = fs.fileRepo.Get(ctx, "ghost.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound))

	// Verify corrective ledger entries were appended.
	entriesAfter, err := fs.led.ReadAll()
	require.NoError(t, err)
	assert.Len(t, entriesAfter, countBefore+3, "should have 3 new corrective ledger entries")
}

// TestHardRebuild_lockBlocksMutations exercises FR-3.4.3 and NFR-6.2.
func TestHardRebuild_lockBlocksMutations(t *testing.T) {
	ctx := context.Background()
	fs := setupFullStack(t, nil)

	// Create a file to attempt creating metadata for later.
	createTestFile(t, fs.b, "blocked.md", "# Blocked\n")

	// Hold exclusive lock (simulating an ongoing hard rebuild) from a separate locker.
	exclusiveLocker, err := filelock.NewFlockLocker(fs.b.LockPath())
	require.NoError(t, err)
	t.Cleanup(func() {
		exclusiveLocker.Close()
	})

	acquired, err := exclusiveLocker.TryLockExclusive()
	require.NoError(t, err)
	require.True(t, acquired, "should acquire exclusive lock")

	// Attempt Manager.Create while exclusive lock is held.
	_, createErr := fs.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    "blocked.md",
		SourceAgent: "blocked-agent",
		Tags:        []string{"blocked"},
		Summary:     "this should fail",
	})

	require.Error(t, createErr)
	assert.True(t, sberrors.HasCode(createErr, sberrors.ErrCodeRebuildInProgress),
		"expected REBUILD_IN_PROGRESS, got: %v", createErr)

	// Release the exclusive lock.
	require.NoError(t, exclusiveLocker.UnlockExclusive())
}

// TestWriteOrderResilience exercises NFR-6.1: crash after frontmatter write but before DB write.
func TestWriteOrderResilience(t *testing.T) {
	ctx := context.Background()
	fs := setupFullStack(t, nil)

	relPath := "resilient.md"
	createTestFile(t, fs.b, relPath, "# Resilient\n")

	// Simulate the write-order scenario:
	// 1. Write frontmatter (via Manager.Create starts this).
	// 2. Ledger entry is appended.
	// 3. Crash before DB insert.
	//
	// We simulate this by doing a normal create, then "corrupting" the DB
	// by dropping tables and rebuilding from ledger.
	created, err := fs.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    relPath,
		SourceAgent: "crash-agent",
		Tags:        []string{"resilient"},
		Summary:     "test write order",
	})
	require.NoError(t, err)

	// Simulate crash: drop DB tables.
	require.NoError(t, fs.db.DropAll())
	require.NoError(t, fs.db.EnsureSchema())

	// Verify DB is empty.
	count, err := fs.fileRepo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Rebuild from ledger (simulates recovery after crash).
	report, err := fs.svc.Rebuild(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, report.EntriesReplayed)
	assert.Equal(t, 1, report.FinalRecordCount)

	// Verify the record is recovered and consistent.
	recovered, err := fs.fileRepo.Get(ctx, relPath)
	require.NoError(t, err)
	assert.Equal(t, created.SourceAgent, recovered.SourceAgent)
	assert.Equal(t, created.Summary, recovered.Summary)
	assert.ElementsMatch(t, created.Tags, recovered.Tags)
}
