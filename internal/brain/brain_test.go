package brain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveFilePath_valid(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	filesDir := filepath.Join(base, "files", "notes")
	require.NoError(t, os.MkdirAll(filesDir, 0o755))

	resolved, err := b.ResolveFilePath("notes/meeting.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "files", "notes", "meeting.md"), resolved)
}

func TestResolveFilePath_traversal_dotdot(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	require.NoError(t, os.MkdirAll(filepath.Join(base, "files"), 0o755))

	_, err := b.ResolveFilePath("../../../etc/passwd")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodePathTraversal))
}

func TestResolveFilePath_traversal_symlink(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	filesDir := filepath.Join(base, "files")
	require.NoError(t, os.MkdirAll(filesDir, 0o755))

	// Create a symlink that escapes files/.
	outsideDir := t.TempDir()
	symlink := filepath.Join(filesDir, "escape")
	require.NoError(t, os.Symlink(outsideDir, symlink))

	_, err := b.ResolveFilePath("escape/secret.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodePathTraversal))
}

func TestResolveArchivePath_valid(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	archiveDir := filepath.Join(base, "archive-files", "notes")
	require.NoError(t, os.MkdirAll(archiveDir, 0o755))

	resolved, err := b.ResolveArchivePath("notes/old.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "archive-files", "notes", "old.md"), resolved)
}

func TestResolveArchivePath_traversal_dotdot(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	require.NoError(t, os.MkdirAll(filepath.Join(base, "archive-files"), 0o755))

	_, err := b.ResolveArchivePath("../../etc/passwd")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodePathTraversal))
}

func TestResolveArchivePath_traversal_symlink(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	archiveDir := filepath.Join(base, "archive-files")
	require.NoError(t, os.MkdirAll(archiveDir, 0o755))

	outsideDir := t.TempDir()
	symlink := filepath.Join(archiveDir, "escape")
	require.NoError(t, os.Symlink(outsideDir, symlink))

	_, err := b.ResolveArchivePath("escape/secret.md")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodePathTraversal))
}

func TestValidateMarkdown_valid(t *testing.T) {
	b := New("/dummy")

	err := b.ValidateMarkdown("notes/meeting.md")
	assert.NoError(t, err)
}

func TestValidateMarkdown_nonmd(t *testing.T) {
	b := New("/dummy")

	err := b.ValidateMarkdown("data.json")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeNotMarkdown))
}

func TestExists_initialized(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	require.NoError(t, os.MkdirAll(filepath.Join(base, "files"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "db"), 0o755))

	assert.True(t, b.Exists())
}

func TestExists_not_initialized(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	assert.False(t, b.Exists())
}

func TestExists_partiallyInitialized(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	// Only files/ exists, db/ does not.
	require.NoError(t, os.MkdirAll(filepath.Join(base, "files"), 0o755))
	assert.False(t, b.Exists())
}

func TestResolveFilePath_nonexistentBaseDir(t *testing.T) {
	// When base dir doesn't exist, symlink check is skipped but clean path is returned.
	b := New("/nonexistent/base")

	resolved, err := b.ResolveFilePath("notes/meeting.md")
	require.NoError(t, err)
	assert.Contains(t, resolved, "notes/meeting.md")
}

func TestResolveFilePath_nestedNewFile(t *testing.T) {
	// File doesn't exist yet but parent dir does — tests the parent eval path.
	base := t.TempDir()
	b := New(base)

	filesDir := filepath.Join(base, "files")
	require.NoError(t, os.MkdirAll(filesDir, 0o755))

	resolved, err := b.ResolveFilePath("newdir/newfile.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "files", "newdir", "newfile.md"), resolved)
}

func TestResolveFilePath_existingFile(t *testing.T) {
	// When the full file path exists, evalDeepest resolves the whole thing.
	base := t.TempDir()
	b := New(base)

	filePath := filepath.Join(base, "files", "notes")
	require.NoError(t, os.MkdirAll(filePath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(filePath, "existing.md"), []byte("hi"), 0o644))

	resolved, err := b.ResolveFilePath("notes/existing.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "files", "notes", "existing.md"), resolved)
}

func TestResolveArchivePath_nestedNewFile(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	archiveDir := filepath.Join(base, "archive-files")
	require.NoError(t, os.MkdirAll(archiveDir, 0o755))

	resolved, err := b.ResolveArchivePath("subdir/file.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "archive-files", "subdir", "file.md"), resolved)
}

func TestResolveFilePath_deeplyNested(t *testing.T) {
	base := t.TempDir()
	b := New(base)

	filesDir := filepath.Join(base, "files")
	require.NoError(t, os.MkdirAll(filesDir, 0o755))

	resolved, err := b.ResolveFilePath("a/b/c/deep.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "files", "a", "b", "c", "deep.md"), resolved)
}

func TestPathAccessors(t *testing.T) {
	base := "/home/user/.grimoire"
	b := New(base)

	assert.Equal(t, filepath.Join(base, "files"), b.FilesDir())
	assert.Equal(t, filepath.Join(base, "archive-files"), b.ArchiveDir())
	assert.Equal(t, filepath.Join(base, "db", "grimoire.sqlite"), b.DBPath())
	assert.Equal(t, filepath.Join(base, "ledger.jsonl"), b.LedgerPath())
	assert.Equal(t, filepath.Join(base, "config.yaml"), b.ConfigPath())
	assert.Equal(t, filepath.Join(base, "grimoire.md"), b.DocPath())
	assert.Equal(t, filepath.Join(base, ".lock"), b.LockPath())
}
