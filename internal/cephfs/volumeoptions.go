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
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/ceph/ceph-csi/internal/util"
)

type volumeOptions struct {
	TopologyPools       *[]util.TopologyConstrainedPool
	TopologyRequirement *csi.TopologyRequirement
	Topology            map[string]string
	RequestName         string
	NamePrefix          string
	Size                int64
	ClusterID           string
	FsName              string
	FscID               int64
	MetadataPool        string
	Monitors            string `json:"monitors"`
	Pool                string `json:"pool"`
	RootPath            string `json:"rootPath"`
	Mounter             string `json:"mounter"`
	ProvisionVolume     bool   `json:"provisionVolume"`
	KernelMountOptions  string `json:"kernelMountOptions"`
	FuseMountOptions    string `json:"fuseMountOptions"`
	SubvolumeGroup      string
	Features            []string
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
	case volumeMounterFuse:
	case volumeMounterKernel:
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

func getClusterInformation(options map[string]string) (*util.ClusterInfo, error) {
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

// newVolumeOptions generates a new instance of volumeOptions from the provided
// CSI request parameters.
func newVolumeOptions(ctx context.Context, requestName string, req *csi.CreateVolumeRequest,
	secret map[string]string) (*volumeOptions, error) {
	var (
		opts volumeOptions
		err  error
	)

	volOptions := req.GetParameters()
	clusterData, err := getClusterInformation(volOptions)

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

	opts.RequestName = requestName

	cr, err := util.NewAdminCredentials(secret)
	if err != nil {
		return nil, err
	}
	defer cr.DeleteCredentials()

	opts.FscID, err = getFscID(ctx, opts.Monitors, cr, opts.FsName)
	if err != nil {
		return nil, err
	}

	opts.MetadataPool, err = getMetadataPool(ctx, opts.Monitors, cr, opts.FsName)
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

// newVolumeOptionsFromVolID generates a new instance of volumeOptions and volumeIdentifier
// from the provided CSI VolumeID.
func newVolumeOptionsFromVolID(ctx context.Context, volID string, volOpt, secrets map[string]string) (*volumeOptions, *volumeIdentifier, error) {
	var (
		vi         util.CSIIdentifier
		volOptions volumeOptions
		vid        volumeIdentifier
		info       Subvolume
	)

	// Decode the VolID first, to detect older volumes or pre-provisioned volumes
	// before other errors
	err := vi.DecomposeCSIID(volID)
	if err != nil {
		err = fmt.Errorf("error decoding volume ID (%s): %w", volID, err)
		return nil, nil, util.JoinErrors(ErrInvalidVolID, err)
	}
	volOptions.ClusterID = vi.ClusterID
	vid.VolumeID = volID
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

	volOptions.FsName, err = getFsName(ctx, volOptions.Monitors, cr, volOptions.FscID)
	if err != nil {
		return nil, nil, err
	}

	volOptions.MetadataPool, err = getMetadataPool(ctx, volOptions.Monitors, cr, volOptions.FsName)
	if err != nil {
		return nil, nil, err
	}

	// Connect to cephfs' default radosNamespace (csi)
	j, err := volJournal.Connect(volOptions.Monitors, radosNamespace, cr)
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

	info, err = getSubVolumeInfo(ctx, &volOptions, cr, volumeID(vid.FsSubvolName))
	if err == nil {
		volOptions.RootPath = info.Path
		volOptions.Features = info.Features
	}

	if errors.Is(err, ErrInvalidCommand) {
		volOptions.RootPath, err = getVolumeRootPathCeph(ctx, &volOptions, cr, volumeID(vid.FsSubvolName))
	}

	return &volOptions, &vid, err
}

// newVolumeOptionsFromMonitorList generates a new instance of volumeOptions and
// volumeIdentifier from the provided CSI volume context.
func newVolumeOptionsFromMonitorList(volID string, options, secrets map[string]string) (*volumeOptions, *volumeIdentifier, error) {
	var (
		opts                volumeOptions
		vid                 volumeIdentifier
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

		opts.RootPath = getVolumeRootPathCephDeprecated(volumeID(volID))
	} else {
		if err = extractOption(&opts.RootPath, "rootPath", options); err != nil {
			return nil, nil, err
		}
	}

	if err = extractMounter(&opts.Mounter, options); err != nil {
		return nil, nil, err
	}

	vid.FsSubvolName = volID
	vid.VolumeID = volID

	return &opts, &vid, nil
}

// newVolumeOptionsFromStaticVolume generates a new instance of volumeOptions and
// volumeIdentifier from the provided CSI volume context, if the provided context is
// detected to be a statically provisioned volume.
func newVolumeOptionsFromStaticVolume(volID string, options map[string]string) (*volumeOptions, *volumeIdentifier, error) {
	var (
		opts      volumeOptions
		vid       volumeIdentifier
		staticVol bool
		err       error
	)

	val, ok := options["staticVolume"]
	if !ok {
		return nil, nil, ErrNonStaticVolume
	}

	if staticVol, err = strconv.ParseBool(val); err != nil {
		return nil, nil, fmt.Errorf("failed to parse preProvisionedVolume: %w", err)
	}

	if !staticVol {
		return nil, nil, ErrNonStaticVolume
	}

	// Volume is static, and ProvisionVolume carries bool stating if it was provisioned, hence
	// store NOT of static boolean
	opts.ProvisionVolume = !staticVol

	clusterData, err := getClusterInformation(options)

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

	if err = extractMounter(&opts.Mounter, options); err != nil {
		return nil, nil, err
	}

	vid.FsSubvolName = opts.RootPath
	vid.VolumeID = volID

	return &opts, &vid, nil
}

// newSnapshotOptionsFromID generates a new instance of volumeOptions and snapshotIdentifier
// from the provided CSI VolumeID.
func newSnapshotOptionsFromID(ctx context.Context, snapID string, cr *util.Credentials) (*volumeOptions, *snapshotInfo, *snapshotIdentifier, error) {
	var (
		vi         util.CSIIdentifier
		volOptions volumeOptions
		sid        snapshotIdentifier
	)
	// Decode the snapID first, to detect pre-provisioned snapshot before other errors
	err := vi.DecomposeCSIID(snapID)
	if err != nil {
		return &volOptions, nil, &sid, ErrInvalidVolID
	}
	volOptions.ClusterID = vi.ClusterID
	sid.SnapshotID = snapID
	volOptions.FscID = vi.LocationID

	if volOptions.Monitors, err = util.Mons(util.CsiConfigFile, vi.ClusterID); err != nil {
		return &volOptions, nil, &sid, fmt.Errorf("failed to fetch monitor list using clusterID (%s): %w", vi.ClusterID, err)
	}

	if volOptions.SubvolumeGroup, err = util.CephFSSubvolumeGroup(util.CsiConfigFile, vi.ClusterID); err != nil {
		return &volOptions, nil, &sid, fmt.Errorf("failed to fetch subvolumegroup list using clusterID (%s): %w", vi.ClusterID, err)
	}

	volOptions.FsName, err = getFsName(ctx, volOptions.Monitors, cr, volOptions.FscID)
	if err != nil {
		return &volOptions, nil, &sid, err
	}

	volOptions.MetadataPool, err = getMetadataPool(ctx, volOptions.Monitors, cr, volOptions.FsName)
	if err != nil {
		return &volOptions, nil, &sid, err
	}

	// Connect to cephfs' default radosNamespace (csi)
	j, err := snapJournal.Connect(volOptions.Monitors, radosNamespace, cr)
	if err != nil {
		return &volOptions, nil, &sid, err
	}
	defer j.Destroy()

	imageAttributes, err := j.GetImageAttributes(
		ctx, volOptions.MetadataPool, vi.ObjectUUID, true)
	if err != nil {
		return &volOptions, nil, &sid, err
	}
	// storing request name in snapshotshot Identifier
	sid.RequestName = imageAttributes.RequestName
	sid.FsSnapshotName = imageAttributes.ImageName
	sid.FsSubvolName = imageAttributes.SourceName

	info, err := getSnapshotInfo(ctx, &volOptions, cr, volumeID(sid.FsSnapshotName), volumeID(sid.FsSubvolName))
	if err != nil {
		return &volOptions, nil, &sid, err
	}

	return &volOptions, &info, &sid, nil
}
