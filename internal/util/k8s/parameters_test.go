/*
Copyright 2022 The Ceph-CSI Authors.

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
package k8s

import (
	"reflect"
	"testing"
)

func TestRemoveCSIPrefixedParameters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		param map[string]string
		want  map[string]string
	}{
		{
			name: "without csi.storage.k8s.io prefix",
			param: map[string]string{
				"foo": "bar",
			},
			want: map[string]string{
				"foo": "bar",
			},
		},
		{
			name: "with csi.storage.k8s.io prefix",
			param: map[string]string{
				"foo":                              "bar",
				"csi.storage.k8s.io/pvc/name":      "foo",
				"csi.storage.k8s.io/pvc/namespace": "bar",
				"csi.storage.k8s.io/pv/name":       "baz",
			},
			want: map[string]string{
				"foo": "bar",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RemoveCSIPrefixedParameters(tt.param)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RemoveCSIPrefixedParameters() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetOwner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]string
		want string
	}{
		{
			name: "namespace is not present in the parameters",
			args: map[string]string{
				"foo": "bar",
			},
			want: "",
		},
		{
			name: "namespace is present in the parameters",
			args: map[string]string{
				"csi.storage.k8s.io/pvc/namespace": "bar",
			},
			want: "bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := GetOwner(tt.args); got != tt.want {
				t.Errorf("GetOwner() = %v, want %v", got, tt.want)
			}
		})
	}
}
