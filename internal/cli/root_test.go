package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewRootCommand verifies the root command has expected flags.
func TestNewRootCommand(t *testing.T) {
	t.Parallel()

	cmd := NewRootCommand()

	assert.Equal(t, "grimoire-cli", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Verify persistent flags exist.
	pathFlag := cmd.PersistentFlags().Lookup("path")
	require.NotNil(t, pathFlag)
	assert.Equal(t, "", pathFlag.DefValue)

	verboseFlag := cmd.PersistentFlags().Lookup("verbose")
	require.NotNil(t, verboseFlag)
	assert.Equal(t, "false", verboseFlag.DefValue)
}

// TestResolveBasePath_flag verifies --path flag takes priority.
func TestResolveBasePath_flag(t *testing.T) {
	t.Parallel()

	cmd := NewRootCommand()
	require.NoError(t, cmd.PersistentFlags().Set("path", "/custom/path"))

	result := ResolveBasePath(cmd)
	assert.Equal(t, "/custom/path", result)
}

// TestResolveBasePath_envVar verifies GRIMOIRE_PATH env var is used when no flag.
func TestResolveBasePath_envVar(t *testing.T) {
	// Not parallel due to os.Setenv.
	cmd := NewRootCommand()

	t.Setenv("GRIMOIRE_PATH", "/env/path")

	result := ResolveBasePath(cmd)
	assert.Equal(t, "/env/path", result)
}

// TestResolveBasePath_default verifies default ~/.grimoire/ is used.
func TestResolveBasePath_default(t *testing.T) {
	// Not parallel due to os.Setenv.
	cmd := NewRootCommand()

	t.Setenv("GRIMOIRE_PATH", "")

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	result := ResolveBasePath(cmd)
	assert.Equal(t, filepath.Join(home, ".grimoire"), result)
}

// TestResolveBasePath_flagOverridesEnv verifies flag takes priority over env.
func TestResolveBasePath_flagOverridesEnv(t *testing.T) {
	// Not parallel due to os.Setenv.
	cmd := NewRootCommand()

	t.Setenv("GRIMOIRE_PATH", "/env/path")
	require.NoError(t, cmd.PersistentFlags().Set("path", "/flag/path"))

	result := ResolveBasePath(cmd)
	assert.Equal(t, "/flag/path", result)
}

// TestResolveBasePath_tildeExpansion verifies ~ is expanded in flag values.
func TestResolveBasePath_tildeExpansion(t *testing.T) {
	t.Parallel()

	cmd := NewRootCommand()
	require.NoError(t, cmd.PersistentFlags().Set("path", "~/my-brain"))

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	result := ResolveBasePath(cmd)
	assert.Equal(t, filepath.Join(home, "my-brain"), result)
}

// TestResolveBasePath_subcommand verifies flag resolution works via subcommand after Execute.
func TestResolveBasePath_subcommand(t *testing.T) {
	t.Parallel()

	var resolved string

	root := NewRootCommand()
	sub := &cobra.Command{
		Use: "test-sub",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved = ResolveBasePath(cmd)
			return nil
		},
	}
	root.AddCommand(sub)

	root.SetArgs([]string{"test-sub", "--path", "/sub/path"})
	require.NoError(t, root.Execute())
	assert.Equal(t, "/sub/path", resolved)
}

// TestExpandHome verifies tilde expansion behavior.
func TestExpandHome(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde only", "~", home},
		{"tilde slash", "~/foo", filepath.Join(home, "foo")},
		{"tilde path", "~/.grimoire/", filepath.Join(home, ".grimoire")},
		{"absolute", "/absolute/path", "/absolute/path"},
		{"relative", "relative/path", "relative/path"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := expandHome(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
