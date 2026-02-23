// Package integration_test contains integration tests exercising the full
// stack for metadata CRUD, archive, and init operations with real SQLite
// and file system.
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/config"
	"github.com/awood45/grimoire-cli/internal/docgen"
	"github.com/awood45/grimoire-cli/internal/embedding"
	embtest "github.com/awood45/grimoire-cli/internal/embedding/testing"
	"github.com/awood45/grimoire-cli/internal/filelock"
	"github.com/awood45/grimoire-cli/internal/frontmatter"
	"github.com/awood45/grimoire-cli/internal/initialize"
	"github.com/awood45/grimoire-cli/internal/ledger"
	"github.com/awood45/grimoire-cli/internal/metadata"
	"github.com/awood45/grimoire-cli/internal/platform"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
)

// testStack bundles the real dependencies created by setupTestStack.
type testStack struct {
	mgr *metadata.Manager
	b   *brain.Brain
	db  *store.DB
	led *ledger.FileLedger
}

// setupTestStack creates a complete real grimoire stack rooted at a temp
// directory. It opens a real SQLite DB, creates a real ledger, real frontmatter
// service, and real file locker. The embedding provider is configurable: pass
// nil to use NoopProvider. All resources are closed via t.Cleanup.
func setupTestStack(t *testing.T, embProvider embedding.Provider) testStack {
	t.Helper()

	dir := t.TempDir()
	b := brain.New(dir)

	// Create directory structure.
	for _, d := range []string{b.FilesDir(), b.ArchiveDir(), filepath.Dir(b.DBPath())} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("failed to create directory %s: %v", d, err)
		}
	}

	// Create empty ledger file.
	if err := os.WriteFile(b.LedgerPath(), []byte(""), 0o600); err != nil {
		t.Fatalf("failed to create ledger file: %v", err)
	}

	// Open real SQLite DB.
	db, err := store.NewDB(b.DBPath())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	if err := db.EnsureSchema(); err != nil {
		db.Close()
		t.Fatalf("failed to ensure schema: %v", err)
	}

	// Create real repos.
	fileRepo := store.NewSQLiteFileRepository(db)
	embRepo := store.NewSQLiteEmbeddingRepository(db)

	// Open real ledger.
	led, err := ledger.NewFileLedger(b.LedgerPath())
	if err != nil {
		db.Close()
		t.Fatalf("failed to open ledger: %v", err)
	}

	// Create real frontmatter service.
	fm := frontmatter.NewFileService()

	// Create real file locker.
	locker, err := filelock.NewFlockLocker(b.LockPath())
	if err != nil {
		led.Close()
		db.Close()
		t.Fatalf("failed to create locker: %v", err)
	}

	// Use NoopProvider by default.
	if embProvider == nil {
		embProvider = &embedding.NoopProvider{}
	}

	mgr := metadata.NewManager(b, fileRepo, embRepo, led, fm, embProvider, locker)

	t.Cleanup(func() {
		locker.Close()
		led.Close()
		db.Close()
	})

	return testStack{mgr: mgr, b: b, db: db, led: led}
}

// createTestFile creates a markdown file in the test brain's files/ directory.
func createTestFile(t *testing.T, b *brain.Brain, relPath, content string) {
	t.Helper()

	fullPath := filepath.Join(b.FilesDir(), relPath)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create directory %s: %v", dir, err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write file %s: %v", fullPath, err)
	}
}

// assertMetadataFields checks that basic metadata fields match expected values.
func assertMetadataFields(t *testing.T, meta *store.FileMetadata, wantPath, wantAgent, wantSummary string, wantTagCount int) {
	t.Helper()
	if meta.Filepath != wantPath {
		t.Errorf("Filepath = %q, want %q", meta.Filepath, wantPath)
	}
	if meta.SourceAgent != wantAgent {
		t.Errorf("SourceAgent = %q, want %q", meta.SourceAgent, wantAgent)
	}
	if meta.Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", meta.Summary, wantSummary)
	}
	if len(meta.Tags) != wantTagCount {
		t.Errorf("Tags count = %d, want %d (tags: %v)", len(meta.Tags), wantTagCount, meta.Tags)
	}
}

// assertLedgerEntries verifies the ledger has the expected number of entries
// with the expected operations.
func assertLedgerEntries(t *testing.T, led *ledger.FileLedger, wantOps ...string) []ledger.Entry {
	t.Helper()
	entries, err := led.ReadAll()
	if err != nil {
		t.Fatalf("Ledger.ReadAll() error: %v", err)
	}
	if len(entries) != len(wantOps) {
		t.Fatalf("Ledger has %d entries, want %d", len(entries), len(wantOps))
	}
	for i, wantOp := range wantOps {
		if entries[i].Operation != wantOp {
			t.Errorf("Entry[%d].Operation = %q, want %q", i, entries[i].Operation, wantOp)
		}
	}
	return entries
}

// assertFileContains checks that a file at absPath exists and its content
// contains (or does not contain) given substrings.
func assertFileContains(t *testing.T, absPath string, mustContain, mustNotContain []string) {
	t.Helper()
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", absPath, err)
	}
	content := string(data)
	for _, s := range mustContain {
		if !strings.Contains(content, s) {
			t.Errorf("File %s missing expected content: %q", absPath, s)
		}
	}
	for _, s := range mustNotContain {
		if strings.Contains(content, s) {
			t.Errorf("File %s unexpectedly contains: %q", absPath, s)
		}
	}
}

// assertMetadataNotFound checks that Get returns METADATA_NOT_FOUND.
func assertMetadataNotFound(t *testing.T, mgr *metadata.Manager, fp string) {
	t.Helper()
	_, err := mgr.Get(context.Background(), fp)
	if err == nil {
		t.Fatal("Expected error getting metadata, got nil")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeMetadataNotFound) {
		t.Errorf("Expected METADATA_NOT_FOUND error, got: %v", err)
	}
}

// newInitializer creates a real Initializer with a TemplateGenerator and a
// platform detector rooted at the given home directory.
func newInitializer(t *testing.T, homeDir string) *initialize.Initializer {
	t.Helper()
	docGen, err := docgen.NewTemplateGenerator()
	if err != nil {
		t.Fatalf("failed to create doc generator: %v", err)
	}
	det := platform.NewDetector(homeDir)
	return initialize.NewInitializer(docGen, det)
}

// TestCreateAndGetMetadata exercises FR-3.2.1 (Create) and FR-3.2.3 (Get).
func TestCreateAndGetMetadata(t *testing.T) {
	ctx := context.Background()
	ts := setupTestStack(t, nil)

	relPath := "notes/test-note.md"
	createTestFile(t, ts.b, relPath, "# Test Note\n\nSome content here.\n")

	created, err := ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    relPath,
		SourceAgent: "test-agent",
		Tags:        []string{"meeting-notes", "weekly"},
		Summary:     "A test note for integration testing",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error: %v", err)
	}

	// Verify created metadata matches input.
	assertMetadataFields(t, &created, relPath, "test-agent", "A test note for integration testing", 2)
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Errorf("CreatedAt (%v) != UpdatedAt (%v)", created.CreatedAt, created.UpdatedAt)
	}

	// Verify Get returns matching metadata.
	got, err := ts.mgr.Get(ctx, relPath)
	if err != nil {
		t.Fatalf("Manager.Get() error: %v", err)
	}
	assertMetadataFields(t, &got, relPath, "test-agent", "A test note for integration testing", 2)

	// Verify frontmatter present in file.
	absPath := filepath.Join(ts.b.FilesDir(), relPath)
	assertFileContains(t, absPath,
		[]string{"---", "source_agent: test-agent", "meeting-notes", "# Test Note"},
		nil,
	)

	// Verify ledger has a create entry.
	entries := assertLedgerEntries(t, ts.led, "create")
	if entries[0].Filepath != relPath {
		t.Errorf("Ledger filepath = %q, want %q", entries[0].Filepath, relPath)
	}

	// Verify DB has a record via direct repo query.
	fileRepo := store.NewSQLiteFileRepository(ts.db)
	dbMeta, err := fileRepo.Get(ctx, relPath)
	if err != nil {
		t.Fatalf("FileRepo.Get() error: %v", err)
	}
	if dbMeta.Filepath != relPath {
		t.Errorf("DB filepath = %q, want %q", dbMeta.Filepath, relPath)
	}
}

// TestCreateAndUpdateMetadata exercises FR-3.2.1 (Create) and FR-3.2.2 (Update).
func TestCreateAndUpdateMetadata(t *testing.T) {
	ctx := context.Background()
	ts := setupTestStack(t, nil)

	relPath := "docs/guide.md"
	createTestFile(t, ts.b, relPath, "# Guide\n\nOriginal content.\n")

	created, err := ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    relPath,
		SourceAgent: "agent-1",
		Tags:        []string{"guide", "docs"},
		Summary:     "An initial guide",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error: %v", err)
	}

	// Small sleep to ensure updated_at changes.
	time.Sleep(10 * time.Millisecond)

	newSummary := "An updated guide with more info"
	updated, err := ts.mgr.Update(ctx, metadata.UpdateOptions{
		Filepath: relPath,
		Tags:     []string{"guide", "updated", "reference"},
		Summary:  &newSummary,
	})
	if err != nil {
		t.Fatalf("Manager.Update() error: %v", err)
	}

	// Assert tags replaced and summary updated.
	assertMetadataFields(t, &updated, relPath, "agent-1", newSummary, 3)

	// Assert updated_at changed and created_at unchanged.
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("updated_at (%v) should be after (%v)", updated.UpdatedAt, created.UpdatedAt)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("created_at changed: got %v, want %v", updated.CreatedAt, created.CreatedAt)
	}

	// Verify frontmatter updated in file.
	absPath := filepath.Join(ts.b.FilesDir(), relPath)
	assertFileContains(t, absPath, []string{newSummary, "reference"}, nil)

	// Verify ledger has both entries.
	assertLedgerEntries(t, ts.led, "create", "update")
}

// TestCreateAndDeleteMetadata exercises FR-3.2.1 (Create) and FR-3.2.4 (Delete).
func TestCreateAndDeleteMetadata(t *testing.T) {
	ctx := context.Background()
	ts := setupTestStack(t, nil)

	relPath := "temp/scratch.md"
	createTestFile(t, ts.b, relPath, "# Scratch\n\nTemporary notes.\n")

	_, err := ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    relPath,
		SourceAgent: "agent-2",
		Tags:        []string{"temporary"},
		Summary:     "Scratch notes",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error: %v", err)
	}

	if err := ts.mgr.Delete(ctx, relPath); err != nil {
		t.Fatalf("Manager.Delete() error: %v", err)
	}

	// Assert DB record gone.
	assertMetadataNotFound(t, ts.mgr, relPath)

	// Assert frontmatter stripped but file still exists.
	absPath := filepath.Join(ts.b.FilesDir(), relPath)
	assertFileContains(t, absPath, nil, []string{"---"})

	if _, statErr := os.Stat(absPath); statErr != nil {
		t.Errorf("File was deleted from disk, should still exist: %v", statErr)
	}

	// Assert ledger has create + delete entries.
	assertLedgerEntries(t, ts.led, "create", "delete")
}

// TestArchiveFile exercises FR-3.2.5 (Archive).
func TestArchiveFile(t *testing.T) {
	ctx := context.Background()
	ts := setupTestStack(t, nil)

	relPath := "projects/old-project.md"
	createTestFile(t, ts.b, relPath, "# Old Project\n\nThis project is done.\n")

	created, err := ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    relPath,
		SourceAgent: "project-agent",
		Tags:        []string{"project", "completed"},
		Summary:     "Completed project documentation",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error: %v", err)
	}

	result, err := ts.mgr.Archive(ctx, relPath)
	if err != nil {
		t.Fatalf("Manager.Archive() error: %v", err)
	}

	// Verify result paths and metadata.
	if result.OriginalPath != relPath {
		t.Errorf("OriginalPath = %q, want %q", result.OriginalPath, relPath)
	}
	if result.ArchivePath != relPath {
		t.Errorf("ArchivePath = %q, want %q", result.ArchivePath, relPath)
	}
	if result.Metadata.SourceAgent != created.SourceAgent {
		t.Errorf("Metadata.SourceAgent = %q, want %q", result.Metadata.SourceAgent, created.SourceAgent)
	}

	// Verify file moved to archive.
	archiveAbs := filepath.Join(ts.b.ArchiveDir(), relPath)
	if _, statErr := os.Stat(archiveAbs); statErr != nil {
		t.Errorf("Archived file not found: %v", statErr)
	}
	srcAbs := filepath.Join(ts.b.FilesDir(), relPath)
	if _, statErr := os.Stat(srcAbs); !os.IsNotExist(statErr) {
		t.Error("Original file still exists")
	}

	// Verify frontmatter stripped from archived file.
	assertFileContains(t, archiveAbs, []string{"# Old Project"}, []string{"---"})

	// Verify DB record gone.
	assertMetadataNotFound(t, ts.mgr, relPath)

	// Verify ledger entries and archive payload.
	entries := assertLedgerEntries(t, ts.led, "create", "archive")
	assertArchivePayload(t, &entries[1], relPath, "project-agent", "Completed project documentation", 2)
}

// assertArchivePayload verifies the archive ledger entry payload.
func assertArchivePayload(t *testing.T, entry *ledger.Entry, wantArchiveTo, wantAgent, wantSummary string, wantTagCount int) {
	t.Helper()
	var payload ledger.ArchivePayload
	if err := json.Unmarshal(entry.Payload, &payload); err != nil {
		t.Fatalf("Failed to unmarshal archive payload: %v", err)
	}
	if payload.SourceAgent != wantAgent {
		t.Errorf("payload source_agent = %q, want %q", payload.SourceAgent, wantAgent)
	}
	if payload.Summary != wantSummary {
		t.Errorf("payload summary = %q, want %q", payload.Summary, wantSummary)
	}
	if len(payload.Tags) != wantTagCount {
		t.Errorf("payload tags count = %d, want %d", len(payload.Tags), wantTagCount)
	}
	if payload.ArchivedTo != wantArchiveTo {
		t.Errorf("payload archived_to = %q, want %q", payload.ArchivedTo, wantArchiveTo)
	}
}

// TestCreateWithEmbedding exercises FR-3.2.1 with an embedding provider.
func TestCreateWithEmbedding(t *testing.T) {
	ctx := context.Background()
	fakeEmb := embtest.NewFakeProvider()
	ts := setupTestStack(t, fakeEmb)

	relPath := "notes/with-embedding.md"
	createTestFile(t, ts.b, relPath, "# Embedded Note\n\nContent for embedding.\n")

	_, err := ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    relPath,
		SourceAgent: "embed-agent",
		Tags:        []string{"embedded"},
		Summary:     "Note with embedding",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error: %v", err)
	}

	if len(fakeEmb.GenerateCalls) != 1 {
		t.Errorf("GenerateCalls = %d, want 1", len(fakeEmb.GenerateCalls))
	}

	embRepo := store.NewSQLiteEmbeddingRepository(ts.db)
	emb, err := embRepo.Get(ctx, relPath)
	if err != nil {
		t.Fatalf("EmbeddingRepo.Get() error: %v", err)
	}
	if emb.ModelID != "fake-model" {
		t.Errorf("model_id = %q, want %q", emb.ModelID, "fake-model")
	}
	if len(emb.Vector) != 3 {
		t.Errorf("vector length = %d, want 3", len(emb.Vector))
	}
}

// TestInit_fullFlow exercises FR-3.1.1.
func TestInit_fullFlow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	initializer := newInitializer(t, dir)

	err := initializer.Init(ctx, initialize.InitOptions{BasePath: dir})
	if err != nil {
		t.Fatalf("Initializer.Init() error: %v", err)
	}

	b := brain.New(dir)

	// Assert directory structure exists.
	for _, d := range []string{b.FilesDir(), b.ArchiveDir(), filepath.Dir(b.DBPath())} {
		if _, statErr := os.Stat(d); statErr != nil {
			t.Errorf("Directory %s does not exist: %v", d, statErr)
		}
	}

	// Assert config is valid.
	cfg, err := config.Load(b.ConfigPath())
	if err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
	if cfg.Embedding.Provider != "none" {
		t.Errorf("provider = %q, want %q", cfg.Embedding.Provider, "none")
	}

	// Assert DB has schema.
	db, err := store.NewDB(b.DBPath())
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()
	if err := db.CheckVersion(store.SchemaVersion); err != nil {
		t.Errorf("DB schema version mismatch: %v", err)
	}

	// Assert ledger and doc exist.
	for _, path := range []string{b.LedgerPath(), b.DocPath()} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("File %s does not exist: %v", path, statErr)
		}
	}
}

// TestInit_withForce exercises FR-3.1.1 (reinitialize with --force).
func TestInit_withForce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	initializer := newInitializer(t, dir)

	// First init.
	err := initializer.Init(ctx, initialize.InitOptions{BasePath: dir})
	if err != nil {
		t.Fatalf("Initial Init() error: %v", err)
	}

	b := brain.New(dir)

	// Create a file and ledger entry to verify they survive reinit.
	testFilePath := filepath.Join(b.FilesDir(), "existing.md")
	if err := os.WriteFile(testFilePath, []byte("# Existing\n"), 0o600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(b.LedgerPath(), []byte(`{"operation":"test"}`+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write ledger: %v", err)
	}

	origDoc, err := os.ReadFile(b.DocPath())
	if err != nil {
		t.Fatalf("failed to read original doc: %v", err)
	}

	// Reinitialize with force.
	err = initializer.Init(ctx, initialize.InitOptions{
		BasePath:          dir,
		Force:             true,
		EmbeddingProvider: "none",
		EmbeddingModel:    "test-model",
	})
	if err != nil {
		t.Fatalf("Reinit error: %v", err)
	}

	// Files preserved.
	if _, statErr := os.Stat(testFilePath); statErr != nil {
		t.Errorf("Test file not preserved: %v", statErr)
	}

	// Ledger preserved.
	assertFileContains(t, b.LedgerPath(), []string{`"operation":"test"`}, nil)

	// Config regenerated with new model.
	cfg, err := config.Load(b.ConfigPath())
	if err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}
	if cfg.Embedding.Model != "test-model" {
		t.Errorf("Config model = %q, want %q", cfg.Embedding.Model, "test-model")
	}

	// Doc regenerated (not empty).
	newDoc, err := os.ReadFile(b.DocPath())
	if err != nil {
		t.Fatalf("failed to read doc: %v", err)
	}
	if len(newDoc) == 0 {
		t.Error("Regenerated doc is empty")
	}
	if !bytes.Equal(origDoc, newDoc) {
		t.Log("Doc regenerated with different content (expected)")
	}

	// Reinit without force fails.
	err = initializer.Init(ctx, initialize.InitOptions{BasePath: dir})
	if err == nil {
		t.Fatal("Expected error for reinit without force")
	}
	if !sberrors.HasCode(err, sberrors.ErrCodeAlreadyInitialized) {
		t.Errorf("Expected ALREADY_INITIALIZED, got: %v", err)
	}
}

// TestInit_claudeCodeIntegration exercises FR-3.1.2.
func TestInit_claudeCodeIntegration(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	fakeHome := t.TempDir()
	claudeDir := filepath.Join(fakeHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	initializer := newInitializer(t, fakeHome)

	err := initializer.Init(ctx, initialize.InitOptions{BasePath: dir})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// Assert CLAUDE.md updated.
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	assertFileContains(t, claudeMDPath,
		[]string{"Grimoire", "<!-- grimoire-integration -->"},
		nil,
	)

	// Assert skill file installed.
	skillPath := filepath.Join(claudeDir, "commands", "write-to-grimoire.md")
	if _, statErr := os.Stat(skillPath); statErr != nil {
		t.Errorf("Skill file not installed: %v", statErr)
	}

	// Assert skill file references brain base path.
	assertFileContains(t, skillPath, []string{dir}, nil)
}
