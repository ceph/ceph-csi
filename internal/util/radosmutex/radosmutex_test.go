/*
Copyright 2022 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package radosmutex

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ceph/ceph-csi/internal/util/radosmutex/retryoptions"
	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"

	"github.com/stretchr/testify/require"
)

const lockName = "hello-lock"
const lockOwner = "helloWorld"

func TestTryToAquireLockWithMultipleClients(ts *testing.T) {
	ts.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
	const lockName = "volume-1-lock"

	retryOptions := retryoptions.RetryOptions{
		MaxAttempts:   10,
		SleepDuration: 500 * time.Millisecond,
	}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		lockResult, err := CreateOrAquireLock(ctx, ioctx, lockName, "pod-1", retryOptions)
		time.Sleep(250 * time.Millisecond)
		require.NoError(ts, err)
		require.Equal(ts, true, lockResult)
		time.Sleep(250 * time.Millisecond)
		releaseResult, err1 := ReleaseLock(ctx, ioctx, lockName, "pod-1")
		require.NoError(ts, err1)
		require.Equal(ts, true, releaseResult)
	}()

	go func() {
		defer wg.Done()
		lockResult, err := CreateOrAquireLock(ctx, ioctx, lockName, "pod-2", retryOptions)
		time.Sleep(250 * time.Millisecond)
		require.NoError(ts, err)
		require.Equal(ts, true, lockResult)
		time.Sleep(250 * time.Millisecond)
		releaseResult, err1 := ReleaseLock(ctx, ioctx, lockName, "pod-2")
		require.NoError(ts, err1)
		require.Equal(ts, true, releaseResult)
	}()

	go func() {
		defer wg.Done()
		lockResult, err := CreateOrAquireLock(ctx, ioctx, lockName, "pod-3", retryOptions)
		time.Sleep(250 * time.Millisecond)
		require.NoError(ts, err)
		require.Equal(ts, true, lockResult)
		time.Sleep(250 * time.Millisecond)
		releaseResult, err1 := ReleaseLock(ctx, ioctx, lockName, "pod-3")
		require.NoError(ts, err1)
		require.Equal(ts, true, releaseResult)
	}()

	wg.Wait()
}
