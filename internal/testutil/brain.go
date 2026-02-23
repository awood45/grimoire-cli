// Package testutil provides shared test helpers for creating temporary
// grimoire instances and test files.
package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/store"
)

// NewTestBrain creates a temporary grimoire directory with the full
// structure (files/, archive-files/, db/, ledger.jsonl, config.yaml)
// and an initialized SQLite database with schema. Returns a Brain and
// a cleanup function that closes the database.
func NewTestBrain(t *testing.T) (b *brain.Brain, cleanup func()) {
	t.Helper()

	dir := t.TempDir()
	b = brain.New(dir)

	// Create directory structure.
	dirs := []string{
		b.FilesDir(),
		b.ArchiveDir(),
		filepath.Join(dir, "db"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("testutil.NewTestBrain: failed to create directory %s: %v", d, err)
		}
	}

	// Create empty config.yaml with minimal defaults.
	if err := os.WriteFile(b.ConfigPath(), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("testutil.NewTestBrain: failed to create config.yaml: %v", err)
	}

	// Create empty ledger.jsonl.
	if err := os.WriteFile(b.LedgerPath(), []byte(""), 0o600); err != nil {
		t.Fatalf("testutil.NewTestBrain: failed to create ledger.jsonl: %v", err)
	}

	// Initialize SQLite database with schema.
	db, err := store.NewDB(b.DBPath())
	if err != nil {
		t.Fatalf("testutil.NewTestBrain: failed to open database: %v", err)
	}

	if err := db.EnsureSchema(); err != nil {
		db.Close()
		t.Fatalf("testutil.NewTestBrain: failed to initialize schema: %v", err)
	}

	cleanup = func() {
		db.Close()
	}

	return b, cleanup
}

// CreateTestFile creates a markdown file in the test brain's files/ directory
// at the given relative path with the specified content. Intermediate
// directories are created as needed.
func CreateTestFile(t *testing.T, b *brain.Brain, relativePath, content string) {
	t.Helper()

	fullPath := filepath.Join(b.FilesDir(), relativePath)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("testutil.CreateTestFile: failed to create directory %s: %v", dir, err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatalf("testutil.CreateTestFile: failed to write file %s: %v", fullPath, err)
	}
}
