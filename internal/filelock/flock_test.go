package filelock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
