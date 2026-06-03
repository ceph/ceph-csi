/*
Copyright 2025 The Ceph-CSI Authors.

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

package kernel

import (
	"strings"
	"testing"
)

func TestGetKernelVersion(t *testing.T) {
	t.Parallel()
	version, err := GetKernelVersion()
	if err != nil {
		t.Errorf("failed to get kernel version: %s", err)
	}
	if version == "" {
		t.Error("version is empty, this is unexpected?!")
	}
	if strings.HasSuffix(version, "\x00") {
		t.Error("version ends with \\x00 byte(s)")
	}
}

func TestParseKernelRelease(t *testing.T) {
	t.Parallel()

	badReleases := []string{"x", "5", "5.", "5.4.", "5.x-2-oops", "4.1.x-7-oh", "5.12.x"}
	for _, release := range badReleases {
		_, _, _, _, err := parseKernelRelease(release)
		if err == nil {
			t.Errorf("release %q must not be parsed successfully", release)
		}
	}

	goodReleases := []string{
		"5.12", "5.12xlinux", "5.1-2-yam", "3.1-5-x", "5.12.14", "5.12.14xlinux",
		"5.12.14-xlinux", "5.12.14-99-x", "3.3x-3",
	}
	goodVersions := [][]int{
		{5, 12, 0, 0},
		{5, 12, 0, 0},
		{5, 1, 0, 2},
		{3, 1, 0, 5},
		{5, 12, 14, 0},
		{5, 12, 14, 0},
		{5, 12, 14, 0},
		{5, 12, 14, 99},
		{3, 3, 0, 0},
	}
	for i, release := range goodReleases {
		version, patchlevel, sublevel, extraversion, err := parseKernelRelease(release)
		if err != nil {
			t.Errorf("parsing error for release %q: %s", release, err)
		}
		good := goodVersions[i]
		if version != good[0] || patchlevel != good[1] || sublevel != good[2] || extraversion != good[3] {
			t.Errorf("release %q parsed incorrectly: expected (%d.%d.%d-%d), actual (%d.%d.%d-%d)",
				release, good[0], good[1], good[2], good[3],
				version, patchlevel, sublevel, extraversion)
		}
	}
}

func TestCheckKernelSupport(t *testing.T) {
	t.Parallel()
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

	quotaSupport := []KernelVersion{
		{4, 17, 0, 0, "", false},       // standard 4.17+ versions
		{3, 10, 0, 1062, ".el7", true}, // RHEL-7.7
	}
	for _, kernel := range supportsQuota {
		ok := CheckKernelSupport(kernel, quotaSupport)
		if !ok {
			t.Errorf("support expected for %s", kernel)
		}
	}

	for _, kernel := range noQuota {
		ok := CheckKernelSupport(kernel, quotaSupport)
		if ok {
			t.Errorf("no support expected for %s", kernel)
		}
	}

	supportsDeepFlatten := []string{
		"5.1.0", // 5.1+ supports deep-flatten
		"5.3.0",
		"4.18.0-193.9.1.el8_2.x86_64", // RHEL 8.2 kernel
	}

	noDeepFlatten := []string{
		"4.18.0",                     // too old
		"3.10.0-123.el7.x86_64",      // too old for backport
		"3.10.0-1062.4.1.el8.x86_64", // nonexisting RHEL-8 kernel
		"3.11.0-123.el7.x86_64",      // nonexisting RHEL-7 kernel
	}

	deepFlattenSupport := []KernelVersion{
		{5, 1, 0, 0, "", false},       // standard 5.1+ versions
		{4, 18, 0, 193, ".el8", true}, // RHEL 8.2 backport
	}
	for _, kernel := range supportsDeepFlatten {
		ok := CheckKernelSupport(kernel, deepFlattenSupport)
		if !ok {
			t.Errorf("support expected for %s", kernel)
		}
	}

	for _, kernel := range noDeepFlatten {
		ok := CheckKernelSupport(kernel, deepFlattenSupport)
		if ok {
			t.Errorf("no support expected for %s", kernel)
		}
	}
}
