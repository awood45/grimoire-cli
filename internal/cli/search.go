package cli

import (
	"context"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/search"
	"github.com/awood45/grimoire-cli/internal/store"
	"github.com/spf13/cobra"
)

// newSearchCommand creates the search subcommand.
func newSearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search files by metadata filters",
		Long:  "Search for files matching tag, source agent, date, and summary filters.",
		RunE:  runSearch,
	}

	cmd.Flags().StringSlice("tag", nil, "require ALL specified tags (repeatable)")
	cmd.Flags().StringSlice("any-tag", nil, "require ANY of the specified tags (repeatable)")
	cmd.Flags().String("source-agent", "", "filter by source agent name")
	cmd.Flags().String("after", "", "filter files updated after this time (RFC3339)")
	cmd.Flags().String("before", "", "filter files updated before this time (RFC3339)")
	cmd.Flags().String("summary-contains", "", "substring match within summary")
	cmd.Flags().Int("limit", 0, "maximum number of results (0 = use config default)")
	cmd.Flags().String("sort", "", "sort order: updated_at, created_at, or filepath")

	return cmd
}

// runSearch is the RunE handler for the search subcommand.
func runSearch(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	tags, _ := cmd.Flags().GetStringSlice("tag")                    //nolint:errcheck // Flag defined on this command.
	anyTags, _ := cmd.Flags().GetStringSlice("any-tag")             //nolint:errcheck // Flag defined on this command.
	sourceAgent, _ := cmd.Flags().GetString("source-agent")         //nolint:errcheck // Flag defined on this command.
	afterStr, _ := cmd.Flags().GetString("after")                   //nolint:errcheck // Flag defined on this command.
	beforeStr, _ := cmd.Flags().GetString("before")                 //nolint:errcheck // Flag defined on this command.
	summaryContains, _ := cmd.Flags().GetString("summary-contains") //nolint:errcheck // Flag defined on this command.
	limit, _ := cmd.Flags().GetInt("limit")                         //nolint:errcheck // Flag defined on this command.
	sortOrder, _ := cmd.Flags().GetString("sort")                   //nolint:errcheck // Flag defined on this command.

	// Build filters.
	filters := store.SearchFilters{
		Tags:            tags,
		AnyTags:         anyTags,
		SourceAgent:     sourceAgent,
		SummaryContains: summaryContains,
		Limit:           limit,
		Sort:            sortOrder,
	}

	// Parse --after.
	if afterStr != "" {
		t, err := time.Parse(time.RFC3339, afterStr)
		if err != nil {
			WriteError(out, sberrors.Newf(sberrors.ErrCodeInvalidInput,
				"invalid --after value %q: must be RFC3339 format", afterStr))
			return nil
		}
		filters.After = &t
	}

	// Parse --before.
	if beforeStr != "" {
		t, err := time.Parse(time.RFC3339, beforeStr)
		if err != nil {
			WriteError(out, sberrors.Newf(sberrors.ErrCodeInvalidInput,
				"invalid --before value %q: must be RFC3339 format", beforeStr))
			return nil
		}
		filters.Before = &t
	}

	// Build AppContext.
	appCtx, err := NewAppContext(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// Apply default limit from config if not specified.
	if filters.Limit == 0 {
		filters.Limit = appCtx.Config.Search.DefaultLimit
	}

	// Search.
	engine := search.NewEngine(appCtx.FileRepo, appCtx.EmbRepo, appCtx.Embedder, appCtx.Config.Embedding.QueryPrefix)
	results, searchErr := engine.Search(context.Background(), filters)
	if searchErr != nil {
		WriteError(out, searchErr)
		return nil
	}

	WriteSuccess(out, results)
	return nil
}
