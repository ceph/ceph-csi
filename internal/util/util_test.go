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
package util // nolint:testpackage // we're testing internals of the package.

import (
	"strings"
	"testing"
)

func TestRoundOffBytes(t *testing.T) {
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
			if got := RoundOffBytes(ts.args.bytes); got != ts.want {
				t.Errorf("RoundOffBytes() = %v, want %v", got, ts.want)
			}
		})
	}
}

func TestRoundOffVolSize(t *testing.T) {
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
			if got := RoundOffVolSize(ts.args.size); got != ts.want {
				t.Errorf("RoundOffVolSize() = %v, want %v", got, ts.want)
			}
		})
	}
}

func TestGetKernelVersion(t *testing.T) {
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
		t.Run(moaTest.name, func(t *testing.T) {
			result := MountOptionsAdd(mt.mountOptions, mt.option...)
			if result != mt.result {
				t.Errorf("MountOptionsAdd(): %v, want %v", result, mt.result)
			}
		})
	}
}
func TestCheckKernelSupport(t *testing.T) {
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
		"5.2.0",
		"5.3.0",
	}

	noDeepFlatten := []string{
		"4.18.0",                     // too old
		"3.10.0-123.el7.x86_64",      // too old for backport
		"3.10.0-1062.4.1.el8.x86_64", // nonexisting RHEL-8 kernel
		"3.11.0-123.el7.x86_64",      // nonexisting RHEL-7 kernel
	}

	deepFlattenSupport := []KernelVersion{
		{5, 2, 0, 0, "", false}, // standard 5.2+ versions
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
