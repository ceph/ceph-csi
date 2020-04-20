/*
Copyright 2018 The Ceph-CSI Authors.

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

func TestCheckROMountFlag(t *testing.T) {
	type args struct {
		options map[string]string
		param   string
		flag    string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"Testing cephfs fuse ro mount option",
			args{
				options: map[string]string{"fuseMountOptions": "debug"},
				param:   "fuseMountOptions",
				flag:    "ro",
			},
			false,
		},

		{
			"Testing cephfs kernel ro mount option",
			args{
				options: map[string]string{"kernelMountOptions": "readdir_max_bytes=1048576,norbytes,ro"},
				param:   "kernelMountOptions",
				flag:    "ro",
			},
			true,
		},
	}
	for _, tt := range tests {
		ts := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckROMountFlag(ts.args.options, ts.args.param, ts.args.flag); got != ts.want {
				t.Errorf("CheckROMountFlag() = %v, want %v", got, ts.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	type args struct {
		mountOptions []string
		opt          string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"Testing mount options contains string",
			args{
				mountOptions: []string{"debug", "ro"},
				opt:          "ro"},
			true,
		},
		{"Testing mount options doesnt contains string",
			args{
				mountOptions: []string{"debug", "ro"},
				opt:          "_netdev"},
			false,
		},
	}

	for _, tt := range tests {
		ts := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := Contains(ts.args.mountOptions, ts.args.opt); got != ts.want {
				t.Errorf("Contains() = %v, want %v", got, ts.want)
			}
		})
	}
}
