// Package integration_test contains integration tests exercising the full
// stack for metadata CRUD, archive, and init operations with real SQLite
// and file system.
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	// Create an embedding.Generator wrapping the provider and embedding repo.
	embGen := embedding.NewGenerator(embProvider, embRepo, "search_document: ", 4096, 512)

	mgr := metadata.NewManager(b, fileRepo, embRepo, led, fm, embGen, locker)

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

// TestCreateEmbedding_LargeFile_OllamaContextLimit replicates a bug where
// large files silently fail to generate embeddings. Ollama returns HTTP 500
// with "the input length exceeds the context length" for prompts that exceed
// the model's token limit. The manager's generateEmbedding method silently
// swallows the error, so Create succeeds but no embedding is stored.
//
// This test uses a mock Ollama server that mimics the real behavior: succeed
// for small inputs, return 500 for inputs exceeding a token threshold. It
// asserts that both small and large files should have embeddings after Create.
func TestCreateEmbedding_LargeFile_OllamaContextLimit(t *testing.T) {
	ctx := context.Background()

	// Token limit threshold (in bytes). The real nomic-embed-text model has
	// a 2048-token context window; Ollama returns 500 when exceeded.
	// We set this above the Generator's maxChunkBytes (4096) + prefix overhead (~18)
	// but below the total file size, so individual chunks succeed but the
	// original unchunked file would have failed.
	const contextLimitBytes = 5000

	// Mock Ollama server (/api/embed): returns a valid embedding for small inputs,
	// 500 error for inputs that exceed the context limit.
	type ollamaReq struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}
	type ollamaResp struct {
		Embeddings [][]float32 `json:"embeddings"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if len(req.Input) > contextLimitBytes {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // test helper; encoding error irrelevant
				"error": "the input length exceeds the context length",
			})
			return
		}

		// Return a deterministic 768-dimension embedding.
		vec := make([]float32, 768)
		for i := range vec {
			vec[i] = 0.01 * float32(i)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaResp{Embeddings: [][]float32{vec}}) //nolint:errcheck // test helper; encoding error irrelevant
	}))
	defer server.Close()

	// Create OllamaProvider pointing at mock server.
	ollamaEmb := embedding.NewOllamaProvider(server.URL, "nomic-embed-text")

	// Set up a full test stack with the real OllamaProvider.
	ts := setupTestStack(t, ollamaEmb)

	// --- Small file: under the context limit ---
	smallContent := "# Quick Research\n\nA short note about Microsoft Graph API.\n"
	smallPath := "research-quick/small-note.md"
	createTestFile(t, ts.b, smallPath, smallContent)

	_, err := ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    smallPath,
		SourceAgent: "quick-researcher",
		Tags:        []string{"type/quick-research"},
		Summary:     "Small research note",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error for small file: %v", err)
	}

	// Verify the small file has an embedding.
	embRepo := store.NewSQLiteEmbeddingRepository(ts.db)
	smallEmb, err := embRepo.Get(ctx, smallPath)
	if err != nil {
		t.Fatalf("Small file should have an embedding: %v", err)
	}
	if len(smallEmb.Vector) != 768 {
		t.Errorf("Small file vector length = %d, want 768", len(smallEmb.Vector))
	}

	// --- Large file: exceeds the context limit ---
	// Generate a realistic large document (~10KB, well over the 4KB threshold).
	var largeBuilder strings.Builder
	largeBuilder.WriteString("# Deep Research: Agent Teams Workflow Implementation Guide\n\n")
	for i := 0; i < 200; i++ {
		largeBuilder.WriteString("## Section " + string(rune('A'+i%26)) + "\n\n")
		largeBuilder.WriteString("This section covers the implementation details for configuring agent teams ")
		largeBuilder.WriteString("in a production workflow. The key considerations include task decomposition, ")
		largeBuilder.WriteString("parallel execution strategies, and error handling patterns.\n\n")
	}
	largeContent := largeBuilder.String()
	if len(largeContent) <= contextLimitBytes {
		t.Fatalf("Test setup: large content (%d bytes) should exceed context limit (%d bytes)", len(largeContent), contextLimitBytes)
	}

	largePath := "research-deep/deep-research/large-guide.md"
	createTestFile(t, ts.b, largePath, largeContent)

	_, err = ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    largePath,
		SourceAgent: "researcher",
		Tags:        []string{"type/deep-research"},
		Summary:     "Large research guide",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error for large file: %v", err)
	}

	// After chunking, the large file should have multiple embeddings (one per chunk).
	// The Generator splits the content into chunks that each fit within the context limit,
	// so the mock server should accept all of them.
	largeEmb, err := embRepo.Get(ctx, largePath)
	if err != nil {
		t.Fatalf("Large file should have an embedding, but got error: %v", err)
	}
	if len(largeEmb.Vector) == 0 {
		t.Error("Large file embedding vector should not be empty")
	}

	// Verify multiple chunks were stored (the file is ~30KB, context limit is 4KB).
	allChunks, err := embRepo.GetForFile(ctx, largePath)
	if err != nil {
		t.Fatalf("GetForFile() error: %v", err)
	}
	if len(allChunks) < 2 {
		t.Errorf("Expected multiple chunks for large file, got %d", len(allChunks))
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

// TestCreateEmbedding_ChunkedFile exercises FR-1, FR-2, FR-3, FR-4: embedding
// a large file produces multiple chunks with correct metadata.
func TestCreateEmbedding_ChunkedFile(t *testing.T) {
	ctx := context.Background()

	// Mock Ollama server at /api/embed that returns deterministic embeddings.
	type ollamaReq struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Return a deterministic 3-dimension embedding based on input length.
		vec := []float32{float32(len(req.Input)%100) / 100.0, 0.5, 0.3}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][][]float32{"embeddings": {vec}}) //nolint:errcheck // test
	}))
	defer server.Close()

	ollamaEmb := embedding.NewOllamaProvider(server.URL, "nomic-embed-text")
	ts := setupTestStack(t, ollamaEmb)

	// Create a large file (>8KB) with paragraph breaks to produce multiple chunks.
	var builder strings.Builder
	builder.WriteString("# Chunked Document\n\n")
	for i := 0; i < 100; i++ {
		builder.WriteString("## Section " + string(rune('A'+i%26)) + "\n\n")
		builder.WriteString("This is a detailed paragraph in section that provides information. ")
		builder.WriteString("It has enough text to contribute to the overall file size. ")
		builder.WriteString("Multiple paragraphs ensure natural chunk boundaries exist.\n\n")
	}
	content := builder.String()
	relPath := "docs/large-doc.md"
	createTestFile(t, ts.b, relPath, content)

	_, err := ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:    relPath,
		SourceAgent: "chunk-agent",
		Tags:        []string{"chunked"},
		Summary:     "A large document for chunk testing",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error: %v", err)
	}

	// Verify multiple chunks are stored.
	embRepo := store.NewSQLiteEmbeddingRepository(ts.db)
	chunks, err := embRepo.GetForFile(ctx, relPath)
	if err != nil {
		t.Fatalf("GetForFile() error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("Expected multiple chunks, got %d", len(chunks))
	}

	// Verify each chunk has correct ChunkIndex (0, 1, 2, ...) and that
	// ChunkStart/ChunkEnd are populated.
	for i, chunk := range chunks {
		if chunk.ChunkIndex != i {
			t.Errorf("Chunk[%d].ChunkIndex = %d, want %d", i, chunk.ChunkIndex, i)
		}
		if chunk.ChunkEnd <= chunk.ChunkStart {
			t.Errorf("Chunk[%d] has invalid offsets: start=%d, end=%d", i, chunk.ChunkStart, chunk.ChunkEnd)
		}
		if len(chunk.Vector) == 0 {
			t.Errorf("Chunk[%d] has empty vector", i)
		}
		if chunk.ModelID != "nomic-embed-text" {
			t.Errorf("Chunk[%d].ModelID = %q, want %q", i, chunk.ModelID, "nomic-embed-text")
		}
	}

	// Verify the first chunk starts at 0 and the last chunk ends at the actual
	// file length (which includes frontmatter prepended by Manager.Create).
	if chunks[0].ChunkStart != 0 {
		t.Errorf("First chunk should start at 0, got %d", chunks[0].ChunkStart)
	}
	absPath := filepath.Join(ts.b.FilesDir(), relPath)
	fileData, readErr := os.ReadFile(absPath)
	if readErr != nil {
		t.Fatalf("failed to read file: %v", readErr)
	}
	actualLen := len(fileData) // includes frontmatter
	lastChunk := chunks[len(chunks)-1]
	if lastChunk.ChunkEnd > actualLen {
		t.Errorf("Last chunk end (%d) exceeds actual file length (%d)", lastChunk.ChunkEnd, actualLen)
	}
}

// TestCreateWithSummaryEmbedding exercises FR-7, FR-8: summary embeddings
// are only stored for multi-chunk files.
func TestCreateWithSummaryEmbedding(t *testing.T) {
	ctx := context.Background()

	// Mock Ollama server at /api/embed.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		vec := []float32{0.1, 0.2, 0.3}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][][]float32{"embeddings": {vec}}) //nolint:errcheck // test
	}))
	defer server.Close()

	ollamaEmb := embedding.NewOllamaProvider(server.URL, "nomic-embed-text")
	ts := setupTestStack(t, ollamaEmb)
	embRepo := store.NewSQLiteEmbeddingRepository(ts.db)

	// --- Large file (multi-chunk) with SummaryEmbeddingText ---
	var builder strings.Builder
	builder.WriteString("# Large Summary Document\n\n")
	for i := 0; i < 100; i++ {
		builder.WriteString("## Topic " + string(rune('A'+i%26)) + "\n\n")
		builder.WriteString("Detailed discussion on this topic with enough text to push file size. ")
		builder.WriteString("Additional context ensures the file exceeds the chunk size limit.\n\n")
	}
	largeContent := builder.String()
	largePath := "docs/large-summary.md"
	createTestFile(t, ts.b, largePath, largeContent)

	_, err := ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:             largePath,
		SourceAgent:          "summary-agent",
		Tags:                 []string{"summary"},
		Summary:              "A large document with summary",
		SummaryEmbeddingText: "This document discusses multiple topics in detail",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error for large file: %v", err)
	}

	// Verify a summary chunk exists at ChunkIndex=-1 with IsSummary=true.
	allChunks, err := embRepo.GetForFile(ctx, largePath)
	if err != nil {
		t.Fatalf("GetForFile() error: %v", err)
	}

	var foundSummary bool
	for _, chunk := range allChunks {
		if chunk.ChunkIndex == -1 && chunk.IsSummary {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Error("Expected summary chunk (ChunkIndex=-1, IsSummary=true) for multi-chunk file, not found")
	}

	// --- Small file (single-chunk) with SummaryEmbeddingText ---
	smallPath := "docs/small-summary.md"
	createTestFile(t, ts.b, smallPath, "# Small Note\n\nJust a short note.\n")

	_, err = ts.mgr.Create(ctx, metadata.CreateOptions{
		Filepath:             smallPath,
		SourceAgent:          "summary-agent",
		Tags:                 []string{"summary"},
		Summary:              "A small note",
		SummaryEmbeddingText: "A brief note about something",
	})
	if err != nil {
		t.Fatalf("Manager.Create() error for small file: %v", err)
	}

	// Verify NO summary chunk for single-chunk files (FR-8).
	smallChunks, err := embRepo.GetForFile(ctx, smallPath)
	if err != nil {
		t.Fatalf("GetForFile() error: %v", err)
	}

	for _, chunk := range smallChunks {
		if chunk.ChunkIndex == -1 || chunk.IsSummary {
			t.Error("Single-chunk file should NOT have a summary chunk, but found one")
		}
	}
	if len(smallChunks) != 1 {
		t.Errorf("Small file should have exactly 1 chunk, got %d", len(smallChunks))
	}
}

// TestSchemaV1ToV2Migration exercises FR-10: automatic v1 to v2 schema migration.
func TestSchemaV1ToV2Migration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create and populate a v1 database.
	seedV1Database(t, dbPath)

	// Reopen and run migration.
	db2, err := store.NewDB(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer db2.Close()

	if err := db2.MigrateIfNeeded(); err != nil {
		t.Fatalf("MigrateIfNeeded() error: %v", err)
	}

	embRepo := store.NewSQLiteEmbeddingRepository(db2)
	ctx := context.Background()

	t.Run("schema_version_is_v2", func(t *testing.T) {
		if err := db2.CheckVersion(store.SchemaVersion); err != nil {
			t.Fatalf("Schema version mismatch after migration: %v", err)
		}
	})

	t.Run("migrated_embedding_defaults", func(t *testing.T) {
		alphaEmb, err := embRepo.Get(ctx, "notes/alpha.md")
		if err != nil {
			t.Fatalf("Get alpha embedding after migration: %v", err)
		}
		if alphaEmb.ChunkIndex != 0 {
			t.Errorf("Migrated embedding ChunkIndex = %d, want 0", alphaEmb.ChunkIndex)
		}
		if alphaEmb.ChunkStart != 0 || alphaEmb.ChunkEnd != 0 {
			t.Errorf("Migrated embedding should have default chunk offsets (0, 0), got (%d, %d)", alphaEmb.ChunkStart, alphaEmb.ChunkEnd)
		}
		if alphaEmb.IsSummary {
			t.Error("Migrated embedding should not be a summary")
		}
		if len(alphaEmb.Vector) != 3 {
			t.Errorf("Vector length = %d, want 3", len(alphaEmb.Vector))
		}
	})

	t.Run("migrated_embedding_preserves_model", func(t *testing.T) {
		betaEmb, err := embRepo.Get(ctx, "notes/beta.md")
		if err != nil {
			t.Fatalf("Get beta embedding after migration: %v", err)
		}
		if betaEmb.ModelID != "nomic-embed-text" {
			t.Errorf("Migrated embedding ModelID = %q, want %q", betaEmb.ModelID, "nomic-embed-text")
		}
	})

	t.Run("v2_composite_pk_allows_multiple_chunks", func(t *testing.T) {
		err := embRepo.Upsert(ctx, store.Embedding{
			Filepath:   "notes/alpha.md",
			ChunkIndex: 1,
			Vector:     []float32{0.7, 0.8, 0.9},
			ModelID:    "nomic-embed-text",
			ChunkStart: 100,
			ChunkEnd:   200,
		})
		if err != nil {
			t.Fatalf("Failed to insert chunk 1 after migration: %v", err)
		}

		allChunks, err := embRepo.GetForFile(ctx, "notes/alpha.md")
		if err != nil {
			t.Fatalf("GetForFile after migration: %v", err)
		}
		if len(allChunks) != 2 {
			t.Errorf("Expected 2 chunks after inserting chunk 1, got %d", len(allChunks))
		}
	})
}

// seedV1Database creates a v1-schema database with test data for migration tests.
func seedV1Database(t *testing.T, dbPath string) {
	t.Helper()

	db, err := store.NewDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	v1Statements := []string{
		`CREATE TABLE IF NOT EXISTS files (
			filepath TEXT PRIMARY KEY,
			source_agent TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS file_tags (
			filepath TEXT NOT NULL REFERENCES files(filepath) ON DELETE CASCADE,
			tag TEXT NOT NULL,
			PRIMARY KEY (filepath, tag)
		)`,
		`CREATE TABLE IF NOT EXISTS embeddings (
			filepath TEXT PRIMARY KEY REFERENCES files(filepath) ON DELETE CASCADE,
			vector BLOB NOT NULL,
			model_id TEXT NOT NULL,
			generated_at DATETIME NOT NULL
		)`,
		"PRAGMA user_version = 1",
	}
	for _, stmt := range v1Statements {
		if _, execErr := db.SQLDB().Exec(stmt); execErr != nil {
			t.Fatalf("failed to create v1 schema: %v", execErr)
		}
	}

	now := "2025-01-15T10:00:00Z"
	for _, f := range []struct{ path, agent, summary string }{
		{"notes/alpha.md", "agent-a", "alpha notes"},
		{"notes/beta.md", "agent-b", "beta notes"},
	} {
		_, err = db.SQLDB().Exec(
			"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			f.path, f.agent, f.summary, now, now,
		)
		if err != nil {
			t.Fatalf("failed to insert file %s: %v", f.path, err)
		}
	}

	for _, e := range []struct {
		path string
		vec  []float32
	}{
		{"notes/alpha.md", []float32{0.1, 0.2, 0.3}},
		{"notes/beta.md", []float32{0.4, 0.5, 0.6}},
	} {
		_, err = db.SQLDB().Exec(
			"INSERT INTO embeddings (filepath, vector, model_id, generated_at) VALUES (?, ?, ?, ?)",
			e.path, store.EncodeVector(e.vec), "nomic-embed-text", now,
		)
		if err != nil {
			t.Fatalf("failed to insert v1 embedding for %s: %v", e.path, err)
		}
	}
}
