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
	"context"
	"fmt"

	"github.com/ceph/ceph-csi/pkg/util"

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
	var err error

	if err = validateNonEmptyField(rbdSnap.RequestName, "RequestName", "rbdSnapshot"); err != nil {
		return err
	}

	if err = validateNonEmptyField(rbdSnap.Monitors, "Monitors", "rbdSnapshot"); err != nil {
		return err
	}

	if err = validateNonEmptyField(rbdSnap.Pool, "Pool", "rbdSnapshot"); err != nil {
		return err
	}

	if err = validateNonEmptyField(rbdSnap.RbdImageName, "RbdImageName", "rbdSnapshot"); err != nil {
		return err
	}

	if err = validateNonEmptyField(rbdSnap.ClusterID, "ClusterID", "rbdSnapshot"); err != nil {
		return err
	}

	return err
}

func validateRbdVol(rbdVol *rbdVolume) error {
	var err error

	if err = validateNonEmptyField(rbdVol.RequestName, "RequestName", "rbdVolume"); err != nil {
		return err
	}

	if err = validateNonEmptyField(rbdVol.Monitors, "Monitors", "rbdVolume"); err != nil {
		return err
	}

	if err = validateNonEmptyField(rbdVol.Pool, "Pool", "rbdVolume"); err != nil {
		return err
	}

	if err = validateNonEmptyField(rbdVol.ClusterID, "ClusterID", "rbdVolume"); err != nil {
		return err
	}

	if rbdVol.VolSize == 0 {
		return errors.New("value 'VolSize' in 'rbdVolume' structure cannot be 0")
	}

	return err
}

/*
checkSnapExists, and its counterpart checkVolExists, function checks if the passed in rbdSnapshot
or rbdVolume exists on the backend.

**NOTE:** These functions manipulate the rados omaps that hold information regarding
volume names as requested by the CSI drivers. Hence, these need to be invoked only when the
respective CSI driver generated snapshot or volume name based locks are held, as otherwise racy
access to these omaps may end up leaving them in an inconsistent state.

These functions need enough information about cluster and pool (ie, Monitors, Pool, IDs filled in)
to operate. They further require that the RequestName element of the structure have a valid value
to operate on and determine if the said RequestName already exists on the backend.

These functions populate the snapshot or the image name, its attributes and the CSI snapshot/volume
ID for the same when successful.

These functions also cleanup omap reservations that are stale. I.e when omap entries exist and
backing images or snapshots are missing, or one of the omaps exist and the next is missing. This is
because, the order of omap creation and deletion are inverse of each other, and protected by the
request name lock, and hence any stale omaps are leftovers from incomplete transactions and are
hence safe to garbage collect.
*/
func checkSnapExists(ctx context.Context, rbdSnap *rbdSnapshot, cr *util.Credentials) (bool, error) {
	err := validateRbdSnap(rbdSnap)
	if err != nil {
		return false, err
	}

	snapUUID, err := snapJournal.CheckReservation(ctx, rbdSnap.Monitors, cr, rbdSnap.Pool,
		rbdSnap.RequestName, rbdSnap.NamePrefix, rbdSnap.RbdImageName, "")
	if err != nil {
		return false, err
	}
	if snapUUID == "" {
		return false, nil
	}

	// now that we now that the reservation exists, let's get the image name from
	// the omap
	_, rbdSnap.RbdSnapName, _, _, err = volJournal.GetObjectUUIDData(ctx, rbdSnap.Monitors, cr,
		rbdSnap.Pool, snapUUID, false)
	if err != nil {
		return false, err
	}

	// Fetch on-disk image attributes
	err = updateSnapWithImageInfo(ctx, rbdSnap, cr)
	if err != nil {
		if _, ok := err.(ErrSnapNotFound); ok {
			err = snapJournal.UndoReservation(ctx, rbdSnap.Monitors, cr, rbdSnap.Pool,
				rbdSnap.RbdSnapName, rbdSnap.RequestName)
			return false, err
		}
		return false, err
	}

	// found a snapshot already available, process and return its information
	rbdSnap.SnapID, err = util.GenerateVolID(ctx, rbdSnap.Monitors, cr, rbdSnap.Pool,
		rbdSnap.ClusterID, snapUUID, volIDVersion)
	if err != nil {
		return false, err
	}

	klog.V(4).Infof(util.Log(ctx, "found existing snap (%s) with snap name (%s) for request (%s)"),
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
func checkVolExists(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) (bool, error) {
	err := validateRbdVol(rbdVol)
	if err != nil {
		return false, err
	}

	kmsID := ""
	if rbdVol.Encrypted {
		kmsID = rbdVol.KMS.GetID()
	}
	imageUUID, err := volJournal.CheckReservation(ctx, rbdVol.Monitors, cr, rbdVol.Pool,
		rbdVol.RequestName, rbdVol.NamePrefix, "", kmsID)
	if err != nil {
		return false, err
	}
	if imageUUID == "" {
		return false, nil
	}

	// now that we now that the reservation exists, let's get the image name from
	// the omap
	_, rbdVol.RbdImageName, _, _, err = volJournal.GetObjectUUIDData(ctx, rbdVol.Monitors, cr,
		rbdVol.Pool, imageUUID, false)
	if err != nil {
		return false, err
	}

	// NOTE: Return volsize should be on-disk volsize, not request vol size, so
	// save it for size checks before fetching image data
	requestSize := rbdVol.VolSize
	// Fetch on-disk image attributes and compare against request
	err = updateVolWithImageInfo(ctx, rbdVol, cr)
	if err != nil {
		if _, ok := err.(ErrImageNotFound); ok {
			err = volJournal.UndoReservation(ctx, rbdVol.Monitors, cr, rbdVol.Pool,
				rbdVol.RbdImageName, rbdVol.RequestName)
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
	rbdVol.VolID, err = util.GenerateVolID(ctx, rbdVol.Monitors, cr, rbdVol.Pool,
		rbdVol.ClusterID, imageUUID, volIDVersion)
	if err != nil {
		return false, err
	}

	klog.V(4).Infof(util.Log(ctx, "found existing volume (%s) with image name (%s) for request (%s)"),
		rbdVol.VolID, rbdVol.RbdImageName, rbdVol.RequestName)

	return true, nil
}

// reserveSnap is a helper routine to request a rbdSnapshot name reservation and generate the
// volume ID for the generated name
func reserveSnap(ctx context.Context, rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	var (
		snapUUID string
		err      error
	)

	snapUUID, rbdSnap.RbdSnapName, err = snapJournal.ReserveName(ctx, rbdSnap.Monitors, cr, rbdSnap.Pool,
		rbdSnap.RequestName, rbdSnap.NamePrefix, rbdSnap.RbdImageName, "", "")
	if err != nil {
		return err
	}

	rbdSnap.SnapID, err = util.GenerateVolID(ctx, rbdSnap.Monitors, cr, rbdSnap.Pool,
		rbdSnap.ClusterID, snapUUID, volIDVersion)
	if err != nil {
		return err
	}

	klog.V(4).Infof(util.Log(ctx, "generated Volume ID (%s) and image name (%s) for request name (%s)"),
		rbdSnap.SnapID, rbdSnap.RbdSnapName, rbdSnap.RequestName)

	return nil
}

// reserveVol is a helper routine to request a rbdVolume name reservation and generate the
// volume ID for the generated name
func reserveVol(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) error {
	var (
		imageUUID string
		imageName string
		err       error
	)

	kmsID := ""
	if rbdVol.Encrypted {
		kmsID = rbdVol.KMS.GetID()
	}

	if rbdVol.Mirrored {
		// list and find mirrored image UUID, based on ImageName header
		imageName, err = findImageMatchingHeader(ctx, rbdVol.Monitors, cr, rbdVol.Pool, rbdVol.NamePrefix)
		if err != nil {
			if _, ok := err.(ErrImageNotFound); !ok {
				return err
			}
		} else {
			rbdVol.RbdImageExists = true
		}
	}

	imageUUID, rbdVol.RbdImageName, err = volJournal.ReserveName(ctx, rbdVol.Monitors, cr, rbdVol.Pool,
		rbdVol.RequestName, rbdVol.NamePrefix, "", kmsID, imageName)
	if err != nil {
		return err
	}

	rbdVol.VolID, err = util.GenerateVolID(ctx, rbdVol.Monitors, cr, rbdVol.Pool,
		rbdVol.ClusterID, imageUUID, volIDVersion)
	if err != nil {
		return err
	}

	klog.V(4).Infof(util.Log(ctx, "generated Volume ID (%s) and image name (%s) for request name (%s)"),
		rbdVol.VolID, rbdVol.RbdImageName, rbdVol.RequestName)

	return nil
}

// undoSnapReservation is a helper routine to undo a name reservation for rbdSnapshot
func undoSnapReservation(ctx context.Context, rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	err := snapJournal.UndoReservation(ctx, rbdSnap.Monitors, cr, rbdSnap.Pool,
		rbdSnap.RbdSnapName, rbdSnap.RequestName)

	return err
}

// undoVolReservation is a helper routine to undo a name reservation for rbdVolume
func undoVolReservation(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) error {
	err := volJournal.UndoReservation(ctx, rbdVol.Monitors, cr, rbdVol.Pool,
		rbdVol.RbdImageName, rbdVol.RequestName)

	return err
}
