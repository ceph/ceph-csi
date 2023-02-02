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

func Test_getCrushLocationMap(t *testing.T) {
	t.Parallel()
	type input struct {
		crushLocationLabels string
		nodeLabels          map[string]string
	}
	tests := []struct {
		name string
		args input
		want map[string]string
	}{
		{
			name: "empty crushLocationLabels",
			args: input{
				crushLocationLabels: "",
				nodeLabels:          map[string]string{},
			},
			want: nil,
		},
		{
			name: "empty nodeLabels",
			args: input{
				crushLocationLabels: "topology.io/zone,topology.io/rack",
				nodeLabels:          map[string]string{},
			},
			want: nil,
		},
		{
			name: "matching crushlocation and node labels",
			args: input{
				crushLocationLabels: "topology.io/zone,topology.io/rack",
				nodeLabels: map[string]string{
					"topology.io/zone": "zone1",
				},
			},
			want: map[string]string{"zone": "zone1"},
		},
		{
			name: "multuple matching crushlocation and node labels",
			args: input{
				crushLocationLabels: "topology.io/zone,topology.io/rack",
				nodeLabels: map[string]string{
					"topology.io/zone": "zone1",
					"topology.io/rack": "rack1",
				},
			},
			want: map[string]string{"zone": "zone1", "rack": "rack1"},
		},
		{
			name: "no match between crushlocation and node labels",
			args: input{
				crushLocationLabels: "topology.io/zone,topology.io/rack",
				nodeLabels: map[string]string{
					"topology.io/region": "region1",
				},
			},
			want: nil,
		},
		{
			name: "check crushlocation value replacement to satisfy ceph requirement",
			args: input{
				crushLocationLabels: "topology.io/zone,topology.io/rack",
				nodeLabels: map[string]string{
					"topology.io/zone": "south.east.1",
				},
			},
			want: map[string]string{"zone": "south-east-1"},
		},
		{
			name: "hostname key should be replaced with host",
			args: input{
				crushLocationLabels: "topology.io/zone,topology.io/hostname",
				nodeLabels: map[string]string{
					"topology.io/hostname": "worker-1",
				},
			},
			want: map[string]string{"host": "worker-1"},
		},
	}
	for _, tt := range tests {
		currentTT := tt
		t.Run(currentTT.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t,
				currentTT.want,
				getCrushLocationMap(currentTT.args.crushLocationLabels, currentTT.args.nodeLabels))
		})
	}
}
