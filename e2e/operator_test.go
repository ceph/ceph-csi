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
	"testing"

	cephcsiv1 "github.com/ceph/ceph-csi-operator/api/v1"
)

func driverSpec(clusterName string, enableFencing *bool, domainLabels []string) *cephcsiv1.DriverSpec {
	spec := &cephcsiv1.DriverSpec{
		ClusterName:   &clusterName,
		EnableFencing: enableFencing,
	}
	if domainLabels != nil {
		spec.NodePlugin = &cephcsiv1.NodePluginSpec{
			Topology: &cephcsiv1.TopologySpec{DomainLabels: domainLabels},
		}
	}

	return spec
}

func boolPtr(b bool) *bool {
	return &b
}

func TestDriverSpecMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		defaults      *cephcsiv1.DriverSpec
		clusterName   string
		enableFencing bool
		domainLabels  []string
		want          bool
	}{
		{
			name:          "nil defaults",
			defaults:      nil,
			clusterName:   "cluster",
			enableFencing: true,
			want:          false,
		},
		{
			name:          "all match",
			defaults:      driverSpec("cluster", boolPtr(true), []string{"topology.kubernetes.io/region"}),
			clusterName:   "cluster",
			enableFencing: true,
			domainLabels:  []string{"topology.kubernetes.io/region"},
			want:          true,
		},
		{
			name:          "no labels requested ignores topology",
			defaults:      driverSpec("cluster", boolPtr(true), []string{"topology.kubernetes.io/region"}),
			clusterName:   "cluster",
			enableFencing: true,
			want:          true,
		},
		{
			name:          "cluster name mismatch",
			defaults:      driverSpec("other", boolPtr(true), nil),
			clusterName:   "cluster",
			enableFencing: true,
			want:          false,
		},
		{
			name:          "fencing mismatch",
			defaults:      driverSpec("cluster", boolPtr(false), nil),
			clusterName:   "cluster",
			enableFencing: true,
			want:          false,
		},
		{
			name:          "fencing unset",
			defaults:      driverSpec("cluster", nil, nil),
			clusterName:   "cluster",
			enableFencing: true,
			want:          false,
		},
		{
			name:          "labels requested but node plugin unset",
			defaults:      driverSpec("cluster", boolPtr(true), nil),
			clusterName:   "cluster",
			enableFencing: true,
			domainLabels:  []string{"topology.kubernetes.io/region"},
			want:          false,
		},
		{
			name:          "labels mismatch",
			defaults:      driverSpec("cluster", boolPtr(true), []string{"topology.kubernetes.io/zone"}),
			clusterName:   "cluster",
			enableFencing: true,
			domainLabels:  []string{"topology.kubernetes.io/region"},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := driverSpecMatches(tt.defaults, tt.clusterName, tt.enableFencing, tt.domainLabels)
			if got != tt.want {
				t.Errorf("driverSpecMatches() = %t, want %t", got, tt.want)
			}
		})
	}
}
