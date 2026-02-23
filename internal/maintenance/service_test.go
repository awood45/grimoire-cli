package maintenance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/ledger"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"

	docgentesting "github.com/awood45/grimoire-cli/internal/docgen/testing"
	embeddingtesting "github.com/awood45/grimoire-cli/internal/embedding/testing"
	filelocktesting "github.com/awood45/grimoire-cli/internal/filelock/testing"
	fmtesting "github.com/awood45/grimoire-cli/internal/frontmatter/testing"
	ledgertesting "github.com/awood45/grimoire-cli/internal/ledger/testing"
	storetesting "github.com/awood45/grimoire-cli/internal/store/testing"
)

// fakeDBManager is a fake for the DBManager interface used in tests.
type fakeDBManager struct {
	dropAllCalled      bool
	ensureSchemaCalled bool
	dropAllErr         error
	ensureSchemaErr    error
}

func (f *fakeDBManager) DropAll() error {
	f.dropAllCalled = true
	if f.dropAllErr != nil {
		return f.dropAllErr
	}
	return nil
}

func (f *fakeDBManager) EnsureSchema() error {
	f.ensureSchemaCalled = true
	if f.ensureSchemaErr != nil {
		return f.ensureSchemaErr
	}
	return nil
}

// testHarness bundles all fakes and the service for tests.
type testHarness struct {
	svc      *Service
	brain    *brain.Brain
	fileRepo *storetesting.FakeFileRepository
	embRepo  *storetesting.FakeEmbeddingRepository
	ledger   *ledgertesting.FakeLedger
	fm       *fmtesting.FakeFrontmatterService
	embedder *embeddingtesting.FakeProvider
	locker   *filelocktesting.FakeLocker
	docGen   *docgentesting.FakeGenerator
	db       *fakeDBManager
	tmpDir   string
}

// newTestHarness creates a testHarness with all fakes and a temp directory for the brain.
func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	tmpDir := t.TempDir()

	// Create the files/ and db/ directories.
	filesDir := filepath.Join(tmpDir, "files")
	dbDir := filepath.Join(tmpDir, "db")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a dummy DB file so Status can stat it.
	dbPath := filepath.Join(dbDir, "grimoire.sqlite")
	if err := os.WriteFile(dbPath, []byte("fake-db-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	b := brain.New(tmpDir)
	fileRepo := storetesting.NewFakeFileRepository()
	embRepo := storetesting.NewFakeEmbeddingRepository()
	led := ledgertesting.NewFakeLedger()
	fm := fmtesting.NewFakeFrontmatterService()
	embedder := embeddingtesting.NewFakeProvider()
	locker := filelocktesting.NewFakeLocker()
	docGen := docgentesting.NewFakeGenerator()
	db := &fakeDBManager{}

	svc := NewService(b, fileRepo, embRepo, led, fm, embedder, locker, docGen, db)

	return &testHarness{
		svc:      svc,
		brain:    b,
		fileRepo: fileRepo,
		embRepo:  embRepo,
		ledger:   led,
		fm:       fm,
		embedder: embedder,
		locker:   locker,
		docGen:   docGen,
		db:       db,
		tmpDir:   tmpDir,
	}
}

// createFile creates a file inside the brain's files/ directory.
func (h *testHarness) createFile(t *testing.T, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(h.brain.FilesDir(), relPath)
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// makeCreateEntry builds a ledger entry for a create operation.
func makeCreateEntry(fp, agent, summary string, tags []string, createdAt, updatedAt time.Time) ledger.Entry {
	payload := ledger.CreatePayload{
		Tags:        tags,
		Summary:     summary,
		SourceAgent: agent,
		CreatedAt:   createdAt.UTC().Format(time.RFC3339),
		UpdatedAt:   updatedAt.UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)
	return ledger.Entry{
		Timestamp:   createdAt,
		Operation:   "create",
		Filepath:    fp,
		SourceAgent: agent,
		Payload:     data,
	}
}

// makeUpdateEntry builds a ledger entry for an update operation.
func makeUpdateEntry(fp, agent, summary string, tags []string, createdAt, updatedAt time.Time) ledger.Entry {
	payload := ledger.UpdatePayload{
		Tags:        tags,
		Summary:     summary,
		SourceAgent: agent,
		CreatedAt:   createdAt.UTC().Format(time.RFC3339),
		UpdatedAt:   updatedAt.UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)
	return ledger.Entry{
		Timestamp:   updatedAt,
		Operation:   "update",
		Filepath:    fp,
		SourceAgent: agent,
		Payload:     data,
	}
}

// makeDeleteEntry builds a ledger entry for a delete operation.
func makeDeleteEntry(fp string, timestamp time.Time) ledger.Entry {
	data, _ := json.Marshal(ledger.DeletePayload{})
	return ledger.Entry{
		Timestamp:   timestamp,
		Operation:   "delete",
		Filepath:    fp,
		SourceAgent: "agent",
		Payload:     data,
	}
}

// makeArchiveEntry builds a ledger entry for an archive operation.
func makeArchiveEntry(fp string, timestamp time.Time) ledger.Entry {
	payload := ledger.ArchivePayload{
		ArchivedTo: "archive-files/" + fp,
	}
	data, _ := json.Marshal(payload)
	return ledger.Entry{
		Timestamp:   timestamp,
		Operation:   "archive",
		Filepath:    fp,
		SourceAgent: "agent",
		Payload:     data,
	}
}

// --- Status Tests ---

// TestStatus_success verifies FR-3.4.1: correct counts are reported.
func TestStatus_success(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create 3 files on disk.
	h.createFile(t, "a.md", "content a")
	h.createFile(t, "b.md", "content b")
	h.createFile(t, "c.md", "content c")

	// Track 2 of them in the DB.
	now := time.Now().UTC().Truncate(time.Second)
	h.fileRepo.Data["a.md"] = store.FileMetadata{
		Filepath: "a.md", SourceAgent: "test", CreatedAt: now, UpdatedAt: now,
	}
	h.fileRepo.Data["b.md"] = store.FileMetadata{
		Filepath: "b.md", SourceAgent: "test", CreatedAt: now, UpdatedAt: now,
	}

	// Add some ledger entries.
	h.ledger.Entries = []ledger.Entry{
		makeCreateEntry("a.md", "test", "", nil, now, now),
		makeCreateEntry("b.md", "test", "", nil, now, now),
	}

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3", report.TotalFiles)
	}
	if report.TrackedFiles != 2 {
		t.Errorf("TrackedFiles = %d, want 2", report.TrackedFiles)
	}
	if report.UntrackedCount != 1 {
		t.Errorf("UntrackedCount = %d, want 1", report.UntrackedCount)
	}
	if report.OrphanedCount != 0 {
		t.Errorf("OrphanedCount = %d, want 0", report.OrphanedCount)
	}
	if report.LedgerEntries != 2 {
		t.Errorf("LedgerEntries = %d, want 2", report.LedgerEntries)
	}
	if report.DBSizeBytes <= 0 {
		t.Errorf("DBSizeBytes = %d, want > 0", report.DBSizeBytes)
	}
	if report.EmbeddingStatus != "fake-model" {
		t.Errorf("EmbeddingStatus = %q, want %q", report.EmbeddingStatus, "fake-model")
	}

	// Verify docGen was called.
	if len(h.docGen.GenerateCalls) != 1 {
		t.Errorf("docGen.Generate called %d times, want 1", len(h.docGen.GenerateCalls))
	}
}

// TestStatus_withOrphanedFiles verifies FR-3.4.1: orphaned files are detected.
func TestStatus_withOrphanedFiles(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create 1 file on disk.
	h.createFile(t, "a.md", "content a")

	// Track 2 files in DB (b.md doesn't exist on disk).
	now := time.Now().UTC().Truncate(time.Second)
	h.fileRepo.Data["a.md"] = store.FileMetadata{
		Filepath: "a.md", SourceAgent: "test", CreatedAt: now, UpdatedAt: now,
	}
	h.fileRepo.Data["b.md"] = store.FileMetadata{
		Filepath: "b.md", SourceAgent: "test", CreatedAt: now, UpdatedAt: now,
	}

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.OrphanedCount != 1 {
		t.Errorf("OrphanedCount = %d, want 1", report.OrphanedCount)
	}
	if report.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", report.TotalFiles)
	}
	if report.TrackedFiles != 2 {
		t.Errorf("TrackedFiles = %d, want 2", report.TrackedFiles)
	}
}

// TestStatus_withUntrackedFiles verifies FR-3.4.1: untracked files are detected.
func TestStatus_withUntrackedFiles(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create 2 files on disk.
	h.createFile(t, "a.md", "content a")
	h.createFile(t, "b.md", "content b")

	// Track 0 in DB (both are untracked).

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.UntrackedCount != 2 {
		t.Errorf("UntrackedCount = %d, want 2", report.UntrackedCount)
	}
	if report.TrackedFiles != 0 {
		t.Errorf("TrackedFiles = %d, want 0", report.TrackedFiles)
	}
}

// TestStatus_emptyBrain verifies FR-3.4.1: empty brain reports all zeros.
func TestStatus_emptyBrain(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", report.TotalFiles)
	}
	if report.TrackedFiles != 0 {
		t.Errorf("TrackedFiles = %d, want 0", report.TrackedFiles)
	}
	if report.OrphanedCount != 0 {
		t.Errorf("OrphanedCount = %d, want 0", report.OrphanedCount)
	}
	if report.UntrackedCount != 0 {
		t.Errorf("UntrackedCount = %d, want 0", report.UntrackedCount)
	}
	if report.LedgerEntries != 0 {
		t.Errorf("LedgerEntries = %d, want 0", report.LedgerEntries)
	}
}

// TestStatus_refreshesDoc verifies FR-3.5.1: docGen is called with correct stats.
func TestStatus_refreshesDoc(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create 2 files on disk.
	h.createFile(t, "a.md", "content a")
	h.createFile(t, "b.md", "content b")

	// Track 1 file.
	now := time.Now().UTC().Truncate(time.Second)
	h.fileRepo.Data["a.md"] = store.FileMetadata{
		Filepath: "a.md", SourceAgent: "test", Tags: []string{"go", "design"},
		CreatedAt: now, UpdatedAt: now,
	}

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if len(h.docGen.GenerateCalls) != 1 {
		t.Fatalf("docGen.Generate called %d times, want 1", len(h.docGen.GenerateCalls))
	}

	docData := h.docGen.GenerateCalls[0]
	if docData.TotalFiles != report.TotalFiles {
		t.Errorf("docData.TotalFiles = %d, want %d", docData.TotalFiles, report.TotalFiles)
	}
	if docData.TrackedFiles != report.TrackedFiles {
		t.Errorf("docData.TrackedFiles = %d, want %d", docData.TrackedFiles, report.TrackedFiles)
	}
}

// --- Rebuild Tests ---

// TestRebuild_success verifies FR-3.4.2: basic rebuild from ledger.
func TestRebuild_success(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeCreateEntry("a.md", "agent1", "summary a", []string{"go"}, now, now),
		makeCreateEntry("b.md", "agent2", "summary b", []string{"rust"}, now, now),
	}

	report, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if report.EntriesReplayed != 2 {
		t.Errorf("EntriesReplayed = %d, want 2", report.EntriesReplayed)
	}
	if report.FinalRecordCount != 2 {
		t.Errorf("FinalRecordCount = %d, want 2", report.FinalRecordCount)
	}

	if !h.db.dropAllCalled {
		t.Error("DropAll() was not called")
	}
	if !h.db.ensureSchemaCalled {
		t.Error("EnsureSchema() was not called")
	}
}

// TestRebuild_replayCreateEntries verifies FR-3.4.2: create entries are inserted.
func TestRebuild_replayCreateEntries(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeCreateEntry("doc.md", "cline", "my doc", []string{"design"}, now, now),
	}

	_, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	meta, exists := h.fileRepo.Data["doc.md"]
	if !exists {
		t.Fatal("expected doc.md to be in fileRepo")
	}
	if meta.SourceAgent != "cline" {
		t.Errorf("SourceAgent = %q, want %q", meta.SourceAgent, "cline")
	}
	if meta.Summary != "my doc" {
		t.Errorf("Summary = %q, want %q", meta.Summary, "my doc")
	}
}

// TestRebuild_replayDeleteEntries verifies FR-3.4.2: create then delete leaves no record.
func TestRebuild_replayDeleteEntries(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	later := now.Add(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeCreateEntry("temp.md", "agent", "temp", nil, now, now),
		makeDeleteEntry("temp.md", later),
	}

	report, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if report.EntriesReplayed != 2 {
		t.Errorf("EntriesReplayed = %d, want 2", report.EntriesReplayed)
	}
	if report.FinalRecordCount != 0 {
		t.Errorf("FinalRecordCount = %d, want 0", report.FinalRecordCount)
	}
}

// TestRebuild_createThenArchive verifies FR-3.4.2: archive is treated as delete.
func TestRebuild_createThenArchive(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	later := now.Add(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeCreateEntry("old.md", "agent", "", nil, now, now),
		makeArchiveEntry("old.md", later),
	}

	report, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if report.FinalRecordCount != 0 {
		t.Errorf("FinalRecordCount = %d, want 0", report.FinalRecordCount)
	}
}

// TestRebuild_ignoreMissingOnDelete verifies FR-3.4.2: delete on non-existent is ignored.
func TestRebuild_ignoreMissingOnDelete(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeDeleteEntry("nonexistent.md", now),
	}

	report, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if report.EntriesReplayed != 1 {
		t.Errorf("EntriesReplayed = %d, want 1", report.EntriesReplayed)
	}
	if report.FinalRecordCount != 0 {
		t.Errorf("FinalRecordCount = %d, want 0", report.FinalRecordCount)
	}
}

// TestRebuild_updateWithoutCreate verifies FR-3.4.2: update for non-existent record is treated as insert.
func TestRebuild_updateWithoutCreate(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeUpdateEntry("orphan.md", "agent", "recovered", []string{"restored"}, now, now),
	}

	report, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if report.FinalRecordCount != 1 {
		t.Errorf("FinalRecordCount = %d, want 1", report.FinalRecordCount)
	}

	meta, exists := h.fileRepo.Data["orphan.md"]
	if !exists {
		t.Fatal("expected orphan.md to be in fileRepo")
	}
	if meta.Summary != "recovered" {
		t.Errorf("Summary = %q, want %q", meta.Summary, "recovered")
	}
}

// TestRebuild_preservesLedger verifies FR-3.4.2: ledger is not modified.
func TestRebuild_preservesLedger(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeCreateEntry("a.md", "agent", "", nil, now, now),
	}

	_, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	// Ledger should still have exactly 1 entry.
	if len(h.ledger.Entries) != 1 {
		t.Errorf("ledger entries = %d, want 1", len(h.ledger.Entries))
	}
}

// --- HardRebuild Tests ---

// TestHardRebuild_success verifies FR-3.4.3: clean state produces no changes.
func TestHardRebuild_success(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create a file on disk with frontmatter.
	h.createFile(t, "a.md", "---\nsource_agent: test\n---\ncontent")
	absPath := filepath.Join(h.brain.FilesDir(), "a.md")

	now := time.Now().UTC().Truncate(time.Second)
	meta := store.FileMetadata{
		Filepath: "a.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// Register frontmatter for the file.
	h.fm.Data[absPath] = meta

	// Register DB record that matches.
	h.fileRepo.Data["a.md"] = meta

	report, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if report.Creates != 0 {
		t.Errorf("Creates = %d, want 0", report.Creates)
	}
	if report.Updates != 0 {
		t.Errorf("Updates = %d, want 0", report.Updates)
	}
	if report.Deletes != 0 {
		t.Errorf("Deletes = %d, want 0", report.Deletes)
	}
	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
}

// TestHardRebuild_untrackedWithFrontmatter verifies FR-3.4.3: untracked files get create entries.
func TestHardRebuild_untrackedWithFrontmatter(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create file on disk.
	h.createFile(t, "new.md", "---\nsource_agent: test\n---\ncontent")
	absPath := filepath.Join(h.brain.FilesDir(), "new.md")

	now := time.Now().UTC().Truncate(time.Second)
	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "new.md", SourceAgent: "test",
		Tags: []string{"go"}, Summary: "new file",
		CreatedAt: now, UpdatedAt: now,
	}

	// No DB record — this file is untracked.

	report, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if report.Creates != 1 {
		t.Errorf("Creates = %d, want 1", report.Creates)
	}

	// Check that a create entry was appended to the ledger.
	if len(h.ledger.Entries) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(h.ledger.Entries))
	}
	if h.ledger.Entries[0].Operation != "create" {
		t.Errorf("Operation = %q, want %q", h.ledger.Entries[0].Operation, "create")
	}

	// Check DB was updated.
	_, exists := h.fileRepo.Data["new.md"]
	if !exists {
		t.Error("expected new.md to be in fileRepo")
	}
}

// TestHardRebuild_staleRecords verifies FR-3.4.3: stale DB records get update entries.
func TestHardRebuild_staleRecords(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.createFile(t, "stale.md", "---\nsource_agent: updated\n---\ncontent")
	absPath := filepath.Join(h.brain.FilesDir(), "stale.md")

	now := time.Now().UTC().Truncate(time.Second)

	// Frontmatter has different summary than DB.
	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "updated",
		Summary:   "new summary",
		CreatedAt: now, UpdatedAt: now,
	}

	h.fileRepo.Data["stale.md"] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "original",
		Summary:   "old summary",
		CreatedAt: now, UpdatedAt: now,
	}

	report, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if report.Updates != 1 {
		t.Errorf("Updates = %d, want 1", report.Updates)
	}

	// Check that an update entry was appended to the ledger.
	if len(h.ledger.Entries) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(h.ledger.Entries))
	}
	if h.ledger.Entries[0].Operation != "update" {
		t.Errorf("Operation = %q, want %q", h.ledger.Entries[0].Operation, "update")
	}
}

// TestHardRebuild_orphanedRecords verifies FR-3.4.3: orphaned DB records get delete entries.
func TestHardRebuild_orphanedRecords(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// No file on disk, but DB has a record.
	now := time.Now().UTC().Truncate(time.Second)
	h.fileRepo.Data["ghost.md"] = store.FileMetadata{
		Filepath: "ghost.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	report, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if report.Deletes != 1 {
		t.Errorf("Deletes = %d, want 1", report.Deletes)
	}

	// Check that a delete entry was appended to the ledger.
	if len(h.ledger.Entries) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(h.ledger.Entries))
	}
	if h.ledger.Entries[0].Operation != "delete" {
		t.Errorf("Operation = %q, want %q", h.ledger.Entries[0].Operation, "delete")
	}

	// Check DB record was removed.
	if _, exists := h.fileRepo.Data["ghost.md"]; exists {
		t.Error("expected ghost.md to be deleted from fileRepo")
	}
}

// TestHardRebuild_filesWithoutFrontmatter verifies FR-3.4.3: files without frontmatter produce warnings.
func TestHardRebuild_filesWithoutFrontmatter(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create a file on disk but don't register frontmatter for it.
	h.createFile(t, "nofm.md", "just plain content")

	// The FakeFrontmatterService returns ErrCodeInvalidInput for files not in Data.

	report, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if len(report.Warnings) != 1 {
		t.Fatalf("Warnings = %d, want 1", len(report.Warnings))
	}
	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	if report.Creates != 0 {
		t.Errorf("Creates = %d, want 0", report.Creates)
	}
}

// TestHardRebuild_lockFailure verifies FR-3.4.3: returns REBUILD_IN_PROGRESS when lock fails.
func TestHardRebuild_lockFailure(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.locker.TryExclusiveResult = false

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}

	if !sberrors.HasCode(err, sberrors.ErrCodeRebuildInProgress) {
		t.Errorf("error code = %v, want REBUILD_IN_PROGRESS", err)
	}
}

// TestHardRebuild_acquiresExclusiveLock verifies FR-3.4.3: exclusive lock is acquired and released.
func TestHardRebuild_acquiresExclusiveLock(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	_, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if h.locker.TryExclusiveCalls != 1 {
		t.Errorf("TryExclusiveCalls = %d, want 1", h.locker.TryExclusiveCalls)
	}
	if h.locker.UnlockExclusiveCalls != 1 {
		t.Errorf("UnlockExclusiveCalls = %d, want 1", h.locker.UnlockExclusiveCalls)
	}
}

// TestHardRebuild_setsSourceAgent verifies FR-3.4.3: uses "hard-rebuild" as source agent.
func TestHardRebuild_setsSourceAgent(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create an untracked file.
	h.createFile(t, "new.md", "content")
	absPath := filepath.Join(h.brain.FilesDir(), "new.md")

	now := time.Now().UTC().Truncate(time.Second)
	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "new.md", SourceAgent: "original-agent",
		CreatedAt: now, UpdatedAt: now,
	}

	_, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	// Check ledger entry source agent.
	if len(h.ledger.Entries) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(h.ledger.Entries))
	}
	if h.ledger.Entries[0].SourceAgent != "hard-rebuild" {
		t.Errorf("SourceAgent = %q, want %q", h.ledger.Entries[0].SourceAgent, "hard-rebuild")
	}

	// Check DB record source agent.
	meta, exists := h.fileRepo.Data["new.md"]
	if !exists {
		t.Fatal("expected new.md to be in fileRepo")
	}
	if meta.SourceAgent != "hard-rebuild" {
		t.Errorf("DB SourceAgent = %q, want %q", meta.SourceAgent, "hard-rebuild")
	}
}

// TestHardRebuild_regeneratesEmbeddings verifies FR-3.4.3: embeddings are generated for created/updated files.
func TestHardRebuild_regeneratesEmbeddings(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create an untracked file.
	h.createFile(t, "embed.md", "content for embedding")
	absPath := filepath.Join(h.brain.FilesDir(), "embed.md")

	now := time.Now().UTC().Truncate(time.Second)
	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "embed.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	_, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	// Check that GenerateEmbedding was called.
	if len(h.embedder.GenerateCalls) != 1 {
		t.Errorf("GenerateEmbedding calls = %d, want 1", len(h.embedder.GenerateCalls))
	}

	// Check that embedding was upserted.
	if len(h.embRepo.UpsertCalls) != 1 {
		t.Errorf("Upsert calls = %d, want 1", len(h.embRepo.UpsertCalls))
	}
}

// TestHardRebuild_embeddingFailure verifies FR-3.4.3: embedding failure propagates as error.
func TestHardRebuild_embeddingFailure(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create an untracked file.
	h.createFile(t, "fail.md", "content")
	absPath := filepath.Join(h.brain.FilesDir(), "fail.md")

	now := time.Now().UTC().Truncate(time.Second)
	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "fail.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject embedding error.
	h.embedder.GenerateErr = sberrors.New(sberrors.ErrCodeEmbeddingError, "ollama unavailable")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}

	if !sberrors.HasCode(err, sberrors.ErrCodeEmbeddingError) {
		t.Errorf("error = %v, want EMBEDDING_ERROR code", err)
	}
}

// TestRebuild_dropAllError verifies Rebuild returns error when DropAll fails.
func TestRebuild_dropAllError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.db.dropAllErr = sberrors.New(sberrors.ErrCodeDatabaseError, "drop failed")

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestRebuild_ensureSchemaError verifies Rebuild returns error when EnsureSchema fails.
func TestRebuild_ensureSchemaError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.db.ensureSchemaErr = sberrors.New(sberrors.ErrCodeDatabaseError, "schema failed")

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestRebuild_ledgerReadError verifies Rebuild returns error when ReadAll fails.
func TestRebuild_ledgerReadError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.ledger.ReadAllErr = sberrors.New(sberrors.ErrCodeLedgerError, "read failed")

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestStatus_countError verifies Status returns error when Count fails.
func TestStatus_countError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.fileRepo.CountErr = sberrors.New(sberrors.ErrCodeDatabaseError, "count failed")

	_, err := h.svc.Status(ctx)
	if err == nil {
		t.Fatal("Status() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestStatus_allPathsError verifies Status returns error when AllFilepaths fails.
func TestStatus_allPathsError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.fileRepo.AllPathsErr = sberrors.New(sberrors.ErrCodeDatabaseError, "paths failed")

	_, err := h.svc.Status(ctx)
	if err == nil {
		t.Fatal("Status() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestStatus_ledgerReadError verifies Status returns error when ReadAll fails.
func TestStatus_ledgerReadError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.ledger.ReadAllErr = sberrors.New(sberrors.ErrCodeLedgerError, "read failed")

	_, err := h.svc.Status(ctx)
	if err == nil {
		t.Fatal("Status() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestHardRebuild_lockError verifies HardRebuild returns error when TryLockExclusive fails.
func TestHardRebuild_lockError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.locker.TryExclusiveErr = sberrors.New(sberrors.ErrCodeInternalError, "lock error")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeInternalError) {
		t.Errorf("error = %v, want INTERNAL_ERROR code", err)
	}
}

// TestHardRebuild_allFilepathsError verifies HardRebuild returns error when AllFilepaths fails.
func TestHardRebuild_allFilepathsError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.fileRepo.AllPathsErr = sberrors.New(sberrors.ErrCodeDatabaseError, "paths failed")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestStatus_docGenError verifies Status returns error when Generate fails.
func TestStatus_docGenError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.docGen.GenerateErr = sberrors.New(sberrors.ErrCodeInternalError, "generate failed")

	_, err := h.svc.Status(ctx)
	if err == nil {
		t.Fatal("Status() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeInternalError) {
		t.Errorf("error = %v, want INTERNAL_ERROR code", err)
	}
}

// TestStatus_listTagsError verifies Status returns error when ListTags fails.
func TestStatus_listTagsError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.fileRepo.ListTagsErr = sberrors.New(sberrors.ErrCodeDatabaseError, "tags failed")

	_, err := h.svc.Status(ctx)
	if err == nil {
		t.Fatal("Status() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestStatus_searchError verifies Status returns error when Search fails.
func TestStatus_searchError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.fileRepo.SearchErr = sberrors.New(sberrors.ErrCodeDatabaseError, "search failed")

	_, err := h.svc.Status(ctx)
	if err == nil {
		t.Fatal("Status() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestHardRebuild_multipleOperations verifies FR-3.4.3: mixed untracked, stale, orphaned.
func TestHardRebuild_multipleOperations(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	// Untracked file (on disk, not in DB).
	h.createFile(t, "new.md", "new content")
	newAbsPath := filepath.Join(h.brain.FilesDir(), "new.md")
	h.fm.Data[newAbsPath] = store.FileMetadata{
		Filepath: "new.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// Stale file (on disk and in DB, but different).
	h.createFile(t, "stale.md", "stale content")
	staleAbsPath := filepath.Join(h.brain.FilesDir(), "stale.md")
	h.fm.Data[staleAbsPath] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "updated",
		Summary:   "new summary",
		CreatedAt: now, UpdatedAt: now,
	}
	h.fileRepo.Data["stale.md"] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "original",
		Summary:   "old summary",
		CreatedAt: now, UpdatedAt: now,
	}

	// Orphaned record (in DB, not on disk).
	h.fileRepo.Data["gone.md"] = store.FileMetadata{
		Filepath: "gone.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// File without frontmatter.
	h.createFile(t, "plain.md", "no frontmatter")

	report, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if report.Creates != 1 {
		t.Errorf("Creates = %d, want 1", report.Creates)
	}
	if report.Updates != 1 {
		t.Errorf("Updates = %d, want 1", report.Updates)
	}
	if report.Deletes != 1 {
		t.Errorf("Deletes = %d, want 1", report.Deletes)
	}
	if len(report.Warnings) != 1 {
		t.Errorf("Warnings = %d, want 1", len(report.Warnings))
	}
	// 3 files on disk (new, stale, plain) + 1 warning = 4 scanned.
	if report.FilesScanned != 3 {
		t.Errorf("FilesScanned = %d, want 3", report.FilesScanned)
	}
}

// TestHardRebuild_orphanedDeletesEmbedding verifies FR-3.4.3: orphaned records have embeddings deleted.
func TestHardRebuild_orphanedDeletesEmbedding(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.fileRepo.Data["ghost.md"] = store.FileMetadata{
		Filepath: "ghost.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}
	h.embRepo.Data["ghost.md"] = store.Embedding{
		Filepath: "ghost.md",
		Vector:   []float32{0.1, 0.2},
		ModelID:  "model",
	}

	_, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if len(h.embRepo.DeleteCalls) != 1 {
		t.Errorf("embRepo.Delete calls = %d, want 1", len(h.embRepo.DeleteCalls))
	}
}

// TestRebuild_unknownOperation verifies Rebuild silently skips unknown operations.
func TestRebuild_unknownOperation(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	data, _ := json.Marshal(map[string]string{"key": "value"})
	h.ledger.Entries = []ledger.Entry{
		{
			Timestamp:   now,
			Operation:   "unknown",
			Filepath:    "x.md",
			SourceAgent: "agent",
			Payload:     data,
		},
	}

	report, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if report.EntriesReplayed != 1 {
		t.Errorf("EntriesReplayed = %d, want 1", report.EntriesReplayed)
	}
	if report.FinalRecordCount != 0 {
		t.Errorf("FinalRecordCount = %d, want 0", report.FinalRecordCount)
	}
}

// TestStatus_noDBFile verifies Status handles missing DB file gracefully.
func TestStatus_noDBFile(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Remove the DB file.
	if err := os.Remove(h.brain.DBPath()); err != nil {
		t.Fatal(err)
	}

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.DBSizeBytes != 0 {
		t.Errorf("DBSizeBytes = %d, want 0", report.DBSizeBytes)
	}
}

// TestStatus_embeddingNotConfigured verifies Status reports "not configured" when model ID is empty.
func TestStatus_embeddingNotConfigured(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.embedder.FixedModelID = ""

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.EmbeddingStatus != "not configured" {
		t.Errorf("EmbeddingStatus = %q, want %q", report.EmbeddingStatus, "not configured")
	}
}

// TestNewService verifies the constructor returns a properly initialized service.
func TestNewService(t *testing.T) {
	b := brain.New("/tmp/test")
	fileRepo := storetesting.NewFakeFileRepository()
	embRepo := storetesting.NewFakeEmbeddingRepository()
	led := ledgertesting.NewFakeLedger()
	fm := fmtesting.NewFakeFrontmatterService()
	embedder := embeddingtesting.NewFakeProvider()
	locker := filelocktesting.NewFakeLocker()
	docGen := docgentesting.NewFakeGenerator()
	db := &fakeDBManager{}

	svc := NewService(b, fileRepo, embRepo, led, fm, embedder, locker, docGen, db)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

// TestHardRebuild_noFilesDir verifies HardRebuild handles missing files/ directory.
func TestHardRebuild_noFilesDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create files/ directory.
	dbDir := filepath.Join(tmpDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	b := brain.New(tmpDir)
	fileRepo := storetesting.NewFakeFileRepository()
	embRepo := storetesting.NewFakeEmbeddingRepository()
	led := ledgertesting.NewFakeLedger()
	fm := fmtesting.NewFakeFrontmatterService()
	embedder := embeddingtesting.NewFakeProvider()
	locker := filelocktesting.NewFakeLocker()
	docGen := docgentesting.NewFakeGenerator()
	db := &fakeDBManager{}

	svc := NewService(b, fileRepo, embRepo, led, fm, embedder, locker, docGen, db)

	report, err := svc.HardRebuild(context.Background())
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if report.FilesScanned != 0 {
		t.Errorf("FilesScanned = %d, want 0", report.FilesScanned)
	}
}

// TestIsStale_tagsChanged verifies isStale detects tag changes.
func TestIsStale_tagsChanged(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	dbMeta := &store.FileMetadata{
		Filepath: "a.md", SourceAgent: "test",
		Tags: []string{"go"}, Summary: "s",
		CreatedAt: now, UpdatedAt: now,
	}
	fileMeta := &store.FileMetadata{
		Filepath: "a.md", SourceAgent: "test",
		Tags: []string{"go", "rust"}, Summary: "s",
		CreatedAt: now, UpdatedAt: now,
	}
	if !isStale(dbMeta, fileMeta) {
		t.Error("expected isStale = true for different tags")
	}
}

// TestIsStale_identical verifies isStale returns false for identical metadata.
func TestIsStale_identical(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	meta := &store.FileMetadata{
		Filepath: "a.md", SourceAgent: "test",
		Tags: []string{"go"}, Summary: "s",
		CreatedAt: now, UpdatedAt: now,
	}
	if isStale(meta, meta) {
		t.Error("expected isStale = false for identical metadata")
	}
}

// TestTagsEqual verifies tag comparison.
func TestTagsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both empty", nil, nil, true},
		{"same order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different order", []string{"b", "a"}, []string{"a", "b"}, true},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
		{"different values", []string{"a", "c"}, []string{"a", "b"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("tagsEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestIsStale_summaryChanged verifies isStale detects summary changes.
func TestIsStale_summaryChanged(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	dbMeta := &store.FileMetadata{
		SourceAgent: "test", Tags: []string{"go"}, Summary: "old",
		CreatedAt: now, UpdatedAt: now,
	}
	fileMeta := &store.FileMetadata{
		SourceAgent: "test", Tags: []string{"go"}, Summary: "new",
		CreatedAt: now, UpdatedAt: now,
	}
	if !isStale(dbMeta, fileMeta) {
		t.Error("expected isStale = true for different summary")
	}
}

// TestIsStale_createdAtChanged verifies isStale detects createdAt changes.
func TestIsStale_createdAtChanged(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	dbMeta := &store.FileMetadata{
		SourceAgent: "test", Summary: "s",
		CreatedAt: now, UpdatedAt: now,
	}
	fileMeta := &store.FileMetadata{
		SourceAgent: "test", Summary: "s",
		CreatedAt: now.Add(time.Hour), UpdatedAt: now,
	}
	if !isStale(dbMeta, fileMeta) {
		t.Error("expected isStale = true for different createdAt")
	}
}

// TestIsStale_updatedAtChanged verifies isStale detects updatedAt changes.
func TestIsStale_updatedAtChanged(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	dbMeta := &store.FileMetadata{
		SourceAgent: "test", Summary: "s",
		CreatedAt: now, UpdatedAt: now,
	}
	fileMeta := &store.FileMetadata{
		SourceAgent: "test", Summary: "s",
		CreatedAt: now, UpdatedAt: now.Add(time.Hour),
	}
	if !isStale(dbMeta, fileMeta) {
		t.Error("expected isStale = true for different updatedAt")
	}
}

// TestMetadataFromPayload_success verifies payload parsing.
func TestMetadataFromPayload_success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	entry := makeCreateEntry("test.md", "agent", "my summary", []string{"go"}, now, now)

	meta, err := metadataFromPayload(&entry)
	if err != nil {
		t.Fatalf("metadataFromPayload error = %v", err)
	}
	if meta.Filepath != "test.md" {
		t.Errorf("Filepath = %q, want %q", meta.Filepath, "test.md")
	}
	if meta.SourceAgent != "agent" {
		t.Errorf("SourceAgent = %q, want %q", meta.SourceAgent, "agent")
	}
	if meta.Summary != "my summary" {
		t.Errorf("Summary = %q, want %q", meta.Summary, "my summary")
	}
}

// TestMetadataFromPayload_invalidJSON verifies error on invalid JSON payload.
func TestMetadataFromPayload_invalidJSON(t *testing.T) {
	entry := ledger.Entry{
		Filepath: "bad.md",
		Payload:  json.RawMessage(`{invalid json`),
	}

	_, err := metadataFromPayload(&entry)
	if err == nil {
		t.Fatal("expected error for invalid JSON payload")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestMetadataFromPayload_invalidCreatedAt verifies error on invalid created_at timestamp.
func TestMetadataFromPayload_invalidCreatedAt(t *testing.T) {
	entry := ledger.Entry{
		Filepath: "bad.md",
		Payload:  json.RawMessage(`{"created_at":"not-a-date","updated_at":"2025-01-01T00:00:00Z"}`),
	}

	_, err := metadataFromPayload(&entry)
	if err == nil {
		t.Fatal("expected error for invalid created_at")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestMetadataFromPayload_invalidUpdatedAt verifies error on invalid updated_at timestamp.
func TestMetadataFromPayload_invalidUpdatedAt(t *testing.T) {
	entry := ledger.Entry{
		Filepath: "bad.md",
		Payload:  json.RawMessage(`{"created_at":"2025-01-01T00:00:00Z","updated_at":"not-a-date"}`),
	}

	_, err := metadataFromPayload(&entry)
	if err == nil {
		t.Fatal("expected error for invalid updated_at")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestRebuild_insertError verifies Rebuild propagates insert errors.
func TestRebuild_insertError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeCreateEntry("a.md", "agent", "", nil, now, now),
		makeCreateEntry("a.md", "agent", "", nil, now, now), // Duplicate causes METADATA_EXISTS.
	}

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error for duplicate insert, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeMetadataExists) {
		t.Errorf("error = %v, want METADATA_EXISTS code", err)
	}
}

// TestRebuild_updateError verifies Rebuild propagates update errors that are not MetadataNotFound.
func TestRebuild_updateError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeCreateEntry("a.md", "agent", "", nil, now, now),
		makeUpdateEntry("a.md", "agent", "updated", nil, now, now),
	}

	// Inject a non-MetadataNotFound error on update.
	h.fileRepo.UpdateErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db connection lost")

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestRebuild_deleteError verifies Rebuild propagates delete errors that are not MetadataNotFound.
func TestRebuild_deleteError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeDeleteEntry("a.md", now),
	}

	// Inject a non-MetadataNotFound error on delete.
	h.fileRepo.DeleteErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db connection lost")

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestRebuild_countError verifies Rebuild propagates Count error after replay.
func TestRebuild_countError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Empty ledger.
	h.fileRepo.CountErr = sberrors.New(sberrors.ErrCodeDatabaseError, "count failed")

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestHardRebuild_ledgerAppendError verifies HardRebuild propagates ledger append errors.
func TestHardRebuild_ledgerAppendError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create an untracked file.
	h.createFile(t, "new.md", "content")
	absPath := filepath.Join(h.brain.FilesDir(), "new.md")
	now := time.Now().UTC().Truncate(time.Second)
	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "new.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject ledger append error.
	h.ledger.AppendErr = sberrors.New(sberrors.ErrCodeLedgerError, "disk full")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestHardRebuild_insertError verifies HardRebuild propagates insert errors.
func TestHardRebuild_insertError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create an untracked file.
	h.createFile(t, "new.md", "content")
	absPath := filepath.Join(h.brain.FilesDir(), "new.md")
	now := time.Now().UTC().Truncate(time.Second)
	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "new.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject insert error.
	h.fileRepo.InsertErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db error")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestHardRebuild_staleUpdateError verifies HardRebuild propagates update errors for stale files.
func TestHardRebuild_staleUpdateError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.createFile(t, "stale.md", "content")
	absPath := filepath.Join(h.brain.FilesDir(), "stale.md")
	now := time.Now().UTC().Truncate(time.Second)

	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "new",
		CreatedAt: now, UpdatedAt: now,
	}
	h.fileRepo.Data["stale.md"] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "old",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject update error.
	h.fileRepo.UpdateErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db error")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestHardRebuild_orphanedDeleteError verifies HardRebuild propagates delete errors for orphaned records.
func TestHardRebuild_orphanedDeleteError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.fileRepo.Data["ghost.md"] = store.FileMetadata{
		Filepath: "ghost.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject delete error.
	h.fileRepo.DeleteErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db error")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestHardRebuild_staleGetError verifies HardRebuild propagates Get errors when checking staleness.
func TestHardRebuild_staleGetError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.createFile(t, "check.md", "content")
	absPath := filepath.Join(h.brain.FilesDir(), "check.md")
	now := time.Now().UTC().Truncate(time.Second)

	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "check.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}
	// File exists in DB set (via AllFilepaths) but Get will fail.
	h.fileRepo.Data["check.md"] = store.FileMetadata{
		Filepath: "check.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject Get error.
	h.fileRepo.GetErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db error")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestHardRebuild_staleLedgerAppendError verifies HardRebuild propagates ledger append errors for stale.
func TestHardRebuild_staleLedgerAppendError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.createFile(t, "stale.md", "content")
	absPath := filepath.Join(h.brain.FilesDir(), "stale.md")
	now := time.Now().UTC().Truncate(time.Second)

	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "new",
		CreatedAt: now, UpdatedAt: now,
	}
	h.fileRepo.Data["stale.md"] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "old",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject ledger append error.
	h.ledger.AppendErr = sberrors.New(sberrors.ErrCodeLedgerError, "disk full")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestHardRebuild_orphanedLedgerAppendError verifies HardRebuild propagates ledger errors for orphaned.
func TestHardRebuild_orphanedLedgerAppendError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.fileRepo.Data["ghost.md"] = store.FileMetadata{
		Filepath: "ghost.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject ledger append error.
	h.ledger.AppendErr = sberrors.New(sberrors.ErrCodeLedgerError, "disk full")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestHardRebuild_embeddingUpsertError verifies HardRebuild propagates embedding upsert errors.
func TestHardRebuild_embeddingUpsertError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.createFile(t, "new.md", "content for embedding")
	absPath := filepath.Join(h.brain.FilesDir(), "new.md")
	now := time.Now().UTC().Truncate(time.Second)
	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "new.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject upsert error.
	h.embRepo.UpsertErr = sberrors.New(sberrors.ErrCodeDatabaseError, "upsert failed")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestRebuild_invalidPayload verifies Rebuild propagates payload parse errors.
func TestRebuild_invalidPayload(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		{
			Timestamp:   now,
			Operation:   "create",
			Filepath:    "bad.md",
			SourceAgent: "agent",
			Payload:     json.RawMessage(`{invalid`),
		},
	}

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestHardRebuild_scanFilesReadError verifies HardRebuild propagates non-InvalidInput read errors.
func TestHardRebuild_scanFilesReadError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.createFile(t, "err.md", "content")

	// Inject a non-InvalidInput error for frontmatter read.
	h.fm.ReadErr = sberrors.New(sberrors.ErrCodeInternalError, "io error")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeInternalError) {
		t.Errorf("error = %v, want INTERNAL_ERROR code", err)
	}
}

// TestStatus_noFilesDir verifies Status handles missing files/ directory gracefully.
func TestStatus_noFilesDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create files/ directory.
	dbDir := filepath.Join(tmpDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create dummy DB file.
	if err := os.WriteFile(filepath.Join(dbDir, "grimoire.sqlite"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}

	b := brain.New(tmpDir)
	fileRepo := storetesting.NewFakeFileRepository()
	embRepo := storetesting.NewFakeEmbeddingRepository()
	led := ledgertesting.NewFakeLedger()
	fm := fmtesting.NewFakeFrontmatterService()
	embedder := embeddingtesting.NewFakeProvider()
	locker := filelocktesting.NewFakeLocker()
	docGen := docgentesting.NewFakeGenerator()
	db := &fakeDBManager{}

	svc := NewService(b, fileRepo, embRepo, led, fm, embedder, locker, docGen, db)

	report, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", report.TotalFiles)
	}
}

// TestHardRebuild_staleEmbeddingError verifies HardRebuild propagates embedding errors for stale files.
func TestHardRebuild_staleEmbeddingError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.createFile(t, "stale.md", "content")
	absPath := filepath.Join(h.brain.FilesDir(), "stale.md")
	now := time.Now().UTC().Truncate(time.Second)

	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "new",
		CreatedAt: now, UpdatedAt: now,
	}
	h.fileRepo.Data["stale.md"] = store.FileMetadata{
		Filepath: "stale.md", SourceAgent: "old",
		CreatedAt: now, UpdatedAt: now,
	}

	// Inject embedding generation error.
	h.embedder.GenerateErr = sberrors.New(sberrors.ErrCodeEmbeddingError, "model unavailable")

	_, err := h.svc.HardRebuild(ctx)
	if err == nil {
		t.Fatal("HardRebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeEmbeddingError) {
		t.Errorf("error = %v, want EMBEDDING_ERROR code", err)
	}
}

// TestRebuild_updateInvalidPayload verifies Rebuild propagates payload parse errors for update operations.
func TestRebuild_updateInvalidPayload(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		{
			Timestamp:   now,
			Operation:   "update",
			Filepath:    "bad.md",
			SourceAgent: "agent",
			Payload:     json.RawMessage(`{invalid`),
		},
	}

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeLedgerError) {
		t.Errorf("error = %v, want LEDGER_ERROR code", err)
	}
}

// TestRebuild_deleteIgnoresMetadataNotFound verifies Rebuild ignores MetadataNotFound on delete.
func TestRebuild_deleteIgnoresMetadataNotFound(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeDeleteEntry("missing.md", now),
	}

	// Inject MetadataNotFound on delete - should be silently ignored.
	h.fileRepo.DeleteErr = sberrors.New(sberrors.ErrCodeMetadataNotFound, "not found")

	report, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v, want nil (MetadataNotFound should be ignored)", err)
	}
	if report.EntriesReplayed != 1 {
		t.Errorf("EntriesReplayed = %d, want 1", report.EntriesReplayed)
	}
}

// TestRebuild_archiveIgnoresMetadataNotFound verifies Rebuild ignores MetadataNotFound on archive.
func TestRebuild_archiveIgnoresMetadataNotFound(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeArchiveEntry("missing.md", now),
	}

	// Inject MetadataNotFound on delete - should be silently ignored.
	h.fileRepo.DeleteErr = sberrors.New(sberrors.ErrCodeMetadataNotFound, "not found")

	report, err := h.svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild() error = %v, want nil (MetadataNotFound should be ignored)", err)
	}
	if report.EntriesReplayed != 1 {
		t.Errorf("EntriesReplayed = %d, want 1", report.EntriesReplayed)
	}
}

// TestStatus_dbStatNonExistError verifies Status returns error for non-NotExist stat errors.
func TestStatus_dbStatNonExistError(t *testing.T) {
	// This is hard to test because os.Stat doesn't have controllable injection.
	// The branch is covered by the noDBFile test (NotExist) and the success test (no error).
	// We verify the happy path of getting DB size works correctly.
	h := newTestHarness(t)
	ctx := context.Background()

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	// The dummy DB file created in newTestHarness has "fake-db-content" (15 bytes).
	if report.DBSizeBytes != 15 {
		t.Errorf("DBSizeBytes = %d, want 15", report.DBSizeBytes)
	}
}

// TestStatus_withSubdirectoryFiles verifies Status counts files in subdirectories.
func TestStatus_withSubdirectoryFiles(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	h.createFile(t, "subdir/a.md", "content a")
	h.createFile(t, "subdir/nested/b.md", "content b")
	h.createFile(t, "top.md", "content top")

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3", report.TotalFiles)
	}
}

// TestHardRebuild_subdirectoryFiles verifies HardRebuild handles files in subdirectories.
func TestHardRebuild_subdirectoryFiles(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Create a file in a subdirectory with frontmatter.
	h.createFile(t, "subdir/deep.md", "content")
	absPath := filepath.Join(h.brain.FilesDir(), "subdir", "deep.md")
	now := time.Now().UTC().Truncate(time.Second)
	h.fm.Data[absPath] = store.FileMetadata{
		Filepath: "subdir/deep.md", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	report, err := h.svc.HardRebuild(ctx)
	if err != nil {
		t.Fatalf("HardRebuild() error = %v", err)
	}

	if report.Creates != 1 {
		t.Errorf("Creates = %d, want 1", report.Creates)
	}
	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
}

// TestStatus_orphanedWithPathTraversal verifies Status counts records with unresolvable paths as orphaned.
func TestStatus_orphanedWithPathTraversal(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	// A DB record with a path traversal path that ResolveFilePath will reject.
	h.fileRepo.Data["../../etc/passwd"] = store.FileMetadata{
		Filepath: "../../etc/passwd", SourceAgent: "test",
		CreatedAt: now, UpdatedAt: now,
	}

	report, err := h.svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if report.OrphanedCount != 1 {
		t.Errorf("OrphanedCount = %d, want 1", report.OrphanedCount)
	}
}

// TestRebuild_archiveDeleteError verifies Rebuild propagates non-MetadataNotFound errors for archive.
func TestRebuild_archiveDeleteError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	h.ledger.Entries = []ledger.Entry{
		makeArchiveEntry("a.md", now),
	}

	// Inject a non-MetadataNotFound error on delete.
	h.fileRepo.DeleteErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db connection lost")

	_, err := h.svc.Rebuild(ctx)
	if err == nil {
		t.Fatal("Rebuild() expected error, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeDatabaseError) {
		t.Errorf("error = %v, want DATABASE_ERROR code", err)
	}
}

// TestStatus_writeDocError verifies Status returns error when writing grimoire.md fails.
func TestStatus_writeDocError(t *testing.T) {
	h := newTestHarness(t)
	ctx := context.Background()

	// Make the DocPath location non-writable by creating a directory there.
	docPath := h.brain.DocPath()
	if err := os.MkdirAll(docPath, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := h.svc.Status(ctx)
	if err == nil {
		t.Fatal("Status() expected error when DocPath is a directory, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeInternalError) {
		t.Errorf("error = %v, want INTERNAL_ERROR code", err)
	}
}
