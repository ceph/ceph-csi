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
	"reflect"
	"testing"
)

func TestIsMigrationVolID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     string
		migVolID bool
	}{
		{
			"correct volume ID",
			"mig_mons-b7f67366bb43f32e07d8a261a7840da9_image-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c", //nolint:lll // migration volID
			true,
		},
		{
			"Wrong volume ID",
			"wrong_volume_ID",
			false,
		},
		{
			"wrong mons prefixed volume ID",
			"mig_mon-b7f67366bb43f32e07d8a261a7840da9_image-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c", //nolint:lll // migration volID
			false,
		},
		{
			"wrong image prefixed volume ID",
			"mig_imae-e0b45b52-7e09-47d3-8f1b-806995fa4412_pool_replica_pool",
			false,
		},
		{
			"wrong volume ID",
			"mig_image-e0b45b52-7e09-47d3-8f1b-806995fa4412_pool_replica_pool",
			false,
		},
	}
	for _, tt := range tests {
		newtt := tt
		t.Run(newtt.name, func(t *testing.T) {
			t.Parallel()
			got := isMigrationVolID(newtt.args)
			if got != newtt.migVolID {
				t.Errorf("isMigrationVolID() = %v, want %v", got, newtt.migVolID)
			}
		})
	}
}

func TestParseMigrationVolID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    string
		want    *migrationVolID
		wantErr bool
	}{
		{
			"correct volume ID",
			"mig_mons-b7f67366bb43f32e07d8a261a7840da9_image-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c", //nolint:lll // migration volID
			&migrationVolID{
				// monitors:  "10.70.53.126:6789",
				imageName: "kubernetes-dynamic-pvc-e0b45b52-7e09-47d3-8f1b-806995fa4412",
				poolName:  "pool_replica_pool",
				clusterID: "b7f67366bb43f32e07d8a261a7840da9",
			},
			false,
		},
		{
			"volume ID without mons",
			"mig_kubernetes-dynamic-pvc-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c",
			nil,
			true,
		},
		{
			"volume ID without image",
			"mig_pool-706f6f6c5f7265706c6963615f706f6f6c",
			nil,
			true,
		},
		{
			"volume ID without pool",
			"mig",
			nil,
			true,
		},
		{
			"correct volume ID with single mon",
			"mig_mons-7982de6a23b77bce50b1ba9f2e879cce_image-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c", //nolint:lll // migration volID
			&migrationVolID{
				// monitors:  "10.70.53.126:6789,10.70.53.156:6789",
				imageName: "kubernetes-dynamic-pvc-e0b45b52-7e09-47d3-8f1b-806995fa4412",
				poolName:  "pool_replica_pool",
				clusterID: "7982de6a23b77bce50b1ba9f2e879cce",
			},
			false,
		},
		{
			"correct volume ID with more than one mon",
			"mig_mons-7982de6a23b77bce50b1ba9f2e879cce_image-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c", //nolint:lll // migration volID
			&migrationVolID{
				// monitors:  "10.70.53.126:6789,10.70.53.156:6789",
				imageName: "kubernetes-dynamic-pvc-e0b45b52-7e09-47d3-8f1b-806995fa4412",
				poolName:  "pool_replica_pool",
				clusterID: "7982de6a23b77bce50b1ba9f2e879cce",
			},
			false,
		},
		{
			"correct volume ID with '_' pool name",
			"mig_mons-7982de6a23b77bce50b1ba9f2e879cce_image-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c", //nolint:lll // migration volID
			&migrationVolID{
				// monitors:  "10.70.53.126:6789,10.70.53.156:6789",
				imageName: "kubernetes-dynamic-pvc-e0b45b52-7e09-47d3-8f1b-806995fa4412",
				poolName:  "pool_replica_pool",
				clusterID: "7982de6a23b77bce50b1ba9f2e879cce",
			},
			false,
		},
		{
			"volume ID with unallowed migration version string",
			"migrate-beta_mons-b7f67366bb43f32e07d8a261a7840da9_kubernetes-pvc-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c", //nolint:lll // migration volID
			nil,
			true,
		},
		{
			"volume ID with unallowed image name",
			"mig_mons-b7f67366bb43f32e07d8a261a7840da9_kubernetes-pvc-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c", //nolint:lll // migration volID
			nil,
			true,
		},

		{
			"volume ID without 'mon-' prefix string",
			"mig_b7f67366bb43f32e07d8a261a7840da9_kubernetes-pvc-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c", //nolint:lll // migration volID
			nil,
			true,
		},
	}
	for _, tt := range tests {
		newtt := tt
		t.Run(newtt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseMigrationVolID(newtt.args)
			if (err != nil) != newtt.wantErr {
				t.Errorf("ParseMigrationVolID() error = %v, wantErr %v", err, newtt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, newtt.want) {
				t.Errorf("ParseMigrationVolID() got = %v, want %v", got, newtt.want)
			}
		})
	}
}
