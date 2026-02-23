// Package testing provides hand-written fake implementations of filelock interfaces for use in tests.
package testing

import (
	"sync"

	"github.com/awood45/grimoire-cli/internal/filelock"
)

// Compile-time interface assertion.
var _ filelock.Locker = (*FakeLocker)(nil)

// FakeLocker is an in-memory implementation of filelock.Locker.
// It tracks lock state and supports configurable contention simulation.
type FakeLocker struct {
	mu sync.Mutex

	// TrySharedResult controls what TryLockShared returns (true = acquired).
	TrySharedResult bool

	// TryExclusiveResult controls what TryLockExclusive returns (true = acquired).
	TryExclusiveResult bool

	// Error injection fields. When set, the corresponding method returns
	// this error instead of succeeding.
	TrySharedErr       error
	UnlockSharedErr    error
	TryExclusiveErr    error
	UnlockExclusiveErr error

	// SharedLocked tracks whether a shared lock is currently held.
	SharedLocked bool

	// ExclusiveLocked tracks whether an exclusive lock is currently held.
	ExclusiveLocked bool

	// Call tracking for test assertions.
	TrySharedCalls       int
	UnlockSharedCalls    int
	TryExclusiveCalls    int
	UnlockExclusiveCalls int
}

// NewFakeLocker creates a FakeLocker that grants all lock requests by default.
func NewFakeLocker() *FakeLocker {
	return &FakeLocker{
		TrySharedResult:    true,
		TryExclusiveResult: true,
	}
}

// TryLockShared attempts a non-blocking shared lock.
func (f *FakeLocker) TryLockShared() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.TrySharedCalls++

	if f.TrySharedErr != nil {
		return false, f.TrySharedErr
	}

	if f.TrySharedResult {
		f.SharedLocked = true
	}

	return f.TrySharedResult, nil
}

// UnlockShared releases the shared lock.
func (f *FakeLocker) UnlockShared() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.UnlockSharedCalls++

	if f.UnlockSharedErr != nil {
		return f.UnlockSharedErr
	}

	f.SharedLocked = false
	return nil
}

// TryLockExclusive attempts a non-blocking exclusive lock.
func (f *FakeLocker) TryLockExclusive() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.TryExclusiveCalls++

	if f.TryExclusiveErr != nil {
		return false, f.TryExclusiveErr
	}

	if f.TryExclusiveResult {
		f.ExclusiveLocked = true
	}

	return f.TryExclusiveResult, nil
}

// UnlockExclusive releases the exclusive lock.
func (f *FakeLocker) UnlockExclusive() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.UnlockExclusiveCalls++

	if f.UnlockExclusiveErr != nil {
		return f.UnlockExclusiveErr
	}

	f.ExclusiveLocked = false
	return nil
}
