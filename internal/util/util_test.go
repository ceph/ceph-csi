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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RoundOffBytes(tt.args.bytes); got != tt.want {
				t.Errorf("RoundOffBytes() = %v, want %v", got, tt.want)
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RoundOffVolSize(tt.args.size); got != tt.want {
				t.Errorf("RoundOffVolSize() = %v, want %v", got, tt.want)
			}
		})
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
		t.Run(moaTest.name, func(t *testing.T) {
			t.Parallel()
			result := MountOptionsAdd(moaTest.mountOptions, moaTest.option...)
			if result != moaTest.result {
				t.Errorf("MountOptionsAdd(): %v, want %v", result, moaTest.result)
			}
		})
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
			"101MB conversion",
			101000000,
			104857600, // 100MiB
		},
		{
			"500MB conversion",
			500000000,
			503316480, // 480MiB
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RoundOffCephFSVolSize(tt.size); got != tt.want {
				t.Errorf("RoundOffCephFSVolSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseClientIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{
			name:    "IPv4 address",
			addr:    "10.244.0.1:0/2686266785",
			want:    "10.244.0.1",
			wantErr: false,
		},
		{
			name:    "IPv6 address",
			addr:    "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:0/2686266785",
			want:    "2001:db8:85a3::8a2e:370:7334",
			wantErr: false,
		},
		{
			name:    "Compressed IPv6 address",
			addr:    "[fd98::4]:0/2686266785",
			want:    "fd98::4",
			wantErr: false,
		},
		{
			name:    "Invalid address",
			addr:    "invalid",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseClientIP(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseClientIP() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("ParseClientIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertIPToCIDR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ip      string
		want    string
		wantErr bool
	}{
		{
			name:    "Valid IPv4 address",
			ip:      "192.168.1.1",
			want:    "192.168.1.1/32",
			wantErr: false,
		},
		{
			name:    "Valid IPv4 address - loopback",
			ip:      "127.0.0.1",
			want:    "127.0.0.1/32",
			wantErr: false,
		},
		{
			name:    "Valid IPv4 address - zero",
			ip:      "0.0.0.0",
			want:    "0.0.0.0/32",
			wantErr: false,
		},
		{
			name:    "Valid IPv6 address",
			ip:      "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			want:    "2001:db8:85a3::8a2e:370:7334/128",
			wantErr: false,
		},
		{
			name:    "Valid IPv6 address - compressed",
			ip:      "fd98::4",
			want:    "fd98::4/128",
			wantErr: false,
		},
		{
			name:    "Valid IPv6 address - loopback",
			ip:      "::1",
			want:    "::1/128",
			wantErr: false,
		},
		{
			name:    "Valid IPv6 address - zero",
			ip:      "::",
			want:    "::/128",
			wantErr: false,
		},
		{
			name:    "Invalid IP address - empty string",
			ip:      "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Invalid IP address - malformed",
			ip:      "invalid",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Invalid IP address - incomplete IPv4",
			ip:      "192.168.1",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Invalid IP address - out of range",
			ip:      "256.256.256.256",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertIPToCIDR(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertIPToCIDR() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if got != tt.want {
				t.Errorf("ConvertIPToCIDR() = %v, want %v", got, tt.want)
			}
		})
	}
}
