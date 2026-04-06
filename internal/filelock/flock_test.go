package filelock

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain allows this test binary to act as a lock-holder subprocess when
// FLOCK_HELPER_PATH is set. The subprocess acquires the lock, prints "locked\n"
// to signal readiness, then holds the lock until stdin is closed.
func TestMain(m *testing.M) {
	if p := os.Getenv("FLOCK_HELPER_PATH"); p != "" {
		os.Exit(runLockHelper(p))
	}
	os.Exit(m.Run())
}

func runLockHelper(path string) int {
	locker, err := NewFlockLocker(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lock-helper:", err)
		return 1
	}
	defer locker.Close()

	exclusive := os.Getenv("FLOCK_HELPER_EXCLUSIVE") == "1"
	var acquired bool
	if exclusive {
		acquired, err = locker.TryLockExclusive()
	} else {
		acquired, err = locker.TryLockShared()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lock-helper acquire error:", err)
		return 1
	}
	if !acquired {
		fmt.Fprintln(os.Stderr, "lock-helper: lock not acquired")
		return 2
	}

	// Signal readiness to parent process.
	fmt.Fprintln(os.Stdout, "locked")

	// Hold lock until parent closes stdin.
	io.ReadAll(os.Stdin) //nolint:errcheck
	return 0
}

// startLockHolder spawns a subprocess that acquires the lock at path and holds
// it until the returned cleanup function is called. The caller must invoke
// cleanup to release the subprocess lock and reap the process.
func startLockHolder(t *testing.T, path string, exclusive bool) (cleanup func()) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^$")
	env := append(os.Environ(), "FLOCK_HELPER_PATH="+path)
	if exclusive {
		env = append(env, "FLOCK_HELPER_EXCLUSIVE=1")
	}
	cmd.Env = env

	stdinPipe, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdoutPipe, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Start())

	// Wait for the subprocess to signal it holds the lock.
	scanner := bufio.NewScanner(stdoutPipe)
	if !scanner.Scan() || scanner.Text() != "locked" {
		stdinPipe.Close()
		cmd.Wait() //nolint:errcheck
		t.Fatal("subprocess did not acquire lock")
	}

	return func() {
		stdinPipe.Close()
		cmd.Wait() //nolint:errcheck
	}
}

func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.lock")
}

// FR-3.4.3: Acquire exclusive and release succeeds.
func TestTryLockExclusive_unlock(t *testing.T) {
	path := lockPath(t)

	locker, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer locker.Close()

	acquired, err := locker.TryLockExclusive()
	require.NoError(t, err)
	assert.True(t, acquired, "should acquire exclusive lock on fresh file")

	err = locker.UnlockExclusive()
	require.NoError(t, err)
}

// FR-3.4.3: Returns true when no exclusive lock held.
func TestTryLockShared_available(t *testing.T) {
	path := lockPath(t)

	locker, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer locker.Close()

	acquired, err := locker.TryLockShared()
	require.NoError(t, err)
	assert.True(t, acquired, "should acquire shared lock when no lock held")

	err = locker.UnlockShared()
	require.NoError(t, err)
}

// FR-3.4.3: Multiple shared locks coexist (from separate fds).
func TestTryLockShared_multipleShared(t *testing.T) {
	path := lockPath(t)

	locker1, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer locker1.Close()

	locker2, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer locker2.Close()

	acquired1, err := locker1.TryLockShared()
	require.NoError(t, err)
	assert.True(t, acquired1, "first shared lock should succeed")

	acquired2, err := locker2.TryLockShared()
	require.NoError(t, err)
	assert.True(t, acquired2, "second shared lock should succeed while first is held")

	require.NoError(t, locker1.UnlockShared())
	require.NoError(t, locker2.UnlockShared())
}

// FR-3.4.3: Returns false when exclusive lock is held by another fd.
func TestTryLockShared_exclusiveHeld(t *testing.T) {
	path := lockPath(t)

	holder, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer holder.Close()

	acquired, err := holder.TryLockExclusive()
	require.NoError(t, err)
	require.True(t, acquired)

	contender, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer contender.Close()

	acquired, err = contender.TryLockShared()
	require.NoError(t, err)
	assert.False(t, acquired, "shared lock should fail when exclusive lock held by another fd")

	require.NoError(t, holder.UnlockExclusive())

	acquired, err = contender.TryLockShared()
	require.NoError(t, err)
	assert.True(t, acquired, "shared lock should succeed after exclusive lock released")
}

// FR-3.4.3: Returns false when shared lock is held by another fd.
func TestTryLockExclusive_sharedHeld(t *testing.T) {
	path := lockPath(t)

	holder, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer holder.Close()

	acquired, err := holder.TryLockShared()
	require.NoError(t, err)
	require.True(t, acquired)

	contender, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer contender.Close()

	acquired, err = contender.TryLockExclusive()
	require.NoError(t, err)
	assert.False(t, acquired, "exclusive lock should fail when shared lock held by another fd")

	require.NoError(t, holder.UnlockShared())

	acquired, err = contender.TryLockExclusive()
	require.NoError(t, err)
	assert.True(t, acquired, "exclusive lock should succeed after shared lock released")
}

// FR-3.4.3: Returns false when exclusive lock is held by another fd.
func TestTryLockExclusive_exclusiveHeld(t *testing.T) {
	path := lockPath(t)

	holder, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer holder.Close()

	acquired, err := holder.TryLockExclusive()
	require.NoError(t, err)
	require.True(t, acquired)

	contender, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer contender.Close()

	acquired, err = contender.TryLockExclusive()
	require.NoError(t, err)
	assert.False(t, acquired, "exclusive lock should fail when exclusive lock held by another fd")

	require.NoError(t, holder.UnlockExclusive())

	acquired, err = contender.TryLockExclusive()
	require.NoError(t, err)
	assert.True(t, acquired, "exclusive lock should succeed after exclusive lock released")
}

func TestNewFlockLocker_createsFile(t *testing.T) {
	path := lockPath(t)

	locker, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer locker.Close()

	_, err = os.Stat(path)
	assert.NoError(t, err, "lock file should be created by constructor")
}

func TestNewFlockLocker_invalidPath(t *testing.T) {
	_, err := NewFlockLocker("/nonexistent/dir/test.lock")
	assert.Error(t, err, "should fail when parent directory does not exist")
}

func TestTryLockShared_afterClose(t *testing.T) {
	path := lockPath(t)

	locker, err := NewFlockLocker(path)
	require.NoError(t, err)
	require.NoError(t, locker.Close())

	_, err = locker.TryLockShared()
	assert.Error(t, err, "should return error when operating on closed file")
}

func TestTryLockExclusive_afterClose(t *testing.T) {
	path := lockPath(t)

	locker, err := NewFlockLocker(path)
	require.NoError(t, err)
	require.NoError(t, locker.Close())

	_, err = locker.TryLockExclusive()
	assert.Error(t, err, "should return error when operating on closed file")
}

// Cross-process tests: these spawn a subprocess that holds a lock and verify
// that the in-process locker correctly observes contention across process boundaries.
// This is the scenario where byte-range vs. whole-file semantics (Unix vs Windows)
// could diverge, so it's important to cover explicitly.

// TestCrossProcess_ExclusiveBlocksAll verifies that an exclusive lock held by a
// separate process prevents both shared and exclusive acquisition in this process.
func TestCrossProcess_ExclusiveBlocksAll(t *testing.T) {
	path := lockPath(t)

	cleanup := startLockHolder(t, path, true /* exclusive */)
	defer cleanup()

	locker, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer locker.Close()

	acquired, err := locker.TryLockExclusive()
	require.NoError(t, err)
	assert.False(t, acquired, "exclusive lock must be blocked by cross-process exclusive lock")

	acquired, err = locker.TryLockShared()
	require.NoError(t, err)
	assert.False(t, acquired, "shared lock must be blocked by cross-process exclusive lock")

	// Release subprocess lock and verify we can now acquire.
	cleanup()
	acquired, err = locker.TryLockExclusive()
	require.NoError(t, err)
	assert.True(t, acquired, "exclusive lock must succeed after cross-process lock released")
}

// TestCrossProcess_SharedBlocksExclusive verifies that a shared lock held by a
// separate process prevents exclusive acquisition but allows shared acquisition.
func TestCrossProcess_SharedBlocksExclusive(t *testing.T) {
	path := lockPath(t)

	cleanup := startLockHolder(t, path, false /* shared */)
	defer cleanup()

	locker, err := NewFlockLocker(path)
	require.NoError(t, err)
	defer locker.Close()

	acquired, err := locker.TryLockExclusive()
	require.NoError(t, err)
	assert.False(t, acquired, "exclusive lock must be blocked by cross-process shared lock")

	acquired, err = locker.TryLockShared()
	require.NoError(t, err)
	assert.True(t, acquired, "shared lock must succeed alongside cross-process shared lock")
	require.NoError(t, locker.UnlockShared())
}
