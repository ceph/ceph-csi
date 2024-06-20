package v1

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ceph/ceph-csi/internal/util/radosmutex/lock"
	"github.com/ceph/ceph-csi/internal/util/radosmutex/lockstate"
	"github.com/ceph/ceph-csi/internal/util/radosmutex/retryoptions"
	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"
	"github.com/stretchr/testify/require"
)

func TestTryToAquireLock(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const lockUnlockedName = "hello"
	const lockLockedName = "bye-lock"
	const lockOwner = "HelloWorld"

	var (
		unlockedLock = lock.Lock{
			LockOwner:  "",
			LockState:  lockstate.Unlocked,
			LockExpiry: time.Time{},
		}

		lockedLock = lock.Lock{
			LockOwner:  "AnotherWorld",
			LockState:  lockstate.Locked,
			LockExpiry: time.Now().Add(24 * time.Hour),
		}

		serializedUnlockedLock, _ = unlockedLock.ToBytes()
		serializedLockedLock, _   = lockedLock.ToBytes()

		unlockedObj = radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
			Objs: map[string]*radoswrapper.FakeObj{
				lockUnlockedName: {
					Oid: lockUnlockedName,
					Omap: map[string][]byte{
						"hello": serializedUnlockedLock,
					},
				},
			},
		})

		lockedObj = radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
			Objs: map[string]*radoswrapper.FakeObj{
				lockLockedName: {
					Oid:  lockLockedName,
					Omap: map[string][]byte{lockLockedName: serializedLockedLock},
				},
			},
		})
	)

	retryOptions := retryoptions.RetryOptions{
		MaxAttempts:   3,
		SleepDuration: 1 * time.Millisecond,
	}

	returnedLock, err := TryToAquireLock(ctx, unlockedObj, lockUnlockedName, lockOwner, retryOptions)
	require.NoError(t, err)
	require.Equal(t, lockOwner, returnedLock.LockOwner)
	require.Equal(t, lockstate.Locked, returnedLock.LockState)
	require.NotEmpty(t, returnedLock.LockExpiry)
	require.True(t, time.Now().Before(returnedLock.LockExpiry), "Lock expiry must be in the future")

	returnedLock, err = TryToAquireLock(ctx, lockedObj, lockLockedName, lockOwner, retryOptions)
	require.Error(t, err)
	require.Equal(t, "AnotherWorld", returnedLock.LockOwner)
	require.Equal(t, lockstate.Locked, returnedLock.LockState)

}

func TestTryToAquireExpiredLock(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const lockUnlockedName = "hello"
	const lockLockedName = "bye-lock"
	const lockOwner = "HelloWorld"

	var (
		unlockedLock = lock.Lock{
			LockOwner:  "AnotherWorld",
			LockState:  lockstate.Locked,
			LockExpiry: time.Now().Add(-24 * time.Hour), // Past time
		}

		serializedUnlockedLock, _ = unlockedLock.ToBytes()

		unlockedObj = radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
			Objs: map[string]*radoswrapper.FakeObj{
				lockUnlockedName: {
					Oid: lockUnlockedName,
					Omap: map[string][]byte{
						lockUnlockedName: serializedUnlockedLock,
					},
				},
			},
		})
	)

	retryOptions := retryoptions.RetryOptions{
		MaxAttempts:   3,
		SleepDuration: 1 * time.Millisecond,
	}

	returnedLock, err := TryToAquireLock(ctx, unlockedObj, lockUnlockedName, lockOwner, retryOptions)
	require.NoError(t, err)
	require.Equal(t, lockOwner, returnedLock.LockOwner)
	require.Equal(t, lockstate.Locked, returnedLock.LockState)
	require.NotEmpty(t, returnedLock.LockExpiry)
	require.True(t, time.Now().Before(returnedLock.LockExpiry), "Lock expiry must be in the future")
}

func TestTryToAquireLockWithMultipleClients(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const lockName = "volume-1-lock"

	var (
		unlockedLock = lock.Lock{
			LockOwner:  "",
			LockState:  lockstate.Unlocked,
			LockExpiry: time.Time{},
		}

		serializedUnlockedLock, _ = unlockedLock.ToBytes()

		unlockedObj = radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
			Objs: map[string]*radoswrapper.FakeObj{
				lockName: {
					Oid: lockName,
					Omap: map[string][]byte{
						lockName: serializedUnlockedLock,
					},
				},
			},
		})
	)

	retryOptions := retryoptions.RetryOptions{
		MaxAttempts:   10,
		SleepDuration: 500 * time.Millisecond,
	}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		returnedLockPod1, err1 := TryToAquireLock(ctx, unlockedObj, lockName, "pod-1", retryOptions)
		time.Sleep(250 * time.Millisecond)
		require.NoError(t, err1)
		require.Equal(t, "pod-1", returnedLockPod1.LockOwner)
		time.Sleep(250 * time.Millisecond)
		gen, _ := unlockedObj.GetLastVersion()
		err1 = ReleaseLock(ctx, unlockedObj, lockName, "pod-1", gen)
		require.NoError(t, err1)
	}()

	go func() {
		defer wg.Done()
		returnedLockPod2, err2 := TryToAquireLock(ctx, unlockedObj, lockName, "pod-2", retryOptions)
		time.Sleep(250 * time.Millisecond)
		require.NoError(t, err2)
		require.Equal(t, "pod-2", returnedLockPod2.LockOwner)
		time.Sleep(250 * time.Millisecond)
		gen, _ := unlockedObj.GetLastVersion()
		err2 = ReleaseLock(ctx, unlockedObj, lockName, "pod-2", gen)
		require.NoError(t, err2)
	}()

	go func() {
		defer wg.Done()
		returnedLockPod3, err3 := TryToAquireLock(ctx, unlockedObj, lockName, "pod-3", retryOptions)
		time.Sleep(250 * time.Millisecond)
		require.NoError(t, err3)
		require.Equal(t, "pod-3", returnedLockPod3.LockOwner)
		time.Sleep(250 * time.Millisecond)
		gen, _ := unlockedObj.GetLastVersion()
		err3 = ReleaseLock(ctx, unlockedObj, lockName, "pod-3", gen)
		require.NoError(t, err3)
	}()

	wg.Wait()
}
