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

package cephfs

import (
	"context"
	"fmt"

	cerrors "github.com/ceph/ceph-csi/internal/cephfs/errors"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

func (vo *volumeOptions) getFscID(ctx context.Context) (int64, error) {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin, can not fetch filesystem ID for %s:", vo.FsName, err)

		return 0, err
	}

	volumes, err := fsa.EnumerateVolumes()
	if err != nil {
		log.ErrorLog(ctx, "could not list volumes, can not fetch filesystem ID for %s:", vo.FsName, err)

		return 0, err
	}

	for _, vol := range volumes {
		if vol.Name == vo.FsName {
			return vol.ID, nil
		}
	}

	log.ErrorLog(ctx, "failed to list volume %s", vo.FsName)

	return 0, cerrors.ErrVolumeNotFound
}

func (vo *volumeOptions) getMetadataPool(ctx context.Context) (string, error) {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin, can not fetch metadata pool for %s:", vo.FsName, err)

		return "", err
	}

	fsPoolInfos, err := fsa.ListFileSystems()
	if err != nil {
		log.ErrorLog(ctx, "could not list filesystems, can not fetch metadata pool for %s:", vo.FsName, err)

		return "", err
	}

	for _, fspi := range fsPoolInfos {
		if fspi.Name == vo.FsName {
			return fspi.MetadataPool, nil
		}
	}

	return "", fmt.Errorf("%w: could not find metadata pool for %s", util.ErrPoolNotFound, vo.FsName)
}

func (vo *volumeOptions) getFsName(ctx context.Context) (string, error) {
	fsa, err := vo.conn.GetFSAdmin()
	if err != nil {
		log.ErrorLog(ctx, "could not get FSAdmin, can not fetch filesystem name for ID %d:", vo.FscID, err)

		return "", err
	}

	volumes, err := fsa.EnumerateVolumes()
	if err != nil {
		log.ErrorLog(ctx, "could not list volumes, can not fetch filesystem name for ID %d:", vo.FscID, err)

		return "", err
	}

	for _, vol := range volumes {
		if vol.ID == vo.FscID {
			return vol.Name, nil
		}
	}

	return "", fmt.Errorf("%w: fscID (%d) not found in Ceph cluster", util.ErrPoolNotFound, vo.FscID)
}
