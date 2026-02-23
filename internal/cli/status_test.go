package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/awood45/grimoire-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStatusCommand_registration verifies the status subcommand is registered (FR-3.4.1).
func TestStatusCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "status" {
			found = true
			break
		}
	}
	assert.True(t, found, "status subcommand should be registered")
}

// TestStatusCommand_notInitialized verifies error when brain is not initialized (FR-3.4.1).
func TestStatusCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"status", "--path", tmpDir})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestStatusCommand_successEmpty verifies status on an empty brain (FR-3.4.1).
func TestStatusCommand_successEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"status", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 0, jsonIntVal(data["TotalFiles"]))
	assert.Equal(t, 0, jsonIntVal(data["TrackedFiles"]))
	assert.Equal(t, 0, jsonIntVal(data["OrphanedCount"]))
	assert.Equal(t, 0, jsonIntVal(data["UntrackedCount"]))
}

// TestStatusCommand_successWithFiles verifies status with tracked files (FR-3.4.1).
func TestStatusCommand_successWithFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create files and metadata.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "tracked.md"), []byte("# Tracked"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "untracked.md"), []byte("# Untracked"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "tracked.md", "--source-agent", "agent", "--tags", "notes"})
	require.NoError(t, root.Execute())

	// Now run status.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"status", "--path", basePath})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, jsonIntVal(data["TotalFiles"]))
	assert.Equal(t, 1, jsonIntVal(data["TrackedFiles"]))
	assert.Equal(t, 1, jsonIntVal(data["UntrackedCount"]))
}

// TestStatusCommand_reportsEmbeddingStatus verifies embedding status is reported (FR-3.4.1).
func TestStatusCommand_reportsEmbeddingStatus(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"status", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	// NoopProvider returns "none" as model ID.
	assert.NotEmpty(t, data["EmbeddingStatus"])
}

// TestStatusCommand_dbError verifies status handles DB errors gracefully (FR-3.4.1).
func TestStatusCommand_dbError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Drop the files table to cause a DB error during status.
	db, err := store.NewDB(filepath.Join(basePath, "db", "grimoire.sqlite"))
	require.NoError(t, err)
	_, execErr := db.SQLDB().Exec("DROP TABLE IF EXISTS file_tags; DROP TABLE IF EXISTS embeddings; DROP TABLE IF EXISTS files")
	require.NoError(t, execErr)
	require.NoError(t, db.Close())

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"status", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
}

// TestStatusCommand_reportsLedgerEntries verifies ledger entry count is reported (FR-3.4.1).
func TestStatusCommand_reportsLedgerEntries(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create a file to generate ledger entries.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "agent", "--tags", "notes"})
	require.NoError(t, root.Execute())

	// Now run status.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"status", "--path", basePath})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	// At least 1 ledger entry from the create.
	assert.GreaterOrEqual(t, jsonIntVal(data["LedgerEntries"]), 1)
}
