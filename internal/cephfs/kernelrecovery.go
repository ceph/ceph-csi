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
	"context"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/proto"
	"github.com/moby/sys/mountinfo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/util"
)

// kernelStagingMissing only authorizes recovery of an absent mount. Corrupt
// mounts and conflicting filesystems require separate remediation.
func (ns *cephfsNodeServer) kernelStagingMissing(path string) (bool, error) {
	mounted, err := ns.Mounter.IsMountPoint(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, status.Errorf(codes.Internal, "cannot inspect kernel staging mount: %v", err)
	}
	if !mounted {
		return true, nil
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, status.Errorf(codes.Internal, "cannot resolve kernel staging path: %v", err)
	}
	// mountinfo decodes escaped mountpoint paths and returns all entries at
	// resolvedPath without walking parent IDs; reject any non-CephFS layer,
	// such as a stray local bind mount left at the staging path.
	mounts, err := ns.getMounts(func(m *mountinfo.Info) (bool, bool) {
		return m.Mountpoint != resolvedPath, false
	})
	if err != nil {
		// Parser errors can contain the raw mount options.
		return false, status.Error(codes.Internal, "cannot inspect staging filesystem")
	}
	if len(mounts) == 0 {
		return false, status.Error(codes.FailedPrecondition, "staging mount disappeared during inspection")
	}
	for _, m := range mounts {
		if m.FSType != "ceph" {
			return false, status.Error(codes.FailedPrecondition, "staging path contains a conflicting filesystem")
		}
	}

	return false, nil
}

func kernelRecoveryRequest(
	req *csi.NodePublishVolumeRequest,
	info *fsutil.NodeStageMountinfo,
) (*csi.NodeStageVolumeRequest, error) {
	if info == nil || !info.IsKernel() || info.StagingTargetPath != req.GetStagingTargetPath() ||
		info.VolumeCapability.GetMount() == nil || len(info.Secrets) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "kernel staging recovery record is missing or invalid")
	}
	if !proto.Equal(info.VolumeCapability, req.GetVolumeCapability()) {
		return nil, status.Error(codes.FailedPrecondition, "volume capability changed since staging")
	}
	for key, value := range info.VolumeContext {
		if current, found := req.GetVolumeContext()[key]; !found || current != value {
			return nil, status.Error(codes.FailedPrecondition, "volume context changed since staging")
		}
	}
	for key, value := range req.GetVolumeContext() {
		switch key {
		case "csi.storage.k8s.io/pod.name", "csi.storage.k8s.io/pod.namespace", "csi.storage.k8s.io/pod.uid",
			"csi.storage.k8s.io/serviceAccount.name", "csi.storage.k8s.io/serviceAccount.tokens",
			"csi.storage.k8s.io/ephemeral":
			continue
		}
		if saved, found := info.VolumeContext[key]; !found || saved != value {
			return nil, status.Error(codes.FailedPrecondition, "volume context changed since staging")
		}
	}

	return &csi.NodeStageVolumeRequest{
		VolumeId:          req.GetVolumeId(),
		StagingTargetPath: info.StagingTargetPath,
		VolumeCapability:  info.VolumeCapability,
		VolumeContext:     info.VolumeContext,
		Secrets:           info.Secrets,
	}, nil
}

// The caller holds the volume ID lock until Publish completes. Recovery never
// unmounts staging or a Pod target, including when the new mount fails.
func (ns *cephfsNodeServer) tryRestoreKernelMountInNodePublish(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
	stageVolume func(context.Context, *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error),
) error {
	missing, err := ns.kernelStagingMissing(req.GetStagingTargetPath())
	if err != nil || !missing {
		return err
	}
	mounted, err := ns.Mounter.IsMountPoint(req.GetTargetPath())
	if err != nil || mounted {
		return status.Error(codes.FailedPrecondition, "cannot recover kernel staging for an existing or unreadable target")
	}
	info, err := fsutil.GetNodeStageMountinfo(fsutil.VolumeID(req.GetVolumeId()))
	if err != nil {
		return status.Errorf(codes.Internal, "cannot read kernel staging recovery record: %v", err)
	}
	stage, err := kernelRecoveryRequest(req, info)
	if err != nil {
		return err
	}
	if err = util.CreateMountPoint(stage.GetStagingTargetPath()); err != nil {
		return status.Errorf(codes.Internal, "cannot recreate kernel staging directory: %v", err)
	}
	_, err = stageVolume(ctx, stage)

	return err
}
