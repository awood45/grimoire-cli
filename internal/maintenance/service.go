package maintenance

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/docgen"
	"github.com/awood45/grimoire-cli/internal/embedding"
	"github.com/awood45/grimoire-cli/internal/filelock"
	"github.com/awood45/grimoire-cli/internal/frontmatter"
	"github.com/awood45/grimoire-cli/internal/ledger"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
)

// DBManager abstracts the store.DB operations needed by the maintenance service.
// The real *store.DB satisfies this interface.
type DBManager interface {
	DropAll() error
	EnsureSchema() error
}

// Service provides maintenance operations for a grimoire.
type Service struct {
	brain    *brain.Brain
	fileRepo store.FileRepository
	embRepo  store.EmbeddingRepository
	ledger   ledger.Ledger
	fm       frontmatter.Service
	embedder embedding.Provider
	locker   filelock.Locker
	docGen   docgen.Generator
	db       DBManager
}

// NewService creates a maintenance Service with all required dependencies.
func NewService(
	b *brain.Brain,
	fileRepo store.FileRepository,
	embRepo store.EmbeddingRepository,
	l ledger.Ledger,
	fm frontmatter.Service,
	embedder embedding.Provider,
	locker filelock.Locker,
	docGen docgen.Generator,
	db DBManager,
) *Service {
	return &Service{
		brain:    b,
		fileRepo: fileRepo,
		embRepo:  embRepo,
		ledger:   l,
		fm:       fm,
		embedder: embedder,
		locker:   locker,
		docGen:   docGen,
		db:       db,
	}
}

// Status computes and returns a health report for the grimoire.
// It walks the files/ directory, queries the DB, reads the ledger,
// and refreshes grimoire.md.
func (s *Service) Status(ctx context.Context) (StatusReport, error) {
	var report StatusReport

	// Walk files/ for total count.
	totalFiles, diskFiles, err := s.walkFilesDir()
	if err != nil {
		return report, err
	}
	report.TotalFiles = totalFiles

	// Get tracked count from DB.
	tracked, err := s.fileRepo.Count(ctx)
	if err != nil {
		return report, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to get tracked file count")
	}
	report.TrackedFiles = tracked

	// Compute orphaned and untracked counts.
	if err := s.computeOrphanedAndUntracked(ctx, diskFiles, &report); err != nil {
		return report, err
	}

	// Get ledger entries count.
	entries, err := s.ledger.ReadAll()
	if err != nil {
		return report, sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to read ledger")
	}
	report.LedgerEntries = len(entries)

	// Get DB size and embedding status.
	s.fillDBAndEmbeddingStatus(&report)

	// Build DocData and refresh grimoire.md.
	if err := s.refreshDoc(ctx, &report); err != nil {
		return report, err
	}

	return report, nil
}

// computeOrphanedAndUntracked computes orphaned and untracked counts for the status report.
func (s *Service) computeOrphanedAndUntracked(ctx context.Context, diskFiles []string, report *StatusReport) error {
	dbPaths, err := s.fileRepo.AllFilepaths(ctx)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to get all filepaths")
	}

	diskSet := make(map[string]bool, len(diskFiles))
	for _, f := range diskFiles {
		diskSet[f] = true
	}

	dbSet := make(map[string]bool, len(dbPaths))
	for _, p := range dbPaths {
		dbSet[p] = true
	}

	// Orphaned: DB records with no file on disk.
	for _, p := range dbPaths {
		absPath, resolveErr := s.brain.ResolveFilePath(p)
		if resolveErr != nil {
			report.OrphanedCount++
			continue
		}
		if _, statErr := os.Stat(absPath); os.IsNotExist(statErr) {
			report.OrphanedCount++
		}
	}

	// Untracked: files on disk with no DB record.
	for _, f := range diskFiles {
		if !dbSet[f] {
			report.UntrackedCount++
		}
	}

	return nil
}

// fillDBAndEmbeddingStatus fills DB size and embedding status in the report.
func (s *Service) fillDBAndEmbeddingStatus(report *StatusReport) {
	dbStat, err := os.Stat(s.brain.DBPath())
	if err == nil {
		report.DBSizeBytes = dbStat.Size()
	}

	report.EmbeddingStatus = s.embedder.ModelID()
	if report.EmbeddingStatus == "" {
		report.EmbeddingStatus = "not configured"
	}
}

// Rebuild drops all DB tables, replays the ledger, and returns a report.
// It does NOT regenerate embeddings and does NOT modify the ledger.
func (s *Service) Rebuild(ctx context.Context) (RebuildReport, error) {
	var report RebuildReport

	// Drop all tables.
	if err := s.db.DropAll(); err != nil {
		return report, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to drop tables")
	}

	// Recreate schema.
	if err := s.db.EnsureSchema(); err != nil {
		return report, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to ensure schema")
	}

	// Read all ledger entries.
	entries, err := s.ledger.ReadAll()
	if err != nil {
		return report, sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to read ledger")
	}

	// Replay each entry.
	for i := range entries {
		if err := s.replayEntry(ctx, &entries[i]); err != nil {
			return report, err
		}
		report.EntriesReplayed++
	}

	// Get final record count.
	count, err := s.fileRepo.Count(ctx)
	if err != nil {
		return report, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to get final record count")
	}
	report.FinalRecordCount = count

	return report, nil
}

// HardRebuild performs a file-based reconciliation of the grimoire.
// It acquires an exclusive lock, walks files/, compares against the DB,
// and applies corrective entries to both the ledger and DB.
func (s *Service) HardRebuild(ctx context.Context) (HardRebuildReport, error) {
	var report HardRebuildReport

	// Acquire exclusive lock.
	acquired, err := s.locker.TryLockExclusive()
	if err != nil {
		return report, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to attempt exclusive lock")
	}
	if !acquired {
		return report, sberrors.New(sberrors.ErrCodeRebuildInProgress, "another rebuild or mutation is in progress")
	}
	defer func() {
		_ = s.locker.UnlockExclusive() //nolint:errcheck // Best-effort unlock in defer.
	}()

	// Walk files/ and read frontmatter.
	fileStates, warnings, err := s.scanFiles()
	if err != nil {
		return report, err
	}
	report.FilesScanned = len(fileStates) + len(warnings)
	report.Warnings = warnings

	// Get all DB filepaths.
	dbPaths, err := s.fileRepo.AllFilepaths(ctx)
	if err != nil {
		return report, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to get all filepaths")
	}

	// Apply corrections based on diff.
	return s.applyCorrections(ctx, report, fileStates, dbPaths)
}

// applyCorrections compares file state vs DB state and applies corrective operations.
func (s *Service) applyCorrections(
	ctx context.Context,
	report HardRebuildReport,
	fileStates map[string]store.FileMetadata,
	dbPaths []string,
) (HardRebuildReport, error) { //nolint:gocritic // hugeParam: report is modified and returned.
	dbSet := make(map[string]bool, len(dbPaths))
	for _, p := range dbPaths {
		dbSet[p] = true
	}

	diskSet := make(map[string]bool, len(fileStates))
	for relPath := range fileStates {
		diskSet[relPath] = true
	}

	// Process untracked and stale files.
	for relPath, meta := range fileStates {
		if !dbSet[relPath] {
			if err := s.handleUntracked(ctx, relPath, &meta); err != nil {
				return report, err
			}
			report.Creates++
		} else {
			dbMeta, getErr := s.fileRepo.Get(ctx, relPath)
			if getErr != nil {
				return report, sberrors.Wrap(getErr, sberrors.ErrCodeDatabaseError, "failed to get DB record for comparison")
			}
			if isStale(&dbMeta, &meta) {
				if err := s.handleStale(ctx, relPath, &meta); err != nil {
					return report, err
				}
				report.Updates++
			}
		}
	}

	// Process orphaned records.
	for _, dbPath := range dbPaths {
		if !diskSet[dbPath] {
			if err := s.handleOrphaned(ctx, dbPath); err != nil {
				return report, err
			}
			report.Deletes++
		}
	}

	return report, nil
}

// walkFilesDir walks the files/ directory and returns the count and relative paths.
func (s *Service) walkFilesDir() (count int, relPaths []string, walkErr error) {
	filesDir := s.brain.FilesDir()

	// If files/ directory doesn't exist, return zero.
	if _, err := os.Stat(filesDir); os.IsNotExist(err) {
		return 0, nil, nil
	}

	err := filepath.WalkDir(filesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(filesDir, path)
		if relErr != nil {
			return relErr
		}
		relPaths = append(relPaths, rel)
		count++
		return nil
	})
	if err != nil {
		return 0, nil, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to walk files directory")
	}

	return count, relPaths, nil
}

// refreshDoc builds DocData from the current status and writes grimoire.md.
func (s *Service) refreshDoc(ctx context.Context, report *StatusReport) error {
	// Get tag inventory for docgen.
	tags, err := s.fileRepo.ListTags(ctx, "count")
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to list tags")
	}

	tagEntries := make([]docgen.TagEntry, len(tags))
	for i, t := range tags {
		tagEntries[i] = docgen.TagEntry{Name: t.Name, Count: t.Count}
	}

	// Get agent summary.
	allMeta, err := s.fileRepo.Search(ctx, store.SearchFilters{})
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to search for agent summary")
	}

	agentCounts := make(map[string]int)
	var lastActivity time.Time
	for i := range allMeta {
		agentCounts[allMeta[i].SourceAgent]++
		if allMeta[i].UpdatedAt.After(lastActivity) {
			lastActivity = allMeta[i].UpdatedAt
		}
	}

	agentEntries := make([]docgen.AgentEntry, 0, len(agentCounts))
	for name, count := range agentCounts {
		agentEntries = append(agentEntries, docgen.AgentEntry{Name: name, FileCount: count})
	}

	docData := &docgen.DocData{
		TotalFiles:     report.TotalFiles,
		TrackedFiles:   report.TrackedFiles,
		OrphanedCount:  report.OrphanedCount,
		UntrackedCount: report.UntrackedCount,
		TagInventory:   tagEntries,
		AgentSummary:   agentEntries,
		LastActivity:   lastActivity,
	}

	content, err := s.docGen.Generate(docData)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to generate grimoire.md")
	}

	if writeErr := os.WriteFile(s.brain.DocPath(), []byte(content), 0o600); writeErr != nil {
		return sberrors.Wrap(writeErr, sberrors.ErrCodeInternalError, "failed to write grimoire.md")
	}

	return nil
}

// replayEntry processes a single ledger entry during rebuild.
func (s *Service) replayEntry(ctx context.Context, entry *ledger.Entry) error {
	switch entry.Operation {
	case "create":
		return s.replayCreateOrUpdate(ctx, entry, false)
	case "update":
		return s.replayCreateOrUpdate(ctx, entry, true)
	case "delete", "archive":
		return s.replayDelete(ctx, entry)
	default:
		// Unknown operation — skip silently for forward compatibility.
		return nil
	}
}

// replayCreateOrUpdate handles create and update operations during replay.
func (s *Service) replayCreateOrUpdate(ctx context.Context, entry *ledger.Entry, isUpdate bool) error {
	meta, err := metadataFromPayload(entry)
	if err != nil {
		return err
	}

	if !isUpdate {
		return s.fileRepo.Insert(ctx, meta)
	}

	updateErr := s.fileRepo.Update(ctx, meta)
	if updateErr != nil {
		if sberrors.HasCode(updateErr, sberrors.ErrCodeMetadataNotFound) {
			return s.fileRepo.Insert(ctx, meta)
		}
		return updateErr
	}
	return nil
}

// replayDelete handles delete and archive operations during replay.
func (s *Service) replayDelete(ctx context.Context, entry *ledger.Entry) error {
	deleteErr := s.fileRepo.Delete(ctx, entry.Filepath)
	if deleteErr != nil {
		if sberrors.HasCode(deleteErr, sberrors.ErrCodeMetadataNotFound) {
			return nil
		}
		return deleteErr
	}
	return nil
}

// metadataFromPayload extracts FileMetadata from a ledger entry's payload.
// Both create and update payloads have the same structure.
func metadataFromPayload(entry *ledger.Entry) (store.FileMetadata, error) {
	var payload struct {
		Tags        []string `json:"tags"`
		Summary     string   `json:"summary"`
		SourceAgent string   `json:"source_agent"`
		CreatedAt   string   `json:"created_at"`
		UpdatedAt   string   `json:"updated_at"`
	}

	if err := json.Unmarshal(entry.Payload, &payload); err != nil {
		return store.FileMetadata{}, sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to unmarshal ledger payload")
	}

	createdAt, err := time.Parse(time.RFC3339, payload.CreatedAt)
	if err != nil {
		return store.FileMetadata{}, sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to parse created_at in ledger payload")
	}

	updatedAt, err := time.Parse(time.RFC3339, payload.UpdatedAt)
	if err != nil {
		return store.FileMetadata{}, sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to parse updated_at in ledger payload")
	}

	return store.FileMetadata{
		Filepath:    entry.Filepath,
		SourceAgent: payload.SourceAgent,
		Tags:        payload.Tags,
		Summary:     payload.Summary,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

// scanFiles walks files/ and reads frontmatter from each file.
// Returns a map of relative path -> metadata for files with frontmatter,
// and a list of warnings for files without frontmatter.
func (s *Service) scanFiles() (states map[string]store.FileMetadata, warnings []string, scanErr error) {
	filesDir := s.brain.FilesDir()
	states = make(map[string]store.FileMetadata)

	if _, err := os.Stat(filesDir); os.IsNotExist(err) {
		return states, warnings, nil
	}

	err := filepath.WalkDir(filesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(filesDir, path)
		if relErr != nil {
			return relErr
		}

		meta, readErr := s.fm.Read(path)
		if readErr != nil {
			// File without frontmatter — add to warnings.
			if sberrors.HasCode(readErr, sberrors.ErrCodeInvalidInput) {
				warnings = append(warnings, "no frontmatter: "+rel)
				return nil
			}
			return readErr
		}

		meta.Filepath = rel
		states[rel] = meta
		return nil
	})
	if err != nil {
		return nil, nil, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to scan files directory")
	}

	return states, warnings, nil
}

// isStale checks whether the DB record differs from the frontmatter metadata.
func isStale(dbMeta, fileMeta *store.FileMetadata) bool {
	if dbMeta.SourceAgent != fileMeta.SourceAgent {
		return true
	}
	if dbMeta.Summary != fileMeta.Summary {
		return true
	}
	if !tagsEqual(dbMeta.Tags, fileMeta.Tags) {
		return true
	}
	if !dbMeta.CreatedAt.Equal(fileMeta.CreatedAt) {
		return true
	}
	if !dbMeta.UpdatedAt.Equal(fileMeta.UpdatedAt) {
		return true
	}
	return false
}

// tagsEqual checks whether two tag slices contain the same tags (order-insensitive).
func tagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aSet := make(map[string]bool, len(a))
	for _, t := range a {
		aSet[t] = true
	}
	for _, t := range b {
		if !aSet[t] {
			return false
		}
	}
	return true
}

// handleUntracked processes an untracked file (frontmatter exists, no DB row).
func (s *Service) handleUntracked(ctx context.Context, relPath string, meta *store.FileMetadata) error {
	now := time.Now().UTC()

	if err := s.appendLedgerEntry("create", relPath, meta, now); err != nil {
		return err
	}

	dbMeta := store.FileMetadata{
		Filepath:    relPath,
		SourceAgent: "hard-rebuild",
		Tags:        meta.Tags,
		Summary:     meta.Summary,
		CreatedAt:   meta.CreatedAt,
		UpdatedAt:   now,
	}
	if err := s.fileRepo.Insert(ctx, dbMeta); err != nil { //nolint:gocritic // hugeParam: value type required by interface.
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to insert untracked file")
	}

	return s.generateAndUpsertEmbedding(ctx, relPath)
}

// handleStale processes a stale file (DB differs from frontmatter).
func (s *Service) handleStale(ctx context.Context, relPath string, meta *store.FileMetadata) error {
	now := time.Now().UTC()

	if err := s.appendLedgerEntry("update", relPath, meta, now); err != nil {
		return err
	}

	dbMeta := store.FileMetadata{
		Filepath:    relPath,
		SourceAgent: "hard-rebuild",
		Tags:        meta.Tags,
		Summary:     meta.Summary,
		CreatedAt:   meta.CreatedAt,
		UpdatedAt:   now,
	}
	if err := s.fileRepo.Update(ctx, dbMeta); err != nil { //nolint:gocritic // hugeParam: value type required by interface.
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to update stale file")
	}

	return s.generateAndUpsertEmbedding(ctx, relPath)
}

// appendLedgerEntry builds and appends a ledger entry for create or update operations.
func (s *Service) appendLedgerEntry(operation, relPath string, meta *store.FileMetadata, now time.Time) error {
	payloadBytes, err := json.Marshal(struct {
		Tags        []string `json:"tags"`
		Summary     string   `json:"summary,omitempty"`
		SourceAgent string   `json:"source_agent"`
		CreatedAt   string   `json:"created_at"`
		UpdatedAt   string   `json:"updated_at"`
	}{
		Tags:        meta.Tags,
		Summary:     meta.Summary,
		SourceAgent: "hard-rebuild",
		CreatedAt:   meta.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   now.Format(time.RFC3339),
	})
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to marshal "+operation+" payload")
	}

	entry := &ledger.Entry{
		Timestamp:   now,
		Operation:   operation,
		Filepath:    relPath,
		SourceAgent: "hard-rebuild",
		Payload:     payloadBytes,
	}
	if err := s.ledger.Append(entry); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to append "+operation+" entry")
	}

	return nil
}

// handleOrphaned processes an orphaned DB record (no file on disk).
func (s *Service) handleOrphaned(ctx context.Context, relPath string) error {
	now := time.Now().UTC()

	payloadBytes, err := json.Marshal(ledger.DeletePayload{})
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to marshal delete payload")
	}

	entry := &ledger.Entry{
		Timestamp:   now,
		Operation:   "delete",
		Filepath:    relPath,
		SourceAgent: "hard-rebuild",
		Payload:     payloadBytes,
	}
	if err := s.ledger.Append(entry); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to append delete entry")
	}

	// Delete embedding first (may not exist, ignore errors).
	_ = s.embRepo.Delete(ctx, relPath) //nolint:errcheck // Best-effort embedding deletion.

	// Delete from DB.
	if err := s.fileRepo.Delete(ctx, relPath); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to delete orphaned record")
	}

	return nil
}

// generateAndUpsertEmbedding reads file content and generates/stores an embedding.
func (s *Service) generateAndUpsertEmbedding(ctx context.Context, relPath string) error {
	absPath, err := s.brain.ResolveFilePath(relPath)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to resolve file path for embedding")
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to read file for embedding")
	}

	vector, err := s.embedder.GenerateEmbedding(ctx, string(content))
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeEmbeddingError, "failed to generate embedding")
	}

	if err := s.embRepo.Upsert(ctx, relPath, vector, s.embedder.ModelID()); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to upsert embedding")
	}

	return nil
}
