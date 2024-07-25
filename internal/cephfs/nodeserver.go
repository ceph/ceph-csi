/*
Copyright 2018 The Ceph-CSI Authors.

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
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	"github.com/ceph/ceph-csi/internal/cephfs/mounter"
	"github.com/ceph/ceph-csi/internal/cephfs/store"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	hc "github.com/ceph/ceph-csi/internal/health-checker"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/fscrypt"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NodeServer struct of ceph CSI driver with supported methods of CSI
// node server spec.
type NodeServer struct {
	*csicommon.DefaultNodeServer
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID) return an Aborted error
	VolumeLocks        *util.VolumeLocks
	kernelMountOptions string
	fuseMountOptions   string
	healthChecker      hc.Manager
}

func getCredentialsForVolume(
	volOptions *store.VolumeOptions,
	secrets map[string]string,
) (*util.Credentials, error) {
	var (
		err error
		cr  *util.Credentials
	)

	if volOptions.ProvisionVolume {
		// The volume is provisioned dynamically, use passed in admin credentials

		cr, err = util.NewAdminCredentials(secrets)
		if err != nil {
			return nil, fmt.Errorf("failed to get admin credentials from node stage secrets: %w", err)
		}
	} else {
		// The volume is pre-made, credentials are in node stage secrets

		cr, err = util.NewUserCredentials(secrets)
		if err != nil {
			return nil, fmt.Errorf("failed to get user credentials from node stage secrets: %w", err)
		}
	}

	return cr, nil
}

func (ns *NodeServer) getVolumeOptions(
	ctx context.Context,
	volID fsutil.VolumeID,
	volContext,
	volSecrets map[string]string,
) (*store.VolumeOptions, error) {
	volOptions, _, err := store.NewVolumeOptionsFromVolID(ctx, string(volID), volContext, volSecrets, "", false)
	if err != nil {
		if !errors.Is(err, cerrors.ErrInvalidVolID) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		volOptions, _, err = store.NewVolumeOptionsFromStaticVolume(string(volID), volContext, volSecrets)
		if err != nil {
			if !errors.Is(err, cerrors.ErrNonStaticVolume) {
				return nil, status.Error(codes.Internal, err.Error())
			}

			volOptions, _, err = store.NewVolumeOptionsFromMonitorList(string(volID), volContext, volSecrets)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}

	return volOptions, nil
}

func validateSnapshotBackedVolCapability(volCap *csi.VolumeCapability) error {
	// Snapshot-backed volumes may be used with read-only volume access modes only.

	mode := volCap.GetAccessMode().GetMode()
	if mode != csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY &&
		mode != csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY {
		return status.Error(codes.InvalidArgument,
			"snapshot-backed volume supports only read-only access mode")
	}

	return nil
}

// maybeUnlockFileEncryption unlocks fscrypt on stagingTargetPath, if volOptions enable encryption.
func maybeUnlockFileEncryption(
	ctx context.Context,
	volOptions *store.VolumeOptions,
	stagingTargetPath string,
	volID fsutil.VolumeID,
) error {
	if !volOptions.IsEncrypted() {
		return nil
	}

	// Define Mutex Lock variables
	lockName := string(volID) + "-mutexLock"
	lockDesc := "Lock for " + string(volID)
	lockDuration := 150 * time.Second
	// Generate a consistent lock cookie for the client using hostname and process ID
	lockCookie := generateLockCookie()
	var flags byte = 0

	log.DebugLog(ctx, "Creating lock for the following volume ID %s", volID)

	ioctx, err := volOptions.GetConnection().GetIoctx(volOptions.MetadataPool)
	if err != nil {
		log.ErrorLog(ctx, "Failed to create ioctx: %s", err)

		return err
	}
	defer ioctx.Destroy()

	res, err := ioctx.LockExclusive(string(volID), lockName, lockCookie, lockDesc, lockDuration, &flags)
	if res != 0 {
		switch res {
		case -int(syscall.EBUSY):
			return fmt.Errorf("Lock is already held by another client and cookie pair for %v volume", volID)
		case -int(syscall.EEXIST):
			return fmt.Errorf("Lock is already held by the same client and cookie pair for %v volume", volID)
		default:
			return fmt.Errorf("Failed to lock volume ID %v: %w", volID, err)
		}
	}
	log.DebugLog(ctx, "Lock successfully created for volume ID %s", volID)

	defer func() {
		ret, unlockErr := ioctx.Unlock(string(volID), lockName, lockCookie)
		switch ret {
		case 0:
			log.DebugLog(ctx, "Lock %s successfully released ", lockName)
		case -int(syscall.ENOENT):
			log.DebugLog(ctx, "Lock is not held by the specified %s, %s pair", lockCookie, lockName)
		default:
			log.ErrorLog(ctx, "Failed to release following lock, this will lead to orphan lock %s: %v",
				lockName, unlockErr)
		}
	}()

	log.DebugLog(ctx, "cephfs: unlocking fscrypt on volume %q path %s", volID, stagingTargetPath)
	err = fscrypt.Unlock(ctx, volOptions.Encryption, stagingTargetPath, string(volID))
	if err != nil {
		return err
	}

	return nil
}

// generateLockCookie generates a consistent lock cookie for the client.
func generateLockCookie() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}
	pid := os.Getpid()

	return fmt.Sprintf("%s-%d", hostname, pid)
}

// maybeInitializeFileEncryption initializes KMS and node specifics, if volContext enables encryption.
func maybeInitializeFileEncryption(
	ctx context.Context,
	mnt mounter.VolumeMounter,
	volOptions *store.VolumeOptions,
) error {
	if volOptions.IsEncrypted() {
		if _, isFuse := mnt.(*mounter.FuseMounter); isFuse {
			return errors.New("FUSE mounter does not support encryption")
		}

		return fscrypt.InitializeNode(ctx)
	}

	return nil
}

// NodeStageVolume mounts the volume to a staging path on the node.
func (ns *NodeServer) NodeStageVolume(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	if err := util.ValidateNodeStageVolumeRequest(req); err != nil {
		return nil, err
	}

	// Configuration

	stagingTargetPath := req.GetStagingTargetPath()
	volID := fsutil.VolumeID(req.GetVolumeId())

	if acquired := ns.VolumeLocks.TryAcquire(req.GetVolumeId()); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, req.GetVolumeId())
	}
	defer ns.VolumeLocks.Release(req.GetVolumeId())

	volOptions, err := ns.getVolumeOptions(ctx, volID, req.GetVolumeContext(), req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer volOptions.Destroy()

	// Skip extracting NetNamespaceFilePath if the clusterID is empty.
	// In case of pre-provisioned volume the clusterID is not set in the
	// volume context.
	if volOptions.ClusterID != "" {
		volOptions.NetNamespaceFilePath, err = util.GetCephFSNetNamespaceFilePath(
			util.CsiConfigFile,
			volOptions.ClusterID)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if volOptions.BackingSnapshot {
		if err = validateSnapshotBackedVolCapability(req.GetVolumeCapability()); err != nil {
			return nil, err
		}
	}

	mnt, err := mounter.New(volOptions)
	if err != nil {
		log.ErrorLog(ctx, "failed to create mounter for volume %s: %v", volID, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	err = maybeInitializeFileEncryption(ctx, mnt, volOptions)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Check if the volume is already mounted

	if _, ok := mnt.(*mounter.FuseMounter); ok {
		if err = ns.tryRestoreFuseMountInNodeStage(ctx, stagingTargetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to try to restore FUSE mounts: %v", err)
		}
	}

	isMnt, err := util.IsMountPoint(ns.Mounter, stagingTargetPath)
	if err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		log.DebugLog(ctx, "cephfs: volume %s is already mounted to %s, skipping", volID, stagingTargetPath)
		if err = maybeUnlockFileEncryption(ctx, volOptions, stagingTargetPath, volID); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		ns.startSharedHealthChecker(ctx, req.GetVolumeId(), stagingTargetPath)

		return &csi.NodeStageVolumeResponse{}, nil
	}

	// It's not, mount now

	if err = ns.mount(
		ctx,
		mnt,
		volOptions,
		fsutil.VolumeID(req.GetVolumeId()),
		req.GetStagingTargetPath(),
		req.GetSecrets(),
		req.GetVolumeCapability(),
	); err != nil {
		return nil, err
	}

	log.DebugLog(ctx, "cephfs: successfully mounted volume %s to %s", volID, stagingTargetPath)

	if err = maybeUnlockFileEncryption(ctx, volOptions, stagingTargetPath, volID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if _, isFuse := mnt.(*mounter.FuseMounter); isFuse {
		// FUSE mount recovery needs NodeStageMountinfo records.

		if err = fsutil.WriteNodeStageMountinfo(volID, &fsutil.NodeStageMountinfo{
			VolumeCapability: req.GetVolumeCapability(),
			Secrets:          req.GetSecrets(),
		}); err != nil {
			log.ErrorLog(ctx, "cephfs: failed to write NodeStageMountinfo for volume %s: %v", volID, err)

			// Try to clean node stage mount.
			if unmountErr := mounter.UnmountAll(ctx, stagingTargetPath); unmountErr != nil {
				log.ErrorLog(ctx, "cephfs: failed to unmount %s in WriteNodeStageMountinfo clean up: %v",
					stagingTargetPath, unmountErr)
			}

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	ns.startSharedHealthChecker(ctx, req.GetVolumeId(), stagingTargetPath)

	return &csi.NodeStageVolumeResponse{}, nil
}

// startSharedHealthChecker starts a health-checker on the stagingTargetPath.
// This checker can be shared between multiple containers.
//
// TODO: start a FileChecker for read-writable volumes that have an app-data subdir.
func (ns *NodeServer) startSharedHealthChecker(ctx context.Context, volumeID, dir string) {
	// The StatChecker works for volumes that do not have a dedicated app-data
	// subdirectory, or are read-only.
	err := ns.healthChecker.StartSharedChecker(volumeID, dir, hc.StatCheckerType)
	if err != nil {
		log.WarningLog(ctx, "failed to start healthchecker: %v", err)
	}
}

func (ns *NodeServer) mount(
	ctx context.Context,
	mnt mounter.VolumeMounter,
	volOptions *store.VolumeOptions,
	volID fsutil.VolumeID,
	stagingTargetPath string,
	secrets map[string]string,
	volCap *csi.VolumeCapability,
) error {
	cr, err := getCredentialsForVolume(volOptions, secrets)
	if err != nil {
		log.ErrorLog(ctx, "failed to get ceph credentials for volume %s: %v", volID, err)

		return status.Error(codes.Internal, err.Error())
	}
	defer cr.DeleteCredentials()

	log.DebugLog(ctx, "cephfs: mounting volume %s with %s", volID, mnt.Name())

	err = ns.setMountOptions(mnt, volOptions, volCap, util.CsiConfigFile)
	if err != nil {
		log.ErrorLog(ctx, "failed to set mount options for volume %s: %v", volID, err)

		return status.Error(codes.Internal, err.Error())
	}

	if err = mnt.Mount(ctx, stagingTargetPath, cr, volOptions); err != nil {
		log.ErrorLog(ctx,
			"failed to mount volume %s: %v Check dmesg logs if required.",
			volID,
			err)

		return status.Error(codes.Internal, err.Error())
	}

	defer func() {
		if err == nil {
			return
		}

		unmountErr := mounter.UnmountAll(ctx, stagingTargetPath)
		if unmountErr != nil {
			log.ErrorLog(ctx, "failed to clean up mounts in rollback procedure: %v", unmountErr)
		}
	}()

	if volOptions.BackingSnapshot {
		snapshotRoot, err := getBackingSnapshotRoot(ctx, volOptions, stagingTargetPath)
		if err != nil {
			return err
		}

		absoluteSnapshotRoot := path.Join(stagingTargetPath, snapshotRoot)
		err = mounter.BindMount(
			ctx,
			absoluteSnapshotRoot,
			stagingTargetPath,
			true,
			[]string{"bind", "_netdev"},
		)
		if err != nil {
			log.ErrorLog(ctx,
				"failed to bind mount snapshot root %s: %v", absoluteSnapshotRoot, err)

			return status.Error(codes.Internal, err.Error())
		}
	}

	return nil
}

func getBackingSnapshotRoot(
	ctx context.Context,
	volOptions *store.VolumeOptions,
	stagingTargetPath string,
) (string, error) {
	if volOptions.ProvisionVolume {
		// Provisioned snapshot-backed volumes should have their BackingSnapshotRoot
		// already populated.
		return volOptions.BackingSnapshotRoot, nil
	}

	// Pre-provisioned snapshot-backed volumes are more involved:
	//
	// Snapshots created with `ceph fs subvolume snapshot create` have following
	// snap directory name format inside <root path>/.snap:
	//
	//   _<snapshot>_<snapshot inode number>
	//
	// We don't know what <snapshot inode number> is, and so <root path>/.snap
	// needs to be traversed in order to determine the full snapshot directory name.

	snapshotsBase := path.Join(stagingTargetPath, ".snap")

	dir, err := os.Open(snapshotsBase)
	if err != nil {
		log.ErrorLog(ctx, "failed to open %s when searching for snapshot root: %v", snapshotsBase, err)

		return "", status.Errorf(codes.Internal, err.Error())
	}
	defer dir.Close()

	// Read the contents of <root path>/.snap directory into a string slice.

	contents, err := dir.Readdirnames(0)
	if err != nil {
		log.ErrorLog(ctx, "failed to read %s when searching for snapshot root: %v", snapshotsBase, err)

		return "", status.Errorf(codes.Internal, err.Error())
	}

	var (
		found           bool
		snapshotDirName string
	)

	// Look through the directory's contents and try to find the correct snapshot
	// dir name. The search must be exhaustive to catch possible ambiguous results.

	for i := range contents {
		if !strings.Contains(contents[i], volOptions.BackingSnapshotID) {
			continue
		}

		if !found {
			found = true
			snapshotDirName = contents[i]
		} else {
			return "", status.Errorf(codes.InvalidArgument, "ambiguous backingSnapshotID %s in %s",
				volOptions.BackingSnapshotID, snapshotsBase)
		}
	}

	if !found {
		return "", status.Errorf(codes.InvalidArgument, "no snapshot with backingSnapshotID %s found in %s",
			volOptions.BackingSnapshotID, snapshotsBase)
	}

	return path.Join(".snap", snapshotDirName), nil
}

// NodePublishVolume mounts the volume mounted to the staging path to the target
// path.
func (ns *NodeServer) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	mountOptions := []string{"bind", "_netdev"}
	if err := util.ValidateNodePublishVolumeRequest(req); err != nil {
		return nil, err
	}

	stagingTargetPath := req.GetStagingTargetPath()
	targetPath := req.GetTargetPath()
	volID := fsutil.VolumeID(req.GetVolumeId())

	volOptions := &store.VolumeOptions{}
	defer volOptions.Destroy()

	if err := volOptions.DetectMounter(req.GetVolumeContext()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to detect mounter for volume %s: %v", volID, err.Error())
	}

	volMounter, err := mounter.New(volOptions)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create mounter for volume %s: %v", volID, err.Error())
	}

	// Considering kubelet make sure the stage and publish operations
	// are serialized, we dont need any extra locking in nodePublish

	if err = util.CreateMountPoint(targetPath); err != nil {
		log.ErrorLog(ctx, "failed to create mount point at %s: %v", targetPath, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	if _, ok := volMounter.(*mounter.FuseMounter); ok {
		if err = ns.tryRestoreFuseMountsInNodePublish(
			ctx,
			volID,
			stagingTargetPath,
			targetPath,
			req.GetVolumeContext(),
		); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to try to restore FUSE mounts: %v", err)
		}
	}

	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	mountOptions = csicommon.ConstructMountOptions(mountOptions, req.GetVolumeCapability())

	// Ensure staging target path is a mountpoint.

	isMnt, err := util.IsMountPoint(ns.Mounter, stagingTargetPath)
	if err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	} else if !isMnt {
		return nil, status.Errorf(
			codes.Internal, "staging path %s for volume %s is not a mountpoint", stagingTargetPath, volID,
		)
	}

	// Check if the volume is already mounted

	isMnt, err = util.IsMountPoint(ns.Mounter, targetPath)
	if err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	if isMnt {
		log.DebugLog(ctx, "cephfs: volume %s is already bind-mounted to %s", volID, targetPath)

		return &csi.NodePublishVolumeResponse{}, nil
	}

	// It's not, mount now
	encrypted, err := store.IsEncrypted(ctx, req.GetVolumeContext())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if encrypted {
		stagingTargetPath = fscrypt.AppendEncyptedSubdirectory(stagingTargetPath)
		if err = fscrypt.IsDirectoryUnlocked(stagingTargetPath, "ceph"); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if err = mounter.BindMount(
		ctx,
		stagingTargetPath,
		targetPath,
		req.GetReadonly(),
		mountOptions); err != nil {
		log.ErrorLog(ctx, "failed to bind-mount volume %s: %v", volID, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "cephfs: successfully bind-mounted volume %s to %s", volID, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the volume from the target path.
func (ns *NodeServer) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, err
	}

	// considering kubelet make sure node operations like unpublish/unstage...etc can not be called
	// at same time, an explicit locking at time of nodeunpublish is not required.
	targetPath := req.GetTargetPath()

	// stop the health-checker that may have been started in NodeGetVolumeStats()
	ns.healthChecker.StopChecker(req.GetVolumeId(), targetPath)

	isMnt, err := util.IsMountPoint(ns.Mounter, targetPath)
	if err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		if os.IsNotExist(err) {
			// targetPath has already been deleted
			log.DebugLog(ctx, "targetPath: %s has already been deleted", targetPath)

			return &csi.NodeUnpublishVolumeResponse{}, nil
		}

		if !util.IsCorruptedMountError(err) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Corrupted mounts need to be unmounted properly too,
		// regardless of the mounter used. Continue as normal.
		log.DebugLog(ctx, "cephfs: detected corrupted mount in publish target path %s, trying to unmount anyway", targetPath)
		isMnt = true
	}
	if !isMnt {
		if err = os.RemoveAll(targetPath); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	// Unmount the bind-mount
	if err = mounter.UnmountVolume(ctx, targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = os.Remove(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "cephfs: successfully unbounded volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeUnstageVolume unstages the volume from the staging path.
func (ns *NodeServer) NodeUnstageVolume(
	ctx context.Context,
	req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeUnstageVolumeRequest(req); err != nil {
		return nil, err
	}

	volID := req.GetVolumeId()

	ns.healthChecker.StopSharedChecker(volID)

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	stagingTargetPath := req.GetStagingTargetPath()

	if err = fsutil.RemoveNodeStageMountinfo(fsutil.VolumeID(volID)); err != nil {
		log.ErrorLog(ctx, "cephfs: failed to remove NodeStageMountinfo for volume %s: %v", volID, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	isMnt, err := util.IsMountPoint(ns.Mounter, stagingTargetPath)
	if err != nil {
		log.ErrorLog(ctx, "stat failed: %v", err)

		if os.IsNotExist(err) {
			// targetPath has already been deleted
			log.DebugLog(ctx, "targetPath: %s has already been deleted", stagingTargetPath)

			return &csi.NodeUnstageVolumeResponse{}, nil
		}

		if !util.IsCorruptedMountError(err) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Corrupted mounts need to be unmounted properly too,
		// regardless of the mounter used. Continue as normal.
		log.DebugLog(ctx,
			"cephfs: detected corrupted mount in staging target path %s, trying to unmount anyway",
			stagingTargetPath)
		isMnt = true
	}
	if !isMnt {
		return &csi.NodeUnstageVolumeResponse{}, nil
	}
	// Unmount the volume
	if err = mounter.UnmountAll(ctx, stagingTargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "cephfs: successfully unmounted volume %s from %s", req.GetVolumeId(), stagingTargetPath)

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server.
func (ns *NodeServer) NodeGetCapabilities(
	ctx context.Context,
	req *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_VOLUME_CONDITION,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
					},
				},
			},
		},
	}, nil
}

// NodeGetVolumeStats returns volume stats.
func (ns *NodeServer) NodeGetVolumeStats(
	ctx context.Context,
	req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {
	var err error
	targetPath := req.GetVolumePath()
	if targetPath == "" {
		err = fmt.Errorf("targetpath %v is empty", targetPath)

		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// health check first, return without stats if unhealthy
	healthy, msg := ns.healthChecker.IsHealthy(req.GetVolumeId(), targetPath)

	// If healthy and an error is returned, it means that the checker was not
	// started. This could happen when the node-plugin was restarted and the
	// volume is already staged and published.
	if healthy && msg != nil {
		// Start a StatChecker for the mounted targetPath, this prevents
		// writing a file in the user-visible location. Ideally a (shared)
		// FileChecker is started with the stagingTargetPath, but we can't
		// get the stagingPath from the request easily.
		// TODO: resolve the stagingPath like rbd.getStagingPath() does
		err = ns.healthChecker.StartChecker(req.GetVolumeId(), targetPath, hc.StatCheckerType)
		if err != nil {
			log.WarningLog(ctx, "failed to start healthchecker: %v", err)
		}
	}

	// !healthy indicates a problem with the volume
	if !healthy {
		return &csi.NodeGetVolumeStatsResponse{
			VolumeCondition: &csi.VolumeCondition{
				Abnormal: true,
				Message:  msg.Error(),
			},
		}, nil
	}

	// warning: stat() may hang on an unhealthy volume
	stat, err := os.Stat(targetPath)
	if err != nil {
		if util.IsCorruptedMountError(err) {
			log.WarningLog(ctx, "corrupted mount detected in %q: %v", targetPath, err)

			return &csi.NodeGetVolumeStatsResponse{
				VolumeCondition: &csi.VolumeCondition{
					Abnormal: true,
					Message:  err.Error(),
				},
			}, nil
		}

		return nil, status.Errorf(codes.InvalidArgument, "failed to get stat for targetpath %q: %v", targetPath, err)
	}

	if stat.Mode().IsDir() {
		return csicommon.FilesystemNodeGetVolumeStats(ctx, ns.Mounter, targetPath, false)
	}

	return nil, status.Errorf(codes.InvalidArgument, "targetpath %q is not a directory or device", targetPath)
}

// setMountOptions updates the kernel/fuse mount options from CSI config file if it exists.
// If not, it falls back to returning the kernelMountOptions/fuseMountOptions from the command line.
func (ns *NodeServer) setMountOptions(
	mnt mounter.VolumeMounter,
	volOptions *store.VolumeOptions,
	volCap *csi.VolumeCapability,
	csiConfigFile string,
) error {
	var (
		configuredMountOptions   string
		readAffinityMountOptions string
		kernelMountOptions       string
		fuseMountOptions         string
		mountOptions             []string
		err                      error
	)
	if m := volCap.GetMount(); m != nil {
		mountOptions = m.GetMountFlags()
	}

	if volOptions.ClusterID != "" {
		kernelMountOptions, fuseMountOptions, err = util.GetCephFSMountOptions(csiConfigFile, volOptions.ClusterID)
		if err != nil {
			return err
		}

		// read affinity mount options
		readAffinityMountOptions, err = util.GetReadAffinityMapOptions(
			csiConfigFile, volOptions.ClusterID, ns.CLIReadAffinityOptions, ns.NodeLabels,
		)
		if err != nil {
			return err
		}
	}

	switch mnt.(type) {
	case *mounter.FuseMounter:
		configuredMountOptions = ns.fuseMountOptions
		// override if fuseMountOptions are set
		if fuseMountOptions != "" {
			configuredMountOptions = fuseMountOptions
		}
		volOptions.FuseMountOptions = util.MountOptionsAdd(volOptions.FuseMountOptions, configuredMountOptions)
		volOptions.FuseMountOptions = util.MountOptionsAdd(volOptions.FuseMountOptions, mountOptions...)
	case mounter.KernelMounter:
		configuredMountOptions = ns.kernelMountOptions
		// override of kernelMountOptions are set
		if kernelMountOptions != "" {
			configuredMountOptions = kernelMountOptions
		}
		volOptions.KernelMountOptions = util.MountOptionsAdd(volOptions.KernelMountOptions, configuredMountOptions)
		volOptions.KernelMountOptions = util.MountOptionsAdd(volOptions.KernelMountOptions, readAffinityMountOptions)
		volOptions.KernelMountOptions = util.MountOptionsAdd(volOptions.KernelMountOptions, mountOptions...)
	}

	const readOnly = "ro"
	mode := volCap.GetAccessMode().GetMode()
	if mode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY ||
		mode == csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY {
		switch mnt.(type) {
		case *mounter.FuseMounter:
			if !csicommon.MountOptionContains(strings.Split(volOptions.FuseMountOptions, ","), readOnly) {
				volOptions.FuseMountOptions = util.MountOptionsAdd(volOptions.FuseMountOptions, readOnly)
			}
		case mounter.KernelMounter:
			if !csicommon.MountOptionContains(strings.Split(volOptions.KernelMountOptions, ","), readOnly) {
				volOptions.KernelMountOptions = util.MountOptionsAdd(volOptions.KernelMountOptions, readOnly)
			}
		}
	}

	return nil
}
