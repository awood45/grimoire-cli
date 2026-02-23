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

// TestListTagsCommand_registration verifies the list-tags subcommand is registered (FR-3.3.3).
func TestListTagsCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "list-tags" {
			found = true
			break
		}
	}
	assert.True(t, found, "list-tags subcommand should be registered")
}

// TestListTagsCommand_flags verifies expected flags exist (FR-3.3.3).
func TestListTagsCommand_flags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "list-tags" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)

	sortFlag := cmd.Flags().Lookup("sort")
	assert.NotNil(t, sortFlag)
	assert.Equal(t, "name", sortFlag.DefValue)
}

// TestListTagsCommand_notInitialized verifies error when brain is not initialized (FR-3.3.3).
func TestListTagsCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"list-tags", "--path", tmpDir})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestListTagsCommand_successEmpty verifies list-tags with no tags (FR-3.3.3).
func TestListTagsCommand_successEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"list-tags", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])
}

// TestListTagsCommand_successWithTags verifies list-tags returns tags with counts (FR-3.3.3).
func TestListTagsCommand_successWithTags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create files with tags.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "one.md"), []byte("# One"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "two.md"), []byte("# Two"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "one.md", "--source-agent", "agent",
		"--tags", "alpha", "--tags", "beta"})
	require.NoError(t, root.Execute())

	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "two.md", "--source-agent", "agent",
		"--tags", "alpha"})
	require.NoError(t, root2.Execute())

	// List tags sorted by name.
	root3 := NewRootCommand()
	var buf3 bytes.Buffer
	root3.SetOut(&buf3)
	root3.SetArgs([]string{"list-tags", "--path", basePath, "--sort", "name"})
	require.NoError(t, root3.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf3.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 2)
}

// TestListTagsCommand_sortByCount verifies list-tags sorted by count (FR-3.3.3).
func TestListTagsCommand_sortByCount(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create files with tags.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "one.md"), []byte("# One"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "two.md"), []byte("# Two"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "one.md", "--source-agent", "agent",
		"--tags", "common", "--tags", "rare"})
	require.NoError(t, root.Execute())

	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "two.md", "--source-agent", "agent",
		"--tags", "common"})
	require.NoError(t, root2.Execute())

	// List tags sorted by count.
	root3 := NewRootCommand()
	var buf3 bytes.Buffer
	root3.SetOut(&buf3)
	root3.SetArgs([]string{"list-tags", "--path", basePath, "--sort", "count"})
	require.NoError(t, root3.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf3.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 2)

	// First entry should be "common" with count 2.
	first, ok := data[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "common", first["Name"])
	assert.Equal(t, 2, jsonIntVal(first["Count"]))
}

// TestListTagsCommand_dbError verifies list-tags handles DB errors gracefully (FR-3.3.3).
func TestListTagsCommand_dbError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Drop the file_tags table to cause a DB error during list-tags.
	db, err := store.NewDB(filepath.Join(basePath, "db", "grimoire.sqlite"))
	require.NoError(t, err)
	_, execErr := db.SQLDB().Exec("DROP TABLE IF EXISTS file_tags")
	require.NoError(t, execErr)
	require.NoError(t, db.Close())

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"list-tags", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
}
