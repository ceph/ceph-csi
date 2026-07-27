/*
Copyright 2026 ceph-csi authors.

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

package healthchecker

import (
	"testing"
	"time"
)

func TestCheckerNotYetChecked(t *testing.T) {
	t.Parallel()

	c := &checker{}
	c.initDefaults()

	healthy, err := c.isHealthy()
	if !healthy || err == nil {
		t.Errorf("expected (true, error) for a checker that has not completed its first check, got (%t, %v)",
			healthy, err)
	}
}

func TestCheckerStuckFirstCheckEventuallyTimesOut(t *testing.T) {
	t.Parallel()

	c := &checker{}
	c.initDefaults()
	c.interval = time.Millisecond
	c.timeout = time.Millisecond

	c.mutex.Lock()
	c.lastUpdate = time.Now().Add(-time.Hour)
	c.mutex.Unlock()

	healthy, err := c.isHealthy()
	if healthy || err == nil {
		t.Errorf("expected (false, error) for a checker whose first check is stuck and overdue, got (%t, %v)",
			healthy, err)
	}
}

func TestCheckerConfirmedHealthyStillExpires(t *testing.T) {
	t.Parallel()

	c := &checker{}
	c.initDefaults()
	c.interval = time.Millisecond
	c.timeout = time.Millisecond

	c.mutex.Lock()
	c.checked = true
	c.healthy = true
	c.err = nil
	c.lastUpdate = time.Now().Add(-time.Hour)
	c.mutex.Unlock()

	healthy, err := c.isHealthy()
	if healthy || err == nil {
		t.Errorf("expected (false, error) for a checker that stopped reporting, got (%t, %v)", healthy, err)
	}
}
