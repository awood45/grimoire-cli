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

// TestSearchCommand_registration verifies the search subcommand is registered (FR-3.3.1).
func TestSearchCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "search" {
			found = true
			break
		}
	}
	assert.True(t, found, "search subcommand should be registered")
}

// TestSearchCommand_flags verifies expected flags exist (FR-3.3.1).
func TestSearchCommand_flags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "search" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("tag"))
	assert.NotNil(t, cmd.Flags().Lookup("any-tag"))
	assert.NotNil(t, cmd.Flags().Lookup("source-agent"))
	assert.NotNil(t, cmd.Flags().Lookup("after"))
	assert.NotNil(t, cmd.Flags().Lookup("before"))
	assert.NotNil(t, cmd.Flags().Lookup("summary-contains"))
	assert.NotNil(t, cmd.Flags().Lookup("limit"))
	assert.NotNil(t, cmd.Flags().Lookup("sort"))
}

// TestSearchCommand_notInitialized verifies error when brain is not initialized (FR-3.3.1).
func TestSearchCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"search", "--path", tmpDir})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestSearchCommand_successNoFilters verifies search returns results with no filters (FR-3.3.1).
func TestSearchCommand_successNoFilters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"search", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])
}

// TestSearchCommand_successWithTags verifies search with tag filter returns correct results (FR-3.3.1).
func TestSearchCommand_successWithTags(t *testing.T) {
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
		"--file", "test.md", "--source-agent", "test-agent",
		"--tags", "meeting-notes"})
	require.NoError(t, root.Execute())

	// Search by tag.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"search", "--path", basePath, "--tag", "meeting-notes"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)
}

// TestSearchCommand_successWithAnyTags verifies search with any-tag filter (FR-3.3.1).
func TestSearchCommand_successWithAnyTags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create two files with different tags.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "one.md"), []byte("# One"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "two.md"), []byte("# Two"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "one.md", "--source-agent", "agent", "--tags", "alpha"})
	require.NoError(t, root.Execute())

	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "two.md", "--source-agent", "agent", "--tags", "beta"})
	require.NoError(t, root2.Execute())

	// Search with any-tag.
	root3 := NewRootCommand()
	var buf3 bytes.Buffer
	root3.SetOut(&buf3)
	root3.SetArgs([]string{"search", "--path", basePath, "--any-tag", "alpha", "--any-tag", "beta"})
	require.NoError(t, root3.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf3.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 2)
}

// TestSearchCommand_successWithSourceAgent verifies search by source agent (FR-3.3.1).
func TestSearchCommand_successWithSourceAgent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "special-agent",
		"--tags", "notes"})
	require.NoError(t, root.Execute())

	// Search by source agent.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"search", "--path", basePath, "--source-agent", "special-agent"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)
}

// TestSearchCommand_successWithSummaryContains verifies search by summary substring (FR-3.3.1).
func TestSearchCommand_successWithSummaryContains(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "agent",
		"--tags", "notes", "--summary", "architecture design decisions"})
	require.NoError(t, root.Execute())

	// Search by summary substring.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"search", "--path", basePath, "--summary-contains", "design"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)
}

// TestSearchCommand_successWithLimit verifies search respects limit (FR-3.3.1).
func TestSearchCommand_successWithLimit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create multiple files.
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", name), []byte("# Test"), 0o600))

		r := NewRootCommand()
		var b bytes.Buffer
		r.SetOut(&b)
		r.SetArgs([]string{"create-file-metadata", "--path", basePath,
			"--file", name, "--source-agent", "agent", "--tags", "notes"})
		require.NoError(t, r.Execute())
	}

	// Search with limit 1.
	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"search", "--path", basePath, "--limit", "1"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)
}

// TestSearchCommand_invalidAfter verifies error for invalid --after format (FR-3.3.1).
func TestSearchCommand_invalidAfter(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"search", "--path", basePath, "--after", "not-a-date"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_INPUT", errObj["code"])
}

// TestSearchCommand_invalidBefore verifies error for invalid --before format (FR-3.3.1).
func TestSearchCommand_invalidBefore(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"search", "--path", basePath, "--before", "not-a-date"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_INPUT", errObj["code"])
}

// TestSearchCommand_successWithDateFilters verifies search with --after and --before (FR-3.3.1).
func TestSearchCommand_successWithDateFilters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "agent", "--tags", "notes"})
	require.NoError(t, root.Execute())

	// Search with date range that includes the file.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"search", "--path", basePath,
		"--after", "2000-01-01T00:00:00Z",
		"--before", "2099-12-31T23:59:59Z"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)
}

// TestSearchCommand_defaultLimitFromConfig verifies that limit defaults to config value (FR-3.3.1).
func TestSearchCommand_defaultLimitFromConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// The default config has search.default_limit = 50. Just verify no error.
	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"search", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])
}

// TestSearchCommand_sortFlag verifies search with --sort (FR-3.3.1).
func TestSearchCommand_sortFlag(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"search", "--path", basePath, "--sort", "filepath"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])
}

// TestSearchCommand_dbError verifies search handles DB errors gracefully (FR-3.3.1).
func TestSearchCommand_dbError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Drop the files table to cause a DB error during search.
	db, err := store.NewDB(filepath.Join(basePath, "db", "grimoire.sqlite"))
	require.NoError(t, err)
	_, execErr := db.SQLDB().Exec("DROP TABLE IF EXISTS file_tags; DROP TABLE IF EXISTS embeddings; DROP TABLE IF EXISTS files")
	require.NoError(t, execErr)
	require.NoError(t, db.Close())

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"search", "--path", basePath})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
}
