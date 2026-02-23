package filelock

// Locker provides advisory file locking with shared/exclusive semantics.
// Shared locks allow concurrent readers; exclusive locks are held by a single writer.
type Locker interface {
	// TryLockShared attempts a non-blocking shared lock.
	// Returns (true, nil) if acquired, (false, nil) if an exclusive lock is held.
	TryLockShared() (bool, error)

	// UnlockShared releases the shared lock.
	UnlockShared() error

	// TryLockExclusive attempts a non-blocking exclusive lock.
	// Returns (true, nil) if acquired, (false, nil) if any lock is held by another process.
	TryLockExclusive() (bool, error)

	// UnlockExclusive releases the exclusive lock.
	UnlockExclusive() error
}
