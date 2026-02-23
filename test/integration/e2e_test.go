// Package integration_test contains end-to-end tests that exercise the CLI
// binary via exec.Command, verifying JSON output envelopes and exit codes.
package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// e2eBinaryPath holds the path to the compiled CLI binary, set once per
// top-level test function that needs it.
var e2eBinaryPath string

// buildE2EBinary compiles the CLI binary into a temp directory and returns
// its path. The binary is cleaned up when the test completes.
func buildE2EBinary(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	binaryName := "grimoire-cli"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	binPath := filepath.Join(tmpDir, binaryName)

	// Build from project root.
	projectRoot := filepath.Join("..", "..")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/grimoire-cli")
	cmd.Dir = projectRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "failed to build binary: %s", stderr.String())

	return binPath
}

// e2eEnvelope is the generic JSON envelope returned by all commands.
type e2eEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error *e2eErrorDetail `json:"error,omitempty"`
}

// e2eErrorDetail represents the error detail in a failure envelope.
type e2eErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// runE2ECLI executes the CLI binary with the given arguments and returns the
// parsed envelope, raw stdout, raw stderr, and any exec error.
func runE2ECLI(t *testing.T, args ...string) (env e2eEnvelope, stdout, stderr string, exitErr error) {
	t.Helper()

	cmd := exec.Command(e2eBinaryPath, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	exitErr = cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	// Only attempt to parse JSON if there is stdout output.
	if stdout != "" {
		if parseErr := json.Unmarshal([]byte(stdout), &env); parseErr != nil {
			t.Logf("stdout was not valid JSON: %q", stdout)
		}
	}

	return env, stdout, stderr, exitErr
}

// requireE2ESuccess asserts exit code 0 and ok=true, then returns the envelope.
func requireE2ESuccess(t *testing.T, args ...string) e2eEnvelope {
	t.Helper()

	env, stdout, stderr, err := runE2ECLI(t, args...)
	require.NoError(t, err, "command %v failed: stderr=%s stdout=%s", args, stderr, stdout)
	require.True(t, env.OK, "expected ok=true for %v, got stdout=%s", args, stdout)

	return env
}

// TestEndToEnd_CLIOutput exercises NFR-6.5 by running the actual CLI binary
// via exec.Command against a temp brain, verifying JSON envelope format and
// exit codes for a full command sequence.
func TestEndToEnd_CLIOutput(t *testing.T) {
	// Build the binary once for all subtests.
	e2eBinaryPath = buildE2EBinary(t)

	brainDir := t.TempDir()

	// --- Step 1: init ---
	t.Run("init", func(t *testing.T) {
		env := requireE2ESuccess(t, "init", "--path", brainDir)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(env.Data, &data))
		assert.Equal(t, brainDir, data["path"])
		assert.Contains(t, data["message"], "initialized")
	})

	// --- Step 2: create a markdown file on disk ---
	filesDir := filepath.Join(brainDir, "files")
	testFile := "e2e-test-note.md"
	testFilePath := filepath.Join(filesDir, testFile)
	require.NoError(t, os.WriteFile(testFilePath, []byte("# E2E Test Note\n\nSome content for end-to-end testing.\n"), 0o600))

	// --- Step 3: create-file-metadata ---
	// Note: struct fields use PascalCase in JSON (no json tags on store.FileMetadata).
	t.Run("create-file-metadata", func(t *testing.T) {
		env := requireE2ESuccess(t,
			"create-file-metadata",
			"--file", testFile,
			"--tags", "e2e,testing",
			"--source-agent", "e2e-agent",
			"--summary", "E2E test note",
			"--path", brainDir,
		)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(env.Data, &data))
		assert.Equal(t, testFile, data["Filepath"])
		assert.Equal(t, "e2e-agent", data["SourceAgent"])
		assert.Equal(t, "E2E test note", data["Summary"])
	})

	// --- Step 4: get-file-metadata ---
	t.Run("get-file-metadata", func(t *testing.T) {
		env := requireE2ESuccess(t,
			"get-file-metadata",
			"--file", testFile,
			"--path", brainDir,
		)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(env.Data, &data))
		assert.Equal(t, testFile, data["Filepath"])
		assert.Equal(t, "e2e-agent", data["SourceAgent"])
		assert.Equal(t, "E2E test note", data["Summary"])

		// Verify tags are present.
		tags, ok := data["Tags"].([]interface{})
		require.True(t, ok, "Tags should be an array")
		assert.Len(t, tags, 2)
	})

	// --- Step 5: update-file-metadata ---
	t.Run("update-file-metadata", func(t *testing.T) {
		env := requireE2ESuccess(t,
			"update-file-metadata",
			"--file", testFile,
			"--tags", "e2e,testing,updated",
			"--summary", "Updated E2E test note",
			"--path", brainDir,
		)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(env.Data, &data))
		assert.Equal(t, testFile, data["Filepath"])
		assert.Equal(t, "Updated E2E test note", data["Summary"])

		tags, ok := data["Tags"].([]interface{})
		require.True(t, ok, "Tags should be an array")
		assert.Len(t, tags, 3)
	})

	// --- Step 6: search --tag ---
	t.Run("search-by-tag", func(t *testing.T) {
		env := requireE2ESuccess(t,
			"search",
			"--tag", "e2e",
			"--path", brainDir,
		)

		var data []map[string]interface{}
		require.NoError(t, json.Unmarshal(env.Data, &data))
		require.Len(t, data, 1)
		assert.Equal(t, testFile, data[0]["Filepath"])
	})

	// --- Step 7: list-tags ---
	t.Run("list-tags", func(t *testing.T) {
		env := requireE2ESuccess(t,
			"list-tags",
			"--path", brainDir,
		)

		var data []map[string]interface{}
		require.NoError(t, json.Unmarshal(env.Data, &data))
		require.GreaterOrEqual(t, len(data), 3, "should have at least 3 tags (e2e, testing, updated)")

		// Check that we have the expected tag names.
		tagNames := make(map[string]bool)
		for _, tag := range data {
			name, _ := tag["Name"].(string)
			tagNames[name] = true
		}
		assert.True(t, tagNames["e2e"], "should contain tag 'e2e'")
		assert.True(t, tagNames["testing"], "should contain tag 'testing'")
		assert.True(t, tagNames["updated"], "should contain tag 'updated'")
	})

	// --- Step 8: status ---
	t.Run("status", func(t *testing.T) {
		env := requireE2ESuccess(t,
			"status",
			"--path", brainDir,
		)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(env.Data, &data))

		// Should report at least 1 tracked file. StatusReport uses PascalCase.
		trackedFiles, ok := data["TrackedFiles"]
		require.True(t, ok, "status should include TrackedFiles")
		assert.GreaterOrEqual(t, trackedFiles, 1.0) // JSON numbers are float64.
	})

	// --- Step 9: archive-file ---
	t.Run("archive-file", func(t *testing.T) {
		env := requireE2ESuccess(t,
			"archive-file",
			"--file", testFile,
			"--path", brainDir,
		)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(env.Data, &data))
		assert.Equal(t, testFile, data["OriginalPath"])

		// Verify file was moved to archive.
		archivePath := filepath.Join(brainDir, "archive-files", testFile)
		_, err := os.Stat(archivePath)
		require.NoError(t, err, "file should exist in archive-files/")

		// Verify file removed from files/.
		_, err = os.Stat(testFilePath)
		assert.True(t, os.IsNotExist(err), "file should be removed from files/")
	})

	// --- Step 10: rebuild ---
	t.Run("rebuild", func(t *testing.T) {
		env := requireE2ESuccess(t,
			"rebuild",
			"--path", brainDir,
		)

		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(env.Data, &data))

		// RebuildReport uses PascalCase: EntriesReplayed, FinalRecordCount.
		_, hasReplayed := data["EntriesReplayed"]
		assert.True(t, hasReplayed, "rebuild response should include EntriesReplayed")
		_, hasFinal := data["FinalRecordCount"]
		assert.True(t, hasFinal, "rebuild response should include FinalRecordCount")
	})
}

// TestEndToEnd_ErrorCases exercises error handling: Cobra missing-flag errors
// (non-zero exit code) and application errors (exit code 0, ok=false).
func TestEndToEnd_ErrorCases(t *testing.T) {
	// Build the binary once for all subtests.
	e2eBinaryPath = buildE2EBinary(t)

	brainDir := t.TempDir()

	// Initialize the brain for application-level error tests.
	requireE2ESuccess(t, "init", "--path", brainDir)

	// --- Case 1: Missing required --file flag on create-file-metadata ---
	// Cobra should exit with non-zero exit code.
	t.Run("missing-required-flag", func(t *testing.T) {
		_, _, _, err := runE2ECLI(t,
			"create-file-metadata",
			"--tags", "test",
			"--source-agent", "agent",
			"--path", brainDir,
		)
		require.Error(t, err, "missing required --file flag should cause non-zero exit")

		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		assert.NotEqual(t, 0, exitErr.ExitCode(), "exit code should be non-zero")
	})

	// --- Case 2: get-file-metadata for a non-existent file ---
	// Application handles this: exit code 0 but ok=false with error code.
	t.Run("get-nonexistent-file", func(t *testing.T) {
		env, stdout, _, err := runE2ECLI(t,
			"get-file-metadata",
			"--file", "does-not-exist.md",
			"--path", brainDir,
		)
		// Exit code should be 0 because RunE returns nil.
		require.NoError(t, err, "get-file-metadata for non-existent file should exit 0")
		assert.False(t, env.OK, "should be ok=false, got stdout=%s", stdout)
		require.NotNil(t, env.Error, "should have error detail")
		assert.NotEmpty(t, env.Error.Code, "error should have a code")
		assert.NotEmpty(t, env.Error.Message, "error should have a message")
	})

	// --- Case 3: Operating on uninitialized brain ---
	t.Run("uninitialized-brain", func(t *testing.T) {
		uninitDir := t.TempDir()
		env, stdout, _, err := runE2ECLI(t,
			"status",
			"--path", uninitDir,
		)
		// Exit code should be 0 because RunE returns nil.
		require.NoError(t, err, "status on uninitialized brain should exit 0")
		assert.False(t, env.OK, "should be ok=false, got stdout=%s", stdout)
		require.NotNil(t, env.Error, "should have error detail")
		assert.Equal(t, "NOT_INITIALIZED", env.Error.Code)
	})

	// --- Case 4: Unknown command ---
	t.Run("unknown-command", func(t *testing.T) {
		_, _, _, err := runE2ECLI(t, "nonexistent-command")
		require.Error(t, err, "unknown command should cause non-zero exit")

		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		assert.NotEqual(t, 0, exitErr.ExitCode())
	})
}
