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

package rbd

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/ceph/go-ceph/rados"
	librbd "github.com/ceph/go-ceph/rbd"
)

func TestEncodeClsMetadataSet(t *testing.T) {
	t.Parallel()

	want := []byte{
		1, 0, 0, 0, // count
		3, 0, 0, 0, 'k', 'e', 'y',
		2, 0, 0, 0, 'v', 'v',
	}
	if got := encodeClsMetadataSet(map[string]string{"key": "vv"}); !bytes.Equal(got, want) {
		t.Errorf("encodeClsMetadataSet() = %v, want %v", got, want)
	}
}

func TestEncodeClsMetadataSetMultiple(t *testing.T) {
	t.Parallel()

	want := []byte{
		2, 0, 0, 0, // count
		1, 0, 0, 0, 'a', 1, 0, 0, 0, '1',
		1, 0, 0, 0, 'b', 0, 0, 0, 0,
	}
	if got := encodeClsMetadataSet(map[string]string{"b": "", "a": "1"}); !bytes.Equal(got, want) {
		t.Errorf("encodeClsMetadataSet() = %v, want %v", got, want)
	}
}

func TestEncodeClsMetadataRemove(t *testing.T) {
	t.Parallel()

	want := []byte{3, 0, 0, 0, 'k', 'e', 'y'}
	if got := encodeClsMetadataRemove("key"); !bytes.Equal(got, want) {
		t.Errorf("encodeClsMetadataRemove() = %v, want %v", got, want)
	}
	if got := encodeClsMetadataRemove(""); !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Errorf("encodeClsMetadataRemove(\"\") = %v", got)
	}
}

func TestIsClsNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "rados ENOENT", err: rados.ErrNotFound, want: true},
		{name: "wrapped rados ENOENT", err: fmt.Errorf("op: %w", rados.ErrNotFound), want: true},
		{name: "librbd ENOENT", err: librbd.ErrNotFound, want: true},
		{name: "wrapped librbd ENOENT", err: fmt.Errorf("op: %w", librbd.ErrNotFound), want: true},
		{name: "other error", err: errors.New("op failed"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isClsNotFound(tt.err); got != tt.want {
				t.Errorf("isClsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
