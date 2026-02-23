package ledger

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"

	"github.com/awood45/grimoire-cli/internal/sberrors"
)

// Ledger provides append-only mutation logging with durable writes.
type Ledger interface {
	Append(entry *Entry) error
	ReadAll() ([]Entry, error)
	Close() error
}

// FileLedger implements Ledger backed by a JSONL file.
type FileLedger struct {
	path   string
	file   *os.File
	mu     sync.Mutex
	closed bool
}

// Compile-time interface check.
var _ Ledger = (*FileLedger)(nil)

// NewFileLedger opens or creates a JSONL ledger file for append-only writes.
func NewFileLedger(path string) (*FileLedger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to open ledger file")
	}
	return &FileLedger{path: path, file: f}, nil
}

// Append serializes an entry as a single JSON line and writes it durably.
func (l *FileLedger) Append(entry *Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return sberrors.New(sberrors.ErrCodeLedgerError, "ledger is closed")
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to marshal ledger entry")
	}

	data = append(data, '\n')

	if _, err := l.file.Write(data); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to write ledger entry")
	}

	if err := l.file.Sync(); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to sync ledger")
	}

	return nil
}

// ReadAll opens the ledger file for reading and returns all entries in order.
func (l *FileLedger) ReadAll() ([]Entry, error) {
	f, err := os.Open(l.path)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to open ledger for reading")
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to unmarshal ledger entry")
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to read ledger")
	}

	return entries, nil
}

// Close syncs and closes the ledger file. Subsequent calls are no-ops.
func (l *FileLedger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	l.closed = true

	if err := l.file.Sync(); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeLedgerError, "failed to sync ledger on close")
	}

	return l.file.Close()
}
