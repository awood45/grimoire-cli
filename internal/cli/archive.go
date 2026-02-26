package cli

import (
	"context"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/metadata"
	"github.com/spf13/cobra"
)

// newArchiveFileCommand creates the archive-file subcommand.
func newArchiveFileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive-file",
		Short: "Archive a tracked file",
		Long:  "Move a tracked file to archive-files/, remove its metadata from the database, and log the archive in the ledger.",
		RunE:  runArchiveFile,
	}

	cmd.Flags().String("file", "", "relative path to the file within files/")

	//nolint:errcheck // Required flags cannot fail to be marked.
	cmd.MarkFlagRequired("file")

	return cmd
}

// runArchiveFile is the RunE handler for archive-file.
func runArchiveFile(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	fp, _ := cmd.Flags().GetString("file") //nolint:errcheck // Flag defined on this command.

	// Validate markdown extension.
	b := brain.New(basePath)
	if err := b.ValidateMarkdown(fp); err != nil {
		WriteError(out, err)
		return nil
	}

	// Build AppContext.
	appCtx, err := NewAppContext(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// Archive file.
	manager := metadata.NewManager(
		appCtx.Brain, appCtx.FileRepo, appCtx.EmbRepo,
		appCtx.Ledger, appCtx.FM, appCtx.EmbGen, appCtx.Locker,
	)

	result, archiveErr := manager.Archive(context.Background(), fp)
	if archiveErr != nil {
		WriteError(out, archiveErr)
		return nil
	}

	WriteSuccess(out, result)
	return nil
}
