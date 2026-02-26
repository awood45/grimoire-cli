package cli

import (
	"context"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/search"
	"github.com/spf13/cobra"
)

// newSimilarCommand creates the similar subcommand.
func newSimilarCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "similar",
		Short: "Find similar files",
		Long:  "Find files similar to an existing file or a text query using embedding similarity.",
		RunE:  runSimilar,
	}

	cmd.Flags().String("file", "", "relative path to a file to find similar files for")
	cmd.Flags().String("text", "", "text query to find similar files for")
	cmd.Flags().Int("limit", 0, "maximum number of results (0 = use config default)")

	return cmd
}

// runSimilar is the RunE handler for the similar subcommand.
func runSimilar(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	filePath, _ := cmd.Flags().GetString("file") //nolint:errcheck // Flag defined on this command.
	text, _ := cmd.Flags().GetString("text")     //nolint:errcheck // Flag defined on this command.
	limit, _ := cmd.Flags().GetInt("limit")      //nolint:errcheck // Flag defined on this command.

	// Validate that exactly one of --file or --text is provided.
	hasFile := cmd.Flags().Changed("file")
	hasText := cmd.Flags().Changed("text")

	if !hasFile && !hasText {
		WriteError(out, sberrors.New(sberrors.ErrCodeInvalidInput,
			"exactly one of --file or --text must be provided"))
		return nil
	}
	if hasFile && hasText {
		WriteError(out, sberrors.New(sberrors.ErrCodeInvalidInput,
			"exactly one of --file or --text must be provided, not both"))
		return nil
	}

	// Build AppContext.
	appCtx, err := NewAppContext(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// Apply default limit from config if not specified.
	if limit == 0 {
		limit = appCtx.Config.Similar.DefaultLimit
	}

	// Build similar input.
	input := search.SimilarInput{
		FilePath: filePath,
		Text:     text,
		Limit:    limit,
	}

	// Search for similar files.
	engine := search.NewEngine(appCtx.FileRepo, appCtx.EmbRepo, appCtx.Embedder, appCtx.Config.Embedding.QueryPrefix)
	results, searchErr := engine.Similar(context.Background(), input)
	if searchErr != nil {
		WriteError(out, searchErr)
		return nil
	}

	WriteSuccess(out, results)
	return nil
}
