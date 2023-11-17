/*
Copyright 2020 The Ceph-CSI Authors.

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
	"context"
	"encoding/json"
	"os"
	"testing"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
)

func TestGetStagingPath(t *testing.T) {
	t.Parallel()
	var stagingPath string
	// test with nodestagevolumerequest
	nsvr := &csi.NodeStageVolumeRequest{
		VolumeId:          "758978be-6331-4925-b25e-e490fe99c9eb",
		StagingTargetPath: "/path/to/stage",
	}

	expect := "/path/to/stage/758978be-6331-4925-b25e-e490fe99c9eb"
	stagingPath = getStagingTargetPath(nsvr)
	if stagingPath != expect {
		t.Errorf("getStagingTargetPath() = %s, got %s", stagingPath, expect)
	}

	// test with nodestagevolumerequest
	nuvr := &csi.NodeUnstageVolumeRequest{
		VolumeId:          "622cfdeb-69bf-4de6-9bd7-5fa0b71a603e",
		StagingTargetPath: "/path/to/unstage",
	}

	expect = "/path/to/unstage/622cfdeb-69bf-4de6-9bd7-5fa0b71a603e"
	stagingPath = getStagingTargetPath(nuvr)
	if stagingPath != expect {
		t.Errorf("getStagingTargetPath() = %s, got %s", stagingPath, expect)
	}

	// test with non-handled interface
	expect = ""
	stagingPath = getStagingTargetPath("")
	if stagingPath != expect {
		t.Errorf("getStagingTargetPath() = %s, got %s", stagingPath, expect)
	}
}

func TestParseBoolOption(t *testing.T) {
	t.Parallel()
	ctx := context.TODO()
	optionName := "myOption"
	defaultValue := false

	tests := []struct {
		name         string
		scParameters map[string]string
		expect       bool
	}{
		{
			name:         "myOption => true",
			scParameters: map[string]string{optionName: "true"},
			expect:       true,
		},
		{
			name:         "myOption => false",
			scParameters: map[string]string{optionName: "false"},
			expect:       false,
		},
		{
			name:         "myOption => empty",
			scParameters: map[string]string{optionName: ""},
			expect:       defaultValue,
		},
		{
			name:         "myOption => not-parsable",
			scParameters: map[string]string{optionName: "non-boolean"},
			expect:       defaultValue,
		},
		{
			name:         "myOption => not-set",
			scParameters: map[string]string{},
			expect:       defaultValue,
		},
	}

	for _, tt := range tests {
		tc := tt
		val := parseBoolOption(ctx, tc.scParameters, optionName, defaultValue)
		if val != tc.expect {
			t.Errorf("parseBoolOption(%v) returned: %t, expected: %t",
				tc.scParameters, val, tc.expect)
		}
	}
}

func TestNodeServer_appendReadAffinityMapOptions(t *testing.T) {
	t.Parallel()
	type input struct {
		mapOptions, readAffinityMapOptions, mounter string
	}
	tests := []struct {
		name string
		args input
		want string
	}{
		{
			name: "both empty mapOptions and crushLocationMap",
			args: input{
				mapOptions:             "",
				readAffinityMapOptions: "",
				mounter:                rbdDefaultMounter,
			},
			want: "",
		},
		{
			name: "empty mapOptions, filled crushLocationMap & default mounter",
			args: input{
				mapOptions:             "",
				readAffinityMapOptions: "read_from_replica=localize,crush_location=region:west",
				mounter:                rbdDefaultMounter,
			},
			want: "read_from_replica=localize,crush_location=region:west",
		},
		{
			name: "empty mapOptions, filled crushLocationMap & non-default mounter",
			args: input{
				mapOptions:             "",
				readAffinityMapOptions: "read_from_replica=localize,crush_location=region:west",
				mounter:                rbdNbdMounter,
			},
			want: "",
		},
		{
			name: "filled mapOptions, filled crushLocationMap & default mounter",
			args: input{
				mapOptions:             "notrim",
				readAffinityMapOptions: "read_from_replica=localize,crush_location=region:west",
				mounter:                rbdDefaultMounter,
			},
			want: "notrim,read_from_replica=localize,crush_location=region:west",
		},
		{
			name: "filled mapOptions, filled crushLocationMap & non-default mounter",
			args: input{
				mapOptions:             "notrim",
				readAffinityMapOptions: "read_from_replica=localize,crush_location=region:west",
				mounter:                rbdNbdMounter,
			},
			want: "notrim",
		},
		{
			name: "filled mapOptions, empty readAffinityMapOptions & default mounter",
			args: input{
				mapOptions:             "notrim",
				readAffinityMapOptions: "",
				mounter:                rbdDefaultMounter,
			},
			want: "notrim",
		},
		{
			name: "filled mapOptions, empty readAffinityMapOptions & non-default mounter",
			args: input{
				mapOptions:             "notrim",
				readAffinityMapOptions: "",
				mounter:                rbdNbdMounter,
			},
			want: "notrim",
		},
	}
	for _, tt := range tests {
		currentTT := tt
		t.Run(currentTT.name, func(t *testing.T) {
			t.Parallel()
			rv := &rbdVolume{
				MapOptions: currentTT.args.mapOptions,
				Mounter:    currentTT.args.mounter,
			}
			rv.appendReadAffinityMapOptions(currentTT.args.readAffinityMapOptions)
			assert.Equal(t, currentTT.want, rv.MapOptions)
		})
	}
}

func TestReadAffinity_GetReadAffinityMapOptions(t *testing.T) {
	t.Parallel()

	nodeLabels := map[string]string{
		"topology.kubernetes.io/zone":   "east-1",
		"topology.kubernetes.io/region": "east",
	}
	topology := map[string]string{}

	csiConfig := []util.ClusterInfo{
		{
			ClusterID: "cluster-1",
			ReadAffinity: struct {
				Enabled             bool     `json:"enabled"`
				CrushLocationLabels []string `json:"crushLocationLabels"`
			}{
				Enabled: true,
				CrushLocationLabels: []string{
					"topology.kubernetes.io/region",
				},
			},
		},
		{
			ClusterID: "cluster-2",
			ReadAffinity: struct {
				Enabled             bool     `json:"enabled"`
				CrushLocationLabels []string `json:"crushLocationLabels"`
			}{
				Enabled: false,
				CrushLocationLabels: []string{
					"topology.kubernetes.io/region",
				},
			},
		},
		{
			ClusterID: "cluster-3",
			ReadAffinity: struct {
				Enabled             bool     `json:"enabled"`
				CrushLocationLabels []string `json:"crushLocationLabels"`
			}{
				Enabled:             true,
				CrushLocationLabels: []string{},
			},
		},
		{
			ClusterID: "cluster-4",
		},
	}

	csiConfigFileContent, err := json.Marshal(csiConfig)
	if err != nil {
		t.Errorf("failed to marshal csi config info %v", err)
	}
	tmpConfPath := util.CsiConfigFile
	err = os.Mkdir("/etc/ceph-csi-config", 0o600)
	if err != nil {
		t.Errorf("failed to create directory %s: %v", "/etc/ceph-csi-config", err)
	}
	err = os.WriteFile(tmpConfPath, csiConfigFileContent, 0o600)
	if err != nil {
		t.Errorf("failed to write %s file content: %v", util.CsiConfigFile, err)
	}

	tests := []struct {
		name                   string
		clusterID              string
		CLICrushLocationLabels string
		want                   string
	}{
		{
			name:                   "Enabled in cluster-1 and Enabled in CLI",
			clusterID:              "cluster-1",
			CLICrushLocationLabels: "topology.kubernetes.io/region",
			want:                   "read_from_replica=localize,crush_location=region:east",
		},
		{
			name:                   "Disabled in cluster-2 and Enabled in CLI",
			clusterID:              "cluster-2",
			CLICrushLocationLabels: "topology.kubernetes.io/zone",
			want:                   "",
		},
		{
			name:                   "Enabled in cluster-3 with empty crush labels and Enabled in CLI",
			clusterID:              "cluster-3",
			CLICrushLocationLabels: "topology.kubernetes.io/zone",
			want:                   "read_from_replica=localize,crush_location=zone:east-1",
		},
		{
			name:                   "Enabled in cluster-3 with empty crush labels and Disabled in CLI",
			clusterID:              "cluster-3",
			CLICrushLocationLabels: "",
			want:                   "",
		},
		{
			name:                   "Absent in cluster-4 and Enabled in CLI",
			clusterID:              "cluster-4",
			CLICrushLocationLabels: "topology.kubernetes.io/zone",
			want:                   "",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			crushLocationMap := util.GetCrushLocationMap(tc.CLICrushLocationLabels, nodeLabels)
			cliReadAffinityMapOptions := util.ConstructReadAffinityMapOption(crushLocationMap)
			driver := &csicommon.CSIDriver{}

			ns := &NodeServer{
				DefaultNodeServer: csicommon.NewDefaultNodeServer(
					driver, "rbd", cliReadAffinityMapOptions, topology, nodeLabels,
				),
			}
			readAffinityMapOptions, err := util.GetReadAffinityMapOptions(
				tmpConfPath, tc.clusterID, ns.CLIReadAffinityOptions, nodeLabels,
			)
			if err != nil {
				assert.Fail(t, err.Error())
			}

			assert.Equal(t, tc.want, readAffinityMapOptions)
		})
	}
}
