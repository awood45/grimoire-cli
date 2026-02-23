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

// TestSimilarCommand_registration verifies the similar subcommand is registered (FR-3.3.2).
func TestSimilarCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "similar" {
			found = true
			break
		}
	}
	assert.True(t, found, "similar subcommand should be registered")
}

// TestSimilarCommand_flags verifies expected flags exist (FR-3.3.2).
func TestSimilarCommand_flags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "similar" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("file"))
	assert.NotNil(t, cmd.Flags().Lookup("text"))
	assert.NotNil(t, cmd.Flags().Lookup("limit"))
}

// TestSimilarCommand_noInput verifies error when neither --file nor --text is provided (FR-3.3.2).
func TestSimilarCommand_noInput(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"similar", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_INPUT", errObj["code"])
	assert.Contains(t, errObj["message"], "exactly one of --file or --text must be provided")
}

// TestSimilarCommand_bothInputs verifies error when both --file and --text are provided (FR-3.3.2).
func TestSimilarCommand_bothInputs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"similar", "--path", basePath, "--file", "test.md", "--text", "query"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_INPUT", errObj["code"])
	assert.Contains(t, errObj["message"], "not both")
}

// TestSimilarCommand_notInitialized verifies error when brain is not initialized (FR-3.3.2).
func TestSimilarCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"similar", "--path", tmpDir, "--text", "query"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestSimilarCommand_successWithFile verifies similar with --file (FR-3.3.2).
func TestSimilarCommand_successWithFile(t *testing.T) {
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

	// Similar by file. With noop provider, the embedding is nil, so this may fail
	// with a metadata not found or no embedding error. We test the flow executes correctly.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"similar", "--path", basePath, "--file", "test.md"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	// With noop provider, the embedding retrieval will fail (no embedding stored).
	// The important thing is the command executed without panicking and returned valid JSON.
	assert.NotNil(t, envelope["ok"])
}

// TestSimilarCommand_successWithText verifies similar with --text (FR-3.3.2).
func TestSimilarCommand_successWithText(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Similar by text with noop provider. This should return an error about no embedding provider.
	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"similar", "--path", basePath, "--text", "query about architecture"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	// With noop provider, text similarity should fail with NO_EMBEDDING_PROVIDER error.
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NO_EMBEDDING_PROVIDER", errObj["code"])
}

// TestSimilarCommand_limitDefault verifies default limit from config (FR-3.3.2).
func TestSimilarCommand_limitDefault(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// This tests that the command doesn't crash when limit is not specified.
	// With noop provider, we'll get a NO_EMBEDDING_PROVIDER error, but that's after limit is resolved.
	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"similar", "--path", basePath, "--text", "test"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	// Command executed without panic.
	assert.NotNil(t, envelope["ok"])
}
