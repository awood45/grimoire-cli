package cli

import (
	"context"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/metadata"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/spf13/cobra"
)

// newCreateFileMetadataCommand creates the create-file-metadata subcommand.
func newCreateFileMetadataCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-file-metadata",
		Short: "Create metadata for a file",
		Long:  "Create metadata (tags, source agent, summary) for a markdown file in the grimoire.",
		RunE:  runCreateFileMetadata,
	}

	cmd.Flags().String("file", "", "relative path to the file within files/")
	cmd.Flags().String("source-agent", "", "name of the agent creating the metadata")
	cmd.Flags().StringSlice("tags", nil, "tags to associate with the file (repeatable)")
	cmd.Flags().String("summary", "", "optional summary of the file contents")
	cmd.Flags().String("summary-embedding-text", "", "optional text to embed as a summary vector for large files")

	//nolint:errcheck // Required flags cannot fail to be marked.
	cmd.MarkFlagRequired("file")
	//nolint:errcheck // Required flags cannot fail to be marked.
	cmd.MarkFlagRequired("source-agent")
	//nolint:errcheck // Required flags cannot fail to be marked.
	cmd.MarkFlagRequired("tags")

	return cmd
}

// newUpdateFileMetadataCommand creates the update-file-metadata subcommand.
func newUpdateFileMetadataCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-file-metadata",
		Short: "Update metadata for a file",
		Long:  "Update tags, source agent, or summary for an existing file's metadata.",
		RunE:  runUpdateFileMetadata,
	}

	cmd.Flags().String("file", "", "relative path to the file within files/")
	cmd.Flags().StringSlice("tags", nil, "new tags to replace existing tags")
	cmd.Flags().String("source-agent", "", "new source agent name")
	cmd.Flags().String("summary", "", "new summary for the file")
	cmd.Flags().String("summary-embedding-text", "", "optional text to embed as a summary vector for large files")

	//nolint:errcheck // Required flags cannot fail to be marked.
	cmd.MarkFlagRequired("file")

	return cmd
}

// newGetFileMetadataCommand creates the get-file-metadata subcommand.
func newGetFileMetadataCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-file-metadata",
		Short: "Get metadata for a file",
		Long:  "Retrieve stored metadata for a specific file.",
		RunE:  runGetFileMetadata,
	}

	cmd.Flags().String("file", "", "relative path to the file within files/")

	//nolint:errcheck // Required flags cannot fail to be marked.
	cmd.MarkFlagRequired("file")

	return cmd
}

// newDeleteFileMetadataCommand creates the delete-file-metadata subcommand.
func newDeleteFileMetadataCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-file-metadata",
		Short: "Delete metadata for a file",
		Long:  "Remove metadata from the database for a specific file. The file itself is preserved.",
		RunE:  runDeleteFileMetadata,
	}

	cmd.Flags().String("file", "", "relative path to the file within files/")

	//nolint:errcheck // Required flags cannot fail to be marked.
	cmd.MarkFlagRequired("file")

	return cmd
}

// runCreateFileMetadata is the RunE handler for create-file-metadata.
func runCreateFileMetadata(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	fp, _ := cmd.Flags().GetString("file")                               //nolint:errcheck // Flag defined on this command.
	sourceAgent, _ := cmd.Flags().GetString("source-agent")              //nolint:errcheck // Flag defined on this command.
	tags, _ := cmd.Flags().GetStringSlice("tags")                        //nolint:errcheck // Flag defined on this command.
	summary, _ := cmd.Flags().GetString("summary")                       //nolint:errcheck // Flag defined on this command.
	summaryEmbText, _ := cmd.Flags().GetString("summary-embedding-text") //nolint:errcheck // Flag defined on this command.

	// Validate markdown extension.
	b := brain.New(basePath)
	if err := b.ValidateMarkdown(fp); err != nil {
		WriteError(out, err)
		return nil
	}

	// Validate path traversal.
	if _, err := b.ResolveFilePath(fp); err != nil {
		WriteError(out, err)
		return nil
	}

	// Validate inputs.
	if err := ValidateTags(tags); err != nil {
		WriteError(out, err)
		return nil
	}
	if err := ValidateSourceAgent(sourceAgent); err != nil {
		WriteError(out, err)
		return nil
	}
	if err := ValidateSummary(summary); err != nil {
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

	// Create metadata.
	manager := metadata.NewManager(
		appCtx.Brain, appCtx.FileRepo, appCtx.EmbRepo,
		appCtx.Ledger, appCtx.FM, appCtx.EmbGen, appCtx.Locker,
	)

	opts := metadata.CreateOptions{
		Filepath:             fp,
		SourceAgent:          sourceAgent,
		Tags:                 tags,
		Summary:              summary,
		SummaryEmbeddingText: summaryEmbText,
	}

	result, createErr := manager.Create(context.Background(), opts)
	if createErr != nil {
		if sberrors.HasCode(createErr, sberrors.ErrCodeEmbeddingWarning) {
			WriteSuccessWithWarning(out, result, createErr.Error())
			return nil
		}
		WriteError(out, createErr)
		return nil
	}

	WriteSuccess(out, result)
	return nil
}

// runUpdateFileMetadata is the RunE handler for update-file-metadata.
func runUpdateFileMetadata(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	fp, _ := cmd.Flags().GetString("file") //nolint:errcheck // Flag defined on this command.

	// Validate markdown extension.
	b := brain.New(basePath)
	if err := b.ValidateMarkdown(fp); err != nil {
		WriteError(out, err)
		return nil
	}

	// Build update options from provided flags.
	opts := metadata.UpdateOptions{
		Filepath: fp,
	}

	if cmd.Flags().Changed("tags") {
		tags, _ := cmd.Flags().GetStringSlice("tags") //nolint:errcheck // Flag defined on this command.
		if err := ValidateTags(tags); err != nil {
			WriteError(out, err)
			return nil
		}
		opts.Tags = tags
	}

	if cmd.Flags().Changed("source-agent") {
		sourceAgent, _ := cmd.Flags().GetString("source-agent") //nolint:errcheck // Flag defined on this command.
		if err := ValidateSourceAgent(sourceAgent); err != nil {
			WriteError(out, err)
			return nil
		}
		opts.SourceAgent = sourceAgent
	}

	if cmd.Flags().Changed("summary") {
		summary, _ := cmd.Flags().GetString("summary") //nolint:errcheck // Flag defined on this command.
		if err := ValidateSummary(summary); err != nil {
			WriteError(out, err)
			return nil
		}
		opts.Summary = &summary
	}

	if cmd.Flags().Changed("summary-embedding-text") {
		summaryEmbText, _ := cmd.Flags().GetString("summary-embedding-text") //nolint:errcheck // Flag defined on this command.
		opts.SummaryEmbeddingText = &summaryEmbText
	}

	// Build AppContext.
	appCtx, err := NewAppContext(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// Update metadata.
	manager := metadata.NewManager(
		appCtx.Brain, appCtx.FileRepo, appCtx.EmbRepo,
		appCtx.Ledger, appCtx.FM, appCtx.EmbGen, appCtx.Locker,
	)

	result, updateErr := manager.Update(context.Background(), opts)
	if updateErr != nil {
		if sberrors.HasCode(updateErr, sberrors.ErrCodeEmbeddingWarning) {
			WriteSuccessWithWarning(out, result, updateErr.Error())
			return nil
		}
		WriteError(out, updateErr)
		return nil
	}

	WriteSuccess(out, result)
	return nil
}

// runGetFileMetadata is the RunE handler for get-file-metadata.
func runGetFileMetadata(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	fp, _ := cmd.Flags().GetString("file") //nolint:errcheck // Flag defined on this command.

	// Build AppContext.
	appCtx, err := NewAppContext(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// Get metadata.
	manager := metadata.NewManager(
		appCtx.Brain, appCtx.FileRepo, appCtx.EmbRepo,
		appCtx.Ledger, appCtx.FM, appCtx.EmbGen, appCtx.Locker,
	)

	result, getErr := manager.Get(context.Background(), fp)
	if getErr != nil {
		WriteError(out, getErr)
		return nil
	}

	WriteSuccess(out, result)
	return nil
}

// runDeleteFileMetadata is the RunE handler for delete-file-metadata.
func runDeleteFileMetadata(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	basePath := ResolveBasePath(cmd)

	fp, _ := cmd.Flags().GetString("file") //nolint:errcheck // Flag defined on this command.

	// Build AppContext.
	appCtx, err := NewAppContext(basePath)
	if err != nil {
		WriteError(out, err)
		return nil
	}
	defer appCtx.Close() //nolint:errcheck // Best-effort cleanup.

	// Delete metadata.
	manager := metadata.NewManager(
		appCtx.Brain, appCtx.FileRepo, appCtx.EmbRepo,
		appCtx.Ledger, appCtx.FM, appCtx.EmbGen, appCtx.Locker,
	)

	if deleteErr := manager.Delete(context.Background(), fp); deleteErr != nil {
		WriteError(out, deleteErr)
		return nil
	}

	WriteSuccess(out, map[string]string{
		"filepath": fp,
		"message":  "Metadata deleted successfully",
	})
	return nil
}
