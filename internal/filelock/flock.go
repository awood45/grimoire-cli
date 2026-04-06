//go:build !windows

package filelock

import (
	"os"
	"syscall"
)

// FlockLocker implements Locker using POSIX advisory file locks (flock).
// Each instance opens its own file descriptor, so separate instances
// on the same file correctly contend across processes.
type FlockLocker struct {
	file    *os.File
	rawConn syscall.RawConn
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
	rc, err := f.SyscallConn()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &FlockLocker{file: f, rawConn: rc}, nil
}

func (l *FlockLocker) TryLockShared() (bool, error) {
	return l.tryFlock(syscall.LOCK_SH | syscall.LOCK_NB)
}

func (l *FlockLocker) UnlockShared() error {
	return l.unlock()
}

func (l *FlockLocker) TryLockExclusive() (bool, error) {
	return l.tryFlock(syscall.LOCK_EX | syscall.LOCK_NB)
}

func (l *FlockLocker) UnlockExclusive() error {
	return l.unlock()
}

// Close releases the file descriptor and any held lock.
func (l *FlockLocker) Close() error {
	return l.file.Close()
}

// tryFlock attempts a non-blocking flock via SyscallConn, which does not alter
// the file's blocking mode — unlike os.File.Fd() which clears O_NONBLOCK.
// Returns (true, nil) on success, (false, nil) if the lock is held,
// and (false, err) on unexpected errors.
func (l *FlockLocker) tryFlock(how int) (bool, error) {
	var flockErr error
	acquired := false
	controlErr := l.rawConn.Control(func(fd uintptr) {
		e := syscall.Flock(int(fd), how)
		if e == nil {
			acquired = true
		} else if e != syscall.EWOULDBLOCK {
			flockErr = e
		}
	})
	if controlErr != nil {
		return false, controlErr
	}
	return acquired, flockErr
}

func (l *FlockLocker) unlock() error {
	var unlockErr error
	controlErr := l.rawConn.Control(func(fd uintptr) {
		unlockErr = syscall.Flock(int(fd), syscall.LOCK_UN)
	})
	if controlErr != nil {
		return controlErr
	}
	return unlockErr
}
