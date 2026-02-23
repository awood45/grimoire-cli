package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArchiveCommand_registration verifies the archive-file subcommand is registered (FR-3.2.5).
func TestArchiveCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "archive-file" {
			found = true
			break
		}
	}
	assert.True(t, found, "archive-file subcommand should be registered")
}

// TestArchiveCommand_flags verifies expected flags exist (FR-3.2.5).
func TestArchiveCommand_flags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "archive-file" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("file"))
}

// TestArchiveCommand_missingFile verifies --file flag is required (FR-3.2.5).
func TestArchiveCommand_missingFile(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"archive-file"})
	err := root.Execute()
	assert.Error(t, err)
}

// TestArchiveCommand_success verifies successful archive (FR-3.2.5).
func TestArchiveCommand_success(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create a test file and metadata.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "test-agent",
		"--tags", "notes"})
	require.NoError(t, root.Execute())

	// Now archive it.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"archive-file", "--path", basePath, "--file", "test.md"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	// File should be moved to archive.
	assert.NoFileExists(t, filepath.Join(basePath, "files", "test.md"))
	assert.FileExists(t, filepath.Join(basePath, "archive-files", "test.md"))
}

// TestArchiveCommand_notMarkdown verifies .md extension is required (FR-3.2.5).
func TestArchiveCommand_notMarkdown(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"archive-file", "--path", basePath, "--file", "test.txt"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_MARKDOWN", errObj["code"])
}

// TestArchiveCommand_notInitialized verifies error when brain is not initialized (FR-3.2.5).
func TestArchiveCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"archive-file", "--path", tmpDir, "--file", "test.md"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestArchiveCommand_metadataNotFound verifies error when file has no metadata (FR-3.2.5).
func TestArchiveCommand_metadataNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create a test file but no metadata.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"archive-file", "--path", basePath, "--file", "test.md"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "METADATA_NOT_FOUND", errObj["code"])
}
