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
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	"github.com/ceph/ceph-csi/pkg/util"
)

type volumeOptions struct {
	RequestName     string
	Size            int64
	ClusterID       string
	FsName          string
	FscID           int64
	MetadataPool    string
	Monitors        string `json:"monitors"`
	Pool            string `json:"pool"`
	RootPath        string `json:"rootPath"`
	Mounter         string `json:"mounter"`
	ProvisionVolume bool   `json:"provisionVolume"`
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

func getMonsAndClusterID(options map[string]string) (string, string, error) {
	clusterID, ok := options["clusterID"]
	if !ok {
		err := fmt.Errorf("clusterID must be set")
		return "", "", err
	}

	if err := validateNonEmptyField(clusterID, "clusterID"); err != nil {
		return "", "", err
	}

	monitors, err := util.Mons(csiConfigFile, clusterID)
	if err != nil {
		err = errors.Wrapf(err, "failed to fetch monitor list using clusterID (%s)", clusterID)
		return "", "", err
	}

	return monitors, clusterID, err
}

// newVolumeOptions generates a new instance of volumeOptions from the provided
// CSI request parameters
func newVolumeOptions(ctx context.Context, requestName string, size int64, volOptions, secret map[string]string) (*volumeOptions, error) {
	var (
		opts volumeOptions
		err  error
	)

	opts.Monitors, opts.ClusterID, err = getMonsAndClusterID(volOptions)
	if err != nil {
		return nil, err
	}

	if err = extractOptionalOption(&opts.Pool, "pool", volOptions); err != nil {
		return nil, err
	}

	if err = extractMounter(&opts.Mounter, volOptions); err != nil {
		return nil, err
	}

	if err = extractOption(&opts.FsName, "fsName", volOptions); err != nil {
		return nil, err
	}

	opts.RequestName = requestName
	opts.Size = size

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

	opts.ProvisionVolume = true

	return &opts, nil
}

// newVolumeOptionsFromVolID generates a new instance of volumeOptions and volumeIdentifier
// from the provided CSI VolumeID
func newVolumeOptionsFromVolID(ctx context.Context, volID string, volOpt, secrets map[string]string) (*volumeOptions, *volumeIdentifier, error) {
	var (
		vi         util.CSIIdentifier
		volOptions volumeOptions
		vid        volumeIdentifier
	)

	// Decode the VolID first, to detect older volumes or pre-provisioned volumes
	// before other errors
	err := vi.DecomposeCSIID(volID)
	if err != nil {
		err = fmt.Errorf("error decoding volume ID (%s) (%s)", err, volID)
		return nil, nil, ErrInvalidVolID{err}
	}
	volOptions.ClusterID = vi.ClusterID
	vid.FsSubvolName = volJournal.NamingPrefix() + vi.ObjectUUID
	vid.VolumeID = volID
	volOptions.FscID = vi.LocationID

	if volOptions.Monitors, err = util.Mons(csiConfigFile, vi.ClusterID); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to fetch monitor list using clusterID (%s)", vi.ClusterID)
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

	volOptions.RequestName, _, err = volJournal.GetObjectUUIDData(ctx, volOptions.Monitors, cr,
		volOptions.MetadataPool, vi.ObjectUUID, false)
	if err != nil {
		return nil, nil, err
	}

	if volOpt != nil {
		if err = extractOptionalOption(&volOptions.Pool, "pool", volOpt); err != nil {
			return nil, nil, err
		}

		if err = extractMounter(&volOptions.Mounter, volOpt); err != nil {
			return nil, nil, err
		}
	}

	volOptions.RootPath, err = getVolumeRootPathCeph(ctx, &volOptions, cr, volumeID(vid.FsSubvolName))
	if err != nil {
		return nil, nil, err
	}

	volOptions.ProvisionVolume = true

	return &volOptions, &vid, nil
}

// newVolumeOptionsFromVersion1Context generates a new instance of volumeOptions and
// volumeIdentifier from the provided CSI volume context, if the provided context was
// for a volume created by version 1.0.0 (or prior) of the CSI plugin
func newVolumeOptionsFromVersion1Context(volID string, options, secrets map[string]string) (*volumeOptions, *volumeIdentifier, error) {
	var (
		opts                volumeOptions
		vid                 volumeIdentifier
		provisionVolumeBool string
		err                 error
	)

	// Check if monitors is part of the options, that is an indicator this is an 1.0.0 volume
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
		return nil, nil, fmt.Errorf("failed to parse provisionVolume: %v", err)
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
// detected to be a statically provisioned volume
func newVolumeOptionsFromStaticVolume(volID string, options map[string]string) (*volumeOptions, *volumeIdentifier, error) {
	var (
		opts      volumeOptions
		vid       volumeIdentifier
		staticVol bool
		err       error
	)

	val, ok := options["staticVolume"]
	if !ok {
		return nil, nil, ErrNonStaticVolume{err}
	}

	if staticVol, err = strconv.ParseBool(val); err != nil {
		return nil, nil, fmt.Errorf("failed to parse preProvisionedVolume: %v", err)
	}

	if !staticVol {
		return nil, nil, ErrNonStaticVolume{err}
	}

	// Volume is static, and ProvisionVolume carries bool stating if it was provisioned, hence
	// store NOT of static boolean
	opts.ProvisionVolume = !staticVol

	opts.Monitors, opts.ClusterID, err = getMonsAndClusterID(options)
	if err != nil {
		return nil, nil, err
	}

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
