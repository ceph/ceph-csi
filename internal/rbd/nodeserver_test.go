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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
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
