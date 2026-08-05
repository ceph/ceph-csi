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

package rbd

import (
	"slices"
	"testing"
)

func TestValidateStriping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		parameters map[string]string
		wantErr    bool
	}{
		{
			name: "when stripeUnit is not specified",
			parameters: map[string]string{
				"stripeUnit":  "",
				"stripeCount": "10",
				"objectSize":  "2",
			},
			wantErr: true,
		},
		{
			name: "when stripeCount is not specified",
			parameters: map[string]string{
				"stripeUnit":  "4096",
				"stripeCount": "",
				"objectSize":  "2",
			},
			wantErr: true,
		},
		{
			name: "when objectSize is not power of 2",
			parameters: map[string]string{
				"stripeUnit":  "4096",
				"stripeCount": "8",
				"objectSize":  "3",
			},
			wantErr: true,
		},
		{
			name: "when objectSize is 0",
			parameters: map[string]string{
				"stripeUnit":  "4096",
				"stripeCount": "8",
				"objectSize":  "0",
			},
			wantErr: true,
		},
		{
			name: "when valid stripe parameters are specified",
			parameters: map[string]string{
				"stripeUnit":  "4096",
				"stripeCount": "8",
				"objectSize":  "131072",
			},
			wantErr: false,
		},
		{
			name:       "when no stripe parameters are specified",
			parameters: map[string]string{},
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateStriping(tt.parameters); (err != nil) != tt.wantErr {
				t.Errorf("validateStriping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToCSIVolume(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rv      *rbdVolume
		wantErr bool
	}{
		{
			name: "all attributes set",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "0001-unique-volume-id",
					Pool:         "ecpool",
					JournalPool:  "replicapool",
					RbdImageName: "csi-vol-01234-5678-90abc",
				},
			},
			wantErr: false,
		},
		{
			name: "missing volume-id",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "",
					Pool:         "ecpool",
					JournalPool:  "replicapool",
					RbdImageName: "csi-vol-01234-5678-90abc",
				},
			},
			wantErr: true,
		},
		{
			name: "missing pool",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "0001-unique-volume-id",
					Pool:         "",
					JournalPool:  "replicapool",
					RbdImageName: "csi-vol-01234-5678-90abc",
				},
			},
			wantErr: true,
		},
		{
			name: "missing journal-pool",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "0001-unique-volume-id",
					Pool:         "ecpool",
					JournalPool:  "",
					RbdImageName: "csi-vol-01234-5678-90abc",
				},
			},
			wantErr: true,
		},
		{
			name: "missing image-name",
			rv: &rbdVolume{
				rbdImage: rbdImage{
					VolID:        "0001-unique-volume-id",
					Pool:         "ecpool",
					JournalPool:  "",
					RbdImageName: "",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.rv.ToCSI(t.Context()); (err != nil) != tt.wantErr {
				t.Errorf("ToCSI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateQoSParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		params  map[string]string
		mounter string
		wantErr bool
	}{
		{
			name:    "krbd with cgroup params only",
			params:  map[string]string{maxReadIops: "1000", maxWriteBps: "10485760"},
			mounter: rbdDefaultMounter,
			wantErr: false,
		},
		{
			name:    "krbd with NBD-only params",
			params:  map[string]string{baseIops: "3000"},
			mounter: rbdDefaultMounter,
			wantErr: true,
		},
		{
			name:    "krbd with mixed cgroup and NBD params",
			params:  map[string]string{maxReadIops: "1000", baseReadIops: "500"},
			mounter: rbdDefaultMounter,
			wantErr: true,
		},
		{
			name:    "krbd with invalid cgroup param value",
			params:  map[string]string{maxReadIops: "-1"},
			mounter: rbdDefaultMounter,
			wantErr: true,
		},
		{
			name:    "krbd with empty params",
			params:  map[string]string{},
			mounter: rbdDefaultMounter,
			wantErr: true,
		},
		{
			name:    "nbd with NBD params and max limits",
			params:  map[string]string{baseReadIops: "500", maxReadIops: "1000"},
			mounter: rbdNbdMounter,
			wantErr: false,
		},
		{
			name:    "nbd with NBD-only params",
			params:  map[string]string{baseIops: "3000", iopsPerGiB: "100"},
			mounter: rbdNbdMounter,
			wantErr: false,
		},
		{
			name:    "nbd with invalid NBD param value",
			params:  map[string]string{baseIops: "invalid"},
			mounter: rbdNbdMounter,
			wantErr: true,
		},
		{
			name:    "nbd with empty params",
			params:  map[string]string{},
			mounter: rbdNbdMounter,
			wantErr: true,
		},
		{
			name:    "krbd with misspelled cgroup param",
			params:  map[string]string{"readIOPS": "1000"},
			mounter: rbdDefaultMounter,
			wantErr: true,
		},
		{
			name:    "krbd with completely unknown param",
			params:  map[string]string{"fooBar": "123"},
			mounter: rbdDefaultMounter,
			wantErr: true,
		},
		{
			name:    "nbd with misspelled param",
			params:  map[string]string{"readIOPS": "1000"},
			mounter: rbdNbdMounter,
			wantErr: true,
		},
		{
			name:    "nbd with valid and unknown params",
			params:  map[string]string{baseIops: "3000", "unknownKey": "value"},
			mounter: rbdNbdMounter,
			wantErr: true,
		},
		{
			name:    "krbd with valid cgroup and unknown params",
			params:  map[string]string{maxReadIops: "1000", "typoParam": "50"},
			mounter: rbdDefaultMounter,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateQoSParameters(tt.params, tt.mounter)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateQoSParameters() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUnrecognizedKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		params   map[string]string
		known    []string
		expected []string
	}{
		{
			name:     "all keys recognized",
			params:   map[string]string{"a": "1", "b": "2"},
			known:    []string{"a", "b", "c"},
			expected: nil,
		},
		{
			name:     "some keys unrecognized",
			params:   map[string]string{"a": "1", "x": "2", "y": "3"},
			known:    []string{"a", "b"},
			expected: []string{"x", "y"},
		},
		{
			name:     "all keys unrecognized",
			params:   map[string]string{"x": "1", "y": "2"},
			known:    []string{"a", "b"},
			expected: []string{"x", "y"},
		},
		{
			name:     "empty params",
			params:   map[string]string{},
			known:    []string{"a"},
			expected: nil,
		},
		{
			name:     "empty known list",
			params:   map[string]string{"a": "1"},
			known:    []string{},
			expected: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := unrecognizedKeys(tt.params, tt.known)
			slices.Sort(got)
			slices.Sort(tt.expected)
			if len(got) != len(tt.expected) {
				t.Fatalf("unrecognizedKeys() = %v, expected %v", got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("unrecognizedKeys() = %v, expected %v", got, tt.expected)

					break
				}
			}
		})
	}
}
