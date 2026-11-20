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
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mount "k8s.io/mount-utils"

	cephcsi "github.com/ceph/ceph-csi/api/deploy/kubernetes"
	"github.com/ceph/ceph-csi/internal/cephfs/mounter"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	hc "github.com/ceph/ceph-csi/internal/health-checker"
	"github.com/ceph/ceph-csi/internal/util"
)

func Test_setMountOptions(t *testing.T) {
	t.Parallel()

	cliKernelMountOptions := "noexec,nodev"
	cliFuseMountOptions := "default_permissions,auto_cache"

	configKernelMountOptions := "crc"
	configFuseMountOptions := "allow_other"

	csiConfig := []cephcsi.ClusterInfo{
		{
			ClusterID: "cluster-1",
			CephFS: cephcsi.CephFS{
				KernelMountOptions: configKernelMountOptions,
				FuseMountOptions:   configFuseMountOptions,
			},
		},
		{
			ClusterID: "cluster-2",
			CephFS: cephcsi.CephFS{
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
		ns         *cephfsNodeServer
		mnt        mounter.VolumeMounter
		volOptions *store.VolumeOptions
		want       string
	}{
		{
			name: "KernelMountOptions set in cluster-1 config and not set in CLI",
			ns:   &cephfsNodeServer{},
			mnt:  mounter.VolumeMounter(mounter.NewKernelMounter()),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-1",
			},
			want: configKernelMountOptions,
		},
		{
			name: "FuseMountOptions set in cluster-1 config and not set in CLI",
			ns:   &cephfsNodeServer{},
			mnt:  mounter.VolumeMounter(&mounter.FuseMounter{}),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-1",
			},
			want: configFuseMountOptions,
		},
		{
			name: "KernelMountOptions set in cluster-1 config and set in CLI",
			ns: &cephfsNodeServer{
				kernelMountOptions: cliKernelMountOptions,
			},
			mnt: mounter.VolumeMounter(mounter.NewKernelMounter()),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-1",
			},
			want: configKernelMountOptions,
		},
		{
			name: "FuseMountOptions not set in cluster-2 config and set in CLI",
			ns: &cephfsNodeServer{
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
			ns: &cephfsNodeServer{
				kernelMountOptions: cliKernelMountOptions,
			},
			mnt: mounter.VolumeMounter(mounter.NewKernelMounter()),
			volOptions: &store.VolumeOptions{
				ClusterID: "cluster-2",
			},
			want: cliKernelMountOptions,
		},
		{
			name: "FuseMountOptions not set in cluster-1 config and set in CLI",
			ns: &cephfsNodeServer{
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			driver := &csicommon.CSIDriver{}
			tt.ns.DefaultNodeServer = csicommon.NewDefaultNodeServer(
				driver, "cephfs", "", map[string]string{}, map[string]string{},
			)

			err := tt.ns.setMountOptions(tt.mnt, tt.volOptions, volCap, tmpConfPath)
			if err != nil {
				t.Errorf("setMountOptions() = %v", err)
			}

			switch tt.mnt.(type) {
			case *mounter.FuseMounter:
				if !strings.Contains(tt.volOptions.FuseMountOptions, tt.want) {
					t.Errorf("Set FuseMountOptions = %v Required FuseMountOptions = %v", tt.volOptions.FuseMountOptions, tt.want)
				}
			case mounter.KernelMounter:
				if !strings.Contains(tt.volOptions.KernelMountOptions, tt.want) {
					t.Errorf("Set KernelMountOptions = %v Required KernelMountOptions = %v", tt.volOptions.KernelMountOptions, tt.want)
				}
			}
		})
	}
}

func TestNodeUnstageVolumeAbortedPreservesHealthChecker(t *testing.T) {
	t.Parallel()

	volID := "0001-0009-cluster-1-0000000000000001-ab2c3d4e-1111-2222-3333-444455556666"
	stagingPath := t.TempDir()

	healthChecker := hc.NewHealthCheckManager()
	require.NoError(t, healthChecker.StartSharedChecker(volID, stagingPath, hc.NoOpCheckerType))

	ns := &cephfsNodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(
			&csicommon.CSIDriver{}, "cephfs", "", map[string]string{}, map[string]string{},
		),
		VolumeLocks:   util.NewIDLocker(),
		healthChecker: healthChecker,
	}

	require.True(t, ns.VolumeLocks.TryAcquire(volID))
	defer ns.VolumeLocks.Release(volID)

	_, err := ns.NodeUnstageVolume(t.Context(), &csi.NodeUnstageVolumeRequest{
		VolumeId:          volID,
		StagingTargetPath: stagingPath,
	})
	require.Equal(t, codes.Aborted, status.Code(err))

	healthy, hcErr := healthChecker.IsHealthy(volID, "")
	require.NoError(t, hcErr)
	require.True(t, healthy)

	healthChecker.StopSharedChecker(volID)
}

type testNodeUnstageMounter struct {
	*mount.FakeMounter
	isMnt    bool
	isMntErr error
}

func (m *testNodeUnstageMounter) IsMountPoint(_ string) (bool, error) {
	return m.isMnt, m.isMntErr
}

func TestNodeUnstageVolumeMountinfoHandling(t *testing.T) {
	t.Parallel()

	fsutil.SetMountinfoDir(t.TempDir())

	tests := []struct {
		name          string
		volID         string
		isMnt         bool
		isMntErr      error
		unmountErr    error
		wantCode      codes.Code
		wantMountinfo bool
	}{
		{
			name:          "fatal IsMountPoint error preserves NodeStageMountinfo",
			volID:         "0001-0009-cluster-1-0000000000000001-11111111-1111-2222-3333-444455556661",
			isMnt:         false,
			isMntErr:      errors.New("stat failed"),
			wantCode:      codes.Internal,
			wantMountinfo: true,
		},
		{
			name:          "failed unmount preserves NodeStageMountinfo",
			volID:         "0001-0009-cluster-1-0000000000000001-11111111-1111-2222-3333-444455556662",
			isMnt:         true,
			unmountErr:    errors.New("device or resource busy"),
			wantCode:      codes.Internal,
			wantMountinfo: true,
		},
		{
			name:          "successful unmount removes NodeStageMountinfo",
			volID:         "0001-0009-cluster-1-0000000000000001-11111111-1111-2222-3333-444455556663",
			isMnt:         true,
			wantCode:      codes.OK,
			wantMountinfo: false,
		},
		{
			name:          "already unmounted staging path removes NodeStageMountinfo",
			volID:         "0001-0009-cluster-1-0000000000000001-11111111-1111-2222-3333-444455556664",
			isMnt:         false,
			wantCode:      codes.OK,
			wantMountinfo: false,
		},
		{
			name:          "missing staging path removes NodeStageMountinfo",
			volID:         "0001-0009-cluster-1-0000000000000001-11111111-1111-2222-3333-444455556665",
			isMnt:         false,
			isMntErr:      os.ErrNotExist,
			wantCode:      codes.OK,
			wantMountinfo: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := fsutil.WriteNodeStageMountinfo(fsutil.VolumeID(tt.volID), &fsutil.NodeStageMountinfo{
				VolumeCapability: &csi.VolumeCapability{},
			})
			require.NoError(t, err)

			defaultNS := csicommon.NewDefaultNodeServer(
				&csicommon.CSIDriver{}, "cephfs", "", map[string]string{}, map[string]string{},
			)
			defaultNS.Mounter = &testNodeUnstageMounter{
				FakeMounter: mount.NewFakeMounter(nil),
				isMnt:       tt.isMnt,
				isMntErr:    tt.isMntErr,
			}

			ns := &cephfsNodeServer{
				DefaultNodeServer: defaultNS,
				VolumeLocks:       util.NewIDLocker(),
				healthChecker:     hc.NewHealthCheckManager(),
				unmountAll: func(_ context.Context, _ string) error {
					return tt.unmountErr
				},
			}

			_, err = ns.NodeUnstageVolume(t.Context(), &csi.NodeUnstageVolumeRequest{
				VolumeId:          tt.volID,
				StagingTargetPath: t.TempDir(),
			})
			require.Equal(t, tt.wantCode, status.Code(err))

			mi, getErr := fsutil.GetNodeStageMountinfo(fsutil.VolumeID(tt.volID))
			require.NoError(t, getErr)
			if tt.wantMountinfo {
				require.NotNil(t, mi)
			} else {
				require.Nil(t, mi)
			}
		})
	}
}
