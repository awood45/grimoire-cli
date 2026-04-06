//go:build windows

package filelock

import (
	"os"

	"golang.org/x/sys/windows"
)

// FlockLocker implements Locker using Windows LockFileEx advisory file locks.
// Each instance opens its own file descriptor, so separate instances
// on the same file correctly contend across processes.
type FlockLocker struct {
	file *os.File
}

// Compile-time interface check.
var _ Locker = (*FlockLocker)(nil)

// NewFlockLocker creates a new FlockLocker for the given path.
// The lock file is created if it does not exist.
func NewFlockLocker(path string) (*FlockLocker, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return &FlockLocker{file: f}, nil
}

func (l *FlockLocker) TryLockShared() (bool, error) {
	return l.tryLock(windows.LOCKFILE_FAIL_IMMEDIATELY)
}

func (l *FlockLocker) UnlockShared() error {
	return l.unlock()
}

func (l *FlockLocker) TryLockExclusive() (bool, error) {
	return l.tryLock(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
}

func (l *FlockLocker) UnlockExclusive() error {
	return l.unlock()
}

// Close releases the file descriptor and any held lock.
func (l *FlockLocker) Close() error {
	return l.file.Close()
}

// tryLock attempts a non-blocking lock with the given flags via LockFileEx.
// Returns (true, nil) on success, (false, nil) if the lock is already held,
// and (false, err) on unexpected errors.
func (l *FlockLocker) tryLock(flags uint32) (bool, error) {
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(l.file.Fd()), flags, 0, 1, 0, ol)
	if err == nil {
		return true, nil
	}
	if err == windows.ERROR_LOCK_VIOLATION {
		return false, nil
	}
	return false, err
}

func (l *FlockLocker) unlock() error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, ol)
}
