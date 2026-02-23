package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	// Default grimoire directory path.
	defaultBasePath = "~/.grimoire/"

	// Environment variable name for overriding the base path.
	envBasePath = "GRIMOIRE_PATH"
)

// NewRootCommand creates the root cobra command with persistent flags.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "grimoire-cli",
		Short:         "A local-first CLI knowledge store for AI agents",
		Long:          "Grimoire is a local-first, agent-native knowledge management system that provides a shared file-based knowledge store for AI agents.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().String("path", "", "base path for the grimoire (default: ~/.grimoire/)")
	cmd.PersistentFlags().Bool("verbose", false, "enable verbose output")

	// Register subcommands.
	cmd.AddCommand(newInitCommand())
	cmd.AddCommand(newCreateFileMetadataCommand())
	cmd.AddCommand(newUpdateFileMetadataCommand())
	cmd.AddCommand(newGetFileMetadataCommand())
	cmd.AddCommand(newDeleteFileMetadataCommand())
	cmd.AddCommand(newArchiveFileCommand())
	cmd.AddCommand(newSearchCommand())
	cmd.AddCommand(newSimilarCommand())
	cmd.AddCommand(newListTagsCommand())
	cmd.AddCommand(newStatusCommand())
	cmd.AddCommand(newRebuildCommand())
	cmd.AddCommand(newHardRebuildCommand())

	return cmd
}

// ResolveBasePath determines the base path from flags, env vars, or default.
// Resolution order: --path flag > GRIMOIRE_PATH env var > ~/.grimoire/ default.
func ResolveBasePath(cmd *cobra.Command) string {
	// Check --path flag. Try the merged flagset first (works after Execute),
	// then the root persistent flags (works before Execute / in tests).
	if f := cmd.Flags().Lookup("path"); f != nil && f.Changed {
		return expandHome(f.Value.String())
	}
	if f := cmd.Root().PersistentFlags().Lookup("path"); f != nil && f.Changed {
		return expandHome(f.Value.String())
	}

	// Check environment variable.
	if p := os.Getenv(envBasePath); p != "" {
		return expandHome(p)
	}

	// Default.
	return expandHome(defaultBasePath)
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if path == "" {
		return path
	}

	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}

	return path
}
