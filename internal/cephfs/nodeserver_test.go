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

package cephfs

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/ceph/ceph-csi/internal/cephfs/mounter"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util"
)

func Test_setMountOptions(t *testing.T) {
	t.Parallel()

	cliKernelMountOptions := "noexec,nodev"
	cliFuseMountOptions := "default_permissions,auto_cache"

	configKernelMountOptions := "crc"
	configFuseMountOptions := "allow_other"

	csiConfig := []util.ClusterInfo{
		{
			ClusterID: "cluster-1",
			CephFS: util.CephFS{
				KernelMountOptions: configKernelMountOptions,
				FuseMountOptions:   configFuseMountOptions,
			},
		},
		{
			ClusterID: "cluster-2",
			CephFS: util.CephFS{
				KernelMountOptions: "",
				FuseMountOptions:   "",
			},
		},
	}

	csiConfigFileContent, err := json.Marshal(csiConfig)
	if err != nil {
		t.Errorf("failed to marshal csi config info %v", err)
	}
	tmpConfPath := t.TempDir() + "/ceph-csi.json"
	t.Logf("path = %s", tmpConfPath)
	err = os.WriteFile(tmpConfPath, csiConfigFileContent, 0o600)
	if err != nil {
		t.Errorf("failed to write %s file content: %v", tmpConfPath, err)
	}

	tests := []struct {
		name       string
		ns         *NodeServer
		mnt        mounter.VolumeMounter
		volOptions *store.VolumeOptions
		want       string
	}{
		{
			name: "KernelMountOptions set in cluster-1 config and not set in CLI",
			ns:   &NodeServer{},
			mnt:  mounter.VolumeMounter(&mounter.KernelMounter{}),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-1",
			},
			want: configKernelMountOptions,
		},
		{
			name: "FuseMountOptions set in cluster-1 config and not set in CLI",
			ns:   &NodeServer{},
			mnt:  mounter.VolumeMounter(&mounter.FuseMounter{}),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-1",
			},
			want: configFuseMountOptions,
		},
		{
			name: "KernelMountOptions set in cluster-1 config and set in CLI",
			ns: &NodeServer{
				kernelMountOptions: cliKernelMountOptions,
			},
			mnt: mounter.VolumeMounter(&mounter.KernelMounter{}),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-1",
			},
			want: configKernelMountOptions,
		},
		{
			name: "FuseMountOptions not set in cluster-2 config and set in CLI",
			ns: &NodeServer{
				fuseMountOptions: cliFuseMountOptions,
			},
			mnt: mounter.VolumeMounter(&mounter.FuseMounter{}),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-1",
			},
			want: configFuseMountOptions,
		},
		{
			name: "KernelMountOptions not set in cluster-2 config and set in CLI",
			ns: &NodeServer{
				kernelMountOptions: cliKernelMountOptions,
			},
			mnt: mounter.VolumeMounter(&mounter.KernelMounter{}),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-2",
			},
			want: cliKernelMountOptions,
		},
		{
			name: "FuseMountOptions not set in cluster-1 config and set in CLI",
			ns: &NodeServer{
				fuseMountOptions: cliFuseMountOptions,
			},
			mnt: mounter.VolumeMounter(&mounter.FuseMounter{}),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-2",
			},
			want: cliFuseMountOptions,
		},
	}

	volCap := &csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			driver := &csicommon.CSIDriver{}
			tc.ns.DefaultNodeServer = csicommon.NewDefaultNodeServer(
				driver, "cephfs", "", map[string]string{}, map[string]string{},
			)

			err := tc.ns.setMountOptions(tc.mnt, tc.volOptions, volCap, tmpConfPath)
			if err != nil {
				t.Errorf("setMountOptions() = %v", err)
			}

			switch tc.mnt.(type) {
			case *mounter.FuseMounter:
				if !strings.Contains(tc.volOptions.FuseMountOptions, tc.want) {
					t.Errorf("Set FuseMountOptions = %v Required FuseMountOptions = %v", tc.volOptions.FuseMountOptions, tc.want)
				}
			case *mounter.KernelMounter:
				if !strings.Contains(tc.volOptions.KernelMountOptions, tc.want) {
					t.Errorf("Set KernelMountOptions = %v Required KernelMountOptions = %v", tc.volOptions.KernelMountOptions, tc.want)
				}
			}
		})
	}
}
