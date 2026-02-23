package cli

import (
	"context"

	"github.com/awood45/grimoire-cli/internal/search"
	"github.com/spf13/cobra"
)

// newListTagsCommand creates the list-tags subcommand.
func newListTagsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-tags",
		Short: "List all tags",
		Long:  "List all tags currently in use with their file counts.",
		RunE:  runListTags,
	}

	cmd.Flags().String("sort", "name", "sort order: name or count")

	return cmd
}

// runListTags is the RunE handler for the list-tags subcommand.
func runListTags(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	sortOrder, _ := cmd.Flags().GetString("sort") //nolint:errcheck // Flag defined on this command.

	// Build AppContext.
	appCtx, err := NewAppContext(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// List tags.
	engine := search.NewEngine(appCtx.FileRepo, appCtx.EmbRepo, appCtx.Embedder)
	results, listErr := engine.ListTags(context.Background(), sortOrder)
	if listErr != nil {
		WriteError(out, listErr)
		return nil
	}

	WriteSuccess(out, results)
	return nil
}
