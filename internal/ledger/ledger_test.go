package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLedger(t *testing.T) *FileLedger {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	l, err := NewFileLedger(path)
	require.NoError(t, err)
	return l
}

func makeEntry(op, fp, agent string, payload interface{}) *Entry {
	data, _ := json.Marshal(payload)
	return &Entry{
		Timestamp:   time.Now().UTC(),
		Operation:   op,
		Filepath:    fp,
		SourceAgent: agent,
		Payload:     data,
	}
}

func TestAppend_single(t *testing.T) {
	l := newTestLedger(t)
	defer l.Close()

	entry := makeEntry("create", "notes/test.md", "claude", CreatePayload{
		Tags:        []string{"test"},
		SourceAgent: "claude",
		CreatedAt:   "2025-01-15T10:00:00Z",
		UpdatedAt:   "2025-01-15T10:00:00Z",
	})

	require.NoError(t, l.Append(entry))

	entries, err := l.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "create", entries[0].Operation)
	assert.Equal(t, "notes/test.md", entries[0].Filepath)
}

func TestAppend_multiple(t *testing.T) {
	l := newTestLedger(t)
	defer l.Close()

	for i, op := range []string{"create", "update", "delete"} {
		entry := makeEntry(op, "file.md", "agent", DeletePayload{})
		entry.Timestamp = time.Date(2025, 1, 1, 0, 0, i, 0, time.UTC)
		require.NoError(t, l.Append(entry))
	}

	entries, err := l.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, "create", entries[0].Operation)
	assert.Equal(t, "update", entries[1].Operation)
	assert.Equal(t, "delete", entries[2].Operation)
}

func TestReadAll_empty(t *testing.T) {
	l := newTestLedger(t)
	defer l.Close()

	entries, err := l.ReadAll()
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestReadAll_preservesOrder(t *testing.T) {
	l := newTestLedger(t)
	defer l.Close()

	for i := range 5 {
		entry := makeEntry("create", "file.md", "agent", DeletePayload{})
		entry.Timestamp = time.Date(2025, 1, 1, 0, 0, i, 0, time.UTC)
		require.NoError(t, l.Append(entry))
	}

	entries, err := l.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 5)

	for i := 1; i < len(entries); i++ {
		assert.False(t, entries[i].Timestamp.Before(entries[i-1].Timestamp),
			"entries should be in chronological order")
	}
}

func TestAppend_atomicity(t *testing.T) {
	l := newTestLedger(t)
	defer l.Close()

	entry := makeEntry("create", "test.md", "claude", CreatePayload{
		Tags: []string{"a", "b"},
	})
	require.NoError(t, l.Append(entry))

	// Read the raw file and verify it's a single complete JSON line.
	entries, err := l.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Re-marshal and verify valid JSON.
	data, err := json.Marshal(entries[0])
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
}

func TestAppend_operations(t *testing.T) {
	l := newTestLedger(t)
	defer l.Close()

	ops := []string{"create", "update", "delete", "archive"}
	for _, op := range ops {
		require.NoError(t, l.Append(makeEntry(op, "file.md", "agent", DeletePayload{})))
	}

	entries, err := l.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 4)
	for i, op := range ops {
		assert.Equal(t, op, entries[i].Operation)
	}
}

func TestAppend_payloadSchemas(t *testing.T) {
	l := newTestLedger(t)
	defer l.Close()

	// Create payload.
	createPl := CreatePayload{Tags: []string{"a"}, Summary: "test", SourceAgent: "claude", CreatedAt: "2025-01-01T00:00:00Z", UpdatedAt: "2025-01-01T00:00:00Z"}
	require.NoError(t, l.Append(makeEntry("create", "file.md", "claude", createPl)))

	// Update payload.
	updatePl := UpdatePayload{Tags: []string{"a", "b"}, SourceAgent: "claude", CreatedAt: "2025-01-01T00:00:00Z", UpdatedAt: "2025-01-02T00:00:00Z"}
	require.NoError(t, l.Append(makeEntry("update", "file.md", "claude", updatePl)))

	// Delete payload.
	require.NoError(t, l.Append(makeEntry("delete", "file.md", "claude", DeletePayload{})))

	// Archive payload.
	archivePl := ArchivePayload{Tags: []string{"a"}, SourceAgent: "claude", CreatedAt: "2025-01-01T00:00:00Z", UpdatedAt: "2025-01-02T00:00:00Z", ArchivedTo: "archive-files/file.md"}
	require.NoError(t, l.Append(makeEntry("archive", "file.md", "claude", archivePl)))

	entries, err := l.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 4)

	// Verify payloads round-trip.
	var gotCreate CreatePayload
	require.NoError(t, json.Unmarshal(entries[0].Payload, &gotCreate))
	assert.Equal(t, createPl.Tags, gotCreate.Tags)
	assert.Equal(t, createPl.Summary, gotCreate.Summary)

	var gotUpdate UpdatePayload
	require.NoError(t, json.Unmarshal(entries[1].Payload, &gotUpdate))
	assert.Equal(t, updatePl.Tags, gotUpdate.Tags)

	var gotArchive ArchivePayload
	require.NoError(t, json.Unmarshal(entries[3].Payload, &gotArchive))
	assert.Equal(t, archivePl.ArchivedTo, gotArchive.ArchivedTo)
}

func TestClose_subsequentAppendFails(t *testing.T) {
	l := newTestLedger(t)

	require.NoError(t, l.Close())

	err := l.Append(makeEntry("create", "file.md", "agent", DeletePayload{}))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}

func TestClose_idempotent(t *testing.T) {
	l := newTestLedger(t)

	require.NoError(t, l.Close())
	require.NoError(t, l.Close()) // Second close is a no-op.
}

func TestNewFileLedger_invalidPath(t *testing.T) {
	_, err := NewFileLedger("/nonexistent/dir/ledger.jsonl")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}

func TestReadAll_afterAppendAndClose(t *testing.T) {
	l := newTestLedger(t)

	require.NoError(t, l.Append(makeEntry("create", "a.md", "agent", DeletePayload{})))
	require.NoError(t, l.Append(makeEntry("update", "a.md", "agent", DeletePayload{})))
	require.NoError(t, l.Close())

	// ReadAll should still work after close (opens its own fd).
	entries, err := l.ReadAll()
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestReadAll_corruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")

	// Write valid entry followed by corrupt line.
	l, err := NewFileLedger(path)
	require.NoError(t, err)
	require.NoError(t, l.Append(makeEntry("create", "a.md", "agent", DeletePayload{})))
	require.NoError(t, l.Close())

	// Append corrupt JSON directly to the file.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("{invalid json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// ReadAll should return an unmarshal error.
	l2, err := NewFileLedger(path)
	require.NoError(t, err)
	defer l2.Close()

	_, err = l2.ReadAll()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}

func TestReadAll_deletedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	l, err := NewFileLedger(path)
	require.NoError(t, err)
	require.NoError(t, l.Close())

	// Remove the file so ReadAll's os.Open fails.
	require.NoError(t, os.Remove(path))

	// ReadAll opens its own fd and should fail.
	_, err = l.ReadAll()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}

func TestAppend_writeError(t *testing.T) {
	l := newTestLedger(t)

	// Close the underlying file descriptor directly to force a write error.
	require.NoError(t, l.file.Close())
	l.closed = false // Reset so Append doesn't short-circuit.

	err := l.Append(makeEntry("create", "a.md", "agent", DeletePayload{}))
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}

func TestReadAll_emptyLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")

	// Write entries with blank lines interspersed.
	l, err := NewFileLedger(path)
	require.NoError(t, err)
	require.NoError(t, l.Append(makeEntry("create", "a.md", "agent", DeletePayload{})))
	require.NoError(t, l.Close())

	// Append blank lines directly to the file.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("\n\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	l2, err := NewFileLedger(path)
	require.NoError(t, err)
	defer l2.Close()

	entries, err := l2.ReadAll()
	require.NoError(t, err)
	assert.Len(t, entries, 1) // Blank lines skipped.
}

func TestReadAll_scannerError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")

	// Write a single line longer than bufio.MaxScanTokenSize (64KB)
	// to trigger a scanner buffer overflow error.
	f, err := os.Create(path)
	require.NoError(t, err)
	longLine := strings.Repeat("x", 70000)
	_, err = f.WriteString(longLine + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	l, err := NewFileLedger(path)
	require.NoError(t, err)
	defer l.Close()

	_, err = l.ReadAll()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeLedgerError))
}
