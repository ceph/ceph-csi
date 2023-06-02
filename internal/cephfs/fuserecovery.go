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

package cephfs

import (
	"context"

	"github.com/ceph/ceph-csi/internal/cephfs/mounter"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	mountutil "k8s.io/mount-utils"
)

type (
	mountState int
)

const (
	msUnknown mountState = iota
	msNotMounted
	msMounted
	msCorrupted

	// ceph-fuse fsType in /proc/<PID>/mountinfo.
	cephFuseFsType = "fuse.ceph-fuse"
)

func (ms mountState) String() string {
	return [...]string{
		"UNKNOWN",
		"NOT_MOUNTED",
		"MOUNTED",
		"CORRUPTED",
	}[int(ms)]
}

func (ns *NodeServer) getMountState(path string) (mountState, error) {
	isMnt, err := util.IsMountPoint(ns.Mounter, path)
	if err != nil {
		if util.IsCorruptedMountError(err) {
			return msCorrupted, nil
		}

		return msUnknown, err
	}

	if isMnt {
		return msMounted, nil
	}

	return msNotMounted, nil
}

func findMountinfo(mountpoint string, mis []mountutil.MountInfo) int {
	for i := range mis {
		if mis[i].MountPoint == mountpoint {
			return i
		}
	}

	return -1
}

// Ensures that given mountpoint is of specified fstype.
// Returns true if fstype matches, or if no such mountpoint exists.
func validateFsType(mountpoint, fsType string, mis []mountutil.MountInfo) bool {
	if idx := findMountinfo(mountpoint, mis); idx > 0 {
		mi := mis[idx]

		if mi.FsType != fsType {
			return false
		}
	}

	return true
}

// tryRestoreFuseMountsInNodePublish tries to restore staging and publish
// volume moutpoints inside the NodePublishVolume call.
//
// Restoration is performed in following steps:
//  1. Detection: staging target path must be a working mountpoint, and target
//     path must not be a corrupted mountpoint (see getMountState()). If either
//     of those checks fail, mount recovery is performed.
//  2. Recovery preconditions:
//     * NodeStageMountinfo is present for this volume,
//     * if staging target path and target path are mountpoints, they must be
//     managed by ceph-fuse,
//     * VolumeOptions.Mounter must evaluate to "fuse".
//  3. Recovery:
//     * staging target path is unmounted and mounted again using ceph-fuse,
//     * target path is only unmounted; NodePublishVolume is then expected to
//     continue normally.
func (ns *NodeServer) tryRestoreFuseMountsInNodePublish(
	ctx context.Context,
	volID fsutil.VolumeID,
	stagingTargetPath string,
	targetPath string,
	volContext map[string]string,
) error {
	// Check if there is anything to restore.

	stagingTargetMs, err := ns.getMountState(stagingTargetPath)
	if err != nil {
		return err
	}

	targetMs, err := ns.getMountState(targetPath)
	if err != nil {
		return err
	}

	if stagingTargetMs == msMounted && targetMs != msCorrupted {
		// Mounts seem to be fine.
		return nil
	}

	// Something is broken. Try to proceed with mount recovery.

	log.WarningLog(ctx, "cephfs: mount problem detected when publishing a volume: %s is %s, %s is %s; attempting recovery",
		stagingTargetPath, stagingTargetMs, targetPath, targetMs)

	// NodeStageMountinfo entry must be present for this volume.

	var nsMountinfo *fsutil.NodeStageMountinfo

	if nsMountinfo, err = fsutil.GetNodeStageMountinfo(volID); err != nil {
		return err
	} else if nsMountinfo == nil {
		log.WarningLog(ctx, "cephfs: cannot proceed with mount recovery because NodeStageMountinfo record is missing")

		return nil
	}

	// Check that the existing stage and publish mounts for this volume are
	// managed by ceph-fuse, and that the mounter is of the FuseMounter type.
	// Then try to restore them.

	var (
		volMounter mounter.VolumeMounter
		volOptions *store.VolumeOptions
	)

	procMountInfo, err := util.ReadMountInfoForProc("self")
	if err != nil {
		return err
	}

	if !validateFsType(stagingTargetPath, cephFuseFsType, procMountInfo) ||
		!validateFsType(targetPath, cephFuseFsType, procMountInfo) {
		// We can't restore mounts not managed by ceph-fuse.
		log.WarningLog(ctx, "cephfs: cannot proceed with mount recovery on non-FUSE mountpoints")

		return nil
	}

	volOptions, err = ns.getVolumeOptions(ctx, volID, volContext, nsMountinfo.Secrets)
	if err != nil {
		return err
	}

	volMounter, err = mounter.New(volOptions)
	if err != nil {
		return err
	}

	if _, ok := volMounter.(*mounter.FuseMounter); !ok {
		// We can't restore mounts with non-FUSE mounter.
		log.WarningLog(ctx, "cephfs: cannot proceed with mount recovery with non-FUSE mounter")

		return nil
	}

	// Try to restore mount in staging target path.
	// Unmount and mount the volume.

	if stagingTargetMs != msMounted {
		if err := mounter.UnmountAll(ctx, stagingTargetPath); err != nil {
			return err
		}

		if err := ns.mount(
			ctx,
			volMounter,
			volOptions,
			volID,
			stagingTargetPath,
			nsMountinfo.Secrets,
			nsMountinfo.VolumeCapability,
		); err != nil {
			return err
		}
	}

	// Try to restore mount in target path.
	// Only unmount the bind mount. NodePublishVolume should then
	// create the bind mount by itself.

	if err := mounter.UnmountVolume(ctx, targetPath); err != nil {
		return err
	}

	return nil
}

// Try to restore FUSE mount of the staging target path in NodeStageVolume.
// If corruption is detected, try to only unmount the volume. NodeStageVolume
// should be able to continue with mounting the volume normally afterwards.
func (ns *NodeServer) tryRestoreFuseMountInNodeStage(
	ctx context.Context,
	mnt mounter.VolumeMounter,
	stagingTargetPath string,
) error {
	// Check if there is anything to restore.

	stagingTargetMs, err := ns.getMountState(stagingTargetPath)
	if err != nil {
		return err
	}

	if stagingTargetMs != msCorrupted {
		// Mounts seem to be fine.
		return nil
	}

	// Something is broken. Try to proceed with mount recovery.

	log.WarningLog(ctx, "cephfs: mountpoint problem detected when staging a volume: %s is %s; attempting recovery",
		stagingTargetPath, stagingTargetMs)

	// Check that the existing stage mount for this volume is  managed by
	// ceph-fuse, and that the mounter is FuseMounter. Then try to restore them.

	procMountInfo, err := util.ReadMountInfoForProc("self")
	if err != nil {
		return err
	}

	if !validateFsType(stagingTargetPath, cephFuseFsType, procMountInfo) {
		// We can't restore mounts not managed by ceph-fuse.
		log.WarningLog(ctx, "cephfs: cannot proceed with mount recovery on non-FUSE mountpoints")

		return nil
	}

	if _, ok := mnt.(*mounter.FuseMounter); !ok {
		// We can't restore mounts with non-FUSE mounter.
		log.WarningLog(ctx, "cephfs: cannot proceed with mount recovery with non-FUSE mounter")

		return nil
	}

	// Restoration here means only unmounting the volume.
	// NodeStageVolume should take care of the rest.
	return mounter.UnmountAll(ctx, stagingTargetPath)
}
