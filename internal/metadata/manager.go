package metadata

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/embedding"
	"github.com/awood45/grimoire-cli/internal/filelock"
	"github.com/awood45/grimoire-cli/internal/frontmatter"
	"github.com/awood45/grimoire-cli/internal/ledger"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
)

// noopModelID is the model ID returned by the noop embedding provider.
const noopModelID = "none"

// Manager orchestrates metadata mutations across frontmatter, ledger, and
// SQLite stores with locking and optional embedding generation.
type Manager struct {
	brain    *brain.Brain
	fileRepo store.FileRepository
	embRepo  store.EmbeddingRepository
	ledger   ledger.Ledger
	fm       frontmatter.Service
	embedder embedding.Provider
	locker   filelock.Locker
}

// NewManager creates a Manager with the given dependencies.
func NewManager(
	b *brain.Brain,
	fileRepo store.FileRepository,
	embRepo store.EmbeddingRepository,
	led ledger.Ledger,
	fm frontmatter.Service,
	embedder embedding.Provider,
	locker filelock.Locker,
) *Manager {
	return &Manager{
		brain:    b,
		fileRepo: fileRepo,
		embRepo:  embRepo,
		ledger:   led,
		fm:       fm,
		embedder: embedder,
		locker:   locker,
	}
}

// Create creates metadata for a file following the sequence:
// TryLockShared -> stat file -> check no existing metadata -> generate timestamps ->
// Write frontmatter -> Append ledger -> Insert DB -> Generate embedding (fail open) -> UnlockShared.
func (m *Manager) Create(ctx context.Context, opts CreateOptions) (store.FileMetadata, error) { //nolint:gocritic // hugeParam: interface-compatible value type.
	if err := m.acquireSharedLock(); err != nil {
		return store.FileMetadata{}, err
	}
	defer m.unlockShared()

	// Resolve and stat the file.
	absPath, err := m.brain.ResolveFilePath(opts.Filepath)
	if err != nil {
		return store.FileMetadata{}, err
	}

	if _, statErr := os.Stat(absPath); statErr != nil {
		return store.FileMetadata{}, sberrors.Newf(sberrors.ErrCodeFileNotFound, "file not found: %s", opts.Filepath)
	}

	// Check no existing metadata.
	_, getErr := m.fileRepo.Get(ctx, opts.Filepath)
	if getErr == nil {
		return store.FileMetadata{}, sberrors.Newf(sberrors.ErrCodeMetadataExists, "metadata already exists: %s", opts.Filepath)
	}
	if !sberrors.HasCode(getErr, sberrors.ErrCodeMetadataNotFound) {
		return store.FileMetadata{}, getErr
	}

	// Generate timestamps.
	now := time.Now().UTC()
	meta := store.FileMetadata{
		Filepath:    opts.Filepath,
		SourceAgent: opts.SourceAgent,
		Tags:        opts.Tags,
		Summary:     opts.Summary,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Write frontmatter.
	if err := m.fm.Write(absPath, meta); err != nil {
		return store.FileMetadata{}, err
	}

	// Append ledger entry.
	if err := m.appendLedgerEntry("create", &meta); err != nil {
		return store.FileMetadata{}, err
	}

	// Insert into DB.
	if err := m.fileRepo.Insert(ctx, meta); err != nil {
		return store.FileMetadata{}, err
	}

	// Generate embedding (fail open).
	m.generateEmbedding(ctx, opts.Filepath, absPath)

	return meta, nil
}

// Update updates metadata for a file following the sequence:
// TryLockShared -> validate at least one change -> Get existing -> merge fields ->
// set updated_at -> Write frontmatter -> Append ledger -> Update DB ->
// Regenerate embedding -> UnlockShared.
func (m *Manager) Update(ctx context.Context, opts UpdateOptions) (store.FileMetadata, error) { //nolint:gocritic // hugeParam: interface-compatible value type.
	// Validate that at least one field is being changed.
	if !hasChanges(&opts) {
		return store.FileMetadata{}, sberrors.New(sberrors.ErrCodeInvalidInput, "no changes requested")
	}

	if err := m.acquireSharedLock(); err != nil {
		return store.FileMetadata{}, err
	}
	defer m.unlockShared()

	// Get existing metadata.
	existing, err := m.fileRepo.Get(ctx, opts.Filepath)
	if err != nil {
		return store.FileMetadata{}, err
	}

	// Merge fields.
	merged := mergeUpdate(&existing, &opts)

	// Set updated_at.
	merged.UpdatedAt = time.Now().UTC()

	// Resolve absolute path.
	absPath, err := m.brain.ResolveFilePath(opts.Filepath)
	if err != nil {
		return store.FileMetadata{}, err
	}

	// Write frontmatter.
	if err := m.fm.Write(absPath, merged); err != nil {
		return store.FileMetadata{}, err
	}

	// Append ledger entry (full snapshot).
	if err := m.appendLedgerEntry("update", &merged); err != nil {
		return store.FileMetadata{}, err
	}

	// Update DB.
	if err := m.fileRepo.Update(ctx, merged); err != nil {
		return store.FileMetadata{}, err
	}

	// Regenerate embedding (fail open).
	m.generateEmbedding(ctx, opts.Filepath, absPath)

	return merged, nil
}

// Get retrieves metadata for a file. No lock needed.
func (m *Manager) Get(ctx context.Context, fp string) (store.FileMetadata, error) {
	return m.fileRepo.Get(ctx, fp)
}

// Delete removes metadata for a file following the sequence:
// TryLockShared -> Get existing -> Remove frontmatter (if file exists) ->
// Append delete ledger -> Delete from DB -> UnlockShared.
func (m *Manager) Delete(ctx context.Context, fp string) error {
	if err := m.acquireSharedLock(); err != nil {
		return err
	}
	defer m.unlockShared()

	// Get existing to confirm it exists.
	existing, err := m.fileRepo.Get(ctx, fp)
	if err != nil {
		return err
	}

	// Remove frontmatter if file still exists on disk.
	absPath, resolveErr := m.brain.ResolveFilePath(fp)
	if resolveErr == nil {
		if _, statErr := os.Stat(absPath); statErr == nil {
			if rmErr := m.fm.Remove(absPath); rmErr != nil {
				return rmErr
			}
		}
	}

	// Append delete ledger entry.
	if err := m.appendDeleteLedgerEntry(&existing); err != nil {
		return err
	}

	// Delete from DB (cascades to tags and embeddings).
	return m.fileRepo.Delete(ctx, fp)
}

// Archive archives a tracked file following the sequence:
// TryLockShared -> Get metadata -> resolve paths -> stat source -> mkdir dest ->
// Remove frontmatter -> Rename file -> Append archive ledger -> Delete embedding ->
// Delete from DB -> UnlockShared.
func (m *Manager) Archive(ctx context.Context, fp string) (ArchiveResult, error) {
	if err := m.acquireSharedLock(); err != nil {
		return ArchiveResult{}, err
	}
	defer m.unlockShared()

	// Get existing metadata.
	existing, err := m.fileRepo.Get(ctx, fp)
	if err != nil {
		return ArchiveResult{}, err
	}

	// Resolve source path.
	srcAbs, err := m.brain.ResolveFilePath(fp)
	if err != nil {
		return ArchiveResult{}, err
	}

	// Stat source file.
	if _, statErr := os.Stat(srcAbs); statErr != nil {
		return ArchiveResult{}, sberrors.Newf(sberrors.ErrCodeFileNotFound, "file not found: %s", fp)
	}

	// Resolve destination path.
	dstAbs, err := m.brain.ResolveArchivePath(fp)
	if err != nil {
		return ArchiveResult{}, err
	}

	// Create intermediate directories.
	if mkdirErr := os.MkdirAll(filepath.Dir(dstAbs), 0o755); mkdirErr != nil {
		return ArchiveResult{}, sberrors.Wrap(mkdirErr, sberrors.ErrCodeInternalError, "failed to create archive directory")
	}

	// Remove frontmatter before move.
	if rmErr := m.fm.Remove(srcAbs); rmErr != nil {
		// If the error is not about missing frontmatter, propagate it.
		if !sberrors.HasCode(rmErr, sberrors.ErrCodeInvalidInput) {
			return ArchiveResult{}, rmErr
		}
	}

	// Rename (move) file.
	if renameErr := os.Rename(srcAbs, dstAbs); renameErr != nil {
		return ArchiveResult{}, sberrors.Wrap(renameErr, sberrors.ErrCodeInternalError, "failed to move file to archive")
	}

	// Append archive ledger entry with original metadata.
	if err := m.appendArchiveLedgerEntry(&existing, fp); err != nil {
		return ArchiveResult{}, err
	}

	// Delete embedding.
	if err := m.embRepo.Delete(ctx, fp); err != nil {
		// Embedding deletion failure is non-fatal; embedding might not exist.
		if !sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound) {
			return ArchiveResult{}, err
		}
	}

	// Delete from DB.
	if err := m.fileRepo.Delete(ctx, fp); err != nil {
		return ArchiveResult{}, err
	}

	return ArchiveResult{
		OriginalPath: fp,
		ArchivePath:  fp,
		Metadata:     existing,
	}, nil
}

// acquireSharedLock attempts to acquire a shared lock.
// Returns REBUILD_IN_PROGRESS if the lock cannot be acquired.
func (m *Manager) acquireSharedLock() error {
	acquired, err := m.locker.TryLockShared()
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to acquire shared lock")
	}
	if !acquired {
		return sberrors.New(sberrors.ErrCodeRebuildInProgress, "a rebuild is in progress, please try again later")
	}
	return nil
}

// unlockShared releases the shared lock, ignoring errors since this
// is called in defer.
func (m *Manager) unlockShared() {
	_ = m.locker.UnlockShared() //nolint:errcheck // Called in defer; nothing to do on error.
}

// hasChanges checks whether an UpdateOptions has at least one change requested.
func hasChanges(opts *UpdateOptions) bool {
	if opts.Tags != nil {
		return true
	}
	if opts.SourceAgent != "" {
		return true
	}
	if opts.Summary != nil {
		return true
	}
	return false
}

// mergeUpdate applies non-nil/non-empty fields from UpdateOptions onto existing metadata.
func mergeUpdate(existing *store.FileMetadata, opts *UpdateOptions) store.FileMetadata {
	merged := *existing

	if opts.Tags != nil {
		merged.Tags = opts.Tags
	}
	if opts.SourceAgent != "" {
		merged.SourceAgent = opts.SourceAgent
	}
	if opts.Summary != nil {
		merged.Summary = *opts.Summary
	}

	return merged
}

// generateEmbedding generates and stores a vector embedding for the file.
// Failures are swallowed (fail open) since embedding is optional.
func (m *Manager) generateEmbedding(ctx context.Context, fp, absPath string) {
	// Skip if no embedding provider is configured.
	if m.embedder.ModelID() == noopModelID {
		return
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return
	}

	vector, err := m.embedder.GenerateEmbedding(ctx, string(content))
	if err != nil {
		return
	}

	_ = m.embRepo.Upsert(ctx, fp, vector, m.embedder.ModelID()) //nolint:errcheck // Embedding upsert is fail-open.
}

// appendLedgerEntry appends a create or update entry to the ledger.
func (m *Manager) appendLedgerEntry(operation string, meta *store.FileMetadata) error {
	payload := ledger.CreatePayload{
		Tags:        meta.Tags,
		Summary:     meta.Summary,
		SourceAgent: meta.SourceAgent,
		CreatedAt:   meta.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   meta.UpdatedAt.UTC().Format(time.RFC3339),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to marshal ledger payload")
	}

	entry := &ledger.Entry{
		Timestamp:   time.Now().UTC(),
		Operation:   operation,
		Filepath:    meta.Filepath,
		SourceAgent: meta.SourceAgent,
		Payload:     payloadBytes,
	}

	return m.ledger.Append(entry)
}

// appendDeleteLedgerEntry appends a delete entry to the ledger.
func (m *Manager) appendDeleteLedgerEntry(meta *store.FileMetadata) error {
	payload := ledger.DeletePayload{}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to marshal delete payload")
	}

	entry := &ledger.Entry{
		Timestamp:   time.Now().UTC(),
		Operation:   "delete",
		Filepath:    meta.Filepath,
		SourceAgent: meta.SourceAgent,
		Payload:     payloadBytes,
	}

	return m.ledger.Append(entry)
}

// appendArchiveLedgerEntry appends an archive entry to the ledger with original metadata.
func (m *Manager) appendArchiveLedgerEntry(meta *store.FileMetadata, archivePath string) error {
	payload := ledger.ArchivePayload{
		Tags:        meta.Tags,
		Summary:     meta.Summary,
		SourceAgent: meta.SourceAgent,
		CreatedAt:   meta.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   meta.UpdatedAt.UTC().Format(time.RFC3339),
		ArchivedTo:  archivePath,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to marshal archive payload")
	}

	entry := &ledger.Entry{
		Timestamp:   time.Now().UTC(),
		Operation:   "archive",
		Filepath:    meta.Filepath,
		SourceAgent: meta.SourceAgent,
		Payload:     payloadBytes,
	}

	return m.ledger.Append(entry)
}
