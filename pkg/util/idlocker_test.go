/*
Copyright 2019 ceph-csi authors.

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
	"testing"
)

// very basic tests for the moment
func TestIDLocker(t *testing.T) {
	myIDLocker := NewIDLocker()

	lk1 := myIDLocker.Lock("lk1")
	lk2 := myIDLocker.Lock("lk2")
	lk3 := myIDLocker.Lock("lk3")

	if lk1 == lk2 || lk2 == lk3 || lk3 == lk1 {
		t.Errorf("Failed: lock variables clash when they should not!")
	}

	myIDLocker.Unlock(lk1, "lk1")
	myIDLocker.Unlock(lk2, "lk2")
	myIDLocker.Unlock(lk3, "lk3")
}
