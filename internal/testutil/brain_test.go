package testutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/awood45/grimoire-cli/internal/store"
	"github.com/awood45/grimoire-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestBrain_createsDirectoryStructure(t *testing.T) {
	b, cleanup := testutil.NewTestBrain(t)
	defer cleanup()

	// Verify files/ directory exists.
	info, err := os.Stat(b.FilesDir())
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify archive-files/ directory exists.
	info, err = os.Stat(b.ArchiveDir())
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify db/ directory exists.
	info, err = os.Stat(filepath.Dir(b.DBPath()))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestNewTestBrain_createsConfigFile(t *testing.T) {
	b, cleanup := testutil.NewTestBrain(t)
	defer cleanup()

	// Verify config.yaml exists and is non-empty.
	data, err := os.ReadFile(b.ConfigPath())
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestNewTestBrain_createsLedgerFile(t *testing.T) {
	b, cleanup := testutil.NewTestBrain(t)
	defer cleanup()

	// Verify ledger.jsonl exists (empty is OK).
	_, err := os.Stat(b.LedgerPath())
	require.NoError(t, err)
}

func TestNewTestBrain_initializesSQLiteDatabase(t *testing.T) {
	b, cleanup := testutil.NewTestBrain(t)
	defer cleanup()

	// Verify the database file exists.
	_, err := os.Stat(b.DBPath())
	require.NoError(t, err)

	// Open the DB independently and verify schema version.
	db, err := store.NewDB(b.DBPath())
	require.NoError(t, err)
	defer db.Close()

	err = db.CheckVersion(store.SchemaVersion)
	assert.NoError(t, err)
}

func TestNewTestBrain_brainExistsReturnsTrue(t *testing.T) {
	b, cleanup := testutil.NewTestBrain(t)
	defer cleanup()

	assert.True(t, b.Exists())
}

func TestNewTestBrain_cleanupClosesDB(t *testing.T) {
	// Just verify cleanup doesn't panic.
	_, cleanup := testutil.NewTestBrain(t)
	cleanup()
}

func TestCreateTestFile_writesFileContent(t *testing.T) {
	b, cleanup := testutil.NewTestBrain(t)
	defer cleanup()

	testutil.CreateTestFile(t, b, "notes/hello.md", "# Hello World\n")

	data, err := os.ReadFile(filepath.Join(b.FilesDir(), "notes", "hello.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Hello World\n", string(data))
}

func TestCreateTestFile_createsIntermediateDirectories(t *testing.T) {
	b, cleanup := testutil.NewTestBrain(t)
	defer cleanup()

	testutil.CreateTestFile(t, b, "deep/nested/dir/file.md", "content")

	// Verify intermediate directories were created.
	info, err := os.Stat(filepath.Join(b.FilesDir(), "deep", "nested", "dir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestCreateTestFile_writesAtTopLevel(t *testing.T) {
	b, cleanup := testutil.NewTestBrain(t)
	defer cleanup()

	testutil.CreateTestFile(t, b, "simple.md", "simple content")

	data, err := os.ReadFile(filepath.Join(b.FilesDir(), "simple.md"))
	require.NoError(t, err)
	assert.Equal(t, "simple content", string(data))
}

func TestCreateTestFile_overwritesExistingFile(t *testing.T) {
	b, cleanup := testutil.NewTestBrain(t)
	defer cleanup()

	testutil.CreateTestFile(t, b, "existing.md", "original")
	testutil.CreateTestFile(t, b, "existing.md", "updated")

	data, err := os.ReadFile(filepath.Join(b.FilesDir(), "existing.md"))
	require.NoError(t, err)
	assert.Equal(t, "updated", string(data))
}
