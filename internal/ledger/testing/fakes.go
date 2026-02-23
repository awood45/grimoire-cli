// Package testing provides hand-written fake implementations of ledger interfaces for use in tests.
package testing

import (
	"sync"

	"github.com/awood45/grimoire-cli/internal/ledger"
)

// Compile-time interface assertion.
var _ ledger.Ledger = (*FakeLedger)(nil)

// FakeLedger is an in-memory implementation of ledger.Ledger.
// Error injection fields allow tests to simulate failures.
type FakeLedger struct {
	mu sync.Mutex

	// Entries is the in-memory slice of ledger entries.
	Entries []ledger.Entry

	// Error injection fields. When set, the corresponding method returns
	// this error instead of succeeding.
	AppendErr  error
	ReadAllErr error
	CloseErr   error

	// Closed tracks whether Close has been called.
	Closed bool
}

// NewFakeLedger creates a FakeLedger with an initialized entries slice.
func NewFakeLedger() *FakeLedger {
	return &FakeLedger{
		Entries: make([]ledger.Entry, 0),
	}
}

// Append adds an entry to the in-memory ledger.
func (f *FakeLedger) Append(entry *ledger.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.AppendErr != nil {
		return f.AppendErr
	}

	f.Entries = append(f.Entries, *entry)
	return nil
}

// ReadAll returns all entries in the in-memory ledger.
func (f *FakeLedger) ReadAll() ([]ledger.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ReadAllErr != nil {
		return nil, f.ReadAllErr
	}

	// Return a copy to avoid mutation.
	result := make([]ledger.Entry, len(f.Entries))
	copy(result, f.Entries)
	return result, nil
}

// Close marks the ledger as closed.
func (f *FakeLedger) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.CloseErr != nil {
		return f.CloseErr
	}

	f.Closed = true
	return nil
}
