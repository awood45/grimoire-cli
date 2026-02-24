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

// TestInitCommand_registration verifies the init subcommand is registered (FR-3.1.1).
func TestInitCommand_registration(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Use == "init" {
			found = true
			break
		}
	}
	assert.True(t, found, "init subcommand should be registered on root")
}

// TestInitCommand_flags verifies the init subcommand has all expected flags (FR-3.1.1).
func TestInitCommand_flags(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	var initCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Use == "init" {
			initCmd = cmd
			break
		}
	}
	require.NotNil(t, initCmd)

	// Check that flags exist.
	assert.NotNil(t, initCmd.Flags().Lookup("force"))
	assert.NotNil(t, initCmd.Flags().Lookup("embedding-provider"))
	assert.NotNil(t, initCmd.Flags().Lookup("embedding-model"))
}

// TestInitCommand_success verifies init creates a grimoire (FR-3.1.1).
func TestInitCommand_success(t *testing.T) {
	t.Setenv("GRIMOIRE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"init", "--path", basePath})

	err := root.Execute()
	require.NoError(t, err)

	// Verify JSON output.
	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	// Verify directory structure was created.
	assert.DirExists(t, filepath.Join(basePath, "files"))
	assert.DirExists(t, filepath.Join(basePath, "archive-files"))
	assert.DirExists(t, filepath.Join(basePath, "db"))
	assert.FileExists(t, filepath.Join(basePath, "config.yaml"))
}

// TestInitCommand_alreadyExists verifies init fails without --force (FR-3.1.1).
func TestInitCommand_alreadyExists(t *testing.T) {
	t.Setenv("GRIMOIRE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")

	// First init.
	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"init", "--path", basePath})
	require.NoError(t, root.Execute())

	// Second init without --force.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"init", "--path", basePath})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, false, envelope["ok"])

	errObj, ok := envelope["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ALREADY_INITIALIZED", errObj["code"])
}

// TestInitCommand_force verifies init with --force reinitializes (FR-3.1.1).
func TestInitCommand_force(t *testing.T) {
	t.Setenv("GRIMOIRE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")

	// First init.
	root := NewRootCommand()
	var buf1 bytes.Buffer
	root.SetOut(&buf1)
	root.SetArgs([]string{"init", "--path", basePath})
	require.NoError(t, root.Execute())

	// Add a file to verify it persists.
	testFile := filepath.Join(basePath, "files", "keep.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Keep me"), 0o600))

	// Second init with --force.
	root2 := NewRootCommand()
	var buf2 bytes.Buffer
	root2.SetOut(&buf2)
	root2.SetArgs([]string{"init", "--path", basePath, "--force"})
	require.NoError(t, root2.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	// File should still exist.
	assert.FileExists(t, testFile)
}

// TestInitCommand_embeddingFlags verifies embedding flags are passed through (FR-3.1.1).
func TestInitCommand_embeddingFlags(t *testing.T) {
	t.Setenv("GRIMOIRE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "test-brain")

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"init", "--path", basePath, "--embedding-provider", "ollama", "--embedding-model", "custom-model"})
	require.NoError(t, root.Execute())

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
	assert.Equal(t, true, envelope["ok"])

	// Verify config has the correct values.
	cfgBytes, err := os.ReadFile(filepath.Join(basePath, "config.yaml"))
	require.NoError(t, err)
	cfgContent := string(cfgBytes)
	assert.Contains(t, cfgContent, "provider: ollama")
	assert.Contains(t, cfgContent, "model: custom-model")
}
