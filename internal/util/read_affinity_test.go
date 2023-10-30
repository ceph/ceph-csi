/*
Copyright 2023 The Ceph-CSI Authors.

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

	"github.com/stretchr/testify/assert"
)

func TestReadAffinity_ConstructReadAffinityMapOption(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		crushLocationmap map[string]string
		wantAny          []string
	}{
		{
			name:             "nil crushLocationmap",
			crushLocationmap: nil,
			wantAny:          []string{""},
		},
		{
			name:             "empty crushLocationmap",
			crushLocationmap: map[string]string{},
			wantAny:          []string{""},
		},
		{
			name: "single entry in crushLocationmap",
			crushLocationmap: map[string]string{
				"region": "east",
			},
			wantAny: []string{"read_from_replica=localize,crush_location=region:east"},
		},
		{
			name: "multiple entries in crushLocationmap",
			crushLocationmap: map[string]string{
				"region": "east",
				"zone":   "east-1",
			},
			wantAny: []string{
				"read_from_replica=localize,crush_location=region:east|zone:east-1",
				"read_from_replica=localize,crush_location=zone:east-1|region:east",
			},
		},
	}
	for _, tt := range tests {
		currentTT := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Contains(t, currentTT.wantAny, ConstructReadAffinityMapOption(currentTT.crushLocationmap))
		})
	}
}
