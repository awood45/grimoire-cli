package cli

import (
	"context"
	"os"

	"github.com/awood45/grimoire-cli/internal/docgen"
	"github.com/awood45/grimoire-cli/internal/initialize"
	"github.com/awood45/grimoire-cli/internal/platform"
	"github.com/spf13/cobra"
)

// newInitCommand creates the init subcommand.
func newInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new grimoire",
		Long:  "Create the directory structure, config, database, and ledger for a new grimoire.",
		RunE:  runInit,
	}

	cmd.Flags().Bool("force", false, "force reinitialization of an existing grimoire")
	cmd.Flags().String("embedding-provider", "", "embedding provider (ollama or none)")
	cmd.Flags().String("embedding-model", "", "embedding model name")

	return cmd
}

// runInit is the RunE handler for the init subcommand.
func runInit(cmd *cobra.Command, _ []string) error {
	basePath := ResolveBasePath(cmd)
	out := cmd.OutOrStdout()

	force, _ := cmd.Flags().GetBool("force")                      //nolint:errcheck // Flag defined on this command.
	embProvider, _ := cmd.Flags().GetString("embedding-provider") //nolint:errcheck // Flag defined on this command.
	embModel, _ := cmd.Flags().GetString("embedding-model")       //nolint:errcheck // Flag defined on this command.

	docGen, err := docgen.NewTemplateGenerator()
	if err != nil {
		WriteError(out, err)
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "" // Best-effort: platform detection works with limited info.
	}
	detector := platform.NewDetector(homeDir)

	initializer := initialize.NewInitializer(docGen, detector)
	opts := initialize.InitOptions{
		BasePath:          basePath,
		Force:             force,
		EmbeddingProvider: embProvider,
		EmbeddingModel:    embModel,
	}

	if initErr := initializer.Init(context.Background(), opts); initErr != nil {
		WriteError(out, initErr)
		return nil
	}

	WriteSuccess(out, map[string]string{
		"path":    basePath,
		"message": "Grimoire initialized successfully",
	})
	return nil
}
