//go:build windows

package filelock

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// FlockLocker implements Locker using Windows LockFileEx advisory file locks.
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

// tryLock attempts a non-blocking lock via SyscallConn and LockFileEx.
// Using SyscallConn avoids the blocking-mode side effect of os.File.Fd().
// Returns (true, nil) on success, (false, nil) if the lock is already held,
// and (false, err) on unexpected errors.
func (l *FlockLocker) tryLock(flags uint32) (bool, error) {
	var lockErr error
	acquired := false
	controlErr := l.rawConn.Control(func(fd uintptr) {
		ol := &windows.Overlapped{}
		e := windows.LockFileEx(windows.Handle(fd), flags, 0, 1, 0, ol)
		if e == nil {
			acquired = true
		} else if e != windows.ERROR_LOCK_VIOLATION {
			lockErr = e
		}
	})
	if controlErr != nil {
		return false, controlErr
	}
	return acquired, lockErr
}

func (l *FlockLocker) unlock() error {
	var unlockErr error
	controlErr := l.rawConn.Control(func(fd uintptr) {
		ol := &windows.Overlapped{}
		unlockErr = windows.UnlockFileEx(windows.Handle(fd), 0, 1, 0, ol)
	})
	if controlErr != nil {
		return controlErr
	}
	return unlockErr
}
