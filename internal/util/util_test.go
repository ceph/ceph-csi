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

func TestKernelVersion(t *testing.T) {
	version, err := KernelVersion()
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
