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

package kms

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetConfigInt(t *testing.T) {
	t.Parallel()
	type args struct {
		option *int
		config map[string]interface{}
		key    string
	}
	option := 1
	tests := []struct {
		name  string
		args  args
		err   error
		value int
	}{
		{
			name: "valid value",
			args: args{
				option: &option,
				config: map[string]interface{}{
					"a": 1.0,
				},
				key: "a",
			},
			err:   nil,
			value: 1,
		},
		{
			name: "invalid value",
			args: args{
				option: &option,
				config: map[string]interface{}{
					"a": "abc",
				},
				key: "a",
			},
			err:   errConfigOptionInvalid,
			value: 0,
		},
		{
			name: "missing value",
			args: args{
				option: &option,
				config: map[string]interface{}{},
				key:    "a",
			},
			err:   errConfigOptionMissing,
			value: 0,
		},
	}
	for _, tt := range tests {
		currentTT := tt
		t.Run(currentTT.name, func(t *testing.T) {
			t.Parallel()
			err := setConfigInt(currentTT.args.option, currentTT.args.config, currentTT.args.key)
			if !errors.Is(err, currentTT.err) {
				t.Errorf("setConfigInt() error = %v, wantErr %v", err, currentTT.err)
			}
			if err != nil {
				assert.NotEqual(t, currentTT.value, currentTT.args.option)
			}
		})
	}
}
