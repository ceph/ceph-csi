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

package util

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSemaphore_TryAcquire(t *testing.T) {
	t.Parallel()
	sem := NewSemaphore(2)

	// Should acquire twice successfully
	if !sem.TryAcquire() || !sem.TryAcquire() {
		t.Error("TryAcquire() failed within capacity")
	}

	// Should fail when at capacity
	if sem.TryAcquire() {
		t.Error("TryAcquire() succeeded when at capacity")
	}

	// Should succeed after release
	sem.Release()
	if !sem.TryAcquire() {
		t.Error("TryAcquire() failed after release")
	}
}

func TestSemaphore_Acquire(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func() (*Semaphore, context.Context, context.CancelFunc)
		wantErr bool
	}{
		{
			name: "acquire with available slot",
			setup: func() (*Semaphore, context.Context, context.CancelFunc) {
				return NewSemaphore(2), context.Background(), func() {}
			},
			wantErr: false,
		},
		{
			name: "acquire fails on canceled context",
			setup: func() (*Semaphore, context.Context, context.CancelFunc) {
				sem := NewSemaphore(1)
				sem.TryAcquire() // Fill capacity
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				return sem, ctx, func() {}
			},
			wantErr: true,
		},
		{
			name: "acquire fails on timeout",
			setup: func() (*Semaphore, context.Context, context.CancelFunc) {
				sem := NewSemaphore(1)
				sem.TryAcquire() // Fill capacity
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)

				return sem, ctx, cancel
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sem, ctx, cancel := tt.setup()
			defer cancel()

			err := sem.Acquire(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Acquire() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSemaphore_Release(t *testing.T) {
	t.Parallel()
	sem := NewSemaphore(3)

	sem.TryAcquire()
	sem.TryAcquire()

	before := sem.Available()
	sem.Release()
	after := sem.Available()

	if after != before+1 {
		t.Errorf("Available() after release = %d, want %d", after, before+1)
	}
}

func TestSemaphore_Counters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		capacity  int
		acquires  int
		releases  int
		wantInUse int
		wantAvail int
	}{
		{"initial state", 5, 0, 0, 0, 5},
		{"after acquires", 5, 3, 0, 3, 2},
		{"after acquire and release", 5, 3, 1, 2, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sem := NewSemaphore(tt.capacity)

			for range tt.acquires {
				sem.TryAcquire()
			}
			for range tt.releases {
				sem.Release()
			}

			if sem.InUse() != tt.wantInUse {
				t.Errorf("InUse() = %d, want %d", sem.InUse(), tt.wantInUse)
			}
			if sem.Available() != tt.wantAvail {
				t.Errorf("Available() = %d, want %d", sem.Available(), tt.wantAvail)
			}
		})
	}
}

func TestSemaphore_Concurrent(t *testing.T) {
	t.Parallel()
	capacity := 10
	goroutines := 100
	sem := NewSemaphore(capacity)

	var successCount int
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Many goroutines try to acquire
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if sem.TryAcquire() {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if successCount != capacity {
		t.Errorf("successful acquisitions = %d, want %d", successCount, capacity)
	}
}
