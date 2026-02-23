package filelock

import (
	"os"
	"syscall"
)

// FlockLocker implements Locker using POSIX advisory file locks (flock).
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
	return l.tryFlock(syscall.LOCK_SH | syscall.LOCK_NB)
}

func (l *FlockLocker) UnlockShared() error {
	return syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
}

func (l *FlockLocker) TryLockExclusive() (bool, error) {
	return l.tryFlock(syscall.LOCK_EX | syscall.LOCK_NB)
}

func (l *FlockLocker) UnlockExclusive() error {
	return syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
}

// Close releases the file descriptor and any held lock.
func (l *FlockLocker) Close() error {
	return l.file.Close()
}

// tryFlock attempts a non-blocking flock with the given flags.
// Returns (true, nil) on success, (false, nil) if the lock is held,
// and (false, err) on unexpected errors.
func (l *FlockLocker) tryFlock(how int) (bool, error) {
	err := syscall.Flock(int(l.file.Fd()), how)
	if err == nil {
		return true, nil
	}
	if err == syscall.EWOULDBLOCK {
		return false, nil
	}
	return false, err
}
