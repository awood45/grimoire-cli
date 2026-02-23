package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/brain"
	embedtesting "github.com/awood45/grimoire-cli/internal/embedding/testing"
	flocktesting "github.com/awood45/grimoire-cli/internal/filelock/testing"
	fmtesting "github.com/awood45/grimoire-cli/internal/frontmatter/testing"
	ledgertesting "github.com/awood45/grimoire-cli/internal/ledger/testing"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
	storetesting "github.com/awood45/grimoire-cli/internal/store/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper creates a Manager with all fakes and a temp brain directory.
// Returns the Manager and all fakes for assertion/injection.
type testHarness struct {
	mgr      *Manager
	brain    *brain.Brain
	fileRepo *storetesting.FakeFileRepository
	embRepo  *storetesting.FakeEmbeddingRepository
	ledger   *ledgertesting.FakeLedger
	fm       *fmtesting.FakeFrontmatterService
	embedder *embedtesting.FakeProvider
	locker   *flocktesting.FakeLocker
	tmpDir   string
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()
	tmpDir := t.TempDir()

	b := brain.New(tmpDir)

	// Create the files/ and archive-files/ directories.
	require.NoError(t, os.MkdirAll(b.FilesDir(), 0o755))
	require.NoError(t, os.MkdirAll(b.ArchiveDir(), 0o755))

	fr := storetesting.NewFakeFileRepository()
	er := storetesting.NewFakeEmbeddingRepository()
	l := ledgertesting.NewFakeLedger()
	fm := fmtesting.NewFakeFrontmatterService()
	emb := embedtesting.NewFakeProvider()
	lock := flocktesting.NewFakeLocker()

	mgr := NewManager(b, fr, er, l, fm, emb, lock)

	return &testHarness{
		mgr:      mgr,
		brain:    b,
		fileRepo: fr,
		embRepo:  er,
		ledger:   l,
		fm:       fm,
		embedder: emb,
		locker:   lock,
		tmpDir:   tmpDir,
	}
}

// createTestFile creates a markdown file in the brain's files directory.
func (h *testHarness) createTestFile(t *testing.T, relPath, content string) string {
	t.Helper()
	absPath := filepath.Join(h.brain.FilesDir(), relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0o755))
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0o600))
	return absPath
}

// --- Create Tests ---

// TestCreate_success tests FR-3.2.1: basic create flow.
func TestCreate_success(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "notes/test.md", "# Test\n\nHello world")

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "notes/test.md",
		SourceAgent: "test-agent",
		Tags:        []string{"go", "testing"},
		Summary:     "A test file",
	}

	meta, err := h.mgr.Create(ctx, opts)
	require.NoError(t, err)

	// Verify returned metadata.
	assert.Equal(t, "notes/test.md", meta.Filepath)
	assert.Equal(t, "test-agent", meta.SourceAgent)
	assert.Equal(t, []string{"go", "testing"}, meta.Tags)
	assert.Equal(t, "A test file", meta.Summary)
	assert.False(t, meta.CreatedAt.IsZero())
	assert.Equal(t, meta.CreatedAt, meta.UpdatedAt)

	// Verify file repo was called.
	assert.Equal(t, []string{"notes/test.md"}, h.fileRepo.InsertCalls)

	// Verify frontmatter was written.
	assert.Len(t, h.fm.WriteCalls, 1)

	// Verify ledger was written.
	require.Len(t, h.ledger.Entries, 1)
	assert.Equal(t, "create", h.ledger.Entries[0].Operation)
	assert.Equal(t, "notes/test.md", h.ledger.Entries[0].Filepath)

	// Verify embedding was generated.
	assert.Len(t, h.embedder.GenerateCalls, 1)
	assert.Len(t, h.embRepo.UpsertCalls, 1)

	// Verify lock was acquired and released.
	assert.Equal(t, 1, h.locker.TrySharedCalls)
	assert.Equal(t, 1, h.locker.UnlockSharedCalls)
}

// TestCreate_fileNotFound tests FR-3.2.1: create with non-existent file.
func TestCreate_fileNotFound(t *testing.T) {
	h := newTestHarness(t)

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "nonexistent.md",
		SourceAgent: "test-agent",
		Tags:        []string{"go"},
	}

	_, err := h.mgr.Create(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeFileNotFound))
}

// TestCreate_metadataExists tests FR-3.2.1: create when metadata already exists.
func TestCreate_metadataExists(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "test.md", "# Test")

	// Pre-populate metadata.
	h.fileRepo.Data["test.md"] = store.FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "existing-agent",
	}

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "test.md",
		SourceAgent: "test-agent",
		Tags:        []string{"go"},
	}

	_, err := h.mgr.Create(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataExists))
}

// TestCreate_setsTimestamps tests FR-3.2.1: created_at = updated_at = now.
func TestCreate_setsTimestamps(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "time-test.md", "# Timestamps")

	before := time.Now().UTC().Add(-time.Second)
	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "time-test.md",
		SourceAgent: "test-agent",
		Tags:        []string{"go"},
	}

	meta, err := h.mgr.Create(ctx, opts)
	require.NoError(t, err)
	after := time.Now().UTC().Add(time.Second)

	assert.True(t, meta.CreatedAt.After(before), "created_at should be after start")
	assert.True(t, meta.CreatedAt.Before(after), "created_at should be before end")
	assert.Equal(t, meta.CreatedAt, meta.UpdatedAt, "created_at should equal updated_at")
}

// TestCreate_embeddingFailure tests FR-3.2.1: embedding failure does not block metadata creation.
func TestCreate_embeddingFailure(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "embed-fail.md", "# Embedding Failure")

	// Inject embedding error.
	h.embedder.GenerateErr = errors.New("ollama down")

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "embed-fail.md",
		SourceAgent: "test-agent",
		Tags:        []string{"go"},
	}

	meta, err := h.mgr.Create(ctx, opts)
	require.NoError(t, err)

	// Metadata should still be created.
	assert.Equal(t, "embed-fail.md", meta.Filepath)

	// Verify metadata was written to all stores.
	assert.Len(t, h.fm.WriteCalls, 1)
	assert.Len(t, h.fileRepo.InsertCalls, 1)
	require.Len(t, h.ledger.Entries, 1)

	// Embedding should NOT have been upserted.
	assert.Empty(t, h.embRepo.UpsertCalls)
}

// TestCreate_rebuildInProgress tests FR-3.2.1: lock contention.
func TestCreate_rebuildInProgress(t *testing.T) {
	h := newTestHarness(t)
	h.locker.TrySharedResult = false

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "test.md",
		SourceAgent: "test-agent",
		Tags:        []string{"go"},
	}

	_, err := h.mgr.Create(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeRebuildInProgress))

	// Verify no stores were touched.
	assert.Empty(t, h.fileRepo.InsertCalls)
	assert.Empty(t, h.fm.WriteCalls)
	assert.Empty(t, h.ledger.Entries)
}

// TestCreate_noEmbeddingProvider tests that noop embedder skips embedding generation.
func TestCreate_noEmbeddingProvider(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "noembed.md", "# No Embed")

	// Set the model ID to "none" to simulate NoopProvider.
	h.embedder.FixedModelID = "none"

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "noembed.md",
		SourceAgent: "test-agent",
		Tags:        []string{"go"},
	}

	meta, err := h.mgr.Create(ctx, opts)
	require.NoError(t, err)
	assert.Equal(t, "noembed.md", meta.Filepath)

	// No embedding calls should have been made.
	assert.Empty(t, h.embedder.GenerateCalls)
	assert.Empty(t, h.embRepo.UpsertCalls)
}

// TestCreate_ledgerPayload tests that the ledger entry has the correct payload.
func TestCreate_ledgerPayload(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "payload-test.md", "# Payload")

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "payload-test.md",
		SourceAgent: "test-agent",
		Tags:        []string{"go", "test"},
		Summary:     "Test summary",
	}

	_, err := h.mgr.Create(ctx, opts)
	require.NoError(t, err)

	require.Len(t, h.ledger.Entries, 1)
	entry := h.ledger.Entries[0]
	assert.Equal(t, "create", entry.Operation)
	assert.Equal(t, "payload-test.md", entry.Filepath)
	assert.Equal(t, "test-agent", entry.SourceAgent)

	// Unmarshal payload.
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(entry.Payload, &payload))
	assert.Equal(t, "test-agent", payload["source_agent"])
	assert.Equal(t, "Test summary", payload["summary"])
}

// --- Update Tests ---

// TestUpdate_success tests FR-3.2.2: basic update flow.
func TestUpdate_success(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "update.md", "# Update")

	// Pre-populate existing metadata.
	existing := store.FileMetadata{
		Filepath:    "update.md",
		SourceAgent: "original-agent",
		Tags:        []string{"old-tag"},
		Summary:     "Old summary",
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	h.fileRepo.Data["update.md"] = existing
	h.fm.Data[absPath] = existing

	newSummary := "New summary"
	ctx := context.Background()
	opts := UpdateOptions{
		Filepath:    "update.md",
		Tags:        []string{"new-tag"},
		SourceAgent: "new-agent",
		Summary:     &newSummary,
	}

	meta, err := h.mgr.Update(ctx, opts)
	require.NoError(t, err)

	assert.Equal(t, "update.md", meta.Filepath)
	assert.Equal(t, "new-agent", meta.SourceAgent)
	assert.Equal(t, []string{"new-tag"}, meta.Tags)
	assert.Equal(t, "New summary", meta.Summary)
	assert.Equal(t, existing.CreatedAt, meta.CreatedAt)
	assert.True(t, meta.UpdatedAt.After(existing.UpdatedAt))

	// Verify stores were updated.
	assert.Len(t, h.fileRepo.UpdateCalls, 1)
	assert.Len(t, h.fm.WriteCalls, 1)
	require.Len(t, h.ledger.Entries, 1)
	assert.Equal(t, "update", h.ledger.Entries[0].Operation)

	// Verify embedding was regenerated.
	assert.Len(t, h.embedder.GenerateCalls, 1)

	// Verify lock lifecycle.
	assert.Equal(t, 1, h.locker.TrySharedCalls)
	assert.Equal(t, 1, h.locker.UnlockSharedCalls)
}

// TestUpdate_tagsReplaced tests FR-3.2.2: tags are fully replaced.
func TestUpdate_tagsReplaced(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "tags.md", "# Tags")

	existing := store.FileMetadata{
		Filepath:    "tags.md",
		SourceAgent: "agent",
		Tags:        []string{"a", "b", "c"},
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	h.fileRepo.Data["tags.md"] = existing
	h.fm.Data[absPath] = existing

	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "tags.md",
		Tags:     []string{"x", "y"},
	}

	meta, err := h.mgr.Update(ctx, opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"x", "y"}, meta.Tags)
	// Agent shouldn't change when not provided.
	assert.Equal(t, "agent", meta.SourceAgent)
}

// TestUpdate_updatesTimestamp tests FR-3.2.2: updated_at is refreshed.
func TestUpdate_updatesTimestamp(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "ts.md", "# Timestamp")

	oldTime := time.Now().UTC().Add(-24 * time.Hour)
	existing := store.FileMetadata{
		Filepath:    "ts.md",
		SourceAgent: "agent",
		Tags:        []string{"old"},
		CreatedAt:   oldTime,
		UpdatedAt:   oldTime,
	}
	h.fileRepo.Data["ts.md"] = existing
	h.fm.Data[absPath] = existing

	before := time.Now().UTC().Add(-time.Second)
	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "ts.md",
		Tags:     []string{"new"},
	}

	meta, err := h.mgr.Update(ctx, opts)
	require.NoError(t, err)

	assert.Equal(t, oldTime, meta.CreatedAt, "created_at should not change")
	assert.True(t, meta.UpdatedAt.After(before), "updated_at should be refreshed")
}

// TestUpdate_noChanges tests FR-3.2.2: no changes returns INVALID_INPUT.
func TestUpdate_noChanges(t *testing.T) {
	h := newTestHarness(t)

	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "test.md",
		// All optional fields are zero/nil — no changes requested.
	}

	_, err := h.mgr.Update(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}

// TestUpdate_rebuildInProgress tests FR-3.2.2: lock contention.
func TestUpdate_rebuildInProgress(t *testing.T) {
	h := newTestHarness(t)
	h.locker.TrySharedResult = false

	newSummary := "summary"
	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "test.md",
		Summary:  &newSummary,
	}

	_, err := h.mgr.Update(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeRebuildInProgress))
}

// TestUpdate_summaryCleared tests that passing a pointer to empty string clears summary.
func TestUpdate_summaryCleared(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "clear.md", "# Clear")

	existing := store.FileMetadata{
		Filepath:    "clear.md",
		SourceAgent: "agent",
		Tags:        []string{"a"},
		Summary:     "Has a summary",
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	h.fileRepo.Data["clear.md"] = existing
	h.fm.Data[absPath] = existing

	empty := ""
	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "clear.md",
		Summary:  &empty,
	}

	meta, err := h.mgr.Update(ctx, opts)
	require.NoError(t, err)
	assert.Equal(t, "", meta.Summary, "summary should be cleared")
}

// TestUpdate_notFound tests that updating non-existent metadata returns METADATA_NOT_FOUND.
func TestUpdate_notFound(t *testing.T) {
	h := newTestHarness(t)

	newSummary := "summary"
	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "nonexistent.md",
		Summary:  &newSummary,
	}

	_, err := h.mgr.Update(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound))
}

// --- Get Tests ---

// TestGet_success tests FR-3.2.3: basic get flow.
func TestGet_success(t *testing.T) {
	h := newTestHarness(t)

	expected := store.FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
		Summary:     "A test",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.fileRepo.Data["test.md"] = expected

	ctx := context.Background()
	meta, err := h.mgr.Get(ctx, "test.md")
	require.NoError(t, err)
	assert.Equal(t, expected, meta)

	// Get should not acquire a lock.
	assert.Equal(t, 0, h.locker.TrySharedCalls)
}

// TestGet_notFound tests FR-3.2.3: get for non-existent metadata.
func TestGet_notFound(t *testing.T) {
	h := newTestHarness(t)

	ctx := context.Background()
	_, err := h.mgr.Get(ctx, "nonexistent.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound))
}

// --- Delete Tests ---

// TestDelete_success tests FR-3.2.4: basic delete flow (file still exists).
func TestDelete_success(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "delete-me.md", "# Delete Me")

	existing := store.FileMetadata{
		Filepath:    "delete-me.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.fileRepo.Data["delete-me.md"] = existing

	ctx := context.Background()
	err := h.mgr.Delete(ctx, "delete-me.md")
	require.NoError(t, err)

	// Verify file repo delete was called.
	assert.Equal(t, []string{"delete-me.md"}, h.fileRepo.DeleteCalls)

	// Verify ledger was written.
	require.Len(t, h.ledger.Entries, 1)
	assert.Equal(t, "delete", h.ledger.Entries[0].Operation)

	// Verify frontmatter was removed (file exists on disk).
	assert.Len(t, h.fm.RemoveCalls, 1)

	// Verify lock lifecycle.
	assert.Equal(t, 1, h.locker.TrySharedCalls)
	assert.Equal(t, 1, h.locker.UnlockSharedCalls)
}

// TestDelete_removeFrontmatter tests FR-3.2.4: frontmatter removal when file exists.
func TestDelete_removeFrontmatter(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "with-fm.md", "---\nsource_agent: test\n---\n# Content")

	h.fileRepo.Data["with-fm.md"] = store.FileMetadata{
		Filepath:    "with-fm.md",
		SourceAgent: "test",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	ctx := context.Background()
	err := h.mgr.Delete(ctx, "with-fm.md")
	require.NoError(t, err)

	// Frontmatter Remove should be called with the absolute path.
	require.Len(t, h.fm.RemoveCalls, 1)
	assert.Equal(t, absPath, h.fm.RemoveCalls[0])
}

// TestDelete_fileGone tests FR-3.2.4: delete when file is gone from disk.
func TestDelete_fileGone(t *testing.T) {
	h := newTestHarness(t)
	// Don't create the file on disk, but metadata exists in repo.

	h.fileRepo.Data["gone.md"] = store.FileMetadata{
		Filepath:    "gone.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	ctx := context.Background()
	err := h.mgr.Delete(ctx, "gone.md")
	require.NoError(t, err)

	// Frontmatter Remove should NOT be called (file doesn't exist).
	assert.Empty(t, h.fm.RemoveCalls)

	// DB delete should still proceed.
	assert.Equal(t, []string{"gone.md"}, h.fileRepo.DeleteCalls)

	// Ledger should still be written.
	require.Len(t, h.ledger.Entries, 1)
}

// TestDelete_rebuildInProgress tests FR-3.2.4: lock contention.
func TestDelete_rebuildInProgress(t *testing.T) {
	h := newTestHarness(t)
	h.locker.TrySharedResult = false

	ctx := context.Background()
	err := h.mgr.Delete(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeRebuildInProgress))
}

// TestDelete_notFound tests that deleting non-existent metadata returns error.
func TestDelete_notFound(t *testing.T) {
	h := newTestHarness(t)

	ctx := context.Background()
	err := h.mgr.Delete(ctx, "nonexistent.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound))
}

// --- Archive Tests ---

// TestArchive_success tests FR-3.2.5: basic archive flow.
func TestArchive_success(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "archive-me.md", "# Archive Me")

	existing := store.FileMetadata{
		Filepath:    "archive-me.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
		Summary:     "To be archived",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.fileRepo.Data["archive-me.md"] = existing

	ctx := context.Background()
	result, err := h.mgr.Archive(ctx, "archive-me.md")
	require.NoError(t, err)

	assert.Equal(t, "archive-me.md", result.OriginalPath)
	assert.Equal(t, "archive-me.md", result.ArchivePath)
	assert.Equal(t, existing.Filepath, result.Metadata.Filepath)

	// Verify source file was moved.
	srcAbs := filepath.Join(h.brain.FilesDir(), "archive-me.md")
	_, statErr := os.Stat(srcAbs)
	assert.True(t, os.IsNotExist(statErr), "source file should be gone")

	dstAbs := filepath.Join(h.brain.ArchiveDir(), "archive-me.md")
	_, statErr = os.Stat(dstAbs)
	require.NoError(t, statErr, "destination file should exist")

	// Verify frontmatter was removed.
	assert.Len(t, h.fm.RemoveCalls, 1)

	// Verify ledger was written.
	require.Len(t, h.ledger.Entries, 1)
	assert.Equal(t, "archive", h.ledger.Entries[0].Operation)

	// Verify DB records were cleaned up.
	assert.Equal(t, []string{"archive-me.md"}, h.embRepo.DeleteCalls)
	assert.Equal(t, []string{"archive-me.md"}, h.fileRepo.DeleteCalls)

	// Verify lock lifecycle.
	assert.Equal(t, 1, h.locker.TrySharedCalls)
	assert.Equal(t, 1, h.locker.UnlockSharedCalls)
}

// TestArchive_createsIntermediateDir tests FR-3.2.5: intermediate dirs are created.
func TestArchive_createsIntermediateDir(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "sub/dir/nested.md", "# Nested")

	existing := store.FileMetadata{
		Filepath:    "sub/dir/nested.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.fileRepo.Data["sub/dir/nested.md"] = existing

	ctx := context.Background()
	result, err := h.mgr.Archive(ctx, "sub/dir/nested.md")
	require.NoError(t, err)

	assert.Equal(t, "sub/dir/nested.md", result.ArchivePath)

	// Verify intermediate dirs were created in archive.
	dstAbs := filepath.Join(h.brain.ArchiveDir(), "sub", "dir", "nested.md")
	_, statErr := os.Stat(dstAbs)
	assert.NoError(t, statErr, "nested archive file should exist")
}

// TestArchive_noMetadata tests FR-3.2.5: archive with no existing metadata.
func TestArchive_noMetadata(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "no-meta.md", "# No Metadata")

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "no-meta.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound))
}

// TestArchive_fileNotFound tests FR-3.2.5: archive when file doesn't exist.
func TestArchive_fileNotFound(t *testing.T) {
	h := newTestHarness(t)
	// Metadata exists but file is gone.

	h.fileRepo.Data["missing.md"] = store.FileMetadata{
		Filepath:    "missing.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "missing.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeFileNotFound))
}

// TestArchive_rebuildInProgress tests FR-3.2.5: lock contention.
func TestArchive_rebuildInProgress(t *testing.T) {
	h := newTestHarness(t)
	h.locker.TrySharedResult = false

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "test.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeRebuildInProgress))
}

// TestArchive_ledgerPayload tests that the archive ledger contains original metadata.
func TestArchive_ledgerPayload(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "archive-payload.md", "# Archive Payload")

	existing := store.FileMetadata{
		Filepath:    "archive-payload.md",
		SourceAgent: "agent",
		Tags:        []string{"go", "test"},
		Summary:     "Original summary",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.fileRepo.Data["archive-payload.md"] = existing

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "archive-payload.md")
	require.NoError(t, err)

	require.Len(t, h.ledger.Entries, 1)
	entry := h.ledger.Entries[0]
	assert.Equal(t, "archive", entry.Operation)

	// Verify the payload contains original metadata.
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(entry.Payload, &payload))
	assert.Equal(t, "agent", payload["source_agent"])
	assert.Equal(t, "Original summary", payload["summary"])
	assert.Contains(t, payload, "archived_to")
}

// TestNewManager_constructor tests that NewManager sets all fields.
func TestNewManager_constructor(t *testing.T) {
	b := brain.New("/tmp/test")
	fr := storetesting.NewFakeFileRepository()
	er := storetesting.NewFakeEmbeddingRepository()
	l := ledgertesting.NewFakeLedger()
	fm := fmtesting.NewFakeFrontmatterService()
	emb := embedtesting.NewFakeProvider()
	lock := flocktesting.NewFakeLocker()

	mgr := NewManager(b, fr, er, l, fm, emb, lock)
	assert.NotNil(t, mgr)
}

// TestCreate_lockError tests Create when TryLockShared returns an error.
func TestCreate_lockError(t *testing.T) {
	h := newTestHarness(t)
	h.locker.TrySharedErr = errors.New("lock error")

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "test.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
	}

	_, err := h.mgr.Create(ctx, opts)
	require.Error(t, err)
}

// TestUpdate_embeddingFailure tests that embedding failure during update does not block.
func TestUpdate_embeddingFailure(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "update-emb-fail.md", "# Update Embed Fail")

	existing := store.FileMetadata{
		Filepath:    "update-emb-fail.md",
		SourceAgent: "agent",
		Tags:        []string{"old"},
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	h.fileRepo.Data["update-emb-fail.md"] = existing
	h.fm.Data[absPath] = existing

	h.embedder.GenerateErr = errors.New("ollama down")

	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "update-emb-fail.md",
		Tags:     []string{"new"},
	}

	meta, err := h.mgr.Update(ctx, opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"new"}, meta.Tags)

	// Metadata should still be updated.
	assert.Len(t, h.fileRepo.UpdateCalls, 1)
}

// TestUpdate_noEmbeddingProvider tests that noop embedder skips re-embedding on update.
func TestUpdate_noEmbeddingProvider(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "noembed-update.md", "# No Embed Update")

	h.embedder.FixedModelID = "none"

	existing := store.FileMetadata{
		Filepath:    "noembed-update.md",
		SourceAgent: "agent",
		Tags:        []string{"old"},
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	h.fileRepo.Data["noembed-update.md"] = existing
	h.fm.Data[absPath] = existing

	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "noembed-update.md",
		Tags:     []string{"new"},
	}

	_, err := h.mgr.Update(ctx, opts)
	require.NoError(t, err)
	assert.Empty(t, h.embedder.GenerateCalls)
}

// TestDelete_lockError tests Delete when TryLockShared returns an error.
func TestDelete_lockError(t *testing.T) {
	h := newTestHarness(t)
	h.locker.TrySharedErr = errors.New("lock error")

	ctx := context.Background()
	err := h.mgr.Delete(ctx, "test.md")
	require.Error(t, err)
}

// TestArchive_lockError tests Archive when TryLockShared returns an error.
func TestArchive_lockError(t *testing.T) {
	h := newTestHarness(t)
	h.locker.TrySharedErr = errors.New("lock error")

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "test.md")
	require.Error(t, err)
}

// TestUpdate_lockError tests Update when TryLockShared returns an error.
func TestUpdate_lockError(t *testing.T) {
	h := newTestHarness(t)
	h.locker.TrySharedErr = errors.New("lock error")

	newSummary := "summary"
	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "test.md",
		Summary:  &newSummary,
	}

	_, err := h.mgr.Update(ctx, opts)
	require.Error(t, err)
}

// TestCreate_frontmatterWriteError tests Create when frontmatter.Write fails.
func TestCreate_frontmatterWriteError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "fm-err.md", "# FM Error")
	h.fm.WriteErr = sberrors.New(sberrors.ErrCodeInternalError, "write failed")

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "fm-err.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
	}

	_, err := h.mgr.Create(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestCreate_ledgerAppendError tests Create when ledger.Append fails.
func TestCreate_ledgerAppendError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "ledger-err.md", "# Ledger Error")
	h.ledger.AppendErr = sberrors.New(sberrors.ErrCodeLedgerError, "append failed")

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "ledger-err.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
	}

	_, err := h.mgr.Create(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}

// TestCreate_insertError tests Create when fileRepo.Insert fails.
func TestCreate_insertError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "insert-err.md", "# Insert Error")
	h.fileRepo.InsertErr = sberrors.New(sberrors.ErrCodeDatabaseError, "insert failed")

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "insert-err.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
	}

	_, err := h.mgr.Create(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestCreate_getUnexpectedError tests Create when fileRepo.Get returns unexpected error.
func TestCreate_getUnexpectedError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "get-err.md", "# Get Error")
	h.fileRepo.GetErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db failed")

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "get-err.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
	}

	_, err := h.mgr.Create(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestUpdate_frontmatterWriteError tests Update when frontmatter.Write fails.
func TestUpdate_frontmatterWriteError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "upd-fm-err.md", "# Update FM Error")

	existing := store.FileMetadata{
		Filepath:    "upd-fm-err.md",
		SourceAgent: "agent",
		Tags:        []string{"old"},
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	h.fileRepo.Data["upd-fm-err.md"] = existing
	h.fm.WriteErr = sberrors.New(sberrors.ErrCodeInternalError, "write failed")

	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "upd-fm-err.md",
		Tags:     []string{"new"},
	}

	_, err := h.mgr.Update(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestUpdate_ledgerAppendError tests Update when ledger.Append fails.
func TestUpdate_ledgerAppendError(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "upd-led-err.md", "# Update Ledger Error")

	existing := store.FileMetadata{
		Filepath:    "upd-led-err.md",
		SourceAgent: "agent",
		Tags:        []string{"old"},
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	h.fileRepo.Data["upd-led-err.md"] = existing
	h.fm.Data[absPath] = existing
	h.ledger.AppendErr = sberrors.New(sberrors.ErrCodeLedgerError, "append failed")

	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "upd-led-err.md",
		Tags:     []string{"new"},
	}

	_, err := h.mgr.Update(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}

// TestUpdate_dbUpdateError tests Update when fileRepo.Update fails.
func TestUpdate_dbUpdateError(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "upd-db-err.md", "# Update DB Error")

	existing := store.FileMetadata{
		Filepath:    "upd-db-err.md",
		SourceAgent: "agent",
		Tags:        []string{"old"},
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	h.fileRepo.Data["upd-db-err.md"] = existing
	h.fm.Data[absPath] = existing
	h.fileRepo.UpdateErr = sberrors.New(sberrors.ErrCodeDatabaseError, "update failed")

	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "upd-db-err.md",
		Tags:     []string{"new"},
	}

	_, err := h.mgr.Update(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestUpdate_onlySourceAgent tests Update with only SourceAgent change.
func TestUpdate_onlySourceAgent(t *testing.T) {
	h := newTestHarness(t)
	absPath := h.createTestFile(t, "agent-only.md", "# Agent Only")

	existing := store.FileMetadata{
		Filepath:    "agent-only.md",
		SourceAgent: "old-agent",
		Tags:        []string{"a"},
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	h.fileRepo.Data["agent-only.md"] = existing
	h.fm.Data[absPath] = existing

	ctx := context.Background()
	opts := UpdateOptions{
		Filepath:    "agent-only.md",
		SourceAgent: "new-agent",
	}

	meta, err := h.mgr.Update(ctx, opts)
	require.NoError(t, err)
	assert.Equal(t, "new-agent", meta.SourceAgent)
	// Tags should remain unchanged.
	assert.Equal(t, []string{"a"}, meta.Tags)
}

// TestDelete_frontmatterRemoveError tests Delete when frontmatter.Remove fails.
func TestDelete_frontmatterRemoveError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "del-fm-err.md", "# Delete FM Error")

	h.fileRepo.Data["del-fm-err.md"] = store.FileMetadata{
		Filepath:    "del-fm-err.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.fm.RemoveErr = sberrors.New(sberrors.ErrCodeInternalError, "remove failed")

	ctx := context.Background()
	err := h.mgr.Delete(ctx, "del-fm-err.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestDelete_ledgerAppendError tests Delete when ledger.Append fails.
func TestDelete_ledgerAppendError(t *testing.T) {
	h := newTestHarness(t)
	// File does not exist on disk, so frontmatter.Remove is skipped.
	h.fileRepo.Data["del-led-err.md"] = store.FileMetadata{
		Filepath:    "del-led-err.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.ledger.AppendErr = sberrors.New(sberrors.ErrCodeLedgerError, "append failed")

	ctx := context.Background()
	err := h.mgr.Delete(ctx, "del-led-err.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}

// TestArchive_frontmatterRemoveError tests Archive when frontmatter.Remove returns a non-InvalidInput error.
func TestArchive_frontmatterRemoveError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "arch-fm-err.md", "# Archive FM Error")

	h.fileRepo.Data["arch-fm-err.md"] = store.FileMetadata{
		Filepath:    "arch-fm-err.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.fm.RemoveErr = sberrors.New(sberrors.ErrCodeInternalError, "remove failed")

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "arch-fm-err.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestArchive_ledgerAppendError tests Archive when ledger.Append fails.
func TestArchive_ledgerAppendError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "arch-led-err.md", "# Archive Ledger Error")

	h.fileRepo.Data["arch-led-err.md"] = store.FileMetadata{
		Filepath:    "arch-led-err.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.ledger.AppendErr = sberrors.New(sberrors.ErrCodeLedgerError, "append failed")

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "arch-led-err.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}

// TestArchive_embeddingDeleteError tests Archive when embRepo.Delete returns non-MetadataNotFound error.
func TestArchive_embeddingDeleteError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "arch-emb-err.md", "# Archive Emb Error")

	h.fileRepo.Data["arch-emb-err.md"] = store.FileMetadata{
		Filepath:    "arch-emb-err.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.embRepo.DeleteErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db error")

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "arch-emb-err.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestArchive_fileRepoDeleteError tests Archive when fileRepo.Delete fails.
func TestArchive_fileRepoDeleteError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "arch-db-err.md", "# Archive DB Error")

	h.fileRepo.Data["arch-db-err.md"] = store.FileMetadata{
		Filepath:    "arch-db-err.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	h.fileRepo.DeleteErr = sberrors.New(sberrors.ErrCodeDatabaseError, "delete failed")

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "arch-db-err.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestCreate_embeddingUpsertError tests Create when embedding upsert fails (fail open).
func TestCreate_embeddingUpsertError(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "upsert-err.md", "# Upsert Error")

	h.embRepo.UpsertErr = sberrors.New(sberrors.ErrCodeDatabaseError, "upsert failed")

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "upsert-err.md",
		SourceAgent: "agent",
		Tags:        []string{"go"},
	}

	meta, err := h.mgr.Create(ctx, opts)
	require.NoError(t, err)
	assert.Equal(t, "upsert-err.md", meta.Filepath)

	// Embedding generation was attempted but upsert failed silently.
	assert.Len(t, h.embedder.GenerateCalls, 1)
}

// TestCreate_pathTraversal tests Create with a path traversal attempt.
func TestCreate_pathTraversal(t *testing.T) {
	h := newTestHarness(t)

	ctx := context.Background()
	opts := CreateOptions{
		Filepath:    "../../../etc/passwd",
		SourceAgent: "agent",
		Tags:        []string{"go"},
	}

	_, err := h.mgr.Create(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodePathTraversal))
}

// TestUpdate_pathTraversal tests Update with a path traversal attempt.
func TestUpdate_pathTraversal(t *testing.T) {
	h := newTestHarness(t)

	// Pre-populate metadata so validation passes.
	h.fileRepo.Data["../../../etc/passwd"] = store.FileMetadata{
		Filepath:    "../../../etc/passwd",
		SourceAgent: "agent",
		Tags:        []string{"old"},
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
		UpdatedAt:   time.Now().UTC().Add(-time.Hour),
	}

	ctx := context.Background()
	opts := UpdateOptions{
		Filepath: "../../../etc/passwd",
		Tags:     []string{"new"},
	}

	_, err := h.mgr.Update(ctx, opts)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodePathTraversal))
}

// TestArchive_pathTraversal tests Archive with a path traversal attempt.
func TestArchive_pathTraversal(t *testing.T) {
	h := newTestHarness(t)

	// Pre-populate metadata.
	h.fileRepo.Data["../../../etc/passwd"] = store.FileMetadata{
		Filepath:    "../../../etc/passwd",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	ctx := context.Background()
	_, err := h.mgr.Archive(ctx, "../../../etc/passwd")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodePathTraversal))
}

// TestArchive_resolveArchivePathError tests Archive when ResolveArchivePath fails.
func TestArchive_resolveArchivePathError(t *testing.T) {
	// Create a brain with a base path that has a symlink that escapes the archive dir.
	// We can trigger this by having a filepath that passes ResolveFilePath
	// but would fail ResolveArchivePath. This is tricky so we test with
	// a brain where archive dir doesn't allow the path.
	tmpDir := t.TempDir()

	b := brain.New(tmpDir)
	require.NoError(t, os.MkdirAll(b.FilesDir(), 0o755))
	// Create the archive dir as a file (not directory) to make MkdirAll fail.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "archive-files"), []byte("not a dir"), 0o600))

	fr := storetesting.NewFakeFileRepository()
	er := storetesting.NewFakeEmbeddingRepository()
	l := ledgertesting.NewFakeLedger()
	fm := fmtesting.NewFakeFrontmatterService()
	emb := embedtesting.NewFakeProvider()
	lock := flocktesting.NewFakeLocker()

	mgr := NewManager(b, fr, er, l, fm, emb, lock)

	// Create file in files/ dir.
	absPath := filepath.Join(b.FilesDir(), "test.md")
	require.NoError(t, os.WriteFile(absPath, []byte("# Test"), 0o600))

	fr.Data["test.md"] = store.FileMetadata{
		Filepath:    "test.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	ctx := context.Background()
	_, err := mgr.Archive(ctx, "test.md")
	require.Error(t, err)
	// Should fail at MkdirAll since archive-files is a file not a directory.
}

// TestArchive_frontmatterRemoveInvalidInput tests Archive when frontmatter.Remove returns InvalidInput (no frontmatter - tolerated).
func TestArchive_frontmatterRemoveInvalidInput(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "no-fm.md", "# No Frontmatter")

	h.fileRepo.Data["no-fm.md"] = store.FileMetadata{
		Filepath:    "no-fm.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	// InvalidInput from fm.Remove means no frontmatter found - should be tolerated.
	h.fm.RemoveErr = sberrors.New(sberrors.ErrCodeInvalidInput, "no frontmatter")

	ctx := context.Background()
	result, err := h.mgr.Archive(ctx, "no-fm.md")
	require.NoError(t, err)
	assert.Equal(t, "no-fm.md", result.OriginalPath)
}

// TestArchive_embeddingDeleteNotFound tests Archive when embedding doesn't exist (MetadataNotFound is ok).
func TestArchive_embeddingDeleteNotFound(t *testing.T) {
	h := newTestHarness(t)
	h.createTestFile(t, "arch-no-emb.md", "# Archive No Emb")

	h.fileRepo.Data["arch-no-emb.md"] = store.FileMetadata{
		Filepath:    "arch-no-emb.md",
		SourceAgent: "agent",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	// Set embedding delete to return MetadataNotFound (which should be tolerated).
	h.embRepo.DeleteErr = sberrors.New(sberrors.ErrCodeMetadataNotFound, "not found")

	ctx := context.Background()
	result, err := h.mgr.Archive(ctx, "arch-no-emb.md")
	require.NoError(t, err)
	assert.Equal(t, "arch-no-emb.md", result.OriginalPath)
}
