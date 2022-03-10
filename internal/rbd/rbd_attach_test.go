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

package rbd

import (
	"strings"
	"testing"
)

func TestParseMapOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		mapOption         string
		expectKrbdOptions string
		expectNbdOptions  string
		expectErr         string
	}{
		{
			name:              "with old format",
			mapOption:         "kOp1,kOp2",
			expectKrbdOptions: "kOp1,kOp2",
			expectNbdOptions:  "",
			expectErr:         "",
		},
		{
			name:              "with new format",
			mapOption:         "krbd:kOp1,kOp2;nbd:nOp1,nOp2",
			expectKrbdOptions: "kOp1,kOp2",
			expectNbdOptions:  "nOp1,nOp2",
			expectErr:         "",
		},
		{
			name:              "without krbd: label",
			mapOption:         "kOp1,kOp2;nbd:nOp1,nOp2",
			expectKrbdOptions: "kOp1,kOp2",
			expectNbdOptions:  "nOp1,nOp2",
			expectErr:         "",
		},
		{
			name:              "with only nbd label",
			mapOption:         "nbd:nOp1,nOp2",
			expectKrbdOptions: "",
			expectNbdOptions:  "nOp1,nOp2",
			expectErr:         "",
		},
		{
			name:              "with `:` delimiter used with in the options",
			mapOption:         "krbd:kOp1,kOp2=kOp21:kOp22;nbd:nOp1,nOp2=nOp21:nOp22",
			expectKrbdOptions: "kOp1,kOp2=kOp21:kOp22",
			expectNbdOptions:  "nOp1,nOp2=nOp21:nOp22",
			expectErr:         "",
		},
		{
			name:              "with `:` delimiter used with in the options, without mounter label",
			mapOption:         "kOp1,kOp2=kOp21:kOp22;nbd:nOp1,nOp2",
			expectKrbdOptions: "",
			expectNbdOptions:  "",
			expectErr:         "unknown mounter type",
		},
		{
			name:              "unknown mounter used",
			mapOption:         "xyz:xOp1,xOp2",
			expectKrbdOptions: "",
			expectNbdOptions:  "",
			expectErr:         "unknown mounter type",
		},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			krbdOpts, nbdOpts, err := parseMapOptions(tc.mapOption)
			if err != nil && !strings.Contains(err.Error(), tc.expectErr) {
				// returned error
				t.Errorf("parseMapOptions(%s) returned error, expected: %v, got: %v",
					tc.mapOption, tc.expectErr, err)
			}
			if krbdOpts != tc.expectKrbdOptions {
				// unexpected krbd option error
				t.Errorf("parseMapOptions(%s) returned unexpected krbd options, expected :%q, got: %q",
					tc.mapOption, tc.expectKrbdOptions, krbdOpts)
			}
			if nbdOpts != tc.expectNbdOptions {
				// unexpected nbd option error
				t.Errorf("parseMapOptions(%s) returned unexpected nbd options, expected: %q, got: %q",
					tc.mapOption, tc.expectNbdOptions, nbdOpts)
			}
		})
	}
}
