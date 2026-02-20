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
	"fmt"
)

// Semaphore is a counting semaphore for limiting concurrent operations.
type Semaphore struct {
	sem chan struct{}
}

// NewSemaphore creates a new semaphore with the given capacity.
func NewSemaphore(capacity int) *Semaphore {
	return &Semaphore{
		sem: make(chan struct{}, capacity),
	}
}

// TryAcquire attempts to acquire the semaphore without blocking.
// Returns true if successful, false if at capacity.
func (s *Semaphore) TryAcquire() bool {
	select {
	case s.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// Acquire acquires the semaphore, blocking until capacity is available or context is canceled.
func (s *Semaphore) Acquire(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("failed to acquire semaphore: %w", ctx.Err())
	}
}

// Release releases the semaphore, making room for another operation.
func (s *Semaphore) Release() {
	select {
	case <-s.sem:
	default:
		// Should not happen.
	}
}

// Available returns the number of available slots in the semaphore.
func (s *Semaphore) Available() int {
	return cap(s.sem) - len(s.sem)
}

// InUse returns the number of currently acquired slots.
func (s *Semaphore) InUse() int {
	return len(s.sem)
}
