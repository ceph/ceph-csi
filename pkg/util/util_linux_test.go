// +build linux

/*
Copyright 2019 The Kubernetes Authors.
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
	"fmt"
	"os"
	"testing"
)

func TestCleanPath(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test_2",
			args: args{
				path: fmt.Sprintf("%s/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38/mount", os.TempDir()),
			},
			want: fmt.Sprintf("%s/pvc-9ae9405c-7fb3-11ea-80b3-246e968d4b38", os.TempDir()),
		}, {
			name: "test_4",
			args: args{
				path: fmt.Sprintf("%s/pvc-8ae9405c-7fb3-11ea-80b3-246e968d4b38/globalmount", os.TempDir()),
			},
			want: fmt.Sprintf("%s/pvc-8ae9405c-7fb3-11ea-80b3-246e968d4b38", os.TempDir()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.MkdirAll(tt.args.path, 0777)
			if err != nil {
				t.Errorf("create a path[%s] for preparing test enviroment fail, err: %v", tt.args.path, err)
			}

			if _, err := os.Stat(tt.args.path); err != nil {
				t.Errorf("preparing test enviroment fail, the path[%s] not exist err: %v", tt.args.path, err)
			}

			if err := CleanPath(tt.args.path); err != nil {
				t.Errorf("CleanPath fail, path:%s, want %v", tt.args.path, tt.want)
			}

			if _, err := os.Stat(tt.want); err != nil {
				if !os.IsNotExist(err) {
					t.Errorf("CleanPath fail, and the expected path[%s] still exist, err: %v", tt.want, err)
				}
			}
		})
	}
}
