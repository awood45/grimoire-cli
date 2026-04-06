//go:build !windows

package filelock

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestSyscallConn_PreservesNonblockFlag verifies that lock operations do not
// alter the O_NONBLOCK flag on the underlying file descriptor.
//
// The previous os.File.Fd()-based implementation cleared O_NONBLOCK as a side
// effect because Go's runtime calls syscall.SetNonblock(fd, false) when you
// retrieve a raw fd via Fd(). The SyscallConn-based implementation passes the
// fd through the runtime's network poller abstraction, which does not change
// the flag.
//
// To make the difference observable we open the lock file with O_NONBLOCK set,
// then perform lock/unlock, and assert the flag is still present. With the old
// implementation this test fails at the final assertion.
func TestSyscallConn_PreservesNonblockFlag(t *testing.T) {
	path := lockPath(t)

	// Open with O_NONBLOCK so we can detect if it gets cleared.
	rawFd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NONBLOCK, 0o600)
	require.NoError(t, err)

	f := os.NewFile(uintptr(rawFd), path)
	require.NotNil(t, f)

	rc, err := f.SyscallConn()
	require.NoError(t, err)

	// Directly construct the locker to use the O_NONBLOCK-opened file.
	locker := &FlockLocker{file: f, rawConn: rc}
	defer locker.Close()

	// Confirm O_NONBLOCK is set before any lock operations.
	assert.NotZero(t, getFcntlFlags(t, f)&syscall.O_NONBLOCK,
		"file should start in non-blocking mode")

	// Lock and unlock. With the old Fd()-based implementation these calls
	// would invoke file.Fd() → SetNonblock(false) → O_NONBLOCK cleared.
	acquired, err := locker.TryLockExclusive()
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, locker.UnlockExclusive())

	acquired, err = locker.TryLockShared()
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, locker.UnlockShared())

	// O_NONBLOCK must still be set — SyscallConn does not alter file flags.
	assert.NotZero(t, getFcntlFlags(t, f)&syscall.O_NONBLOCK,
		"lock operations must not clear O_NONBLOCK flag")
}

// getFcntlFlags returns the file status flags for f via F_GETFL.
func getFcntlFlags(t *testing.T, f *os.File) int {
	t.Helper()
	rc, err := f.SyscallConn()
	require.NoError(t, err)
	var flags int
	require.NoError(t, rc.Control(func(fd uintptr) {
		flags, _ = unix.FcntlInt(uintptr(fd), syscall.F_GETFL, 0)
	}))
	return flags
}
