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

package store

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/ceph/ceph-csi/internal/cephfs/core"
	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	kmsapi "github.com/ceph/ceph-csi/internal/kms"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/k8s"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	cephfsDefaultEncryptionType = util.EncryptionTypeFile
)

type VolumeOptions struct {
	core.SubVolume

	RequestName  string
	NamePrefix   string
	ClusterID    string
	MetadataPool string
	// ReservedID represents the ID reserved for a subvolume
	ReservedID           string
	Monitors             string `json:"monitors"`
	RootPath             string `json:"rootPath"`
	Mounter              string `json:"mounter"`
	BackingSnapshotRoot  string // Snapshot root relative to RootPath.
	BackingSnapshotID    string
	KernelMountOptions   string `json:"kernelMountOptions"`
	FuseMountOptions     string `json:"fuseMountOptions"`
	NetNamespaceFilePath string
	TopologyPools        *[]util.TopologyConstrainedPool
	TopologyRequirement  *csi.TopologyRequirement
	Topology             map[string]string
	FscID                int64

	// Encryption provides access to optional VolumeEncryption functions
	Encryption *util.VolumeEncryption
	// Owner is the creator (tenant, Kubernetes Namespace) of the volume
	Owner string

	// conn is a connection to the Ceph cluster obtained from a ConnPool
	conn *util.ClusterConnection

	ProvisionVolume bool `json:"provisionVolume"`
	BackingSnapshot bool `json:"backingSnapshot"`
}

// Connect a CephFS volume to the Ceph cluster.
func (vo *VolumeOptions) Connect(cr *util.Credentials) error {
	if vo.conn != nil {
		return nil
	}

	conn := &util.ClusterConnection{}
	if err := conn.Connect(vo.Monitors, cr); err != nil {
		return err
	}

	vo.conn = conn

	return nil
}

// Destroy cleans up the CephFS volume object and closes the connection to the
// Ceph cluster in case one was setup.
func (vo *VolumeOptions) Destroy() {
	if vo.conn != nil {
		vo.conn.Destroy()
	}
	if vo.IsEncrypted() {
		vo.Encryption.Destroy()
	}
}

func validateNonEmptyField(field, fieldName string) error {
	if field == "" {
		return fmt.Errorf("parameter '%s' cannot be empty", fieldName)
	}

	return nil
}

func extractOptionalOption(dest *string, optionLabel string, options map[string]string) error {
	opt, ok := options[optionLabel]
	if !ok {
		// Option not found, no error as it is optional
		return nil
	}

	if err := validateNonEmptyField(opt, optionLabel); err != nil {
		return err
	}

	*dest = opt

	return nil
}

func extractOption(dest *string, optionLabel string, options map[string]string) error {
	opt, ok := options[optionLabel]
	if !ok {
		return fmt.Errorf("missing required field %s", optionLabel)
	}

	if err := validateNonEmptyField(opt, optionLabel); err != nil {
		return err
	}

	*dest = opt

	return nil
}

func validateMounter(m string) error {
	switch m {
	case "fuse":
	case "kernel":
	default:
		return fmt.Errorf("unknown mounter '%s'. Valid options are 'fuse' and 'kernel'", m)
	}

	return nil
}

func extractMounter(dest *string, options map[string]string) error {
	if err := extractOptionalOption(dest, "mounter", options); err != nil {
		return err
	}

	if *dest != "" {
		if err := validateMounter(*dest); err != nil {
			return err
		}
	}

	return nil
}

func GetClusterInformation(options map[string]string) (*util.ClusterInfo, error) {
	clusterID, ok := options["clusterID"]
	if !ok {
		err := fmt.Errorf("clusterID must be set")

		return nil, err
	}

	if err := validateNonEmptyField(clusterID, "clusterID"); err != nil {
		return nil, err
	}

	monitors, err := util.Mons(util.CsiConfigFile, clusterID)
	if err != nil {
		err = fmt.Errorf("failed to fetch monitor list using clusterID (%s): %w", clusterID, err)

		return nil, err
	}

	subvolumeGroup, err := util.CephFSSubvolumeGroup(util.CsiConfigFile, clusterID)
	if err != nil {
		err = fmt.Errorf("failed to fetch subvolumegroup using clusterID (%s): %w", clusterID, err)

		return nil, err
	}
	clusterData := &util.ClusterInfo{
		ClusterID: clusterID,
		Monitors:  strings.Split(monitors, ","),
	}
	clusterData.CephFS.SubvolumeGroup = subvolumeGroup

	return clusterData, nil
}

// GetConnection returns the cluster connection.
func (vo *VolumeOptions) GetConnection() *util.ClusterConnection {
	return vo.conn
}

func fmtBackingSnapshotOptionMismatch(optName, expected, actual string) error {
	return fmt.Errorf("%s option mismatch with backing snapshot: got %s, expected %s",
		optName, actual, expected)
}

// NewVolumeOptions generates a new instance of volumeOptions from the provided
// CSI request parameters.
//
//nolint:gocyclo,cyclop // TODO: reduce complexity
func NewVolumeOptions(
	ctx context.Context,
	requestName,
	clusterName string,
	setMetadata bool,
	req *csi.CreateVolumeRequest,
	cr *util.Credentials,
) (*VolumeOptions, error) {
	var (
		opts                VolumeOptions
		backingSnapshotBool string
		err                 error
	)

	volOptions := req.GetParameters()
	clusterData, err := GetClusterInformation(volOptions)
	if err != nil {
		return nil, err
	}

	opts.ClusterID = clusterData.ClusterID
	opts.Monitors = strings.Join(clusterData.Monitors, ",")
	opts.SubvolumeGroup = clusterData.CephFS.SubvolumeGroup
	opts.Owner = k8s.GetOwner(volOptions)
	opts.BackingSnapshot = IsShallowVolumeSupported(req)

	if err = extractOptionalOption(&opts.Pool, "pool", volOptions); err != nil {
		return nil, err
	}

	if err = extractMounter(&opts.Mounter, volOptions); err != nil {
		return nil, err
	}

	if err = extractOption(&opts.FsName, "fsName", volOptions); err != nil {
		return nil, err
	}

	if err = extractOptionalOption(&opts.KernelMountOptions, "kernelMountOptions", volOptions); err != nil {
		return nil, err
	}

	if err = extractOptionalOption(&opts.FuseMountOptions, "fuseMountOptions", volOptions); err != nil {
		return nil, err
	}

	if err = extractOptionalOption(&opts.NamePrefix, "volumeNamePrefix", volOptions); err != nil {
		return nil, err
	}

	if err = extractOptionalOption(&backingSnapshotBool, "backingSnapshot", volOptions); err != nil {
		return nil, err
	}

	if err = opts.InitKMS(ctx, volOptions, req.GetSecrets()); err != nil {
		return nil, fmt.Errorf("failed to init KMS: %w", err)
	}

	if backingSnapshotBool != "" {
		if opts.BackingSnapshot, err = strconv.ParseBool(backingSnapshotBool); err != nil {
			return nil, fmt.Errorf("failed to parse backingSnapshot: %w", err)
		}
	}

	opts.RequestName = requestName

	err = opts.Connect(cr)
	if err != nil {
		return nil, err
	}

	fs := core.NewFileSystem(opts.conn)
	opts.FscID, err = fs.GetFscID(ctx, opts.FsName)
	if err != nil {
		return nil, err
	}

	opts.MetadataPool, err = fs.GetMetadataPool(ctx, opts.FsName)
	if err != nil {
		return nil, err
	}

	// store topology information from the request
	opts.TopologyPools, opts.TopologyRequirement, err = util.GetTopologyFromRequest(req)
	if err != nil {
		return nil, err
	}

	// TODO: we need an API to fetch subvolume attributes (size/datapool and others), based
	// on which we can evaluate which topology this belongs to.
	// CephFS tracker: https://tracker.ceph.com/issues/44277
	if opts.TopologyPools != nil {
		return nil, errors.New("topology based provisioning is not supported for CephFS backed volumes")
	}

	opts.ProvisionVolume = true

	if opts.BackingSnapshot {
		if req.GetVolumeContentSource() == nil || req.GetVolumeContentSource().GetSnapshot() == nil {
			return nil, errors.New("backingSnapshot option requires snapshot volume source")
		}

		opts.BackingSnapshotID = req.GetVolumeContentSource().GetSnapshot().GetSnapshotId()

		err = opts.populateVolumeOptionsFromBackingSnapshot(ctx, cr, req.GetSecrets(), clusterName, setMetadata)
		if err != nil {
			return nil, err
		}
	}

	return &opts, nil
}

// IsShallowVolumeSupported returns true only for ReadOnly volume requests
// with datasource as snapshot.
func IsShallowVolumeSupported(req *csi.CreateVolumeRequest) bool {
	isRO := IsVolumeCreateRO(req.VolumeCapabilities)

	return isRO && (req.GetVolumeContentSource() != nil && req.GetVolumeContentSource().GetSnapshot() != nil)
}

func IsVolumeCreateRO(caps []*csi.VolumeCapability) bool {
	for _, cap := range caps {
		if cap.AccessMode != nil {
			switch cap.AccessMode.Mode { //nolint:exhaustive // only check what we want
			case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
				csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY:
				return true
			}
		}
	}

	return false
}

// newVolumeOptionsFromVolID generates a new instance of volumeOptions and VolumeIdentifier
// from the provided CSI VolumeID.
//
//nolint:gocyclo,cyclop // TODO: reduce complexity
func NewVolumeOptionsFromVolID(
	ctx context.Context,
	volID string,
	volOpt, secrets map[string]string,
	clusterName string,
	setMetadata bool,
) (*VolumeOptions, *VolumeIdentifier, error) {
	var (
		vi         util.CSIIdentifier
		volOptions VolumeOptions
		vid        VolumeIdentifier
	)

	// Decode the VolID first, to detect older volumes or pre-provisioned volumes
	// before other errors
	err := vi.DecomposeCSIID(volID)
	if err != nil {
		err = fmt.Errorf("error decoding volume ID (%s): %w", volID, err)

		return nil, nil, util.JoinErrors(cerrors.ErrInvalidVolID, err)
	}
	volOptions.ClusterID = vi.ClusterID
	vid.VolumeID = volID
	volOptions.VolID = volID
	volOptions.FscID = vi.LocationID

	if volOptions.Monitors, err = util.Mons(util.CsiConfigFile, vi.ClusterID); err != nil {
		return nil, nil, fmt.Errorf("failed to fetch monitor list using clusterID (%s): %w", vi.ClusterID, err)
	}

	if volOptions.SubvolumeGroup, err = util.CephFSSubvolumeGroup(util.CsiConfigFile, vi.ClusterID); err != nil {
		return nil, nil, fmt.Errorf("failed to fetch subvolumegroup list using clusterID (%s): %w", vi.ClusterID, err)
	}

	cr, err := util.NewAdminCredentials(secrets)
	if err != nil {
		return nil, nil, err
	}
	defer cr.DeleteCredentials()

	err = volOptions.Connect(cr)
	if err != nil {
		return nil, nil, err
	}
	// in case of an error, volOptions is not returned, release any
	// resources that may have been allocated
	defer func() {
		if err != nil {
			volOptions.Destroy()
		}
	}()

	fs := core.NewFileSystem(volOptions.conn)
	volOptions.FsName, err = fs.GetFsName(ctx, volOptions.FscID)
	if err != nil {
		return nil, nil, err
	}

	volOptions.MetadataPool, err = fs.GetMetadataPool(ctx, volOptions.FsName)
	if err != nil {
		return nil, nil, err
	}

	// Connect to cephfs' default radosNamespace (csi)
	j, err := VolJournal.Connect(volOptions.Monitors, fsutil.RadosNamespace, cr)
	if err != nil {
		return nil, nil, err
	}
	defer j.Destroy()

	imageAttributes, err := j.GetImageAttributes(
		ctx, volOptions.MetadataPool, vi.ObjectUUID, false)
	if err != nil {
		return nil, nil, err
	}
	volOptions.RequestName = imageAttributes.RequestName
	vid.FsSubvolName = imageAttributes.ImageName
	volOptions.Owner = imageAttributes.Owner

	if volOpt != nil {
		if err = extractOptionalOption(&volOptions.Pool, "pool", volOpt); err != nil {
			return nil, nil, err
		}

		if err = extractOptionalOption(&volOptions.KernelMountOptions, "kernelMountOptions", volOpt); err != nil {
			return nil, nil, err
		}

		if err = extractOptionalOption(&volOptions.FuseMountOptions, "fuseMountOptions", volOpt); err != nil {
			return nil, nil, err
		}

		if err = extractOptionalOption(&volOptions.SubvolumeGroup, "subvolumeGroup", volOpt); err != nil {
			return nil, nil, err
		}

		if err = extractMounter(&volOptions.Mounter, volOpt); err != nil {
			return nil, nil, err
		}

		if err = volOptions.InitKMS(ctx, volOpt, secrets); err != nil {
			return nil, nil, err
		}
	}

	if imageAttributes.BackingSnapshotID != "" || volOptions.BackingSnapshotID != "" {
		volOptions.BackingSnapshot = true
		volOptions.BackingSnapshotID = imageAttributes.BackingSnapshotID
	}

	volOptions.ProvisionVolume = true
	volOptions.SubVolume.VolID = vid.FsSubvolName

	if volOptions.BackingSnapshot {
		err = volOptions.populateVolumeOptionsFromBackingSnapshot(ctx, cr, secrets, clusterName, setMetadata)
	} else {
		err = volOptions.populateVolumeOptionsFromSubvolume(ctx, clusterName, setMetadata)
	}

	if volOpt == nil && imageAttributes.KmsID != "" && volOptions.Encryption == nil {
		err = volOptions.ConfigureEncryption(ctx, imageAttributes.KmsID, secrets)
		if err != nil {
			return &volOptions, &vid, err
		}
	}

	return &volOptions, &vid, err
}

func (vo *VolumeOptions) populateVolumeOptionsFromSubvolume(
	ctx context.Context,
	clusterName string,
	setMetadata bool,
) error {
	vol := core.NewSubVolume(vo.conn, &vo.SubVolume, vo.ClusterID, clusterName, setMetadata)

	var info *core.Subvolume
	info, err := vol.GetSubVolumeInfo(ctx)
	if err == nil {
		vo.RootPath = info.Path
		vo.Features = info.Features
		vo.Size = info.BytesQuota
	}

	if errors.Is(err, cerrors.ErrInvalidCommand) {
		vo.RootPath, err = vol.GetVolumeRootPathCeph(ctx)
	}

	return err
}

func (vo *VolumeOptions) populateVolumeOptionsFromBackingSnapshot(
	ctx context.Context,
	cr *util.Credentials,
	secrets map[string]string,
	clusterName string,
	setMetadata bool,
) error {
	// As of CephFS snapshot v2 API, snapshots may be found in two locations:
	//
	// (a) /volumes/<volume group>/<subvolume>/.snap/<snapshot>/<UUID>
	// (b) /volumes/<volume group>/<subvolume>/<UUID>/.snap/_<snapshot>_<snapshot inode number>

	if !vo.ProvisionVolume {
		// Case (b)
		//
		// If the volume is not provisioned by us, we assume that we have access only
		// to snapshot's parent volume root. In this case, o.RootPath is expected to
		// be already set in the volume context.

		// BackingSnapshotRoot cannot be determined at this stage, because the
		// full directory name is not known (see snapshot path format for case
		// (b) above). RootPath/.snap must be traversed in order to find out
		// the snapshot directory name.

		return nil
	}

	parentBackingSnapVolOpts, _, snapID, err := NewSnapshotOptionsFromID(ctx,
		vo.BackingSnapshotID, cr, secrets, clusterName, setMetadata)
	if err != nil {
		return fmt.Errorf("failed to retrieve backing snapshot %s: %w", vo.BackingSnapshotID, err)
	}

	// Ensure that backing snapshot parent's volume options match the context.
	// Snapshot-backed volume inherits all its parent's (parent of the snapshot) options.

	if vo.ClusterID != parentBackingSnapVolOpts.ClusterID {
		return fmtBackingSnapshotOptionMismatch("clusterID", vo.ClusterID, parentBackingSnapVolOpts.ClusterID)
	}

	if vo.Pool != "" {
		return errors.New("cannot set pool for snapshot-backed volume")
	}

	if vo.MetadataPool != parentBackingSnapVolOpts.MetadataPool {
		return fmtBackingSnapshotOptionMismatch("MetadataPool", vo.MetadataPool, parentBackingSnapVolOpts.MetadataPool)
	}

	if vo.FsName != parentBackingSnapVolOpts.FsName {
		return fmtBackingSnapshotOptionMismatch("fsName", vo.FsName, parentBackingSnapVolOpts.FsName)
	}

	if vo.SubvolumeGroup != parentBackingSnapVolOpts.SubvolumeGroup {
		return fmtBackingSnapshotOptionMismatch("SubvolumeGroup", vo.SubvolumeGroup, parentBackingSnapVolOpts.SubvolumeGroup)
	}

	vo.Features = parentBackingSnapVolOpts.Features
	vo.Size = parentBackingSnapVolOpts.Size

	// For case (a) (o.ProvisionVolume==true is assumed), snapshot root path
	// can be built out of subvolume root path, which is in following format:
	//
	//   /volumes/<volume group>/<subvolume>/<subvolume UUID>

	subvolRoot, subvolUUID := path.Split(parentBackingSnapVolOpts.RootPath)

	vo.RootPath = subvolRoot
	vo.BackingSnapshotRoot = path.Join(".snap", snapID.FsSnapshotName, subvolUUID)

	return nil
}

// NewVolumeOptionsFromMonitorList generates a new instance of VolumeOptions and
// VolumeIdentifier from the provided CSI volume context.
func NewVolumeOptionsFromMonitorList(
	volID string,
	options, secrets map[string]string,
) (*VolumeOptions, *VolumeIdentifier, error) {
	var (
		opts                VolumeOptions
		vid                 VolumeIdentifier
		provisionVolumeBool string
		err                 error
	)

	// Check if monitors is part of the options
	if err = extractOption(&opts.Monitors, "monitors", options); err != nil {
		return nil, nil, err
	}

	// check if there are mon values in secret and if so override option retrieved monitors from
	// monitors in the secret
	mon, err := util.GetMonValFromSecret(secrets)
	if err == nil && len(mon) > 0 {
		opts.Monitors = mon
	}

	if err = extractOption(&provisionVolumeBool, "provisionVolume", options); err != nil {
		return nil, nil, err
	}

	if opts.ProvisionVolume, err = strconv.ParseBool(provisionVolumeBool); err != nil {
		return nil, nil, fmt.Errorf("failed to parse provisionVolume: %w", err)
	}

	if opts.ProvisionVolume {
		if err = extractOption(&opts.Pool, "pool", options); err != nil {
			return nil, nil, err
		}

		opts.RootPath = core.GetVolumeRootPathCephDeprecated(fsutil.VolumeID(volID))
	} else {
		if err = extractOption(&opts.RootPath, "rootPath", options); err != nil {
			return nil, nil, err
		}
	}

	if err = extractOptionalOption(&opts.KernelMountOptions, "kernelMountOptions", options); err != nil {
		return nil, nil, err
	}

	if err = extractOptionalOption(&opts.FuseMountOptions, "fuseMountOptions", options); err != nil {
		return nil, nil, err
	}

	if err = extractMounter(&opts.Mounter, options); err != nil {
		return nil, nil, err
	}

	if err = extractOptionalOption(&opts.BackingSnapshotID, "backingSnapshotID", options); err != nil {
		return nil, nil, err
	}

	opts.Owner = k8s.GetOwner(options)
	if err = opts.InitKMS(context.TODO(), options, secrets); err != nil {
		return nil, nil, err
	}

	vid.FsSubvolName = volID
	vid.VolumeID = volID

	if opts.BackingSnapshotID != "" {
		opts.BackingSnapshot = true
	}

	return &opts, &vid, nil
}

// NewVolumeOptionsFromStaticVolume generates a new instance of volumeOptions and
// VolumeIdentifier from the provided CSI volume context, if the provided context is
// detected to be a statically provisioned volume.
func NewVolumeOptionsFromStaticVolume(
	volID string,
	options, secrets map[string]string,
) (*VolumeOptions, *VolumeIdentifier, error) {
	var (
		opts      VolumeOptions
		vid       VolumeIdentifier
		staticVol bool
		err       error
	)

	val, ok := options["staticVolume"]
	if !ok {
		return nil, nil, cerrors.ErrNonStaticVolume
	}

	if staticVol, err = strconv.ParseBool(val); err != nil {
		return nil, nil, fmt.Errorf("failed to parse preProvisionedVolume: %w", err)
	}

	if !staticVol {
		return nil, nil, cerrors.ErrNonStaticVolume
	}

	// Volume is static, and ProvisionVolume carries bool stating if it was provisioned, hence
	// store NOT of static boolean
	opts.ProvisionVolume = !staticVol

	clusterData, err := GetClusterInformation(options)
	if err != nil {
		return nil, nil, err
	}

	opts.ClusterID = clusterData.ClusterID
	opts.Monitors = strings.Join(clusterData.Monitors, ",")
	opts.SubvolumeGroup = clusterData.CephFS.SubvolumeGroup
	opts.Owner = k8s.GetOwner(options)

	if err = extractOption(&opts.RootPath, "rootPath", options); err != nil {
		return nil, nil, err
	}

	if err = extractOption(&opts.FsName, "fsName", options); err != nil {
		return nil, nil, err
	}

	if err = extractOptionalOption(&opts.KernelMountOptions, "kernelMountOptions", options); err != nil {
		return nil, nil, err
	}

	if err = extractOptionalOption(&opts.FuseMountOptions, "fuseMountOptions", options); err != nil {
		return nil, nil, err
	}

	if err = extractOptionalOption(&opts.SubvolumeGroup, "subvolumeGroup", options); err != nil {
		return nil, nil, err
	}

	if err = extractMounter(&opts.Mounter, options); err != nil {
		return nil, nil, err
	}

	if err = opts.InitKMS(context.TODO(), options, secrets); err != nil {
		return nil, nil, err
	}

	vid.FsSubvolName = opts.RootPath
	vid.VolumeID = volID

	if opts.BackingSnapshotID != "" {
		opts.BackingSnapshot = true
	}

	return &opts, &vid, nil
}

// NewSnapshotOptionsFromID generates a new instance of volumeOptions and SnapshotIdentifier
// from the provided CSI VolumeID.
func NewSnapshotOptionsFromID(
	ctx context.Context,
	snapID string,
	cr *util.Credentials,
	secrets map[string]string,
	clusterName string,
	setMetadata bool,
) (*VolumeOptions, *core.SnapshotInfo, *SnapshotIdentifier, error) {
	var (
		vi         util.CSIIdentifier
		volOptions VolumeOptions
		sid        SnapshotIdentifier
	)
	// Decode the snapID first, to detect pre-provisioned snapshot before other errors
	err := vi.DecomposeCSIID(snapID)
	if err != nil {
		return &volOptions, nil, &sid, cerrors.ErrInvalidVolID
	}
	volOptions.ClusterID = vi.ClusterID
	sid.SnapshotID = snapID
	volOptions.FscID = vi.LocationID

	if volOptions.Monitors, err = util.Mons(util.CsiConfigFile, vi.ClusterID); err != nil {
		return &volOptions, nil, &sid, fmt.Errorf(
			"failed to fetch monitor list using clusterID (%s): %w",
			vi.ClusterID,
			err)
	}

	if volOptions.SubvolumeGroup, err = util.CephFSSubvolumeGroup(util.CsiConfigFile, vi.ClusterID); err != nil {
		return &volOptions, nil, &sid, fmt.Errorf(
			"failed to fetch subvolumegroup list using clusterID (%s): %w",
			vi.ClusterID,
			err)
	}

	err = volOptions.Connect(cr)
	if err != nil {
		return &volOptions, nil, &sid, err
	}
	// in case of an error, volOptions is returned, but callers may not
	// expect to need to call Destroy() on it. So, make sure to release any
	// resources that may have been allocated
	defer func() {
		if err != nil {
			volOptions.Destroy()
		}
	}()

	fs := core.NewFileSystem(volOptions.conn)
	volOptions.FsName, err = fs.GetFsName(ctx, volOptions.FscID)
	if err != nil {
		return &volOptions, nil, &sid, err
	}

	volOptions.MetadataPool, err = fs.GetMetadataPool(ctx, volOptions.FsName)
	if err != nil {
		return &volOptions, nil, &sid, err
	}

	// Connect to cephfs' default radosNamespace (csi)
	j, err := SnapJournal.Connect(volOptions.Monitors, fsutil.RadosNamespace, cr)
	if err != nil {
		return &volOptions, nil, &sid, err
	}
	defer j.Destroy()

	imageAttributes, err := j.GetImageAttributes(
		ctx, volOptions.MetadataPool, vi.ObjectUUID, true)
	if err != nil {
		return &volOptions, nil, &sid, err
	}
	// storing request name in snapshot Identifier
	sid.RequestName = imageAttributes.RequestName
	sid.FsSnapshotName = imageAttributes.ImageName
	sid.FsSubvolName = imageAttributes.SourceName

	volOptions.SubVolume.VolID = sid.FsSubvolName
	volOptions.Owner = imageAttributes.Owner
	vol := core.NewSubVolume(volOptions.conn, &volOptions.SubVolume, volOptions.ClusterID, clusterName, setMetadata)

	if imageAttributes.KmsID != "" && volOptions.Encryption == nil {
		err = volOptions.ConfigureEncryption(ctx, imageAttributes.KmsID, secrets)
		if err != nil {
			return &volOptions, nil, &sid, err
		}
	}

	subvolInfo, err := vol.GetSubVolumeInfo(ctx)
	if err != nil {
		return &volOptions, nil, &sid, err
	}
	volOptions.Features = subvolInfo.Features
	volOptions.Size = subvolInfo.BytesQuota
	volOptions.RootPath = subvolInfo.Path
	snap := core.NewSnapshot(volOptions.conn, sid.FsSnapshotName,
		volOptions.ClusterID, clusterName, setMetadata, &volOptions.SubVolume)
	info, err := snap.GetSnapshotInfo(ctx)
	if err != nil {
		return &volOptions, nil, &sid, err
	}

	return &volOptions, &info, &sid, nil
}

// SnapshotOption is a struct that holds the information about the snapshot.
type SnapshotOption struct {
	ReservedID  string // ID reserved for the snapshot.
	RequestName string // Request name of the snapshot.
	ClusterID   string // Cluster ID of to identify ceph cluster connection information.
	Monitors    string // Monitors of the ceph cluster.
	NamePrefix  string // Name prefix of the snapshot.
}

func GenSnapFromOptions(ctx context.Context, req *csi.CreateSnapshotRequest) (*SnapshotOption, error) {
	cephfsSnap := &SnapshotOption{}
	cephfsSnap.RequestName = req.GetName()
	snapOptions := req.GetParameters()

	clusterID, err := util.GetClusterID(snapOptions)
	if err != nil {
		return nil, err
	}
	cephfsSnap.Monitors, cephfsSnap.ClusterID, err = util.GetMonsAndClusterID(ctx, clusterID, false)
	if err != nil {
		log.ErrorLog(ctx, "failed getting mons (%s)", err)

		return nil, err
	}
	if namePrefix, ok := snapOptions["snapshotNamePrefix"]; ok {
		cephfsSnap.NamePrefix = namePrefix
	}

	return cephfsSnap, nil
}

func parseEncryptionOpts(volOptions map[string]string) (string, util.EncryptionType, error) {
	var (
		err              error
		ok               bool
		encrypted, kmsID string
	)
	encrypted, ok = volOptions["encrypted"]
	if !ok {
		return "", util.EncryptionTypeNone, nil
	}
	kmsID, err = util.FetchEncryptionKMSID(encrypted, volOptions["encryptionKMSID"])
	if err != nil {
		return "", util.EncryptionTypeInvalid, err
	}

	encType := util.FetchEncryptionType(volOptions, cephfsDefaultEncryptionType)

	return kmsID, encType, nil
}

// IsEncrypted returns true if volOptions enables file encryption.
func IsEncrypted(ctx context.Context, volOptions map[string]string) (bool, error) {
	_, encType, err := parseEncryptionOpts(volOptions)
	if err != nil {
		return false, err
	}

	return encType == util.EncryptionTypeFile, nil
}

// CopyEncryptionConfig copies passphrases and initializes a fresh
// Encryption struct if necessary from (vo, vID) to (cp, cpVID).
func (vo *VolumeOptions) CopyEncryptionConfig(cp *VolumeOptions, vID, cpVID string) error {
	var err error

	if !vo.IsEncrypted() {
		return nil
	}

	if vID == cpVID {
		return fmt.Errorf("BUG: %v and %v have the same VolID %q "+
			"set!? Call stack: %s", vo, cp, vID, util.CallStack())
	}

	if cp.Encryption == nil {
		cp.Encryption, err = util.NewVolumeEncryption(vo.Encryption.GetID(), vo.Encryption.KMS)
		if errors.Is(err, util.ErrDEKStoreNeeded) {
			_, err := vo.Encryption.KMS.GetSecret("")
			if errors.Is(err, kmsapi.ErrGetSecretUnsupported) {
				return err
			}
		}
	}

	if vo.Encryption.KMS.RequiresDEKStore() == kmsapi.DEKStoreIntegrated {
		passphrase, err := vo.Encryption.GetCryptoPassphrase(vID)
		if err != nil {
			return fmt.Errorf("failed to fetch passphrase for %q (%+v): %w",
				vID, vo, err)
		}

		err = cp.Encryption.StoreCryptoPassphrase(cpVID, passphrase)
		if err != nil {
			return fmt.Errorf("failed to store passphrase for %q (%+v): %w",
				cpVID, cp, err)
		}
	}

	return nil
}

// ConfigureEncryption initializes the Ceph CSI key management from
// kmsID and credentials. Sets vo.Encryption on success.
func (vo *VolumeOptions) ConfigureEncryption(
	ctx context.Context,
	kmsID string,
	credentials map[string]string,
) error {
	kms, err := kmsapi.GetKMS(vo.Owner, kmsID, credentials)
	if err != nil {
		log.ErrorLog(ctx, "get KMS failed %+v: %v", vo, err)

		return err
	}

	vo.Encryption, err = util.NewVolumeEncryption(kmsID, kms)

	if errors.Is(err, util.ErrDEKStoreNeeded) {
		// fscrypt uses secrets directly from the KMS.
		// Therefore we do not support an additional DEK
		// store. Since not all "metadata" KMS support
		// GetSecret, test for support here. Postpone any
		// other error handling
		_, err := vo.Encryption.KMS.GetSecret("")
		if errors.Is(err, kmsapi.ErrGetSecretUnsupported) {
			return err
		}
	}

	return nil
}

// InitKMS initialized the Ceph CSI key management by parsing the
// configuration from volume options + credentials. Sets vo.Encryption
// on success.
func (vo *VolumeOptions) InitKMS(
	ctx context.Context,
	volOptions, credentials map[string]string,
) error {
	var err error

	kmsID, encType, err := parseEncryptionOpts(volOptions)
	if err != nil {
		return err
	}

	if encType == util.EncryptionTypeNone {
		return nil
	}

	if encType != util.EncryptionTypeFile {
		return fmt.Errorf("unsupported encryption type %v. only supported type is 'file'", encType)
	}

	err = vo.ConfigureEncryption(ctx, kmsID, credentials)
	if err != nil {
		return fmt.Errorf("invalid encryption kms configuration: %w", err)
	}

	return nil
}

func (vo *VolumeOptions) IsEncrypted() bool {
	return vo.Encryption != nil
}
