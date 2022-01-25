/*
Copyright 2019 The Ceph-CSI Authors.

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
	"fmt"

	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// FileSystem is the interface that holds the signature of filesystem methods
// that interacts with CephFS filesystem API's.
type FileSystem interface {
	// GetFscID returns the ID of the filesystem with the given name.
	GetFscID(context.Context, string) (int64, error)
	// GetMetadataPool returns the metadata pool name of the filesystem with the given name.
	GetMetadataPool(context.Context, string) (string, error)
	// GetFsName returns the name of the filesystem with the given ID.
	GetFsName(context.Context, int64) (string, error)
}

// fileSystem is the implementation of FileSystem interface.
type fileSystem struct {
	conn *util.ClusterConnection
}

// NewFileSystem returns a new instance of fileSystem.
func NewFileSystem(conn *util.ClusterConnection) FileSystem {
	return &fileSystem{
		conn: conn,
	}
}

// GetFscID returns the ID of the filesystem with the given name.
func (f *fileSystem) GetFscID(ctx context.Context, fsName string) (int64, error) {
	fsa, err := f.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin, can not fetch filesystem ID for %s: %s", fsName, err)

		return 0, err
	}

	volumes, err := fsa.EnumerateVolumes()
	if err != nil {
		log.ErrorLog(ctx, "could not list volumes, can not fetch filesystem ID for %s: %s", fsName, err)

		return 0, err
	}

	for _, vol := range volumes {
		if vol.Name == fsName {
			return vol.ID, nil
		}
	}

	log.ErrorLog(ctx, "failed to list volume %s", fsName)

	return 0, cerrors.ErrVolumeNotFound
}

// GetMetadataPool returns the metadata pool name of the filesystem with the given name.
func (f *fileSystem) GetMetadataPool(ctx context.Context, fsName string) (string, error) {
	fsa, err := f.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin, can not fetch metadata pool for %s: %s", fsName, err)

		return "", err
	}

	fsPoolInfos, err := fsa.ListFileSystems()
	if err != nil {
		log.ErrorLog(ctx, "could not list filesystems, can not fetch metadata pool for %s: %s", fsName, err)

		return "", err
	}

	for _, fspi := range fsPoolInfos {
		if fspi.Name == fsName {
			return fspi.MetadataPool, nil
		}
	}

	return "", fmt.Errorf("%w: could not find metadata pool for %s", util.ErrPoolNotFound, fsName)
}

// GetFsName returns the name of the filesystem with the given ID.
func (f *fileSystem) GetFsName(ctx context.Context, fscID int64) (string, error) {
	fsa, err := f.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin, can not fetch filesystem name for ID %d: %s", fscID, err)

		return "", err
	}

	volumes, err := fsa.EnumerateVolumes()
	if err != nil {
		log.ErrorLog(ctx, "could not list volumes, can not fetch filesystem name for ID %d: %s", fscID, err)

		return "", err
	}

	for _, vol := range volumes {
		if vol.ID == fscID {
			return vol.Name, nil
		}
	}

	return "", fmt.Errorf("%w: fscID (%d) not found in Ceph cluster", util.ErrPoolNotFound, fscID)
}
