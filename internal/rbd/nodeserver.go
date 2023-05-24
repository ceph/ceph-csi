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

package rbd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/fscrypt"
	"github.com/ceph/ceph-csi/internal/util/log"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/volume"
	mount "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

// NodeServer struct of ceph rbd driver with supported methods of CSI
// node server spec.
type NodeServer struct {
	*csicommon.DefaultNodeServer
	// A map storing all volumes with ongoing operations so that additional operations
	// for that same volume (as defined by VolumeID) return an Aborted error
	VolumeLocks *util.VolumeLocks
	// readAffinityMapOptions contains map options to enable read affinity.
	readAffinityMapOptions string
}

// stageTransaction struct represents the state a transaction was when it either completed
// or failed
// this transaction state can be used to rollback the transaction.
type stageTransaction struct {
	// isStagePathCreated represents whether the mount path to stage the volume on was created or not
	isStagePathCreated bool
	// isMounted represents if the volume was mounted or not
	isMounted bool
	// isBlockEncrypted represents if the volume was encrypted or not
	isBlockEncrypted bool
	// devicePath represents the path where rbd device is mapped
	devicePath string
}

const (
	// values for xfsHasReflink.
	xfsReflinkUnset int = iota
	xfsReflinkNoSupport
	xfsReflinkSupport

	staticVol        = "staticVolume"
	volHealerCtx     = "volumeHealerContext"
	tryOtherMounters = "tryOtherMounters"
)

var (
	kernelRelease = ""
	// deepFlattenSupport holds the list of kernel which support mapping rbd
	// image with deep-flatten image feature
	//nolint:gomnd // numbers specify Kernel versions.
	deepFlattenSupport = []util.KernelVersion{
		{
			Version:      5,
			PatchLevel:   1,
			SubLevel:     0,
			ExtraVersion: 0,
			Distribution: "",
			Backport:     false,
		}, // standard 5.1+ versions
		{
			Version:      4,
			PatchLevel:   18,
			SubLevel:     0,
			ExtraVersion: 193,
			Distribution: ".el8",
			Backport:     true,
		}, // RHEL 8.2
	}

	// xfsHasReflink is set by xfsSupportsReflink(), use the function when
	// checking the support for reflink.
	xfsHasReflink = xfsReflinkUnset

	mkfsDefaultArgs = map[string][]string{
		"ext4": {"-m0", "-Enodiscard,lazy_itable_init=1,lazy_journal_init=1"},
		"xfs":  {"-K"},
	}

	mountDefaultOpts = map[string][]string{
		"xfs": {"nouuid"},
	}
)

// parseBoolOption checks if parameters contain option and parse it. If it is
// empty or not set return default.
//
//nolint:unparam // currently defValue is always false, this can change in the future
func parseBoolOption(ctx context.Context, parameters map[string]string, optionName string, defValue bool) bool {
	boolVal := defValue

	if val, ok := parameters[optionName]; ok {
		var err error
		if boolVal, err = strconv.ParseBool(val); err != nil {
			log.ErrorLog(ctx, "failed to parse value of %q: %q", optionName, val)
		}
	}

	return boolVal
}

// healerStageTransaction attempts to attach the rbd Image with previously
// updated device path at stashFile.
func healerStageTransaction(ctx context.Context, cr *util.Credentials, volOps *rbdVolume, metaDataPath string) error {
	imgInfo, err := lookupRBDImageMetadataStash(metaDataPath)
	if err != nil {
		log.ErrorLog(ctx, "failed to find image metadata, at stagingPath: %s, err: %v", metaDataPath, err)

		return err
	}
	if imgInfo.DevicePath == "" {
		return fmt.Errorf("device is empty in image metadata, at stagingPath: %s", metaDataPath)
	}
	var devicePath string
	devicePath, err = attachRBDImage(ctx, volOps, imgInfo.DevicePath, cr)
	if err != nil {
		return err
	}
	log.DebugLog(ctx, "rbd volID: %s was successfully attached to device: %s", volOps.VolID, devicePath)

	return nil
}

// populateRbdVol update the fields in rbdVolume struct based on the request it received.
// this function also receive the credentials and secrets args as it differs in its data.
// The credentials are used directly by functions like voljournal.Connect() and other functions
// like genVolFromVolumeOptions() make use of secrets.
func (ns *NodeServer) populateRbdVol(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
	cr *util.Credentials,
) (*rbdVolume, error) {
	var err error
	volID := req.GetVolumeId()
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	disableInUseChecks := false
	// MULTI_NODE_MULTI_WRITER is supported by default for Block access type volumes
	if req.VolumeCapability.AccessMode.Mode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
		if !isBlock {
			log.WarningLog(
				ctx,
				"MULTI_NODE_MULTI_WRITER currently only supported with volumes of access type `block`,"+
					"invalid AccessMode for volume: %v",
				req.GetVolumeId(),
			)

			return nil, status.Error(
				codes.InvalidArgument,
				"rbd: RWX access mode request is only valid for volumes with access type `block`",
			)
		}

		disableInUseChecks = true
	}
	var rv *rbdVolume

	isStaticVol := parseBoolOption(ctx, req.GetVolumeContext(), staticVol, false)
	// get rbd image name from the volume journal
	// for static volumes, the image name is actually the volume ID itself
	if isStaticVol {
		if req.GetVolumeContext()[intreeMigrationKey] == intreeMigrationLabel {
			// if migration static volume, use imageName as volID
			volID = req.GetVolumeContext()["imageName"]
		}
		rv, err = genVolFromVolumeOptions(ctx, req.GetVolumeContext(), disableInUseChecks, true)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		rv.RbdImageName = volID
	} else {
		rv, err = GenVolFromVolID(ctx, volID, cr, req.GetSecrets())
		if err != nil {
			rv.Destroy()
			log.ErrorLog(ctx, "error generating volume %s: %v", volID, err)

			return nil, status.Errorf(codes.Internal, "error generating volume %s: %v", volID, err)
		}
		rv.DataPool = req.GetVolumeContext()["dataPool"]
		var ok bool
		if rv.Mounter, ok = req.GetVolumeContext()["mounter"]; !ok {
			rv.Mounter = rbdDefaultMounter
		}
	}

	rv.DisableInUseChecks = disableInUseChecks

	err = rv.Connect(cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to connect to volume %s: %v", rv, err)

		return nil, status.Error(codes.Internal, err.Error())
	}
	// in case of any error call Destroy for cleanup.
	defer func() {
		if err != nil {
			rv.Destroy()
		}
	}()
	// get the image details from the ceph cluster.
	err = rv.getImageInfo()
	if err != nil {
		log.ErrorLog(ctx, "failed to get image details %s: %v", rv, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	err = rv.initKMS(req.GetVolumeContext(), req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	features := strings.Join(rv.ImageFeatureSet.Names(), ",")
	isFeatureExist, err := isKrbdFeatureSupported(ctx, features)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.ErrorLog(ctx, "failed checking krbd features %q: %v", features, err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	if rv.Mounter == rbdDefaultMounter && !isFeatureExist {
		if !parseBoolOption(ctx, req.GetVolumeContext(), tryOtherMounters, false) {
			log.ErrorLog(ctx, "unsupported krbd Feature, set `tryOtherMounters:true` or fix krbd driver")
			err = errors.New("unsupported krbd Feature")

			return nil, status.Error(codes.Internal, err.Error())
		}
		// fallback to rbd-nbd,
		rv.Mounter = rbdNbdMounter
	}

	err = getMapOptions(req, rv)
	if err != nil {
		return nil, err
	}
	ns.appendReadAffinityMapOptions(rv)

	rv.VolID = volID

	rv.LogDir = req.GetVolumeContext()["cephLogDir"]
	if rv.LogDir == "" {
		rv.LogDir = defaultLogDir
	}
	rv.LogStrategy = req.GetVolumeContext()["cephLogStrategy"]
	if rv.LogStrategy == "" {
		rv.LogStrategy = defaultLogStrategy
	}

	return rv, err
}

// appendReadAffinityMapOptions appends readAffinityMapOptions to mapOptions
// if mounter is rbdDefaultMounter and readAffinityMapOptions is not empty.
func (ns NodeServer) appendReadAffinityMapOptions(rv *rbdVolume) {
	switch {
	case ns.readAffinityMapOptions == "" || rv.Mounter != rbdDefaultMounter:
		return
	case rv.MapOptions != "":
		rv.MapOptions += "," + ns.readAffinityMapOptions
	default:
		rv.MapOptions = ns.readAffinityMapOptions
	}
}

// NodeStageVolume mounts the volume to a staging path on the node.
// Implementation notes:
// - stagingTargetPath is the directory passed in the request where the volume needs to be staged
//   - We stage the volume into a directory, named after the VolumeID inside stagingTargetPath if
//     it is a file system
//   - We stage the volume into a file, named after the VolumeID inside stagingTargetPath if it is
//     a block volume
//   - Order of operation execution: (useful for defer stacking and when Unstaging to ensure steps
//     are done in reverse, this is done in undoStagingTransaction)
//   - Stash image metadata under staging path
//   - Map the image (creates a device)
//   - Create the staging file/directory under staging path
//   - Stage the device (mount the device mapped for image)
func (ns *NodeServer) NodeStageVolume(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	var err error
	if err = util.ValidateNodeStageVolumeRequest(req); err != nil {
		return nil, err
	}

	volID := req.GetVolumeId()
	cr, err := util.NewUserCredentialsWithMigration(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer cr.DeleteCredentials()
	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	stagingParentPath := req.GetStagingTargetPath()
	stagingTargetPath := stagingParentPath + "/" + volID

	isHealer := parseBoolOption(ctx, req.GetVolumeContext(), volHealerCtx, false)
	if !isHealer {
		var isNotMnt bool
		// check if stagingPath is already mounted
		isNotMnt, err = isNotMountPoint(ns.Mounter, stagingTargetPath)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		} else if !isNotMnt {
			log.DebugLog(ctx, "rbd: volume %s is already mounted to %s, skipping", volID, stagingTargetPath)

			return &csi.NodeStageVolumeResponse{}, nil
		}
	}

	isStaticVol := parseBoolOption(ctx, req.GetVolumeContext(), staticVol, false)
	rv, err := ns.populateRbdVol(ctx, req, cr)
	if err != nil {
		return nil, err
	}
	defer rv.Destroy()

	rv.NetNamespaceFilePath, err = util.GetRBDNetNamespaceFilePath(util.CsiConfigFile, rv.ClusterID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if isHealer {
		err = healerStageTransaction(ctx, cr, rv, stagingParentPath)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.NodeStageVolumeResponse{}, nil
	}

	// Stash image details prior to mapping the image (useful during Unstage as it has no
	// voloptions passed to the RPC as per the CSI spec)
	err = stashRBDImageMetadata(rv, stagingParentPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// perform the actual staging and if this fails, have undoStagingTransaction
	// cleans up for us
	txn, err := ns.stageTransaction(ctx, req, cr, rv, isStaticVol)
	defer func() {
		if err != nil {
			ns.undoStagingTransaction(ctx, req, txn, rv)
		}
	}()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(
		ctx,
		"rbd: successfully mounted volume %s to stagingTargetPath %s",
		volID,
		stagingTargetPath)

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *NodeServer) stageTransaction(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
	cr *util.Credentials,
	volOptions *rbdVolume,
	staticVol bool,
) (*stageTransaction, error) {
	transaction := &stageTransaction{}

	var err error

	// Allow image to be mounted on multiple nodes if it is ROX
	if req.VolumeCapability.AccessMode.Mode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		log.ExtendedLog(ctx, "setting disableInUseChecks on rbd volume to: %v", req.GetVolumeId)
		volOptions.DisableInUseChecks = true
		volOptions.readOnly = true
	}

	err = flattenImageBeforeMapping(ctx, volOptions)
	if err != nil {
		return transaction, err
	}

	// Mapping RBD image
	var devicePath string
	devicePath, err = attachRBDImage(ctx, volOptions, devicePath, cr)
	if err != nil {
		return transaction, err
	}
	transaction.devicePath = devicePath

	log.DebugLog(ctx, "rbd image: %s was successfully mapped at %s\n",
		volOptions, devicePath)

	// userspace mounters like nbd need the device path as a reference while
	// restarting the userspace processes on a nodeplugin restart. For kernel
	// mounter(krbd) we don't need it as there won't be any process running
	// in userspace, hence we don't store the device path for krbd devices.
	if volOptions.Mounter == rbdNbdMounter {
		err = updateRBDImageMetadataStash(req.GetStagingTargetPath(), devicePath)
		if err != nil {
			return transaction, err
		}
	}

	if volOptions.isBlockEncrypted() {
		devicePath, err = ns.processEncryptedDevice(ctx, volOptions, devicePath)
		if err != nil {
			return transaction, err
		}
		transaction.isBlockEncrypted = true
	}

	if volOptions.isFileEncrypted() {
		if err = fscrypt.InitializeNode(ctx); err != nil {
			return transaction, fmt.Errorf("file encryption setup for %s failed: %w", volOptions.VolID, err)
		}
	}

	stagingTargetPath := getStagingTargetPath(req)

	isBlock := req.GetVolumeCapability().GetBlock() != nil
	err = ns.createStageMountPoint(ctx, stagingTargetPath, isBlock)
	if err != nil {
		return transaction, err
	}

	transaction.isStagePathCreated = true

	// nodeStage Path
	err = ns.mountVolumeToStagePath(ctx, req, staticVol, stagingTargetPath, devicePath, volOptions.isFileEncrypted())
	if err != nil {
		return transaction, err
	}
	transaction.isMounted = true

	if volOptions.isFileEncrypted() {
		log.DebugLog(ctx, "rbd fscrypt: trying to unlock filesystem on %s image %s", stagingTargetPath, volOptions.VolID)
		err = fscrypt.Unlock(ctx, volOptions.fileEncryption, stagingTargetPath, volOptions.VolID)
		if err != nil {
			return transaction, fmt.Errorf("file system encryption unlock in %s image %s failed: %w",
				stagingTargetPath, volOptions.VolID, err)
		}
	}

	// As we are supporting the restore of a volume to a bigger size and
	// creating bigger size clone from a volume, we need to check filesystem
	// resize is required, if required resize filesystem.
	// in case of encrypted block PVC resize only the LUKS device.
	err = resizeNodeStagePath(ctx, isBlock, transaction, req.GetVolumeId(), stagingTargetPath)
	if err != nil {
		return transaction, err
	}

	return transaction, err
}

// resizeNodeStagePath resizes the device if its encrypted and it also resizes
// the stagingTargetPath if filesystem needs resize.
func resizeNodeStagePath(ctx context.Context,
	isBlock bool,
	transaction *stageTransaction,
	volID,
	stagingTargetPath string,
) error {
	var err error
	devicePath := transaction.devicePath
	var ok bool

	// if its a non encrypted block device we dont need any expansion
	if isBlock && !transaction.isBlockEncrypted {
		return nil
	}

	resizer := mount.NewResizeFs(utilexec.New())

	if transaction.isBlockEncrypted {
		devicePath, err = resizeEncryptedDevice(ctx, volID, stagingTargetPath, devicePath)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}
	// check stagingPath needs resize.
	ok, err = resizer.NeedResize(devicePath, stagingTargetPath)
	if err != nil {
		return status.Errorf(codes.Internal,
			"need resize check failed on devicePath %s and staingPath %s, error: %v",
			devicePath,
			stagingTargetPath,
			err)
	}
	// return nil if no resize is required
	if !ok {
		return nil
	}
	ok, err = resizer.Resize(devicePath, stagingTargetPath)
	if !ok {
		return status.Errorf(codes.Internal,
			"resize failed on path %s, error: %v", stagingTargetPath, err)
	}

	return nil
}

func resizeEncryptedDevice(ctx context.Context, volID, stagingTargetPath, devicePath string) (string, error) {
	rbdDevSize, err := getDeviceSize(ctx, devicePath)
	if err != nil {
		return "", fmt.Errorf(
			"failed to get device size of %s and staingPath %s, error: %w",
			devicePath,
			stagingTargetPath,
			err)
	}
	_, mapperPath := util.VolumeMapper(volID)
	encDevSize, err := getDeviceSize(ctx, mapperPath)
	if err != nil {
		return "", fmt.Errorf(
			"failed to get device size of %s and staingPath %s, error: %w",
			mapperPath,
			stagingTargetPath,
			err)
	}
	// if the rbd device `/dev/rbd0` size is greater than LUKS device size
	// we need to resize the LUKS device.
	if rbdDevSize > encDevSize {
		// The volume is encrypted, resize an active mapping
		err = util.ResizeEncryptedVolume(ctx, mapperPath)
		if err != nil {
			log.ErrorLog(ctx, "failed to resize device %s: %v",
				mapperPath, err)

			return "", fmt.Errorf(
				"failed to resize device %s: %w", mapperPath, err)
		}
	}

	return mapperPath, nil
}

func flattenImageBeforeMapping(
	ctx context.Context,
	volOptions *rbdVolume,
) error {
	var err error
	var feature bool
	var depth uint

	if kernelRelease == "" {
		// fetch the current running kernel info
		kernelRelease, err = util.GetKernelVersion()
		if err != nil {
			return err
		}
	}
	if !util.CheckKernelSupport(kernelRelease, deepFlattenSupport) && !skipForceFlatten {
		feature, err = volOptions.checkImageChainHasFeature(ctx, librbd.FeatureDeepFlatten)
		if err != nil {
			return err
		}
		depth, err = volOptions.getCloneDepth(ctx)
		if err != nil {
			return err
		}
		if feature || depth != 0 {
			err = volOptions.flattenRbdImage(ctx, true, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (ns *NodeServer) undoStagingTransaction(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
	transaction *stageTransaction,
	volOptions *rbdVolume,
) {
	var err error

	stagingTargetPath := getStagingTargetPath(req)
	if transaction.isMounted {
		err = ns.Mounter.Unmount(stagingTargetPath)
		if err != nil {
			log.ErrorLog(ctx, "failed to unmount stagingtargetPath: %s with error: %v", stagingTargetPath, err)

			return
		}
	}

	// remove the file/directory created on staging path
	if transaction.isStagePathCreated {
		err = os.Remove(stagingTargetPath)
		if err != nil {
			log.ErrorLog(ctx, "failed to remove stagingtargetPath: %s with error: %v", stagingTargetPath, err)
			// continue on failure to unmap the image, as leaving stale images causes more issues than a stale
			// file/directory
		}
	}

	volID := req.GetVolumeId()

	// Unmapping rbd device
	if transaction.devicePath != "" {
		err = detachRBDDevice(ctx, transaction.devicePath, volID, volOptions.UnmapOptions, transaction.isBlockEncrypted)
		if err != nil {
			log.ErrorLog(
				ctx,
				"failed to unmap rbd device: %s for volume %s with error: %v",
				transaction.devicePath,
				volID,
				err)
			// continue on failure to delete the stash file, as kubernetes will fail to delete the staging path
			// otherwise
		}
	}

	// Cleanup the stashed image metadata
	if err = cleanupRBDImageMetadataStash(req.GetStagingTargetPath()); err != nil {
		log.ErrorLog(ctx, "failed to cleanup image metadata stash (%v)", err)

		return
	}
}

func (ns *NodeServer) createStageMountPoint(ctx context.Context, mountPath string, isBlock bool) error {
	if isBlock {
		// #nosec:G304, intentionally creating file mountPath, not a security issue
		pathFile, err := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			log.ErrorLog(ctx, "failed to create mountPath:%s with error: %v", mountPath, err)

			return status.Error(codes.Internal, err.Error())
		}
		if err = pathFile.Close(); err != nil {
			log.ErrorLog(ctx, "failed to close mountPath:%s with error: %v", mountPath, err)

			return status.Error(codes.Internal, err.Error())
		}

		return nil
	}

	err := os.Mkdir(mountPath, 0o750)
	if err != nil {
		if !os.IsExist(err) {
			log.ErrorLog(ctx, "failed to create mountPath:%s with error: %v", mountPath, err)

			return status.Error(codes.Internal, err.Error())
		}
	}

	return nil
}

// NodePublishVolume mounts the volume mounted to the device path to the target
// path.
func (ns *NodeServer) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	err := util.ValidateNodePublishVolumeRequest(req)
	if err != nil {
		return nil, err
	}
	targetPath := req.GetTargetPath()
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	stagingPath := req.GetStagingTargetPath()
	volID := req.GetVolumeId()
	stagingPath += "/" + volID

	// Considering kubelet make sure the stage and publish operations
	// are serialized, we dont need any extra locking in nodePublish

	// Check if that target path exists properly
	notMnt, err := ns.createTargetMountPath(ctx, targetPath, isBlock)
	if err != nil {
		return nil, err
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	fileEncrypted, err := IsFileEncrypted(ctx, req.GetVolumeContext())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if fileEncrypted {
		stagingPath = fscrypt.AppendEncyptedSubdirectory(stagingPath)
		if err = fscrypt.IsDirectoryUnlocked(stagingPath, req.GetVolumeCapability().GetMount().GetFsType()); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	// Publish Path
	err = ns.mountVolume(ctx, stagingPath, req)
	if err != nil {
		return nil, err
	}

	log.DebugLog(ctx, "rbd: successfully mounted stagingPath %s to targetPath %s", stagingPath, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeServer) mountVolumeToStagePath(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest,
	staticVol bool,
	stagingPath, devicePath string,
	fileEncryption bool,
) error {
	readOnly := false
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	diskMounter := &mount.SafeFormatAndMount{Interface: ns.Mounter, Exec: utilexec.New()}
	// rbd images are thin-provisioned and return zeros for unwritten areas.  A freshly created
	// image will not benefit from discard and we also want to avoid as much unnecessary zeroing
	// as possible.  Open-code mkfs here because FormatAndMount() doesn't accept custom mkfs
	// options.
	//
	// Note that "freshly" is very important here.  While discard is more of a nice to have,
	// lazy_journal_init=1 is plain unsafe if the image has been written to before and hasn't
	// been zeroed afterwards (unlike the name suggests, it leaves the journal completely
	// uninitialized and carries a risk until the journal is overwritten and wraps around for
	// the first time).
	existingFormat, err := diskMounter.GetDiskFormat(devicePath)
	if err != nil {
		log.ErrorLog(ctx, "failed to get disk format for path %s, error: %v", devicePath, err)

		return err
	}

	opt := mountDefaultOpts[fsType]
	opt = append(opt, "_netdev")
	opt = csicommon.ConstructMountOptions(opt, req.GetVolumeCapability())
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	rOnly := "ro"

	if req.VolumeCapability.AccessMode.Mode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY ||
		req.VolumeCapability.AccessMode.Mode == csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY {
		if !csicommon.MountOptionContains(opt, rOnly) {
			opt = append(opt, rOnly)
		}
	}
	if csicommon.MountOptionContains(opt, rOnly) {
		readOnly = true
	}

	if existingFormat == "" && !staticVol && !readOnly && !isBlock {
		args := mkfsDefaultArgs[fsType]

		// if the VolumeContext contains "mkfsOptions", use those as args instead
		volumeCtx := req.GetVolumeContext()
		if volumeCtx != nil {
			mkfsOptions := volumeCtx["mkfsOptions"]
			if mkfsOptions != "" {
				args = strings.Split(mkfsOptions, " ")
			}
		}

		// add extra arguments depending on the filesystem
		mkfs := "mkfs." + fsType
		switch fsType {
		case "ext4":
			if fileEncryption {
				args = append(args, "-Oencrypt")
			}
		case "xfs":
			// always disable reflink
			// TODO: make enabling an option, see ceph/ceph-csi#1256
			if ns.xfsSupportsReflink() {
				args = append(args, "-m", "reflink=0")
			}
		case "":
			// no filesystem type specified, just use "mkfs"
			mkfs = "mkfs"
		}

		// add device as last argument
		args = append(args, devicePath)
		cmdOut, cmdErr := diskMounter.Exec.Command(mkfs, args...).CombinedOutput()
		if cmdErr != nil {
			log.ErrorLog(ctx, "failed to run mkfs.%s (%v) error: %v, output: %v", fsType, args, cmdErr, string(cmdOut))

			return cmdErr
		}
	}

	if isBlock {
		opt = append(opt, "bind")
		err = diskMounter.MountSensitiveWithoutSystemd(devicePath, stagingPath, fsType, opt, nil)
	} else {
		err = diskMounter.FormatAndMount(devicePath, stagingPath, fsType, opt)
	}
	if err != nil {
		log.ErrorLog(ctx,
			"failed to mount device path (%s) to staging path (%s) for volume "+
				"(%s) error: %s Check dmesg logs if required.",
			devicePath,
			stagingPath,
			req.GetVolumeId(),
			err)
	}

	return err
}

func (ns *NodeServer) mountVolume(ctx context.Context, stagingPath string, req *csi.NodePublishVolumeRequest) error {
	// Publish Path
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	readOnly := req.GetReadonly()
	mountOptions := []string{"bind", "_netdev"}
	isBlock := req.GetVolumeCapability().GetBlock() != nil
	targetPath := req.GetTargetPath()

	mountOptions = csicommon.ConstructMountOptions(mountOptions, req.GetVolumeCapability())

	log.DebugLog(ctx, "target %v\nisBlock %v\nfstype %v\nstagingPath %v\nreadonly %v\nmountflags %v\n",
		targetPath, isBlock, fsType, stagingPath, readOnly, mountOptions)

	if readOnly {
		mountOptions = append(mountOptions, "ro")
	}
	if err := util.Mount(ns.Mounter, stagingPath, targetPath, fsType, mountOptions); err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (ns *NodeServer) createTargetMountPath(ctx context.Context, mountPath string, isBlock bool) (bool, error) {
	// Check if that mount path exists properly
	notMnt, err := ns.Mounter.IsLikelyNotMountPoint(mountPath)
	if err == nil {
		return notMnt, nil
	}
	if !os.IsNotExist(err) {
		return false, status.Error(codes.Internal, err.Error())
	}
	if isBlock {
		// #nosec
		pathFile, e := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0o750)
		if e != nil {
			log.DebugLog(ctx, "Failed to create mountPath:%s with error: %v", mountPath, err)

			return notMnt, status.Error(codes.Internal, e.Error())
		}
		if err = pathFile.Close(); err != nil {
			log.DebugLog(ctx, "Failed to close mountPath:%s with error: %v", mountPath, err)

			return notMnt, status.Error(codes.Internal, err.Error())
		}
	} else {
		// Create a mountpath directory
		if err = util.CreateMountPoint(mountPath); err != nil {
			return notMnt, status.Error(codes.Internal, err.Error())
		}
	}
	notMnt = true

	return notMnt, err
}

// NodeUnpublishVolume unmounts the volume from the target path.
func (ns *NodeServer) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {
	err := util.ValidateNodeUnpublishVolumeRequest(req)
	if err != nil {
		return nil, err
	}

	targetPath := req.GetTargetPath()
	// considering kubelet make sure node operations like unpublish/unstage...etc can not be called
	// at same time, an explicit locking at time of nodeunpublish is not required.
	isMnt, err := ns.Mounter.IsMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// targetPath has already been deleted
			log.DebugLog(ctx, "targetPath: %s has already been deleted", targetPath)

			return &csi.NodeUnpublishVolumeResponse{}, nil
		}

		return nil, status.Error(codes.NotFound, err.Error())
	}
	if !isMnt {
		if err = os.RemoveAll(targetPath); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if err = ns.Mounter.Unmount(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err = os.RemoveAll(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "rbd: successfully unbound volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// getStagingTargetPath concats either NodeStageVolumeRequest's or
// NodeUnstageVolumeRequest's target path with the volumeID.
func getStagingTargetPath(req interface{}) string {
	switch vr := req.(type) {
	case *csi.NodeStageVolumeRequest:
		return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()
	case *csi.NodeUnstageVolumeRequest:
		return vr.GetStagingTargetPath() + "/" + vr.GetVolumeId()
	}

	return ""
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

	if acquired := ns.VolumeLocks.TryAcquire(volID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ns.VolumeLocks.Release(volID)

	stagingParentPath := req.GetStagingTargetPath()
	stagingTargetPath := getStagingTargetPath(req)

	isMnt, err := ns.Mounter.IsMountPoint(stagingTargetPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		// Continue on ENOENT errors as we may still have the image mapped
		isMnt = false
	}
	if isMnt {
		// Unmounting the image
		err = ns.Mounter.Unmount(stagingTargetPath)
		if err != nil {
			log.ExtendedLog(ctx, "failed to unmount targetPath: %s with error: %v", stagingTargetPath, err)

			return nil, status.Error(codes.Internal, err.Error())
		}
		log.DebugLog(ctx, "successfully unmounted volume (%s) from staging path (%s)",
			req.GetVolumeId(), stagingTargetPath)
	}

	if err = os.Remove(stagingTargetPath); err != nil {
		// Any error is critical as Staging path is expected to be empty by Kubernetes, it otherwise
		// keeps invoking Unstage. Hence any errors removing files within this path is a critical
		// error
		if !os.IsNotExist(err) {
			log.ErrorLog(ctx, "failed to remove staging target path (%s): (%v)", stagingTargetPath, err)

			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	imgInfo, err := lookupRBDImageMetadataStash(stagingParentPath)
	if err != nil {
		log.UsefulLog(ctx, "failed to find image metadata: %v", err)
		// It is an error if it was mounted, as we should have found the image metadata file with
		// no errors
		if isMnt {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// If not mounted, and error is anything other than metadata file missing, it is an error
		if !errors.Is(err, ErrMissingStash) {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// It was not mounted and image metadata is also missing, we are done as the last step in
		// the staging transaction is complete
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	// Unmapping rbd device
	imageSpec := imgInfo.String()

	dArgs := detachRBDImageArgs{
		imageOrDeviceSpec: imageSpec,
		isImageSpec:       true,
		isNbd:             imgInfo.NbdAccess,
		encrypted:         imgInfo.Encrypted,
		volumeID:          req.GetVolumeId(),
		unmapOptions:      imgInfo.UnmapOptions,
		logDir:            imgInfo.LogDir,
		logStrategy:       imgInfo.LogStrategy,
	}
	if err = detachRBDImageOrDeviceSpec(ctx, &dArgs); err != nil {
		log.ErrorLog(
			ctx,
			"error unmapping volume (%s) from staging path (%s): (%v)",
			req.GetVolumeId(),
			stagingTargetPath,
			err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	log.DebugLog(ctx, "successfully unmapped volume (%s)", req.GetVolumeId())

	if err = cleanupRBDImageMetadataStash(stagingParentPath); err != nil {
		log.ErrorLog(ctx, "failed to cleanup image metadata stash (%v)", err)

		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// getStagingPath returns the staging path for the volume from the volume path.
// The staingTargetPath looks like
// /var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-08937eb8-7e00-4033-b6ce-bc36147b4ed0/
// globalmount/0001-0009-rook-ceph-0000000000000002-50d53503-9da1-11ed-847e-bacf8f2f1297
// the last directory is the volumeID returned in the NodeStageVolumeRespose.
// The image-meta.json file is present in the staging path without volumeID.
func (ns *NodeServer) getStagingPath(volPath string) (string, error) {
	mounts, err := ns.Mounter.GetMountRefs(volPath)
	if err != nil {
		return "", err
	}
	for _, mount := range mounts {
		// strip the last directory from the staging path
		stp := strings.Split(mount, "/")
		stagingTargetPath := strings.Join(stp[:len(stp)-1], "/")
		if checkRBDImageMetadataStashExists(stagingTargetPath) {
			return stagingTargetPath, nil
		}
	}

	return "", fmt.Errorf("failed to get staging path for volume %s", volPath)
}

// NodeExpandVolume resizes rbd volumes.
func (ns *NodeServer) NodeExpandVolume(
	ctx context.Context,
	req *csi.NodeExpandVolumeRequest,
) (*csi.NodeExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}

	// Get volume path
	// With Kubernetes version>=v1.19.0, expand request carries volume_path and
	// staging_target_path, what csi requires is staging_target_path.
	volumePath := req.GetStagingTargetPath()
	if volumePath == "" {
		// If Kubernetes version < v1.19.0 the volume_path would be
		// having the staging_target_path information.
		// stagingTargetPath is optional parameter in NodeExpandVolumeRequest request
		// Kubernetes will not send the staging_target_path in the volume_path in some cases
		// Refer https://github.com/kubernetes/kubernetes/issues/115343
		var err error
		volumePath, err = ns.getStagingPath(req.GetVolumePath())
		if err != nil {
			// If stagingTargetPath is not found in volumePath then use
			// volumePath
			volumePath = req.GetVolumePath()
			log.UsefulLog(ctx, "failed to find stagingTargetPath from volumePath %v: %v", volumePath, err)
		}
	}
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path must be provided")
	}

	if acquired := ns.VolumeLocks.TryAcquire(volumeID); !acquired {
		log.ErrorLog(ctx, util.VolumeOperationAlreadyExistsFmt, volumeID)

		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer ns.VolumeLocks.Release(volumeID)

	imgInfo, err := lookupRBDImageMetadataStash(volumePath)
	if err != nil {
		log.ErrorLog(ctx, "failed to find image metadata: %v", err)

		return nil, status.Error(codes.Internal, err.Error())
	}
	devicePath, found := findDeviceMappingImage(
		ctx,
		imgInfo.Pool,
		imgInfo.RadosNamespace,
		imgInfo.ImageName,
		imgInfo.NbdAccess)
	if !found {
		return nil, status.Errorf(codes.Internal,
			"failed to get device for stagingtarget path %v", volumePath)
	}

	mapperFile, mapperPath := util.VolumeMapper(volumeID)
	if imgInfo.Encrypted {
		// The volume is encrypted, resize an active mapping
		err = util.ResizeEncryptedVolume(ctx, mapperFile)
		if err != nil {
			log.ErrorLog(ctx, "failed to resize device %s, mapper %s: %w",
				devicePath, mapperFile, err)

			return nil, status.Errorf(codes.Internal,
				"failed to resize device %s, mapper %s: %v", devicePath, mapperFile, err)
		}
		// Use mapper device path for fs resize
		devicePath = mapperPath
	}

	if req.GetVolumeCapability().GetBlock() == nil {
		// TODO check size and return success or error
		volumePath += "/" + volumeID
		resizer := mount.NewResizeFs(utilexec.New())
		var ok bool
		ok, err = resizer.Resize(devicePath, volumePath)
		if !ok {
			return nil, status.Errorf(codes.Internal,
				"rbd: resize failed on path %s, error: %v", req.GetVolumePath(), err)
		}
	}

	return &csi.NodeExpandVolumeResponse{}, nil
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
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
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

func (ns *NodeServer) processEncryptedDevice(
	ctx context.Context,
	volOptions *rbdVolume,
	devicePath string,
) (string, error) {
	imageSpec := volOptions.String()
	encrypted, err := volOptions.checkRbdImageEncrypted(ctx)
	if err != nil {
		log.ErrorLog(ctx, "failed to get encryption status for rbd image %s: %v",
			imageSpec, err)

		return "", err
	}

	switch {
	case encrypted == rbdImageRequiresEncryption:
		// If we get here, it means the image was created with a
		// ceph-csi version that creates a passphrase for the encrypted
		// device in NodeStage. New versions moved that to
		// CreateVolume.
		// Use the same setupEncryption() as CreateVolume does, and
		// continue with the common process to crypt-format the device.
		err = volOptions.setupBlockEncryption(ctx)
		if err != nil {
			log.ErrorLog(ctx, "failed to setup encryption for rbd"+
				"image %s: %v", imageSpec, err)

			return "", err
		}

		// make sure we continue with the encrypting of the device
		fallthrough
	case encrypted == rbdImageEncryptionPrepared:
		diskMounter := &mount.SafeFormatAndMount{Interface: ns.Mounter, Exec: utilexec.New()}
		// TODO: update this when adding support for static (pre-provisioned) PVs
		var existingFormat string
		existingFormat, err = diskMounter.GetDiskFormat(devicePath)
		if err != nil {
			return "", fmt.Errorf("failed to get disk format for path %s: %w", devicePath, err)
		}

		switch existingFormat {
		case "":
			err = volOptions.encryptDevice(ctx, devicePath)
			if err != nil {
				return "", fmt.Errorf("failed to encrypt rbd image %s: %w", imageSpec, err)
			}
		case "crypt", "crypto_LUKS":
			log.WarningLog(ctx, "rbd image %s is encrypted, but encryption state was not updated",
				imageSpec)
			err = volOptions.ensureEncryptionMetadataSet(rbdImageEncrypted)
			if err != nil {
				return "", fmt.Errorf("failed to update encryption state for rbd image %s", imageSpec)
			}
		default:
			return "", fmt.Errorf("can not encrypt rbdImage %s that already has file system: %s",
				imageSpec, existingFormat)
		}
	case encrypted != rbdImageEncrypted:
		return "", fmt.Errorf("rbd image %s found mounted with unexpected encryption status %s",
			imageSpec, encrypted)
	}

	devicePath, err = volOptions.openEncryptedDevice(ctx, devicePath)
	if err != nil {
		return "", err
	}

	return devicePath, nil
}

// xfsSupportsReflink checks if mkfs.xfs supports the "-m reflink=0|1"
// argument. In case it is supported, return true.
func (ns *NodeServer) xfsSupportsReflink() bool {
	// return cached value, if set
	if xfsHasReflink != xfsReflinkUnset {
		return xfsHasReflink == xfsReflinkSupport
	}

	// run mkfs.xfs in the same namespace as formatting would be done in
	// mountVolumeToStagePath()
	diskMounter := &mount.SafeFormatAndMount{Interface: ns.Mounter, Exec: utilexec.New()}
	out, err := diskMounter.Exec.Command("mkfs.xfs").CombinedOutput()
	if err != nil {
		// mkfs.xfs should fail with an error message (and help text)
		if strings.Contains(string(out), "reflink=0|1") {
			xfsHasReflink = xfsReflinkSupport

			return true
		}
	}

	xfsHasReflink = xfsReflinkNoSupport

	return false
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
		return csicommon.FilesystemNodeGetVolumeStats(ctx, ns.Mounter, targetPath, true)
	} else if (stat.Mode() & os.ModeDevice) == os.ModeDevice {
		return blockNodeGetVolumeStats(ctx, targetPath)
	}

	return nil, fmt.Errorf("targetpath %q is not a block device", targetPath)
}

// blockNodeGetVolumeStats gets the metrics for a `volumeMode: Block` type of
// volume. At the moment, only the size of the block-device can be returned, as
// there are no secrets in the NodeGetVolumeStats request that enables us to
// connect to the Ceph cluster.
//
// TODO: https://github.com/container-storage-interface/spec/issues/371#issuecomment-756834471
func blockNodeGetVolumeStats(ctx context.Context, targetPath string) (*csi.NodeGetVolumeStatsResponse, error) {
	mp := volume.NewMetricsBlock(targetPath)
	m, err := mp.GetMetrics()
	if err != nil {
		err = fmt.Errorf("failed to get metrics: %w", err)
		log.ErrorLog(ctx, err.Error())

		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Total: m.Capacity.Value(),
				Unit:  csi.VolumeUsage_BYTES,
			},
		},
	}, nil
}

// getDeviceSize gets the block device size.
func getDeviceSize(ctx context.Context, devicePath string) (uint64, error) {
	output, _, err := util.ExecCommand(ctx, "blockdev", "--getsize64", devicePath)
	if err != nil {
		return 0, fmt.Errorf("blockdev %v returned an error: %w", devicePath, err)
	}

	outStr := strings.TrimSpace(output)
	size, err := strconv.ParseUint(outStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse size of device %s %s: %w", devicePath, outStr, err)
	}

	return size, nil
}

func (ns *NodeServer) SetReadAffinityMapOptions(crushLocationMap map[string]string) {
	if len(crushLocationMap) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString("read_from_replica=localize,crush_location=")
	first := true
	for key, val := range crushLocationMap {
		if first {
			b.WriteString(fmt.Sprintf("%s:%s", key, val))
			first = false
		} else {
			b.WriteString(fmt.Sprintf("|%s:%s", key, val))
		}
	}
	ns.readAffinityMapOptions = b.String()
}
