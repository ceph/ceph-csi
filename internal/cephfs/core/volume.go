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

package core

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	fsAdmin "github.com/ceph/go-ceph/cephfs/admin"
	"github.com/ceph/go-ceph/rados"
)

// clusterAdditionalInfo contains information regarding if resize is
// supported in the particular cluster and subvolumegroup is
// created or not.
// Subvolumegroup creation and volume resize decisions are
// taken through this additional cluster information.
var clusterAdditionalInfo = make(map[string]*localClusterState)

// Subvolume holds subvolume information. This includes only the needed members
// from fsAdmin.SubVolumeInfo.
type Subvolume struct {
	BytesQuota int64
	Path       string
	Features   []string
}

// SubVolumeClient is the interface that holds the signature of subvolume methods
// that interacts with CephFS subvolume API's.
//
//nolint:interfacebloat // SubVolumeClient has more than 10 methods, that is ok.
type SubVolumeClient interface {
	// GetVolumeRootPathCeph returns the root path of the subvolume.
	GetVolumeRootPathCeph(ctx context.Context) (string, error)
	// CreateVolume creates a subvolume.
	CreateVolume(ctx context.Context) error
	// GetSubVolumeInfo returns the subvolume information.
	GetSubVolumeInfo(ctx context.Context) (*Subvolume, error)
	// ExpandVolume expands the volume if the requested size is greater than
	// the subvolume size.
	ExpandVolume(ctx context.Context, bytesQuota int64) error
	// ResizeVolume resizes the volume.
	ResizeVolume(ctx context.Context, bytesQuota int64) error
	// PurgSubVolume removes the subvolume.
	PurgeVolume(ctx context.Context, force bool) error

	// CreateCloneFromSubVolume creates a clone from the subvolume.
	CreateCloneFromSubvolume(ctx context.Context, parentvolOpt *SubVolume) error
	// GetCloneState returns the clone state of the subvolume.
	GetCloneState(ctx context.Context) (cephFSCloneState, error)
	// CreateCloneFromSnapshot creates a clone from the subvolume snapshot.
	CreateCloneFromSnapshot(ctx context.Context, snap Snapshot) error
	// CleanupSnapshotFromSubvolume removes the snapshot from the subvolume.
	CleanupSnapshotFromSubvolume(ctx context.Context, parentVol *SubVolume) error

	// SetAllMetadata set all the metadata from arg parameters on Ssubvolume.
	SetAllMetadata(parameters map[string]string) error
	// UnsetAllMetadata unset all the metadata from arg keys on subvolume.
	UnsetAllMetadata(keys []string) error
}

// subVolumeClient implements SubVolumeClient interface.
type subVolumeClient struct {
	*SubVolume                             // Embedded SubVolume struct.
	clusterID      string                  // Cluster ID to check subvolumegroup and resize functionality.
	clusterName    string                  // Cluster name
	enableMetadata bool                    // Set metadata on volume
	conn           *util.ClusterConnection // Cluster connection.
}

// SubVolume holds the information about the subvolume.
type SubVolume struct {
	VolID          string   // subvolume id.
	FsName         string   // filesystem name.
	SubvolumeGroup string   // subvolume group name where subvolume will be created.
	Pool           string   // pool name where subvolume will be created.
	Features       []string // subvolume features.
	Size           int64    // subvolume size.
}

// NewSubVolume returns a new subvolume client.
func NewSubVolume(
	conn *util.ClusterConnection,
	vol *SubVolume,
	clusterID,
	clusterName string,
	setMetadata bool,
) SubVolumeClient {
	return &subVolumeClient{
		SubVolume:      vol,
		clusterID:      clusterID,
		clusterName:    clusterName,
		enableMetadata: setMetadata,
		conn:           conn,
	}
}

// GetVolumeRootPathCephDeprecated returns the root path of the subvolume.
func GetVolumeRootPathCephDeprecated(volID fsutil.VolumeID) string {
	return path.Join("/", "csi-volumes", string(volID))
}

// GetVolumeRootPathCeph returns the root path of the subvolume.
func (s *subVolumeClient) GetVolumeRootPathCeph(ctx context.Context) (string, error) {
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin err %s", err)

		return "", err
	}
	svPath, err := fsa.SubVolumePath(s.FsName, s.SubvolumeGroup, s.VolID)
	if err != nil {
		log.ErrorLog(ctx, "failed to get the rootpath for the vol %s: %s", s.VolID, err)
		if errors.Is(err, rados.ErrNotFound) {
			return "", util.JoinErrors(cerrors.ErrVolumeNotFound, err)
		}

		return "", err
	}

	return svPath, nil
}

// GetSubVolumeInfo returns the subvolume information.
func (s *subVolumeClient) GetSubVolumeInfo(ctx context.Context) (*Subvolume, error) {
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin, can not fetch metadata pool for %s:", s.FsName, err)

		return nil, err
	}

	info, err := fsa.SubVolumeInfo(s.FsName, s.SubvolumeGroup, s.VolID)
	if err != nil {
		log.ErrorLog(ctx, "failed to get subvolume info for the vol %s: %s", s.VolID, err)
		if errors.Is(err, rados.ErrNotFound) {
			return nil, cerrors.ErrVolumeNotFound
		}
		// In case the error is invalid command return error to the caller.
		var invalid fsAdmin.NotImplementedError
		if errors.As(err, &invalid) {
			return nil, cerrors.ErrInvalidCommand
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
		if !(info.BytesQuota == fsAdmin.Infinite || info.State == fsAdmin.StateSnapRetained) {
			return nil, fmt.Errorf("subvolume %s has unsupported quota: %v", s.VolID, info.BytesQuota)
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
	resizeState                 operationState
	subVolMetadataState         operationState
	subVolSnapshotMetadataState operationState
	// A cluster can have multiple filesystem for that we need to have a map of
	// subvolumegroups to check filesystem is created nor not.
	// set true once a subvolumegroup is created
	// for corresponding filesystem in a cluster.
	subVolumeGroupsCreated map[string]bool
}

func newLocalClusterState(clusterID string) {
	// verify if corresponding clusterID key is present in the map,
	// and if not, initialize with default values(false).
	if _, keyPresent := clusterAdditionalInfo[clusterID]; !keyPresent {
		clusterAdditionalInfo[clusterID] = &localClusterState{}
		clusterAdditionalInfo[clusterID].subVolumeGroupsCreated = make(map[string]bool)
	}
}

// CreateVolume creates a subvolume.
func (s *subVolumeClient) CreateVolume(ctx context.Context) error {
	newLocalClusterState(s.clusterID)

	ca, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin, can not create subvolume %s: %s", s.VolID, err)

		return err
	}

	// create subvolumegroup if not already created for the cluster.
	if !clusterAdditionalInfo[s.clusterID].subVolumeGroupsCreated[s.FsName] {
		opts := fsAdmin.SubVolumeGroupOptions{}
		err = ca.CreateSubVolumeGroup(s.FsName, s.SubvolumeGroup, &opts)
		if err != nil {
			log.ErrorLog(
				ctx,
				"failed to create subvolume group %s, for the vol %s: %s",
				s.SubvolumeGroup,
				s.VolID,
				err)

			return err
		}
		log.DebugLog(ctx, "cephfs: created subvolume group %s", s.SubvolumeGroup)
		clusterAdditionalInfo[s.clusterID].subVolumeGroupsCreated[s.FsName] = true
	}

	opts := fsAdmin.SubVolumeOptions{
		Size: fsAdmin.ByteCount(s.Size),
	}
	if s.Pool != "" {
		opts.PoolLayout = s.Pool
	}

	// FIXME: check if the right credentials are used ("-n", cephEntityClientPrefix + cr.ID)
	err = ca.CreateSubVolume(s.FsName, s.SubvolumeGroup, s.VolID, &opts)
	if err != nil {
		log.ErrorLog(ctx, "failed to create subvolume %s in fs %s: %s", s.VolID, s.FsName, err)

		if errors.Is(err, rados.ErrNotFound) {
			// Reset the subVolumeGroupsCreated so that we can try again to create the
			// subvolumegroup in next request if the error is Not Found.
			clusterAdditionalInfo[s.clusterID].subVolumeGroupsCreated[s.FsName] = false
		}

		return err
	}

	return nil
}

// ExpandVolume will expand the volume if the requested size is greater than
// the subvolume size.
func (s *subVolumeClient) ExpandVolume(ctx context.Context, bytesQuota int64) error {
	// get the subvolume size for comparison with the requested size.
	info, err := s.GetSubVolumeInfo(ctx)
	if err != nil {
		return err
	}
	// resize if the requested size is greater than the current size.
	if s.Size > info.BytesQuota {
		log.DebugLog(ctx, "clone %s size %d is greater than requested size %d", s.VolID, info.BytesQuota, bytesQuota)
		err = s.ResizeVolume(ctx, bytesQuota)
	}

	return err
}

// ResizeVolume will try to use ceph fs subvolume resize command to resize the
// subvolume. If the command is not available as a fallback it will use
// CreateVolume to resize the subvolume.
func (s *subVolumeClient) ResizeVolume(ctx context.Context, bytesQuota int64) error {
	newLocalClusterState(s.clusterID)
	// resize subvolume when either it's supported, or when corresponding
	// clusterID key was not present.
	if clusterAdditionalInfo[s.clusterID].resizeState == unknown ||
		clusterAdditionalInfo[s.clusterID].resizeState == supported {
		fsa, err := s.conn.GetFSAdmin()
		if err != nil {
			log.ErrorLog(ctx, "could not get FSAdmin, can not resize volume %s:", s.FsName, err)

			return err
		}
		_, err = fsa.ResizeSubVolume(s.FsName, s.SubvolumeGroup, s.VolID, fsAdmin.ByteCount(bytesQuota), true)
		if err == nil {
			clusterAdditionalInfo[s.clusterID].resizeState = supported

			return nil
		}
		var invalid fsAdmin.NotImplementedError
		// In case the error is other than invalid command return error to the caller.
		if !errors.As(err, &invalid) {
			log.ErrorLog(ctx, "failed to resize subvolume %s in fs %s: %s", s.VolID, s.FsName, err)

			return err
		}
	}
	clusterAdditionalInfo[s.clusterID].resizeState = unsupported
	s.Size = bytesQuota

	return s.CreateVolume(ctx)
}

// PurgSubVolume removes the subvolume.
func (s *subVolumeClient) PurgeVolume(ctx context.Context, force bool) error {
	fsa, err := s.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin %s:", err)

		return err
	}

	opt := fsAdmin.SubVolRmFlags{}
	opt.Force = force

	if checkSubvolumeHasFeature("snapshot-retention", s.Features) {
		opt.RetainSnapshots = true
	}

	err = fsa.RemoveSubVolumeWithFlags(s.FsName, s.SubvolumeGroup, s.VolID, opt)
	if err != nil {
		log.ErrorLog(ctx, "failed to purge subvolume %s in fs %s: %s", s.VolID, s.FsName, err)
		if strings.Contains(err.Error(), cerrors.VolumeNotEmpty) {
			return util.JoinErrors(cerrors.ErrVolumeHasSnapshots, err)
		}
		if errors.Is(err, rados.ErrNotFound) {
			return util.JoinErrors(cerrors.ErrVolumeNotFound, err)
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
