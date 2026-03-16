/*
Copyright 2026 The Ceph-CSI Authors.

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

package lock

import (
	mathrand "math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestGroupLock_MultipleGroupA verifies multiple Group A operations can run concurrently.
func TestGroupLock_MultipleGroupA(t *testing.T) {
	// This test can run in parallel with other tests,
	// it does not rely on any shared state outside of the GroupLock instance it creates.
	t.Parallel()

	gl := NewGroupLock()
	const numGoroutines = 10

	var activeCount int32
	var maxConcurrent int32
	var wg sync.WaitGroup
	for range numGoroutines {
		wg.Go(func() { // wg.Go(func()) automatically runs Add(1) + defer Done()
			gl.AcquireGroupA()
			defer gl.ReleaseGroupA()

			// Track concurrent executions
			current := atomic.AddInt32(&activeCount, 1)
			defer atomic.AddInt32(&activeCount, -1)

			// Update the maximum concurrent operations count.
			// This uses a compare-and-swap loop to handle race conditions:
			// - If our count is not higher than current max, skip
			// - Otherwise, try to atomically update the max
			// - If compare-and-swap fails (another thread updated it), retry
			for {
				oldMax := atomic.LoadInt32(&maxConcurrent)

				if current <= oldMax {
					break // Not a new maximum
				}

				success := atomic.CompareAndSwapInt32(&maxConcurrent, oldMax, current)
				if success {
					break // Successfully set new maximum
				}

				// compare-and-swap failed, loop and try again, it means another thread updated maxConcurrent in the meantime
				// We will check the new value of maxConcurrent in the next iteration
				// If current is still greater than the new maxConcurrent, we will attempt to update it again
				// If current is no longer greater, we will exit the loop
				// This ensures we always end up with the correct maximum concurrent count without race conditions
				// and without needing locks or other synchronization primitives
			}

			// Simulate work
			time.Sleep(10 * time.Millisecond)
		})
	}

	wg.Wait()

	// All Group A operations should have run concurrently
	if maxConcurrent < 2 {
		t.Errorf("Expected concurrent Group A operations, got max concurrent: %d", maxConcurrent)
	}

	t.Logf("Max concurrent Group A operations: %d", maxConcurrent)
}

// TestGroupLock_GroupABlocksGroupB verifies Group A blocks Group B.
func TestGroupLock_GroupABlocksGroupB(t *testing.T) {
	// This test can run in parallel with other tests,
	// it does not rely on any shared state outside of the GroupLock instance it creates.
	t.Parallel()

	gl := NewGroupLock()

	groupBStarted := make(chan bool, 1)
	groupADone := make(chan bool, 1)

	// Start Group A
	go func() {
		gl.AcquireGroupA()
		defer gl.ReleaseGroupA()

		// Hold for a bit
		time.Sleep(100 * time.Millisecond)
		groupADone <- true
	}()

	// Give Group A time to acquire
	time.Sleep(10 * time.Millisecond)

	// Try to start Group B
	go func() {
		gl.AcquireGroupB()
		defer gl.ReleaseGroupB()

		groupBStarted <- true
	}()

	// Group B should not start while Group A is active
	select {
	case <-groupBStarted:
		t.Error("Group B started while Group A was active!")
	case <-time.After(50 * time.Millisecond):
		t.Log("Group B correctly blocked while Group A active")
	}

	// Wait for Group A to finish
	<-groupADone
	t.Log("Group A finished, Group B should now start")

	// Now Group B should be able to start
	select {
	case <-groupBStarted:
		t.Log("Group B started after Group A released")
	case <-time.After(100 * time.Millisecond):
		t.Error("Group B did not start after Group A finished")
	}
}

// TestGroupLock_NoDeadlock verifies no deadlock occurs with mixed operations.
func TestGroupLock_NoDeadlock(t *testing.T) {
	// This test can run in parallel with other tests,
	// it does not rely on any shared state outside of the GroupLock instance it creates.
	t.Parallel()

	gl := NewGroupLock()
	const (
		numWorkers   = 50                   // Number of concurrent workers for each group
		opsPerWorker = 20                   // Number of operations each worker will perform
		workTime     = 5 * time.Millisecond // Simulate some work time for each operation
	)

	var wg sync.WaitGroup

	// Launch Group A workers
	for range numWorkers {
		wg.Go(func() {
			for range opsPerWorker {
				gl.AcquireGroupA()
				time.Sleep(workTime)
				gl.ReleaseGroupA()
				// This sleep lets other threads a chance (same or different group) to acquire the lock
				time.Sleep(time.Millisecond)
			}
		})
	}

	// Launch Group B workers
	for range numWorkers {
		wg.Go(func() {
			for range opsPerWorker {
				gl.AcquireGroupB()
				time.Sleep(workTime)
				gl.ReleaseGroupB()
				// This sleep lets other threads a chance (same or different group) to acquire the lock
				time.Sleep(time.Millisecond)
			}
		})
	}

	done := make(chan bool)
	go func() {
		// Wait for all workers to finish
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		t.Logf("All %d operations completed without deadlock",
			numWorkers*opsPerWorker*2)
	case <-time.After(30 * time.Second):
		t.Fatal("Deadlock detected")
	}
}

// TestGroupLock_MutualExclusion verifies Groups A and B never run simultaneously.
func TestGroupLock_MutualExclusion(t *testing.T) {
	// This test can run in parallel with other tests,
	// it does not rely on any shared state outside of the GroupLock instance it creates.
	t.Parallel()

	gl := NewGroupLock()
	const duration = 500 * time.Millisecond

	var groupAActive int32 // How many Group A threads are currently working
	var groupBActive int32 // How many Group B threads are currently working
	var violations int32   // How many times we caught both groups active together

	done := make(chan bool)

	// Monitor for violations
	go func() {
		ticker := time.NewTicker(time.Millisecond) // Check every 1ms
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return // Test finished, stop monitoring
			case <-ticker.C: // Every 1ms
				a := atomic.LoadInt32(&groupAActive)
				b := atomic.LoadInt32(&groupBActive)
				if a > 0 && b > 0 {
					atomic.AddInt32(&violations, 1)
					t.Errorf("VIOLATION: Group A (%d) and Group B (%d) active simultaneously!", a, b)
				}
			}
		}
	}()

	var wg sync.WaitGroup

	// Launch Group A workers
	for range 10 {
		wg.Go(func() {
			start := time.Now()
			for time.Since(start) < duration { // Run for 500ms
				gl.AcquireGroupA()
				atomic.AddInt32(&groupAActive, 1)

				time.Sleep(5 * time.Millisecond)

				atomic.AddInt32(&groupAActive, -1)
				gl.ReleaseGroupA()

				time.Sleep(time.Millisecond)
			}
		})
	}

	// Launch Group B workers
	for range 10 {
		wg.Go(func() {
			start := time.Now()
			for time.Since(start) < duration { // Run for 500ms
				gl.AcquireGroupB()
				atomic.AddInt32(&groupBActive, 1)

				time.Sleep(5 * time.Millisecond)

				atomic.AddInt32(&groupBActive, -1)
				gl.ReleaseGroupB()

				time.Sleep(time.Millisecond)
			}
		})
	}

	wg.Wait()
	close(done) // Tell the monitoring goroutine to stop

	// Check if any violations were detected
	if v := atomic.LoadInt32(&violations); v > 0 {
		t.Errorf("Found %d mutual exclusion violations", v)
	} else {
		t.Log("No mutual exclusion violations detected")
	}
}

// TestGroupLock_AllOrNothing verifies all waiting operations of same group start together.
func TestGroupLock_AllOrNothing(t *testing.T) {
	// This test can run in parallel with other tests,
	// it does not rely on any shared state outside of the GroupLock instance it creates.
	t.Parallel()

	gl := NewGroupLock()
	const numWaiters = 5

	// Start Group A
	gl.AcquireGroupA()

	var startTimes [numWaiters]time.Time
	var wg sync.WaitGroup

	// Queue up Group B waiters
	for i := range numWaiters {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			gl.AcquireGroupB()                // internal logic will block until Group A releases
			startTimes[id] = time.Now()       // record when this worker actually started
			time.Sleep(10 * time.Millisecond) // Simulate work
			gl.ReleaseGroupB()                // Release after work is done
		}(i)
	}

	// Give them time to queue up
	time.Sleep(100 * time.Millisecond)

	// Release Group A - this should wake all Group B waiters
	gl.ReleaseGroupA()

	wg.Wait()

	// Check that all Group B operations started within a small time window
	minTime := startTimes[0]
	maxTime := startTimes[0]

	for _, t := range startTimes[1:] { // Skip first, already used
		if t.Before(minTime) {
			minTime = t
		}
		if t.After(maxTime) {
			maxTime = t
		}
	}

	// We expect all Group B operations to start within a very short time window after Group A releases
	spread := maxTime.Sub(minTime)
	if spread > 50*time.Millisecond {
		t.Errorf("Group B operations started over %v (expected near-simultaneous)", spread)
	} else {
		t.Logf("All Group B operations started within %v", spread)
	}
}

// TestGroupLock_StressTest -
// This test will run a large number of concurrent Group A and Group B operations for a fixed duration.
// they will run for a fixed duration, repeatedly acquiring and releasing the lock to simulate
// a high load scenario and check for any issues under stress conditions.
func TestGroupLock_StressTest(t *testing.T) {
	// This test can run in parallel with other tests,
	// it does not rely on any shared state outside of the GroupLock instance it creates.
	t.Parallel()

	gl := NewGroupLock()
	const (
		numWorkers = 100
		duration   = 2 * time.Second
	)

	var opsA, opsB int64
	var wg sync.WaitGroup

	start := time.Now()

	// Group A workers
	for range numWorkers {
		wg.Go(func() {
			for time.Since(start) < duration { // Run for 2 seconds
				gl.AcquireGroupA()
				atomic.AddInt64(&opsA, 1)
				time.Sleep(time.Microsecond * 100)
				gl.ReleaseGroupA()
			}
		})
	}

	// Group B workers
	for range numWorkers {
		wg.Go(func() {
			for time.Since(start) < duration { // Run for 2 seconds
				gl.AcquireGroupB()
				atomic.AddInt64(&opsB, 1)
				time.Sleep(time.Microsecond * 100)
				gl.ReleaseGroupB()
			}
		})
	}

	wg.Wait()

	t.Logf("Stress test completed: %d Group A ops, %d Group B ops in %v",
		opsA, opsB, duration)

	if opsA == 0 || opsB == 0 {
		t.Error("One group was starved - no operations completed")
	}
}

// BenchmarkGroupLock measures lock performance
// This benchmark runs a large number of Group A
// operations in parallel to measure the overhead of the lock under contention.
// Example of output:
// BenchmarkGroupLock_GroupA
// BenchmarkGroupLock_GroupA-8      5155239               223.2 ns/op
//
// Explanation of output:
// In this example, "BenchmarkGroupLock_GroupA-8" indicates the benchmark name and that it was run with 8 CPU cores.
// "5155239" is the number of iterations the benchmark ran,
// and "223.2 ns/op" means that each operation (acquire + release) took an average of 223.2 nanoseconds.
//
// Note: Benchmark results can vary based on the machine and current load,
// so they should be interpreted as relative performance indicators rather than absolute numbers.
func BenchmarkGroupLock_GroupA(b *testing.B) {
	gl := NewGroupLock()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			gl.AcquireGroupA()
			gl.ReleaseGroupA()
		}
	})
}

// BenchmarkGroupLock_Alternating measures lock performance under alternating contention
// This benchmark simulates a scenario where Group A and Group B operations alternate rapidly,
// which can be a common real-world pattern. It helps to measure how well the lock handles
// frequent context switches between groups and the overhead of waking up waiting threads.
func BenchmarkGroupLock_Alternating(b *testing.B) {
	gl := NewGroupLock()

	b.RunParallel(func(pb *testing.PB) {
		useGroupA := 1
		for pb.Next() {
			if useGroupA == 1 {
				gl.AcquireGroupA()
				gl.ReleaseGroupA()
			} else {
				gl.AcquireGroupB()
				gl.ReleaseGroupB()
			}
			// #nosec G404 -- This is a benchmark, not production code,
			//  so using math/rand is acceptable here for simulating alternating groups.
			useGroupA = mathrand.Intn(2) // Randomly switch between Group A and Group B (0 or 1)
		}
	})
}
