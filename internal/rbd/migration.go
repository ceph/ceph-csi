/*
Copyright 2021 The Ceph-CSI Authors.

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
	"encoding/hex"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// isMigrationVolID validates if the passed in volID is a volumeID
// of a migrated volume.
func isMigrationVolID(volHash string) bool {
	return strings.Contains(volHash, migIdentifier) &&
		strings.Contains(volHash, migImageNamePrefix) && strings.Contains(volHash, migMonPrefix)
}

// parseMigrationVolID decodes the volume ID and generates a migrationVolID
// struct which consists of mon, image name, pool and clusterID information.
func parseMigrationVolID(vh string) (*migrationVolID, error) {
	mh := &migrationVolID{}
	handSlice := strings.Split(vh, migVolIDFieldSep)
	if len(handSlice) < migVolIDTotalLength {
		// its short of length in this case, so return error
		return nil, ErrInvalidVolID
	}
	// Store pool
	poolHash := strings.Join(handSlice[migVolIDSplitLength:], migVolIDFieldSep)
	poolByte, dErr := hex.DecodeString(poolHash)
	if dErr != nil {
		return nil, ErrMissingPoolNameInVolID
	}
	mh.poolName = string(poolByte)
	// Parse migration mons( for clusterID) and image
	for _, field := range handSlice[:migVolIDSplitLength] {
		switch {
		case strings.Contains(field, migImageNamePrefix):
			imageSli := strings.Split(field, migImageNamePrefix)
			if len(imageSli) > 0 {
				mh.imageName = migInTreeImagePrefix + imageSli[1]
			}
		case strings.Contains(field, migMonPrefix):
			// ex: mons-7982de6a23b77bce50b1ba9f2e879cce
			mh.clusterID = strings.Trim(field, migMonPrefix)
		}
	}
	if mh.imageName == "" {
		return nil, ErrMissingImageNameInVolID
	}
	if mh.poolName == "" {
		return nil, ErrMissingPoolNameInVolID
	}
	if mh.clusterID == "" {
		return nil, ErrDecodeClusterIDFromMonsInVolID
	}

	return mh, nil
}

// deleteMigratedVolume get rbd volume details from the migration volID
// and delete the volume from the cluster, return err if there was an error on the process.
func deleteMigratedVolume(ctx context.Context, parsedMigHandle *migrationVolID, cr *util.Credentials) error {
	rv, err := genVolFromMigVolID(ctx, parsedMigHandle, cr)
	if err != nil {
		return err
	}
	defer rv.Destroy()
	err = rv.deleteImage(ctx)
	if err != nil {
		log.ErrorLog(ctx, "failed to delete rbd image: %s, err: %v", rv, err)
	}

	return err
}

// genVolFromMigVolID populate rbdVol struct from the migration volID.
func genVolFromMigVolID(ctx context.Context, migVolID *migrationVolID, cr *util.Credentials) (*rbdVolume, error) {
	var err error
	rv := &rbdVolume{}

	// fill details to rv struct from parsed migration handle
	rv.RbdImageName = migVolID.imageName
	rv.Pool = migVolID.poolName
	rv.ClusterID = migVolID.clusterID
	rv.Monitors, err = util.Mons(util.CsiConfigFile, rv.ClusterID)
	if err != nil {
		log.ErrorLog(ctx, "failed to fetch monitors using clusterID: %s, err: %v", rv.ClusterID, err)

		return nil, err
	}
	// connect to the volume.
	err = rv.Connect(cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to get connected to the rbd image : %s, err: %v", rv.RbdImageName, err)

		return nil, err
	}

	return rv, nil
}
