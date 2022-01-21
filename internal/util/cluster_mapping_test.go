/*
Copyright 2021 The Ceph-CSI Authors.

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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestGetClusterMappingInfo(t *testing.T) {
	t.Parallel()
	mappingBasePath := t.TempDir()

	// clusterID,poolID on site1
	clusterIDOfSite1 := "site1-storage"
	rbdPoolIDOfSite1 := "1"
	cephfsFscIDOfSite1 := "11"

	// clusterID,poolID on site2
	clusterIDOfSite2 := "site2-storage"
	rbdPoolIDOfSite2 := "3"
	cephfsFscIDOfSite2 := "5"

	// clusterID,poolID on site3
	clusterIDOfSite3 := "site3-storage"
	rbdPoolIDOfSite3 := "8"
	cephfsFscIDOfSite3 := "10"

	clusterMappingInfos := make([]ClusterMappingInfo, 2)

	// create mapping between site1 and site2
	clusterIDmappingOfSite1To2 := make(map[string]string)
	clusterIDmappingOfSite1To2[clusterIDOfSite1] = clusterIDOfSite2
	rbdMappingOfSite1To2 := make([]map[string]string, 1)
	rbdMappingOfSite1To2[0] = map[string]string{rbdPoolIDOfSite1: rbdPoolIDOfSite2}
	cephFSMappingOfSite1To2 := make([]map[string]string, 1)
	cephFSMappingOfSite1To2[0] = map[string]string{cephfsFscIDOfSite1: cephfsFscIDOfSite2}
	mappingOfSite1To2 := ClusterMappingInfo{
		clusterIDmappingOfSite1To2,
		rbdMappingOfSite1To2,
		cephFSMappingOfSite1To2,
	}

	// create mapping between site3 and site2
	clusterIDmappingOfSite3To2 := make(map[string]string)
	clusterIDmappingOfSite3To2[clusterIDOfSite3] = clusterIDOfSite2
	rbdMappingOfSite3To2 := make([]map[string]string, 1)
	rbdMappingOfSite3To2[0] = map[string]string{rbdPoolIDOfSite3: rbdPoolIDOfSite2}
	cephFSMappingOfSite3To2 := make([]map[string]string, 1)
	cephFSMappingOfSite3To2[0] = map[string]string{cephfsFscIDOfSite3: cephfsFscIDOfSite2}
	mappingOfSite3To2 := ClusterMappingInfo{
		clusterIDmappingOfSite3To2,
		rbdMappingOfSite3To2,
		cephFSMappingOfSite3To2,
	}

	clusterMappingInfos[0] = mappingOfSite1To2
	clusterMappingInfos[1] = mappingOfSite3To2

	mappingFileContent, err := json.Marshal(clusterMappingInfos)
	if err != nil {
		t.Errorf("failed to marshal mapping info %v", err)
	}
	// expected output of mapping
	expectedSite2Data := clusterMappingInfos
	expectedSite1To2Data := clusterMappingInfos[:1]
	expectedSite3To2Data := clusterMappingInfos[1:]

	tests := []struct {
		name               string
		clusterID          string
		mappingFilecontent []byte
		expectedData       *[]ClusterMappingInfo
		expectErr          bool
	}{
		{
			name:               "mapping file not found",
			clusterID:          "site-a-clusterid",
			mappingFilecontent: []byte{},
			expectedData:       nil,
			expectErr:          false,
		},
		{
			name:               "mapping file found with empty data",
			clusterID:          "site-a-clusterid",
			mappingFilecontent: []byte{},
			expectedData:       nil,
			expectErr:          false,
		},
		{
			name:               "cluster-id mapping not found",
			clusterID:          "site-a-clusterid",
			mappingFilecontent: mappingFileContent,
			expectedData:       nil,
			expectErr:          false,
		},
		{
			name:               "site2-storage cluster-id mapping",
			clusterID:          clusterIDOfSite2,
			mappingFilecontent: mappingFileContent,
			expectedData:       &expectedSite2Data,
			expectErr:          false,
		},
		{
			name:               "site1-storage cluster-id mapping",
			clusterID:          clusterIDOfSite1,
			mappingFilecontent: mappingFileContent,
			expectedData:       &expectedSite1To2Data,
			expectErr:          false,
		},
		{
			name:               "site3-storage cluster-id mapping",
			clusterID:          clusterIDOfSite3,
			mappingFilecontent: mappingFileContent,
			expectedData:       &expectedSite3To2Data,
			expectErr:          false,
		},
	}
	for i, tt := range tests {
		currentI := i
		currentTT := tt
		t.Run(currentTT.name, func(t *testing.T) {
			t.Parallel()
			mappingConfigFile := fmt.Sprintf("%s/mapping-%d.json", mappingBasePath, currentI)
			if len(currentTT.mappingFilecontent) != 0 {
				err = os.WriteFile(mappingConfigFile, currentTT.mappingFilecontent, 0o600)
				if err != nil {
					t.Errorf("failed to write to %q, error = %v", mappingConfigFile, err)
				}
			}
			data, mErr := getClusterMappingInfo(currentTT.clusterID, mappingConfigFile)
			if (mErr != nil) != currentTT.expectErr {
				t.Errorf("getClusterMappingInfo() error = %v, expected Error %v", mErr, currentTT.expectErr)
			}
			if !reflect.DeepEqual(data, currentTT.expectedData) {
				t.Errorf("getClusterMappingInfo() = %v, expected data %v", data, currentTT.expectedData)
			}
		})
	}

	clusterMappingConfigFile = fmt.Sprintf("%s/mapping.json", mappingBasePath)
	err = os.WriteFile(clusterMappingConfigFile, mappingFileContent, 0o600)
	if err != nil {
		t.Errorf("failed to write mapping content error = %v", err)
	}

	// validate site-3 to site-2 and site-1 to site-2 mappings when failover to
	// site-2.
	// The volumeId's from site-1 looks like `0001-0013-site1-storage-xyz` we
	// need to have validate `site1-storage` to `site2-storage` mapping exists
	// The volumeId's from site-3 looks like `0001-0013-site3-storage-xyz` we
	// need to have validate `site3-storage` to `site2-storage` mapping exists
	mappedClusterCount := 2
	err = validateMapping(t, clusterIDOfSite2, rbdPoolIDOfSite2, cephfsFscIDOfSite2, mappedClusterCount)
	if err != nil {
		t.Error(err)
	}
	// validate site-2 and site-1 mappings when failback to site-1.
	// The volumeId's from site-2 looks like `0001-0013-site2-storage-xyz` we
	// need to have validate `site2-storage` to `site3-storage` mapping exists
	mappedClusterCount = 1
	err = validateMapping(t, clusterIDOfSite1, rbdPoolIDOfSite1, cephfsFscIDOfSite1, mappedClusterCount)
	if err != nil {
		t.Error(err)
	}
	// validate site-2 and site-3 mappings when failback to site-3
	// The volumeId's from site-2 looks like `0001-0013-site2-storage-xyz` we
	// need to have validate `site2-storage` to `site3-storage` mapping exists
	mappedClusterCount = 1
	err = validateMapping(t, clusterIDOfSite3, rbdPoolIDOfSite3, cephfsFscIDOfSite3, mappedClusterCount)
	if err != nil {
		t.Error(err)
	}
}

func validateMapping(t *testing.T, clusterID, rbdPoolID, cephFSPoolID string, mappingCount int) error {
	t.Helper()

	mapping, err := GetClusterMappingInfo(clusterID)
	if err != nil {
		return fmt.Errorf("failed to retrieve mapping %w", err)
	}
	// verify we are able to retrieve both site-1:site2 and site-2:site3 mapping
	if mapping == nil || len(*mapping) != mappingCount {
		return fmt.Errorf(
			"clusterID mapping got length=%v, expected length=%v",
			len(*mapping),
			mappingCount)
	}
	// check mapping rbd pool mapping exists in mapping
	foundRBDPoolMappingCount := 0
	foundCephFSPoolMappingCount := 0
	for _, c := range *mapping {
		for _, rp := range c.RBDpoolIDMappingInfo {
			for k, v := range rp {
				if k == rbdPoolID || v == rbdPoolID {
					foundRBDPoolMappingCount++
				}
			}
		}

		for _, cp := range c.CephFSFscIDMappingInfo {
			for k, v := range cp {
				if k == cephFSPoolID || v == cephFSPoolID {
					foundCephFSPoolMappingCount++
				}
			}
		}
	}
	if foundRBDPoolMappingCount != mappingCount {
		return fmt.Errorf(
			"rbd pool mapping got length= %v, expected length=%v",
			foundRBDPoolMappingCount,
			mappingCount)
	}
	if foundCephFSPoolMappingCount != mappingCount {
		return fmt.Errorf(
			"cephFS filesystem mapping got length= %v, expected length=%v",
			foundCephFSPoolMappingCount,
			mappingCount)
	}

	return nil
}

func TestGetMappedID(t *testing.T) {
	t.Parallel()
	type args struct {
		key   string
		value string
		id    string
	}
	tests := []struct {
		name     string
		args     args
		expected string
	}{
		{
			name: "test for matching key",
			args: args{
				key:   "cluster1",
				value: "cluster2",
				id:    "cluster1",
			},
			expected: "cluster2",
		},
		{
			name: "test for matching value",
			args: args{
				key:   "cluster1",
				value: "cluster2",
				id:    "cluster2",
			},
			expected: "cluster1",
		},
		{
			name: "test for invalid match",
			args: args{
				key:   "cluster1",
				value: "cluster2",
				id:    "cluster3",
			},
			expected: "",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val := GetMappedID(tt.args.key, tt.args.value, tt.args.id)
			if val != tt.expected {
				t.Errorf("getMappedID() got = %v, expected %v", val, tt.expected)
			}
		})
	}
}

func TestFetchMappedClusterIDAndMons(t *testing.T) {
	t.Parallel()
	ctx := context.TODO()
	type args struct {
		ctx       context.Context
		clusterID string
	}
	mappingBasePath := t.TempDir()
	csiConfigFile := mappingBasePath + "/config.json"
	clusterMappingConfigFile := mappingBasePath + "/cluster-mapping.json"
	csiConfig := []ClusterInfo{
		{
			ClusterID: "cluster-1",
			Monitors:  []string{"ip-1", "ip-2"},
		},
		{
			ClusterID: "cluster-2",
			Monitors:  []string{"ip-3", "ip-4"},
		},
	}
	csiConfigFileContent, err := json.Marshal(csiConfig)
	if err != nil {
		t.Errorf("failed to marshal csi config info %v", err)
	}
	err = os.WriteFile(csiConfigFile, csiConfigFileContent, 0o600)
	if err != nil {
		t.Errorf("failed to write %s file content: %v", CsiConfigFile, err)
	}

	t.Run("cluster-mapping.json does not exist", func(t *testing.T) {
		_, _, err = fetchMappedClusterIDAndMons(ctx, "cluster-2", clusterMappingConfigFile, csiConfigFile)
		if err != nil {
			t.Errorf("FetchMappedClusterIDAndMons() error = %v, wantErr %v", err, nil)
		}
	})

	clusterMapping := []ClusterMappingInfo{
		{
			ClusterIDMapping: map[string]string{
				"cluster-1": "cluster-3",
			},
		},
		{
			ClusterIDMapping: map[string]string{
				"cluster-1": "cluster-4",
			},
		},
		{
			ClusterIDMapping: map[string]string{
				"cluster-4": "cluster-3",
			},
		},
	}
	clusterMappingFileContent, err := json.Marshal(clusterMapping)
	if err != nil {
		t.Errorf("failed to marshal mapping info %v", err)
	}
	err = os.WriteFile(clusterMappingConfigFile, clusterMappingFileContent, 0o600)
	if err != nil {
		t.Errorf("failed to write %s file content: %v", clusterMappingFileContent, err)
	}

	tests := []struct {
		name    string
		args    args
		want    string
		want1   string
		wantErr bool
	}{
		{
			name: "test cluster id=cluster-1",
			args: args{
				ctx:       ctx,
				clusterID: "cluster-1",
			},
			want:    strings.Join(csiConfig[0].Monitors, ","),
			want1:   "cluster-1",
			wantErr: false,
		},
		{
			name: "test cluster id=cluster-3",
			args: args{
				ctx:       ctx,
				clusterID: "cluster-3",
			},
			want:    strings.Join(csiConfig[0].Monitors, ","),
			want1:   "cluster-1",
			wantErr: false,
		},
		{
			name: "test cluster id=cluster-4",
			args: args{
				ctx:       ctx,
				clusterID: "cluster-4",
			},
			want:    strings.Join(csiConfig[0].Monitors, ","),
			want1:   "cluster-1",
			wantErr: false,
		},
		{
			name: "test missing cluster id=cluster-6",
			args: args{
				ctx:       ctx,
				clusterID: "cluster-6",
			},
			want:    "",
			want1:   "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, got1, err := fetchMappedClusterIDAndMons(ctx, tt.args.clusterID, clusterMappingConfigFile, csiConfigFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchMappedClusterIDAndMons() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if got != tt.want {
				t.Errorf("FetchMappedClusterIDAndMons() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("FetchMappedClusterIDAndMons() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
