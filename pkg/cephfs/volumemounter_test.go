/*
Copyright 2019 The Ceph-CSI Authors.

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

package cephfs

import (
	"testing"
)

func init() {
}

func TestKernelSupportsQuota(t *testing.T) {
	supportsQuota := []string{
		"4.17.0",
		"5.0.0",
		"4.17.0-rc1",
		"4.18.0-80.el8",
		"3.10.0-1062.el7.x86_64",     // 1st backport
		"3.10.0-1062.4.1.el7.x86_64", // updated backport
	}

	noQuota := []string{
		"2.6.32-754.15.3.el6.x86_64", // too old
		"3.10.0-123.el7.x86_64",      // too old for backport
		"3.10.0-1062.4.1.el8.x86_64", // nonexisting RHEL-8 kernel
		"3.11.0-123.el7.x86_64",      // nonexisting RHEL-7 kernel
	}

	for _, kernel := range supportsQuota {
		ok := kernelSupportsQuota(kernel)
		if !ok {
			t.Errorf("support expected for %s", kernel)
		}
	}

	for _, kernel := range noQuota {
		ok := kernelSupportsQuota(kernel)
		if ok {
			t.Errorf("no support expected for %s", kernel)
		}
	}
}
