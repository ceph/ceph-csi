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

package e2e

import (
	"math"
	"testing"
)

func TestParseCephCSIVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input        string
		wantMajor    int
		wantMinor    int
		wantErr      bool
	}{
		{input: "canary", wantMajor: math.MaxInt, wantMinor: math.MaxInt},
		{input: "v3.16-canary", wantMajor: 3, wantMinor: 16},
		{input: "v3.17-canary", wantMajor: 3, wantMinor: 17},
		{input: "v3.16.8", wantMajor: 3, wantMinor: 16},
		{input: "v3.16.0", wantMajor: 3, wantMinor: 16},
		{input: "invalid", wantErr: true},
		{input: "v3", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			major, minor, err := parseCephCSIVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCephCSIVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)

				return
			}
			if err != nil {
				return
			}
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Errorf("parseCephCSIVersion(%q) = (%d, %d), want (%d, %d)",
					tt.input, major, minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}
