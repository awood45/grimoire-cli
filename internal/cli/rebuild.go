package cli

import (
	"context"

	"github.com/awood45/grimoire-cli/internal/maintenance"
	"github.com/spf13/cobra"
)

// newRebuildCommand creates the rebuild subcommand.
func newRebuildCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild database from ledger",
		Long:  "Drop and recreate the SQLite database by replaying the JSONL ledger.",
		RunE:  runRebuild,
	}

	return cmd
}

// newHardRebuildCommand creates the hard-rebuild subcommand.
func newHardRebuildCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hard-rebuild",
		Short: "Rebuild database from files",
		Long:  "Walk all files, compare with database state, and apply corrective entries.",
		RunE:  runHardRebuild,
	}

	return cmd
}

// runRebuild is the RunE handler for the rebuild subcommand.
func runRebuild(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	// Build AppContext, skipping version check since rebuild drops and recreates the schema.
	appCtx, err := NewAppContextSkipVersionCheck(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// Build maintenance service.
	service := maintenance.NewService(
		appCtx.Brain, appCtx.FileRepo, appCtx.EmbRepo,
		appCtx.Ledger, appCtx.FM, appCtx.EmbGen,
		appCtx.Locker, appCtx.DocGen, appCtx.DB,
	)

	report, rebuildErr := service.Rebuild(context.Background())
	if rebuildErr != nil {
		WriteError(out, rebuildErr)
		return nil
	}

	WriteSuccess(out, report)
	return nil
}

// runHardRebuild is the RunE handler for the hard-rebuild subcommand.
func runHardRebuild(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	// Build AppContext, skipping version check since hard-rebuild drops and recreates the schema.
	appCtx, err := NewAppContextSkipVersionCheck(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// Build maintenance service.
	service := maintenance.NewService(
		appCtx.Brain, appCtx.FileRepo, appCtx.EmbRepo,
		appCtx.Ledger, appCtx.FM, appCtx.EmbGen,
		appCtx.Locker, appCtx.DocGen, appCtx.DB,
	)

	report, rebuildErr := service.HardRebuild(context.Background())
	if rebuildErr != nil {
		WriteError(out, rebuildErr)
		return nil
	}

	WriteSuccess(out, report)
	return nil
}
