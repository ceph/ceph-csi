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
	"path"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"

	fsAdmin "github.com/ceph/go-ceph/cephfs/admin"
	"github.com/ceph/go-ceph/rados"
)

var (
	// clusterAdditionalInfo contains information regarding if resize is
	// supported in the particular cluster and subvolumegroup is
	// created or not.
	// Subvolumegroup creation and volume resize decisions are
	// taken through this additional cluster information.
	clusterAdditionalInfo = make(map[string]*localClusterState)
)

const (
	cephEntityClientPrefix = "client."

	// modeAllRWX can be used for setting permissions to Read-Write-eXecute
	// for User, Group and Other.
	modeAllRWX = 0777
)

// Subvolume holds subvolume information. This includes only the needed members
// from fsAdmin.SubVolumeInfo.
type Subvolume struct {
	BytesQuota int64
	Path       string
	Features   []string
}

func getVolumeRootPathCephDeprecated(volID volumeID) string {
	return path.Join("/", "csi-volumes", string(volID))
}

func (vo *volumeOptions) getVolumeRootPathCeph(ctx context.Context, volID volumeID) (string, error) {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin err %s", err)
		return "", err
	}
	svPath, err := fsa.SubVolumePath(vo.FsName, vo.SubvolumeGroup, string(volID))
	if err != nil {
		util.ErrorLog(ctx, "failed to get the rootpath for the vol %s: %s", string(volID), err)
		if errors.Is(err, rados.ErrNotFound) {
			return "", util.JoinErrors(ErrVolumeNotFound, err)
		}
		return "", err
	}
	return svPath, nil
}

func (vo *volumeOptions) getSubVolumeInfo(ctx context.Context, volID volumeID) (*Subvolume, error) {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin, can not fetch metadata pool for %s:", vo.FsName, err)
		return nil, err
	}

	info, err := fsa.SubVolumeInfo(vo.FsName, vo.SubvolumeGroup, string(volID))
	if err != nil {
		util.ErrorLog(ctx, "failed to get subvolume info for the vol %s: %s", string(volID), err)
		if errors.Is(err, rados.ErrNotFound) {
			return nil, ErrVolumeNotFound
		}
		// In case the error is invalid command return error to the caller.
		var invalid fsAdmin.NotImplementedError
		if errors.As(err, &invalid) {
			return nil, ErrInvalidCommand
		}

		return nil, err
	}

	subvol := Subvolume{
		// only set BytesQuota when it is of type ByteCount
		Path:     info.Path,
		Features: make([]string, len(info.Features)),
	}
	bc, ok := info.BytesQuota.(fsAdmin.ByteCount)
	if !ok {
		// If info.BytesQuota == Infinite (in case it is not set)
		// or nil (in case the subvolume is in snapshot-retained state),
		// just continue without returning quota information.
		// TODO: make use of subvolume "state" attribute once
		// https://github.com/ceph/go-ceph/issues/453 is fixed.
		if !(info.BytesQuota == fsAdmin.Infinite || info.BytesQuota == nil) {
			return nil, fmt.Errorf("subvolume %s has unsupported quota: %v", string(volID), info.BytesQuota)
		}
	} else {
		subvol.BytesQuota = int64(bc)
	}
	for i, feature := range info.Features {
		subvol.Features[i] = string(feature)
	}

	return &subvol, nil
}

type operationState int64

const (
	unknown operationState = iota
	supported
	unsupported
)

type localClusterState struct {
	// set the enum value i.e., unknown, supported,
	// unsupported as per the state of the cluster.
	resizeState operationState
	// set true once a subvolumegroup is created
	// for corresponding cluster.
	subVolumeGroupCreated bool
}

func createVolume(ctx context.Context, volOptions *volumeOptions, volID volumeID, bytesQuota int64) error {
	// verify if corresponding ClusterID key is present in the map,
	// and if not, initialize with default values(false).
	if _, keyPresent := clusterAdditionalInfo[volOptions.ClusterID]; !keyPresent {
		clusterAdditionalInfo[volOptions.ClusterID] = &localClusterState{}
	}

	ca, err := volOptions.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin, can not create subvolume %s: %s", string(volID), err)
		return err
	}

	// create subvolumegroup if not already created for the cluster.
	if !clusterAdditionalInfo[volOptions.ClusterID].subVolumeGroupCreated {
		opts := fsAdmin.SubVolumeGroupOptions{}
		err = ca.CreateSubVolumeGroup(volOptions.FsName, volOptions.SubvolumeGroup, &opts)
		if err != nil {
			util.ErrorLog(ctx, "failed to create subvolume group %s, for the vol %s: %s", volOptions.SubvolumeGroup, string(volID), err)
			return err
		}
		util.DebugLog(ctx, "cephfs: created subvolume group %s", volOptions.SubvolumeGroup)
		clusterAdditionalInfo[volOptions.ClusterID].subVolumeGroupCreated = true
	}

	opts := fsAdmin.SubVolumeOptions{
		Size: fsAdmin.ByteCount(bytesQuota),
		Mode: modeAllRWX,
	}
	if volOptions.Pool != "" {
		opts.PoolLayout = volOptions.Pool
	}

	// FIXME: check if the right credentials are used ("-n", cephEntityClientPrefix + cr.ID)
	err = ca.CreateSubVolume(volOptions.FsName, volOptions.SubvolumeGroup, string(volID), &opts)
	if err != nil {
		util.ErrorLog(ctx, "failed to create subvolume %s in fs %s: %s", string(volID), volOptions.FsName, err)
		return err
	}

	return nil
}

// resizeVolume will try to use ceph fs subvolume resize command to resize the
// subvolume. If the command is not available as a fallback it will use
// CreateVolume to resize the subvolume.
func (vo *volumeOptions) resizeVolume(ctx context.Context, volID volumeID, bytesQuota int64) error {
	// keyPresent checks whether corresponding clusterID key is present in clusterAdditionalInfo
	var keyPresent bool
	// verify if corresponding ClusterID key is present in the map,
	// and if not, initialize with default values(false).
	if _, keyPresent = clusterAdditionalInfo[vo.ClusterID]; !keyPresent {
		clusterAdditionalInfo[vo.ClusterID] = &localClusterState{}
	}
	// resize subvolume when either it's supported, or when corresponding
	// clusterID key was not present.
	if clusterAdditionalInfo[vo.ClusterID].resizeState == unknown ||
		clusterAdditionalInfo[vo.ClusterID].resizeState == supported {
		fsa, err := vo.conn.GetFSAdmin()
		if err != nil {
			util.ErrorLog(ctx, "could not get FSAdmin, can not resize volume %s:", vo.FsName, err)
			return err
		}
		_, err = fsa.ResizeSubVolume(vo.FsName, vo.SubvolumeGroup, string(volID), fsAdmin.ByteCount(bytesQuota), true)
		if err == nil {
			clusterAdditionalInfo[vo.ClusterID].resizeState = supported
			return nil
		}
		var invalid fsAdmin.NotImplementedError
		// In case the error is other than invalid command return error to the caller.
		if !errors.As(err, &invalid) {
			util.ErrorLog(ctx, "failed to resize subvolume %s in fs %s: %s", string(volID), vo.FsName, err)
			return err
		}
	}
	clusterAdditionalInfo[vo.ClusterID].resizeState = unsupported
	return createVolume(ctx, vo, volID, bytesQuota)
}

func (vo *volumeOptions) purgeVolume(ctx context.Context, volID volumeID, force bool) error {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		util.ErrorLog(ctx, "could not get FSAdmin %s:", err)
		return err
	}

	opt := fsAdmin.SubVolRmFlags{}
	opt.Force = force

	if checkSubvolumeHasFeature("snapshot-retention", vo.Features) {
		opt.RetainSnapshots = true
	}

	err = fsa.RemoveSubVolumeWithFlags(vo.FsName, vo.SubvolumeGroup, string(volID), opt)
	if err != nil {
		util.ErrorLog(ctx, "failed to purge subvolume %s in fs %s: %s", string(volID), vo.FsName, err)
		if strings.Contains(err.Error(), volumeNotEmpty) {
			return util.JoinErrors(ErrVolumeHasSnapshots, err)
		}
		if errors.Is(err, rados.ErrNotFound) {
			return util.JoinErrors(ErrVolumeNotFound, err)
		}
		return err
	}

	return nil
}

// checkSubvolumeHasFeature verifies if the referred subvolume has
// the required feature.
func checkSubvolumeHasFeature(feature string, subVolFeatures []string) bool {
	// The subvolume "features" are based on the internal version of the subvolume.
	// Verify if subvolume supports the required feature.
	for _, subvolFeature := range subVolFeatures {
		if subvolFeature == feature {
			return true
		}
	}
	return false
}
