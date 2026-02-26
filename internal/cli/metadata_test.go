package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateMetadataCommand_registration verifies the create-file-metadata subcommand is registered (FR-3.2.1).
func TestCreateMetadataCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "create-file-metadata" {
			found = true
			break
		}
	}
	assert.True(t, found, "create-file-metadata subcommand should be registered")
}

// TestUpdateMetadataCommand_registration verifies the update-file-metadata subcommand is registered (FR-3.2.2).
func TestUpdateMetadataCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "update-file-metadata" {
			found = true
			break
		}
	}
	assert.True(t, found, "update-file-metadata subcommand should be registered")
}

// TestGetMetadataCommand_registration verifies the get-file-metadata subcommand is registered (FR-3.2.3).
func TestGetMetadataCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "get-file-metadata" {
			found = true
			break
		}
	}
	assert.True(t, found, "get-file-metadata subcommand should be registered")
}

// TestDeleteMetadataCommand_registration verifies the delete-file-metadata subcommand is registered (FR-3.2.4).
func TestDeleteMetadataCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "delete-file-metadata" {
			found = true
			break
		}
	}
	assert.True(t, found, "delete-file-metadata subcommand should be registered")
}

// TestCreateMetadataCommand_flags verifies expected flags exist (FR-3.2.1).
func TestCreateMetadataCommand_flags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "create-file-metadata" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("file"))
	assert.NotNil(t, cmd.Flags().Lookup("source-agent"))
	assert.NotNil(t, cmd.Flags().Lookup("tags"))
	assert.NotNil(t, cmd.Flags().Lookup("summary"))
	assert.NotNil(t, cmd.Flags().Lookup("summary-embedding-text"))
}

// TestUpdateMetadataCommand_flags verifies expected flags exist (FR-3.2.2).
func TestUpdateMetadataCommand_flags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "update-file-metadata" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("file"))
	assert.NotNil(t, cmd.Flags().Lookup("source-agent"))
	assert.NotNil(t, cmd.Flags().Lookup("tags"))
	assert.NotNil(t, cmd.Flags().Lookup("summary"))
	assert.NotNil(t, cmd.Flags().Lookup("summary-embedding-text"))
}

// TestGetMetadataCommand_flags verifies expected flags exist (FR-3.2.3).
func TestGetMetadataCommand_flags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "get-file-metadata" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("file"))
}

// TestDeleteMetadataCommand_flags verifies expected flags exist (FR-3.2.4).
func TestDeleteMetadataCommand_flags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Use == "delete-file-metadata" {
			cmd = c
			break
		}
	}
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("file"))
}

// TestCreateMetadataCommand_missingRequiredFlags verifies required flags are enforced (FR-3.2.1).
func TestCreateMetadataCommand_missingRequiredFlags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	// Missing required flags entirely.
	root.SetArgs([]string{"create-file-metadata"})
	err := root.Execute()
	// Cobra enforces required flags and returns an error.
	assert.Error(t, err)
}

// TestCreateMetadataCommand_invalidTags verifies tag validation in the CLI (FR-3.2.1).
func TestCreateMetadataCommand_invalidTags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create a test file.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "test-agent", "--tags", "INVALID"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_INPUT", errObj["code"])
}

// TestCreateMetadataCommand_invalidSourceAgent verifies source agent validation (FR-3.2.1).
func TestCreateMetadataCommand_invalidSourceAgent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "bad agent", "--tags", "valid"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_INPUT", errObj["code"])
}

// TestCreateMetadataCommand_notMarkdown verifies .md extension is required (FR-3.2.1).
func TestCreateMetadataCommand_notMarkdown(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.txt", "--source-agent", "agent", "--tags", "valid"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_MARKDOWN", errObj["code"])
}

// TestCreateMetadataCommand_pathTraversal verifies path traversal is blocked (FR-3.2.1).
func TestCreateMetadataCommand_pathTraversal(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "../../../etc/passwd.md", "--source-agent", "agent", "--tags", "valid"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "PATH_TRAVERSAL", errObj["code"])
}

// TestCreateMetadataCommand_success verifies successful metadata creation (FR-3.2.1).
func TestCreateMetadataCommand_success(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create a test file.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "test-agent",
		"--tags", "meeting-notes", "--tags", "project/backend",
		"--summary", "A test summary"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])
	assert.NotNil(t, envelope["data"])
}

// TestCreateMetadataCommand_withSummaryEmbeddingText verifies the --summary-embedding-text flag is accepted (FR-7).
func TestCreateMetadataCommand_withSummaryEmbeddingText(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Create a test file.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "test-agent",
		"--tags", "notes",
		"--summary-embedding-text", "This is a summary for embedding"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])
	assert.NotNil(t, envelope["data"])
}

// TestGetMetadataCommand_success verifies successful metadata retrieval (FR-3.2.3).
func TestGetMetadataCommand_success(t *testing.T) {
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

	// Now get it.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"get-file-metadata", "--path", basePath, "--file", "test.md"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test.md", data["Filepath"])
}

// TestGetMetadataCommand_missingFile verifies --file flag is required (FR-3.2.3).
func TestGetMetadataCommand_missingFile(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"get-file-metadata"})
	err := root.Execute()
	assert.Error(t, err)
}

// TestDeleteMetadataCommand_success verifies successful metadata deletion (FR-3.2.4).
func TestDeleteMetadataCommand_success(t *testing.T) {
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

	// Now delete it.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"delete-file-metadata", "--path", basePath, "--file", "test.md"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])
}

// TestUpdateMetadataCommand_success verifies successful metadata update (FR-3.2.2).
func TestUpdateMetadataCommand_success(t *testing.T) {
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

	// Now update it.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"update-file-metadata", "--path", basePath,
		"--file", "test.md", "--tags", "updated-tag", "--summary", "New summary"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	tags, ok := data["Tags"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, tags, "updated-tag")
}

// TestUpdateMetadataCommand_invalidTagsValidation verifies tag validation on update (FR-3.2.2).
func TestUpdateMetadataCommand_invalidTagsValidation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"update-file-metadata", "--path", basePath,
		"--file", "test.md", "--tags", "BAD TAG"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
}

// TestUpdateMetadataCommand_invalidSummary verifies summary validation on update (FR-3.2.2).
func TestUpdateMetadataCommand_invalidSummary(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	longSummary := strings.Repeat("a", 1025)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"update-file-metadata", "--path", basePath,
		"--file", "test.md", "--summary", longSummary})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
}

// TestCreateMetadataCommand_notInitialized verifies error when brain is not initialized (FR-3.2.1).
func TestCreateMetadataCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"create-file-metadata", "--path", tmpDir,
		"--file", "test.md", "--source-agent", "agent", "--tags", "valid"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestCreateMetadataCommand_fileNotFound verifies error when file does not exist on disk (FR-3.2.1).
func TestCreateMetadataCommand_fileNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	// Note: file does not exist, but path is valid .md within files/.
	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "nonexistent.md", "--source-agent", "agent", "--tags", "valid"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "FILE_NOT_FOUND", errObj["code"])
}

// TestUpdateMetadataCommand_metadataNotFound verifies error when metadata does not exist (FR-3.2.2).
func TestUpdateMetadataCommand_metadataNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"update-file-metadata", "--path", basePath,
		"--file", "nonexistent.md", "--tags", "valid"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "METADATA_NOT_FOUND", errObj["code"])
}

// TestCreateMetadataCommand_invalidSummary verifies summary validation on create (FR-3.2.1).
func TestCreateMetadataCommand_invalidSummary(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	require.NoError(t, os.WriteFile(filepath.Join(basePath, "files", "test.md"), []byte("# Test"), 0o600))

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"create-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "agent",
		"--tags", "valid", "--summary", strings.Repeat("a", 1025)})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_INPUT", errObj["code"])
}

// TestGetMetadataCommand_notInitialized verifies error when brain is not initialized (FR-3.2.3).
func TestGetMetadataCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"get-file-metadata", "--path", tmpDir, "--file", "test.md"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestGetMetadataCommand_notFound verifies error when metadata does not exist (FR-3.2.3).
func TestGetMetadataCommand_notFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"get-file-metadata", "--path", basePath, "--file", "nonexistent.md"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "METADATA_NOT_FOUND", errObj["code"])
}

// TestDeleteMetadataCommand_notInitialized verifies error when brain is not initialized (FR-3.2.4).
func TestDeleteMetadataCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"delete-file-metadata", "--path", tmpDir, "--file", "test.md"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestDeleteMetadataCommand_notFound verifies error when metadata does not exist (FR-3.2.4).
func TestDeleteMetadataCommand_notFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"delete-file-metadata", "--path", basePath, "--file", "nonexistent.md"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "METADATA_NOT_FOUND", errObj["code"])
}

// TestDeleteMetadataCommand_missingFileFlag verifies --file flag is required (FR-3.2.4).
func TestDeleteMetadataCommand_missingFileFlag(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"delete-file-metadata"})
	err := root.Execute()
	assert.Error(t, err)
}

// TestUpdateMetadataCommand_notInitialized verifies error when brain is not initialized (FR-3.2.2).
func TestUpdateMetadataCommand_notInitialized(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"update-file-metadata", "--path", tmpDir,
		"--file", "test.md", "--tags", "valid"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_INITIALIZED", errObj["code"])
}

// TestUpdateMetadataCommand_invalidSourceAgent verifies source agent validation on update (FR-3.2.2).
func TestUpdateMetadataCommand_invalidSourceAgent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"update-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "bad agent"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_INPUT", errObj["code"])
}

// TestUpdateMetadataCommand_notMarkdown verifies .md extension is required on update (FR-3.2.2).
func TestUpdateMetadataCommand_notMarkdown(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")
	setupGrimoire(t, basePath, nil)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"update-file-metadata", "--path", basePath,
		"--file", "test.txt", "--tags", "valid"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])
	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "NOT_MARKDOWN", errObj["code"])
}

// TestUpdateMetadataCommand_missingFileFlag verifies --file flag is required (FR-3.2.2).
func TestUpdateMetadataCommand_missingFileFlag(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"update-file-metadata"})
	err := root.Execute()
	assert.Error(t, err)
}

// TestUpdateMetadataCommand_sourceAgentOnly verifies update with only source-agent (FR-3.2.2).
func TestUpdateMetadataCommand_sourceAgentOnly(t *testing.T) {
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

	// Update with only source-agent.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"update-file-metadata", "--path", basePath,
		"--file", "test.md", "--source-agent", "new-agent"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	data, ok := envelope["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "new-agent", data["SourceAgent"])
}
