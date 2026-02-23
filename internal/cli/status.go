package cli

import (
	"context"

	"github.com/awood45/grimoire-cli/internal/maintenance"
	"github.com/spf13/cobra"
)

// newStatusCommand creates the status subcommand.
func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show grimoire health status",
		Long:  "Report the health of the grimoire including file counts, orphaned records, and embedding status.",
		RunE:  runStatus,
	}

	return cmd
}

// runStatus is the RunE handler for the status subcommand.
func runStatus(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	// Build AppContext.
	appCtx, err := NewAppContext(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// Build maintenance service.
	service := maintenance.NewService(
		appCtx.Brain, appCtx.FileRepo, appCtx.EmbRepo,
		appCtx.Ledger, appCtx.FM, appCtx.Embedder,
		appCtx.Locker, appCtx.DocGen, appCtx.DB,
	)

	report, statusErr := service.Status(context.Background())
	if statusErr != nil {
		WriteError(out, statusErr)
		return nil
	}

	WriteSuccess(out, report)
	return nil
}
