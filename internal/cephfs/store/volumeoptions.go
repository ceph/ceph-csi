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
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/ceph/ceph-csi/internal/cephfs/core"
	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

type VolumeOptions struct {
	core.SubVolume
	TopologyPools       *[]util.TopologyConstrainedPool
	TopologyRequirement *csi.TopologyRequirement
	Topology            map[string]string
	RequestName         string
	NamePrefix          string
	ClusterID           string
	FscID               int64
	MetadataPool        string
	// ReservedID represents the ID reserved for a subvolume
	ReservedID         string
	Monitors           string `json:"monitors"`
	RootPath           string `json:"rootPath"`
	Mounter            string `json:"mounter"`
	ProvisionVolume    bool   `json:"provisionVolume"`
	KernelMountOptions string `json:"kernelMountOptions"`
	FuseMountOptions   string `json:"fuseMountOptions"`
	// Network namespace file path to execute nsenter command
	NetNamespaceFilePath string

	// conn is a connection to the Ceph cluster obtained from a ConnPool
	conn *util.ClusterConnection
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

// NewVolumeOptions generates a new instance of volumeOptions from the provided
// CSI request parameters.
func NewVolumeOptions(ctx context.Context, requestName string, req *csi.CreateVolumeRequest,
	cr *util.Credentials) (*VolumeOptions, error) {
	var (
		opts VolumeOptions
		err  error
	)

	volOptions := req.GetParameters()
	clusterData, err := GetClusterInformation(volOptions)
	if err != nil {
		return nil, err
	}

	opts.ClusterID = clusterData.ClusterID
	opts.Monitors = strings.Join(clusterData.Monitors, ",")
	opts.SubvolumeGroup = clusterData.CephFS.SubvolumeGroup

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

	return &opts, nil
}

// newVolumeOptionsFromVolID generates a new instance of volumeOptions and VolumeIdentifier
// from the provided CSI VolumeID.
func NewVolumeOptionsFromVolID(
	ctx context.Context,
	volID string,
	volOpt, secrets map[string]string) (*VolumeOptions, *VolumeIdentifier, error) {
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
	}

	volOptions.ProvisionVolume = true
	volOptions.SubVolume.VolID = vid.FsSubvolName
	vol := core.NewSubVolume(volOptions.conn, &volOptions.SubVolume, volOptions.ClusterID)
	info, err := vol.GetSubVolumeInfo(ctx)
	if err == nil {
		volOptions.RootPath = info.Path
		volOptions.Features = info.Features
		volOptions.Size = info.BytesQuota
	}

	if errors.Is(err, cerrors.ErrInvalidCommand) {
		volOptions.RootPath, err = vol.GetVolumeRootPathCeph(ctx)
	}

	return &volOptions, &vid, err
}

// NewVolumeOptionsFromMonitorList generates a new instance of VolumeOptions and
// VolumeIdentifier from the provided CSI volume context.
func NewVolumeOptionsFromMonitorList(
	volID string,
	options, secrets map[string]string) (*VolumeOptions, *VolumeIdentifier, error) {
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

	vid.FsSubvolName = volID
	vid.VolumeID = volID

	return &opts, &vid, nil
}

// NewVolumeOptionsFromStaticVolume generates a new instance of volumeOptions and
// VolumeIdentifier from the provided CSI volume context, if the provided context is
// detected to be a statically provisioned volume.
func NewVolumeOptionsFromStaticVolume(
	volID string,
	options map[string]string) (*VolumeOptions, *VolumeIdentifier, error) {
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

	vid.FsSubvolName = opts.RootPath
	vid.VolumeID = volID

	return &opts, &vid, nil
}

// NewSnapshotOptionsFromID generates a new instance of volumeOptions and SnapshotIdentifier
// from the provided CSI VolumeID.
func NewSnapshotOptionsFromID(
	ctx context.Context,
	snapID string,
	cr *util.Credentials) (*VolumeOptions, *core.SnapshotInfo, *SnapshotIdentifier, error) {
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
	vol := core.NewSubVolume(volOptions.conn, &volOptions.SubVolume, volOptions.ClusterID)

	subvolInfo, err := vol.GetSubVolumeInfo(ctx)
	if err != nil {
		return &volOptions, nil, &sid, err
	}
	volOptions.Features = subvolInfo.Features
	volOptions.Size = subvolInfo.BytesQuota
	snap := core.NewSnapshot(volOptions.conn, sid.FsSnapshotName, &volOptions.SubVolume)
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
