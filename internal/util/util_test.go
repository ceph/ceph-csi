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
package util

import (
	"strings"
	"testing"
)

func TestRoundOffBytes(t *testing.T) {
	t.Parallel()
	type args struct {
		bytes int64
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			"1MiB conversions",
			args{
				bytes: 1048576,
			},
			1048576,
		},
		{
			"1000kiB conversion",
			args{
				bytes: 1000,
			},
			1048576, // equal to 1MiB
		},
		{
			"1.5Mib conversion",
			args{
				bytes: 1572864,
			},
			2097152, // equal to 2MiB
		},
		{
			"1.1MiB conversion",
			args{
				bytes: 1153434,
			},
			2097152, // equal to 2MiB
		},
		{
			"1.5GiB conversion",
			args{
				bytes: 1610612736,
			},
			2147483648, // equal to 2GiB
		},
		{
			"1.1GiB conversion",
			args{
				bytes: 1181116007,
			},
			2147483648, // equal to 2GiB
		},
	}
	for _, tt := range tests {
		ts := tt
		t.Run(ts.name, func(t *testing.T) {
			t.Parallel()
			if got := RoundOffBytes(ts.args.bytes); got != ts.want {
				t.Errorf("RoundOffBytes() = %v, want %v", got, ts.want)
			}
		})
	}
}

func TestRoundOffVolSize(t *testing.T) {
	t.Parallel()
	type args struct {
		size int64
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			"1MiB conversions",
			args{
				size: 1048576,
			},
			1, // MiB
		},
		{
			"1000kiB conversion",
			args{
				size: 1000,
			},
			1, // MiB
		},
		{
			"1.5Mib conversion",
			args{
				size: 1572864,
			},
			2, // MiB
		},
		{
			"1.1MiB conversion",
			args{
				size: 1153434,
			},
			2, // MiB
		},
		{
			"1.5GiB conversion",
			args{
				size: 1610612736,
			},
			2048, // MiB
		},
		{
			"1.1GiB conversion",
			args{
				size: 1181116007,
			},
			2048, // MiB
		},
	}
	for _, tt := range tests {
		ts := tt
		t.Run(ts.name, func(t *testing.T) {
			t.Parallel()
			if got := RoundOffVolSize(ts.args.size); got != ts.want {
				t.Errorf("RoundOffVolSize() = %v, want %v", got, ts.want)
			}
		})
	}
}

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

func TestMountOptionsAdd(t *testing.T) {
	t.Parallel()
	moaTests := []struct {
		name         string
		mountOptions string
		option       []string
		result       string
	}{
		{
			"add option to empty string",
			"",
			[]string{"new_option"},
			"new_option",
		},
		{
			"add empty option to string",
			"orig_option",
			[]string{""},
			"orig_option",
		},
		{
			"add empty option to empty string",
			"",
			[]string{""},
			"",
		},
		{
			"add option to single option string",
			"orig_option",
			[]string{"new_option"},
			"orig_option,new_option",
		},
		{
			"add option to multi option string",
			"orig_option,2nd_option",
			[]string{"new_option"},
			"orig_option,2nd_option,new_option",
		},
		{
			"add redundant option to multi option string",
			"orig_option,2nd_option",
			[]string{"2nd_option"},
			"orig_option,2nd_option",
		},
		{
			"add option to multi option string starting with ,",
			",orig_option,2nd_option",
			[]string{"new_option"},
			"orig_option,2nd_option,new_option",
		},
		{
			"add option to multi option string with trailing ,",
			"orig_option,2nd_option,",
			[]string{"new_option"},
			"orig_option,2nd_option,new_option",
		},
		{
			"add options to multi option string",
			"orig_option,2nd_option,",
			[]string{"new_option", "another_option"},
			"orig_option,2nd_option,new_option,another_option",
		},
		{
			"add options (one redundant) to multi option string",
			"orig_option,2nd_option,",
			[]string{"new_option", "2nd_option", "another_option"},
			"orig_option,2nd_option,new_option,another_option",
		},
	}

	for _, moaTest := range moaTests {
		mt := moaTest
		moaTest := moaTest
		t.Run(moaTest.name, func(t *testing.T) {
			t.Parallel()
			result := MountOptionsAdd(mt.mountOptions, mt.option...)
			if result != mt.result {
				t.Errorf("MountOptionsAdd(): %v, want %v", result, mt.result)
			}
		})
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

func TestRoundOffCephFSVolSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		size int64
		want int64
	}{
		{
			"1000kiB conversion",
			1000,
			4194304, // 4 MiB
		},
		{
			"1MiB conversions",
			1048576,
			4194304, // 4 MiB
		},
		{
			"1.5Mib conversion",
			1677722,
			4194304, // 4 MiB
		},
		{
			"1023MiB conversion",
			1072693248,
			1073741824, // 1024 MiB
		},
		{
			"1.5GiB conversion",
			1585446912,
			2147483648, // 2 GiB
		},
		{
			"1555MiB conversion",
			1630535680,
			2147483648, // 2 GiB
		},
	}
	for _, tt := range tests {
		ts := tt
		t.Run(ts.name, func(t *testing.T) {
			t.Parallel()
			if got := RoundOffCephFSVolSize(ts.size); got != ts.want {
				t.Errorf("RoundOffCephFSVolSize() = %v, want %v", got, ts.want)
			}
		})
	}
}
