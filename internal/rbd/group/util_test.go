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

package group

import (
	"errors"
	"testing"

	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/ceph/go-ceph/rados"
)

func Test_shouldRetryVolumeGroupGeneration(t *testing.T) {
	t.Parallel()
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "No error (stop searching)",
			args: args{err: nil},
			want: false, // No error, stop searching
		},
		{
			name: "ErrPoolNotFound (continue searching)",
			args: args{err: util.ErrPoolNotFound},
			want: true, // Known error, continue searching
		},
		{
			name: "ErrRBDGroupNotFound (continue searching)",
			args: args{err: rbderrors.ErrGroupNotFound},
			want: true, // Known error, continue searching
		},
		{
			name: "ErrPermissionDenied (continue searching)",
			args: args{err: rados.ErrPermissionDenied},
			want: true, // Known error, continue searching
		},
		{
			name: "Different error (stop searching)",
			args: args{err: errors.New("unknown error")},
			want: false, // Unknown error, stop searching
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ShouldRetryVolumeGroupGeneration(tt.args.err); got != tt.want {
				t.Errorf("ShouldRetryVolumeGeneration() = %v, want %v", got, tt.want)
			}
		})
	}
}
