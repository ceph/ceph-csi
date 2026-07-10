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

package cephfs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateMDSPinParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		params      map[string]string
		wantType    string
		wantSetting string
		wantErr     bool
	}{
		{
			name:        "no pin parameters",
			params:      map[string]string{},
			wantType:    "",
			wantSetting: "",
			wantErr:     false,
		},
		{
			name:    "unrelated parameters only",
			params:  map[string]string{"foo": "bar"},
			wantErr: true,
		},
		{
			name: "unknown parameter mixed with a valid pin",
			params: map[string]string{
				paramMDSPinExport: "1",
				"foo":             "bar",
			},
			wantErr: true,
		},
		{
			name:        "export pin with valid rank",
			params:      map[string]string{paramMDSPinExport: "2"},
			wantType:    "export",
			wantSetting: "2",
			wantErr:     false,
		},
		{
			name:        "export pin with unpin value -1",
			params:      map[string]string{paramMDSPinExport: "-1"},
			wantType:    "export",
			wantSetting: "-1",
			wantErr:     false,
		},
		{
			name:    "export pin with invalid non-integer value",
			params:  map[string]string{paramMDSPinExport: "abc"},
			wantErr: true,
		},
		{
			name:    "export pin with value below -1",
			params:  map[string]string{paramMDSPinExport: "-2"},
			wantErr: true,
		},
		{
			name:        "distributed pin enabled",
			params:      map[string]string{paramMDSPinDistributed: "1"},
			wantType:    "distributed",
			wantSetting: "1",
			wantErr:     false,
		},
		{
			name:        "distributed pin disabled",
			params:      map[string]string{paramMDSPinDistributed: "0"},
			wantType:    "distributed",
			wantSetting: "0",
			wantErr:     false,
		},
		{
			name:    "distributed pin with invalid value",
			params:  map[string]string{paramMDSPinDistributed: "2"},
			wantErr: true,
		},
		{
			name:        "random pin with valid ratio",
			params:      map[string]string{paramMDSPinRandom: "0.5"},
			wantType:    "random",
			wantSetting: "0.5",
			wantErr:     false,
		},
		{
			name:        "random pin at lower bound",
			params:      map[string]string{paramMDSPinRandom: "0.0"},
			wantType:    "random",
			wantSetting: "0.0",
			wantErr:     false,
		},
		{
			name:        "random pin at upper bound",
			params:      map[string]string{paramMDSPinRandom: "1.0"},
			wantType:    "random",
			wantSetting: "1.0",
			wantErr:     false,
		},
		{
			name:    "random pin above upper bound",
			params:  map[string]string{paramMDSPinRandom: "1.5"},
			wantErr: true,
		},
		{
			name:    "random pin with negative ratio",
			params:  map[string]string{paramMDSPinRandom: "-0.1"},
			wantErr: true,
		},
		{
			name:    "random pin with non-float value",
			params:  map[string]string{paramMDSPinRandom: "half"},
			wantErr: true,
		},
		{
			name: "mutually exclusive export and distributed",
			params: map[string]string{
				paramMDSPinExport:      "1",
				paramMDSPinDistributed: "1",
			},
			wantErr: true,
		},
		{
			name: "mutually exclusive export and random",
			params: map[string]string{
				paramMDSPinExport: "1",
				paramMDSPinRandom: "0.5",
			},
			wantErr: true,
		},
		{
			name: "mutually exclusive distributed and random",
			params: map[string]string{
				paramMDSPinDistributed: "1",
				paramMDSPinRandom:      "0.5",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotType, gotSetting, err := validateMDSPinParameters(tt.params)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantType, gotType)
			require.Equal(t, tt.wantSetting, gotSetting)
		})
	}
}

func TestValidateMDSPinSetting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		setting string
		wantErr bool
	}{
		{name: "export valid rank", key: paramMDSPinExport, setting: "3", wantErr: false},
		{name: "export unpin", key: paramMDSPinExport, setting: "-1", wantErr: false},
		{name: "export invalid string", key: paramMDSPinExport, setting: "x", wantErr: true},
		{name: "export below -1", key: paramMDSPinExport, setting: "-5", wantErr: true},
		{name: "distributed enable", key: paramMDSPinDistributed, setting: "1", wantErr: false},
		{name: "distributed disable", key: paramMDSPinDistributed, setting: "0", wantErr: false},
		{name: "distributed invalid", key: paramMDSPinDistributed, setting: "true", wantErr: true},
		{name: "random valid", key: paramMDSPinRandom, setting: "0.25", wantErr: false},
		{name: "random out of range", key: paramMDSPinRandom, setting: "2.0", wantErr: true},
		{name: "random invalid", key: paramMDSPinRandom, setting: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateMDSPinSetting(tt.key, tt.setting)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
		})
	}
}
