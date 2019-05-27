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

package rbd

import (
	"fmt"
	"strings"

	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

func validateNonEmptyField(field, fieldName, structName string) error {
	if field == "" {
		return fmt.Errorf("value '%s' in '%s' structure cannot be empty", fieldName, structName)
	}

	return nil
}

func validateRbdSnap(rbdSnap *rbdSnapshot) error {
	if err := validateNonEmptyField(rbdSnap.RequestName, "RequestName", "rbdSnapshot"); err != nil {
		return err
	}

	if err := validateNonEmptyField(rbdSnap.Monitors, "Monitors", "rbdSnapshot"); err != nil {
		return err
	}

	if err := validateNonEmptyField(rbdSnap.AdminID, "AdminID", "rbdSnapshot"); err != nil {
		return err
	}

	if err := validateNonEmptyField(rbdSnap.Pool, "Pool", "rbdSnapshot"); err != nil {
		return err
	}

	if err := validateNonEmptyField(rbdSnap.RbdImageName, "RbdImageName", "rbdSnapshot"); err != nil {
		return err
	}

	if err := validateNonEmptyField(rbdSnap.ClusterID, "ClusterID", "rbdSnapshot"); err != nil {
		return err
	}

	return nil
}

func validateRbdVol(rbdVol *rbdVolume) error {
	if err := validateNonEmptyField(rbdVol.RequestName, "RequestName", "rbdVolume"); err != nil {
		return err
	}

	if err := validateNonEmptyField(rbdVol.Monitors, "Monitors", "rbdVolume"); err != nil {
		return err
	}

	if err := validateNonEmptyField(rbdVol.AdminID, "AdminID", "rbdVolume"); err != nil {
		return err
	}

	if err := validateNonEmptyField(rbdVol.Pool, "Pool", "rbdVolume"); err != nil {
		return err
	}

	if err := validateNonEmptyField(rbdVol.ClusterID, "ClusterID", "rbdVolume"); err != nil {
		return err
	}

	if rbdVol.VolSize == 0 {
		return errors.New("value 'VolSize' in 'rbdVolume' structure cannot be 0")
	}

	return nil
}

/*
checkSnapExists, and its counterpart checkVolExists, function as checks to determine if passed
in rbdSnapshot or rbdVolume exists on the backend.

**NOTE:** These functions manipulate the rados omaps that hold information regarding
volume names as requested by the CSI drivers. Hence, these need to be invoked only when the
respective CSI driver generated snapshot or volume name based locks are held, as otherwise racy
access to these omaps may end up leaving them in an inconsistent state.

These functions need enough information about cluster and pool (ie, Monitors, Pool, IDs filled in)
to operate. They further require that the RequestName element of the structure have a valid value
to operate on and determine if the said RequestName already exists on the backend.

These functions populate the snapshot or the image name, its attributes and the CSI snapshot/volume
ID for the same when succesful.

These functions also cleanup omap reservations that are stale. I.e when omap entries exist and
backing images or snapshots are missing, or one of the omaps exist and the next is missing. This is
because, the order of omap creation and deletion are inverse of each other, and protected by the
request name lock, and hence any stale omaps are leftovers from incomplete transactions and are
hence safe to garbage collect.
*/
func checkSnapExists(rbdSnap *rbdSnapshot, credentials map[string]string) (found bool, err error) {
	if err = validateRbdSnap(rbdSnap); err != nil {
		return false, err
	}

	key, err := getKey(rbdSnap.AdminID, credentials)
	if err != nil {
		return false, err
	}

	// check if request name is already part of the snaps omap
	snapUUID, err := util.GetOMapValue(rbdSnap.Monitors, rbdSnap.AdminID,
		key, rbdSnap.Pool, csiSnapsDirectory, csiSnapNameKeyPrefix+rbdSnap.RequestName)
	if err != nil {
		// error should specifically be not found, for image to be absent, any other error
		// is not conclusive, and we should not proceed
		if _, ok := err.(util.ErrKeyNotFound); ok {
			return false, nil
		}

		return false, err
	}

	rbdSnap.RbdSnapName = rbdSnapNamePrefix + snapUUID

	// TODO: use listomapvals to dump all keys instead of reading them one-by-one
	// check if the snapshot image omap is present
	savedSnapName, err := util.GetOMapValue(rbdSnap.Monitors, rbdSnap.AdminID,
		key, rbdSnap.Pool, rbdSnapOMapPrefix+snapUUID, rbdSnapCSISnapNameKey)
	if err != nil {
		if _, ok := err.(util.ErrKeyNotFound); ok {
			err = unreserveSnap(rbdSnap, credentials)
		}
		return false, err
	}

	// check if snapshot image omap points back to the request name
	if savedSnapName != rbdSnap.RequestName {
		// NOTE: This should never be possible, hence no cleanup, but log error
		// and return, as cleanup may need to occur manually!
		return false, fmt.Errorf("internal state inconsistent, omap snap"+
			" names disagree, request name (%s) snap name (%s) image omap"+
			" snap name (%s)", rbdSnap.RequestName, rbdSnap.RbdSnapName, savedSnapName)
	}

	// check if the snapshot source image omap is present
	savedVolName, err := util.GetOMapValue(rbdSnap.Monitors, rbdSnap.AdminID,
		key, rbdSnap.Pool, rbdSnapOMapPrefix+snapUUID, rbdSnapSourceImageKey)
	if err != nil {
		if _, ok := err.(util.ErrKeyNotFound); ok {
			err = unreserveSnap(rbdSnap, credentials)
		}
		return false, err
	}

	// check if snapshot source image omap points back to the source volume passed in
	if savedVolName != rbdSnap.RbdImageName {
		// NOTE: This can happen if there is a snapname conflict, and we alerady have a snapshot
		// with the same name pointing to a different RBD image as the source
		err = fmt.Errorf("snapname points to different image, request name (%s)"+
			" image name (%s) image omap"+" volume name (%s)",
			rbdSnap.RequestName, rbdSnap.RbdImageName, savedVolName)
		return false, ErrSnapNameConflict{rbdSnap.RequestName, err}
	}

	// Fetch on-disk image attributes
	err = updateSnapWithImageInfo(rbdSnap, credentials)
	if err != nil {
		if _, ok := err.(ErrSnapNotFound); ok {
			err = unreserveSnap(rbdSnap, credentials)
			return false, err
		}

		return false, err
	}

	// found a snapshot already available, process and return its information
	poolID, err := util.GetPoolID(rbdSnap.Monitors, rbdSnap.AdminID, key, rbdSnap.Pool)
	if err != nil {
		return false, err
	}

	vi := util.CSIIdentifier{
		PoolID:          poolID,
		EncodingVersion: volIDVersion,
		ClusterID:       rbdSnap.ClusterID,
		ObjectUUID:      snapUUID,
	}
	rbdSnap.SnapID, err = vi.ComposeCSIID()
	if err != nil {
		return false, err
	}

	klog.V(4).Infof("Found existing snap (%s) with snap name (%s) for request (%s)",
		rbdSnap.SnapID, rbdSnap.RbdSnapName, rbdSnap.RequestName)

	return true, nil
}

/*
Check comment on checkSnapExists, to understand how this function behaves

**NOTE:** These functions manipulate the rados omaps that hold information regarding
volume names as requested by the CSI drivers. Hence, these need to be invoked only when the
respective CSI snapshot or volume name based locks are held, as otherwise racy access to these
omaps may end up leaving the omaps in an inconsistent state.
*/
func checkVolExists(rbdVol *rbdVolume, credentials map[string]string) (found bool, err error) {
	var vi util.CSIIdentifier

	if err = validateRbdVol(rbdVol); err != nil {
		return false, err
	}

	key, err := getKey(rbdVol.AdminID, credentials)
	if err != nil {
		return false, err
	}

	// check if request name is already part of the volumes omap
	imageUUID, err := util.GetOMapValue(rbdVol.Monitors, rbdVol.AdminID,
		key, rbdVol.Pool, csiVolsDirectory, csiVolNameKeyPrefix+rbdVol.RequestName)
	if err != nil {
		// error should specifically be not found, for image to be absent, any other error
		// is not conclusive, and we should not proceed
		if _, ok := err.(util.ErrKeyNotFound); ok {
			return false, nil
		}

		return false, err
	}

	rbdVol.RbdImageName = rbdImgNamePrefix + imageUUID

	// check if the image omap is present
	savedVolName, err := util.GetOMapValue(rbdVol.Monitors, rbdVol.AdminID,
		key, rbdVol.Pool, rbdImageOMapPrefix+imageUUID, rbdImageCSIVolNameKey)
	if err != nil {
		if _, ok := err.(util.ErrKeyNotFound); ok {
			err = unreserveVol(rbdVol, credentials)
		}
		return false, err
	}

	// check if image omap points back to the request name
	if savedVolName != rbdVol.RequestName {
		// NOTE: This should never be possible, hence no cleanup, but log error
		// and return, as cleanup may need to occur manually!
		return false, fmt.Errorf("internal state inconsistent, omap volume"+
			" names disagree, request name (%s) image name (%s) image omap"+
			" volume name (%s)", rbdVol.RequestName, rbdVol.RbdImageName, savedVolName)
	}

	// NOTE: Return volsize should be on-disk volsize, not request vol size, so
	// save it for size checks before fetching image data
	requestSize := rbdVol.VolSize
	// Fetch on-disk image attributes and compare against request
	err = updateVolWithImageInfo(rbdVol, credentials)
	if err != nil {
		if _, ok := err.(ErrImageNotFound); ok {
			err = unreserveVol(rbdVol, credentials)
			return false, err
		}

		return false, err
	}

	// size checks
	if rbdVol.VolSize < requestSize {
		err = fmt.Errorf("image with the same name (%s) but with different size already exists",
			rbdVol.RbdImageName)
		return false, ErrVolNameConflict{rbdVol.RbdImageName, err}
	}
	// TODO: We should also ensure image features and format is the same

	// found a volume already available, process and return it!
	poolID, err := util.GetPoolID(rbdVol.Monitors, rbdVol.AdminID, key, rbdVol.Pool)
	if err != nil {
		return false, err
	}

	vi = util.CSIIdentifier{
		PoolID:          poolID,
		EncodingVersion: volIDVersion,
		ClusterID:       rbdVol.ClusterID,
		ObjectUUID:      imageUUID,
	}
	rbdVol.VolID, err = vi.ComposeCSIID()
	if err != nil {
		return false, err
	}

	klog.V(4).Infof("Found existng volume (%s) with image name (%s) for request (%s)",
		rbdVol.VolID, rbdVol.RbdImageName, rbdVol.RequestName)

	return true, nil
}

/*
unreserveSnap and unreserveVol remove omaps associated with the snapshot and the image name,
and also remove the corresponding request name key in the snaps or volumes omaps respectively.

This is performed within the request name lock, to ensure that requests with the same name do not
manipulate the omap entries concurrently.
*/
func unreserveSnap(rbdSnap *rbdSnapshot, credentials map[string]string) error {
	key, err := getKey(rbdSnap.AdminID, credentials)
	if err != nil {
		return err
	}

	// delete snap image omap (first, inverse of create order)
	snapUUID := strings.TrimPrefix(rbdSnap.RbdSnapName, rbdSnapNamePrefix)
	err = util.RemoveObject(rbdSnap.Monitors, rbdSnap.AdminID, key, rbdSnap.Pool, rbdSnapOMapPrefix+snapUUID)
	if err != nil {
		if _, ok := err.(util.ErrObjectNotFound); !ok {
			klog.Errorf("failed removing oMap %s (%s)", rbdSnapOMapPrefix+snapUUID, err)
			return err
		}
	}

	// delete the request name omap key (last, inverse of create order)
	err = util.RemoveOMapKey(rbdSnap.Monitors, rbdSnap.AdminID, key, rbdSnap.Pool,
		csiSnapsDirectory, csiSnapNameKeyPrefix+rbdSnap.RequestName)
	if err != nil {
		klog.Errorf("failed removing oMap key %s (%s)", csiSnapNameKeyPrefix+rbdSnap.RequestName, err)
		return err
	}

	return nil
}

func unreserveVol(rbdVol *rbdVolume, credentials map[string]string) error {
	key, err := getKey(rbdVol.AdminID, credentials)
	if err != nil {
		return err
	}

	// delete image omap (first, inverse of create order)
	imageUUID := strings.TrimPrefix(rbdVol.RbdImageName, rbdImgNamePrefix)
	err = util.RemoveObject(rbdVol.Monitors, rbdVol.AdminID, key, rbdVol.Pool, rbdImageOMapPrefix+imageUUID)
	if err != nil {
		if _, ok := err.(util.ErrObjectNotFound); !ok {
			klog.Errorf("failed removing oMap %s (%s)", rbdImageOMapPrefix+imageUUID, err)
			return err
		}
	}

	// delete the request name omap key (last, inverse of create order)
	err = util.RemoveOMapKey(rbdVol.Monitors, rbdVol.AdminID, key, rbdVol.Pool,
		csiVolsDirectory, csiVolNameKeyPrefix+rbdVol.RequestName)
	if err != nil {
		klog.Errorf("failed removing oMap key %s (%s)", csiVolNameKeyPrefix+rbdVol.RequestName, err)
		return err
	}

	return nil
}

// reserveOMapName creates an omap with passed in oMapNamePrefix and a generated <uuid>.
// It ensures generated omap name does not already exist and if conflicts are detected, a set
// number of retires with newer uuids are attempted before returning an error
func reserveOMapName(monitors, adminID, key, poolName, oMapNamePrefix string) (string, error) {
	var iterUUID string

	maxAttempts := 5
	attempt := 1
	for attempt <= maxAttempts {
		// generate a uuid for the image name
		iterUUID = uuid.NewUUID().String()

		err := util.CreateObject(monitors, adminID, key, poolName, oMapNamePrefix+iterUUID)
		if err != nil {
			if _, ok := err.(util.ErrObjectExists); ok {
				attempt++
				// try again with a different uuid, for maxAttempts tries
				klog.V(4).Infof("uuid (%s) conflict detected, retrying (attempt %d of %d)",
					iterUUID, attempt, maxAttempts)
				continue
			}

			return "", err
		}

		break
	}

	if attempt > maxAttempts {
		return "", errors.New("uuid conflicts exceeds retry threshold")
	}

	return iterUUID, nil
}

/*
reserveSnap and reserveVol add respective entries to the volumes and snapshots omaps, post
generating a target snapshot or image name for use. Further, these functions create the snapshot or
image name omaps, to store back pointers to the CSI generated request names.

This is performed within the request name lock, to ensure that requests with the same name do not
manipulate the omap entries concurrently.
*/
func reserveSnap(rbdSnap *rbdSnapshot, credentials map[string]string) error {
	var vi util.CSIIdentifier

	key, err := getKey(rbdSnap.AdminID, credentials)
	if err != nil {
		return err
	}

	poolID, err := util.GetPoolID(rbdSnap.Monitors, rbdSnap.AdminID, key,
		rbdSnap.Pool)
	if err != nil {
		return err
	}

	// Create the snapUUID based omap first, to reserve the same and avoid conflicts
	// NOTE: If any service loss occurs post creation of the snap omap, and before
	// setting the omap key (rbdSnapCSISnapNameKey) to point back to the snaps omap, the
	// snap omap key will leak
	snapUUID, err := reserveOMapName(rbdSnap.Monitors, rbdSnap.AdminID, key, rbdSnap.Pool,
		rbdSnapOMapPrefix)
	if err != nil {
		return err
	}

	// Create request snapUUID key in csi snaps omap and store the uuid based
	// snap name into it
	err = util.SetOMapKeyValue(rbdSnap.Monitors, rbdSnap.AdminID, key,
		rbdSnap.Pool, csiSnapsDirectory, csiSnapNameKeyPrefix+rbdSnap.RequestName, snapUUID)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			klog.Warningf("reservation failed for volume: %s", rbdSnap.RequestName)
			errDefer := unreserveSnap(rbdSnap, credentials)
			if errDefer != nil {
				klog.Warningf("failed undoing reservation of snapshot: %s (%v)",
					rbdSnap.RequestName, errDefer)
			}
		}
	}()

	// Create snap name based omap and store CSI request name key and source information
	err = util.SetOMapKeyValue(rbdSnap.Monitors, rbdSnap.AdminID, key, rbdSnap.Pool,
		rbdSnapOMapPrefix+snapUUID, rbdSnapCSISnapNameKey, rbdSnap.RequestName)
	if err != nil {
		return err
	}
	err = util.SetOMapKeyValue(rbdSnap.Monitors, rbdSnap.AdminID, key, rbdSnap.Pool,
		rbdSnapOMapPrefix+snapUUID, rbdSnapSourceImageKey, rbdSnap.RbdImageName)
	if err != nil {
		return err
	}

	// generate the volume ID to return to the CO system
	vi = util.CSIIdentifier{
		PoolID:          poolID,
		EncodingVersion: volIDVersion,
		ClusterID:       rbdSnap.ClusterID,
		ObjectUUID:      snapUUID,
	}
	rbdSnap.SnapID, err = vi.ComposeCSIID()
	if err != nil {
		return err
	}
	rbdSnap.RbdSnapName = rbdSnapNamePrefix + snapUUID
	klog.V(4).Infof("Generated Volume ID (%s) and image name (%s) for request name (%s)",
		rbdSnap.SnapID, rbdSnap.RbdImageName, rbdSnap.RequestName)

	return nil
}

func reserveVol(rbdVol *rbdVolume, credentials map[string]string) error {
	var vi util.CSIIdentifier

	key, err := getKey(rbdVol.AdminID, credentials)
	if err != nil {
		return err
	}

	poolID, err := util.GetPoolID(rbdVol.Monitors, rbdVol.AdminID, key,
		rbdVol.Pool)
	if err != nil {
		return err
	}

	// Create the imageUUID based omap first, to reserve the same and avoid conflicts
	// NOTE: If any service loss occurs post creation of the image omap, and before
	// setting the omap key (rbdImageCSIVolNameKey) to point back to the volumes omap,
	// the image omap key will leak
	imageUUID, err := reserveOMapName(rbdVol.Monitors, rbdVol.AdminID, key, rbdVol.Pool, rbdImageOMapPrefix)
	if err != nil {
		return err
	}

	// Create request volName key in csi volumes omap and store the uuid based
	// image name into it
	err = util.SetOMapKeyValue(rbdVol.Monitors, rbdVol.AdminID, key,
		rbdVol.Pool, csiVolsDirectory, csiVolNameKeyPrefix+rbdVol.RequestName, imageUUID)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			klog.Warningf("reservation failed for volume: %s", rbdVol.RequestName)
			errDefer := unreserveVol(rbdVol, credentials)
			if errDefer != nil {
				klog.Warningf("failed undoing reservation of volume: %s (%v)",
					rbdVol.RequestName, errDefer)
			}
		}
	}()

	// Create image name based omap and store CSI request volume name key and data
	err = util.SetOMapKeyValue(rbdVol.Monitors, rbdVol.AdminID, key, rbdVol.Pool,
		rbdImageOMapPrefix+imageUUID, rbdImageCSIVolNameKey, rbdVol.RequestName)
	if err != nil {
		return err
	}

	// generate the volume ID to return to the CO system
	vi = util.CSIIdentifier{
		PoolID:          poolID,
		EncodingVersion: volIDVersion,
		ClusterID:       rbdVol.ClusterID,
		ObjectUUID:      imageUUID,
	}
	rbdVol.VolID, err = vi.ComposeCSIID()
	if err != nil {
		return err
	}
	rbdVol.RbdImageName = rbdImgNamePrefix + imageUUID
	klog.V(4).Infof("Generated Volume ID (%s) and image name (%s) for request name (%s)",
		rbdVol.VolID, rbdVol.RbdImageName, rbdVol.RequestName)

	return nil
}
