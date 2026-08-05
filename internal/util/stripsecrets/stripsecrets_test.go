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

package stripsecrets

import (
	"slices"
	"strings"
	"testing"
)

func TestInArgsStripsSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "secret is the first option",
			args: []string{"-o", "secret=AQBsecretkey,mds_namespace=abc"},
			want: []string{"-o", "secret=***stripped***,mds_namespace=abc"},
		},
		{
			name: "secret is preceded by another option",
			args: []string{"-o", "name=admin,secret=AQBsecretkey,mds_namespace=abc"},
			want: []string{"-o", "name=admin,secret=***stripped***,mds_namespace=abc"},
		},
		{
			name: "prefix longer than the secret",
			args: []string{"-o", "mon_addr=10.0.0.1:6789/10.0.0.2:6789,secret=AQtiny,mds_namespace=abc"},
			want: []string{"-o", "mon_addr=10.0.0.1:6789/10.0.0.2:6789,secret=***stripped***,mds_namespace=abc"},
		},
		{
			name: "secret is the last option",
			args: []string{"-o", "name=admin,secret=AQBsecretkey"},
			want: []string{"-o", "name=admin,secret=***stripped***"},
		},
		{
			name: "no secret present",
			args: []string{"-o", "name=admin,mds_namespace=abc"},
			want: []string{"-o", "name=admin,mds_namespace=abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := InArgs(tt.args)
			if !slices.Equal(got, tt.want) {
				t.Errorf("InArgs()\n got: %q\nwant: %q", got, tt.want)
			}
			if joined := strings.Join(got, " "); strings.Contains(joined, "AQ") {
				t.Errorf("secret material leaked into stripped output: %q", joined)
			}
		})
	}
}

func TestInArgsStripsKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "--key is stripped",
			args: []string{"rbd", "map", "--key=AQBsecretkey", "--pool=rbd"},
			want: []string{"rbd", "map", "--key=***stripped***", "--pool=rbd"},
		},
		{
			name: "--keyfile is stripped",
			args: []string{"rbd", "map", "--keyfile=/tmp/csi/keys/keyfile-abc", "--pool=rbd"},
			want: []string{"rbd", "map", "--keyfile=***stripped***", "--pool=rbd"},
		},
		{
			name: "only one of key or secret is stripped, as documented",
			args: []string{"--key=AQBsecretkey", "-o", "name=admin,secret=AQBsecretkey"},
			want: []string{"--key=***stripped***", "-o", "name=admin,secret=AQBsecretkey"},
		},
		{
			name: "neither key nor keyfile present",
			args: []string{"rbd", "map", "--pool=rbd"},
			want: []string{"rbd", "map", "--pool=rbd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := InArgs(tt.args)
			if !slices.Equal(got, tt.want) {
				t.Errorf("InArgs()\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestInArgsLeavesInputUnchanged(t *testing.T) {
	t.Parallel()

	args := []string{"-o", "name=admin,secret=AQBsecretkey"}
	original := slices.Clone(args)

	InArgs(args)

	if !slices.Equal(args, original) {
		t.Errorf("InArgs() modified its input: got %q, want %q", args, original)
	}
}
