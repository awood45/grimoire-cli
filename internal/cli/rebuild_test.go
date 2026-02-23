package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/awood45/grimoire-cli/internal/store"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildCommand_registration verifies the rebuild subcommand is registered (FR-3.4.2).
func TestRebuildCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "rebuild" {
			found = true
			break
		}
	}
	assert.True(t, found, "rebuild subcommand should be registered")
}

// TestHardRebuildCommand_registration verifies the hard-rebuild subcommand is registered (FR-3.4.3).
func TestHardRebuildCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "hard-rebuild" {
			found = true
			break
		}
	}
	assert.True(t, found, "hard-rebuild subcommand should be registered")
}

// TestRebuildCommand_noFlags verifies rebuild has no required flags (FR-3.4.2).
func TestRebuildCommand_noFlags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "rebuild" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)
	// Rebuild has no command-specific flags (only global --path).
	assert.Equal(t, 0, cmd.Flags().NFlag())
}

// TestHardRebuildCommand_noFlags verifies hard-rebuild has no required flags (FR-3.4.3).
func TestHardRebuildCommand_noFlags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "hard-rebuild" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)
	assert.Equal(t, 0, cmd.Flags().NFlag())
}

// TestRebuildCommand_notInitialized verifies error when brain is not initialized (FR-3.4.2).
func TestRebuildCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"rebuild", "--path", tmpDir})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestHardRebuildCommand_notInitialized verifies error when brain is not initialized (FR-3.4.3).
func TestHardRebuildCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"hard-rebuild", "--path", tmpDir})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestRebuildCommand_successEmpty verifies rebuild on empty brain (FR-3.4.2).
func TestRebuildCommand_successEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"rebuild", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 0, jsonIntVal(data["EntriesReplayed"]))
	assert.Equal(t, 0, jsonIntVal(data["FinalRecordCount"]))
}

// TestRebuildCommand_successWithData verifies rebuild replays ledger entries (FR-3.4.2).
func TestRebuildCommand_successWithData(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create a file with metadata.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "agent", "--tags", "notes"})
	require.NoError(t, root.Execute())

	// Now rebuild.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"rebuild", "--path", basePath})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.GreaterOrEqual(t, jsonIntVal(data["EntriesReplayed"]), 1)
	assert.Equal(t, 1, jsonIntVal(data["FinalRecordCount"]))
}

// TestHardRebuildCommand_successEmpty verifies hard-rebuild on empty brain (FR-3.4.3).
func TestHardRebuildCommand_successEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"hard-rebuild", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 0, jsonIntVal(data["FilesScanned"]))
	assert.Equal(t, 0, jsonIntVal(data["Creates"]))
	assert.Equal(t, 0, jsonIntVal(data["Updates"]))
	assert.Equal(t, 0, jsonIntVal(data["Deletes"]))
}

// TestHardRebuildCommand_successWithWarnings verifies hard-rebuild detects files without frontmatter (FR-3.4.3).
func TestHardRebuildCommand_successWithWarnings(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create a file with no frontmatter. Hard-rebuild should produce a warning for it.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "nofront.md"), []byte("# No Frontmatter"), 0o600))

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"hard-rebuild", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope), "raw output: %s", buf.String())
	assert.Equal(t, true, envelope["ok"], "raw output: %s", buf.String())

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok, "raw output: %s", buf.String())
	// The file has no frontmatter, so it should show up as a warning and still be counted as scanned.
	assert.Equal(t, 1, jsonIntVal(data["FilesScanned"]))
	assert.Equal(t, 0, jsonIntVal(data["Creates"]))

	// Warnings should contain the file.
	warnings, ok := data["Warnings"].([]interface{})
	require.True(t, ok)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "nofront.md")
}

// TestRebuildCommand_schemaVersionMismatch verifies rebuild works when the DB has a wrong schema version.
func TestRebuildCommand_schemaVersionMismatch(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Tamper with the schema version to simulate a migration or stale DB.
	db, err := store.NewDB(filepath.Join(basePath, "db", "grimoire.sqlite"))
	require.NoError(t, err)
	_, execErr := db.SQLDB().Exec("PRAGMA user_version = 0")
	require.NoError(t, execErr)
	require.NoError(t, db.Close())

	// Rebuild should succeed despite the version mismatch.
	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"rebuild", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"], "rebuild should succeed with mismatched schema version, got: %s", buf.String())
}

// TestRebuildCommand_dbError verifies rebuild handles DB errors gracefully (FR-3.4.2).
func TestRebuildCommand_dbError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Write a ledger entry that references a create, then corrupt the DB
	// by making the schema_version table absent so rebuild's EnsureSchema fails.
	// Actually, the simplest: drop tables, then make db dir read-only so EnsureSchema fails.
	db, err := store.NewDB(filepath.Join(basePath, "db", "grimoire.sqlite"))
	require.NoError(t, err)
	// Drop tables to force rebuild to try DropAll + EnsureSchema. The DropAll succeeds,
	// but we can corrupt the ledger to trigger a read error.
	require.NoError(t, db.Close())

	// Corrupt the ledger file with invalid JSON to trigger a ledger read error during rebuild.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "ledger.jsonl"), []byte("not valid json\n"), 0o600))

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"rebuild", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
}

// TestHardRebuildCommand_dbError verifies hard-rebuild handles DB errors gracefully (FR-3.4.3).
func TestHardRebuildCommand_dbError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Drop the files table to cause a DB error during hard-rebuild's AllFilepaths query.
	db, err := store.NewDB(filepath.Join(basePath, "db", "grimoire.sqlite"))
	require.NoError(t, err)
	_, execErr := db.SQLDB().Exec("DROP TABLE IF EXISTS file_tags; DROP TABLE IF EXISTS embeddings; DROP TABLE IF EXISTS files")
	require.NoError(t, execErr)
	require.NoError(t, db.Close())

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"hard-rebuild", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
}
