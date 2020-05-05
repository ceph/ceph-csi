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

	"github.com/ceph/ceph-csi/internal/util"

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

	snapData, err := snapJournal.CheckReservation(ctx, rbdSnap.Monitors, cr, rbdSnap.JournalPool,
		rbdSnap.RequestName, rbdSnap.NamePrefix, rbdSnap.RbdImageName, "")
	if err != nil {
		return false, err
	}
	if snapData == nil {
		return false, nil
	}
	snapUUID := snapData.ImageUUID
	rbdSnap.RbdSnapName = snapData.ImageAttributes.ImageName

	// it should never happen that this disagrees, but check
	if rbdSnap.Pool != snapData.ImagePool {
		return false, fmt.Errorf("stored snapshot pool (%s) and expected snapshot pool (%s) mismatch",
			snapData.ImagePool, rbdSnap.Pool)
	}

	// Fetch on-disk image attributes
	err = updateSnapWithImageInfo(ctx, rbdSnap, cr)
	if err != nil {
		if _, ok := err.(ErrSnapNotFound); ok {
			err = snapJournal.UndoReservation(ctx, rbdSnap.Monitors, cr, rbdSnap.JournalPool,
				rbdSnap.Pool, rbdSnap.RbdSnapName, rbdSnap.RequestName)
			return false, err
		}
		return false, err
	}

	// found a snapshot already available, process and return its information
	rbdSnap.SnapID, err = util.GenerateVolID(ctx, rbdSnap.Monitors, cr, snapData.ImagePoolID, rbdSnap.Pool,
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
func (rv *rbdVolume) Exists(ctx context.Context) (bool, error) {
	err := validateRbdVol(rv)
	if err != nil {
		return false, err
	}

	kmsID := ""
	if rv.Encrypted {
		kmsID = rv.KMS.GetID()
	}

	imageData, err := volJournal.CheckReservation(ctx, rv.Monitors, rv.conn.Creds, rv.JournalPool,
		rv.RequestName, rv.NamePrefix, "", kmsID)
	if err != nil {
		return false, err
	}
	if imageData == nil {
		return false, nil
	}

	imageUUID := imageData.ImageUUID
	rv.RbdImageName = imageData.ImageAttributes.ImageName

	// check if topology constraints match what is found
	rv.Topology, err = util.MatchTopologyForPool(rv.TopologyPools, rv.TopologyRequirement,
		imageData.ImagePool)
	if err != nil {
		// TODO check if need any undo operation here, or ErrVolNameConflict
		return false, err
	}
	// update Pool, if it was topology constrained
	if rv.Topology != nil {
		rv.Pool = imageData.ImagePool
	}

	// NOTE: Return volsize should be on-disk volsize, not request vol size, so
	// save it for size checks before fetching image data
	requestSize := rv.VolSize
	// Fetch on-disk image attributes and compare against request
	err = updateVolWithImageInfo(ctx, rv, rv.conn.Creds)
	if err != nil {
		if _, ok := err.(ErrImageNotFound); ok {
			err = volJournal.UndoReservation(ctx, rv.Monitors, rv.conn.Creds, rv.JournalPool, rv.Pool,
				rv.RbdImageName, rv.RequestName)
			return false, err
		}
		return false, err
	}

	// size checks
	if rv.VolSize < requestSize {
		err = fmt.Errorf("image with the same name (%s) but with different size already exists",
			rv.RbdImageName)
		return false, ErrVolNameConflict{rv.RbdImageName, err}
	}
	// TODO: We should also ensure image features and format is the same

	// found a volume already available, process and return it!
	rv.VolID, err = util.GenerateVolID(ctx, rv.Monitors, rv.conn.Creds, imageData.ImagePoolID, rv.Pool,
		rv.ClusterID, imageUUID, volIDVersion)
	if err != nil {
		return false, err
	}

	klog.V(4).Infof(util.Log(ctx, "found existing volume (%s) with image name (%s) for request (%s)"),
		rv.VolID, rv.RbdImageName, rv.RequestName)

	return true, nil
}

// reserveSnap is a helper routine to request a rbdSnapshot name reservation and generate the
// volume ID for the generated name
func reserveSnap(ctx context.Context, rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	var (
		snapUUID string
		err      error
	)

	journalPoolID, imagePoolID, err := util.GetPoolIDs(ctx, rbdSnap.Monitors, rbdSnap.JournalPool, rbdSnap.Pool, cr)
	if err != nil {
		return err
	}

	snapUUID, rbdSnap.RbdSnapName, err = snapJournal.ReserveName(ctx, rbdSnap.Monitors, cr, rbdSnap.JournalPool, journalPoolID,
		rbdSnap.Pool, imagePoolID, rbdSnap.RequestName, rbdSnap.NamePrefix, rbdSnap.RbdImageName, "")
	if err != nil {
		return err
	}

	rbdSnap.SnapID, err = util.GenerateVolID(ctx, rbdSnap.Monitors, cr, imagePoolID, rbdSnap.Pool,
		rbdSnap.ClusterID, snapUUID, volIDVersion)
	if err != nil {
		return err
	}

	klog.V(4).Infof(util.Log(ctx, "generated Volume ID (%s) and image name (%s) for request name (%s)"),
		rbdSnap.SnapID, rbdSnap.RbdSnapName, rbdSnap.RequestName)

	return nil
}

func updateTopologyConstraints(rbdVol *rbdVolume, rbdSnap *rbdSnapshot) error {
	var err error
	if rbdSnap != nil {
		// check if topology constraints matches snapshot pool
		rbdVol.Topology, err = util.MatchTopologyForPool(rbdVol.TopologyPools,
			rbdVol.TopologyRequirement, rbdSnap.Pool)
		if err != nil {
			return err
		}

		// update Pool, if it was topology constrained
		if rbdVol.Topology != nil {
			rbdVol.Pool = rbdSnap.Pool
		}

		return nil
	}
	// update request based on topology constrained parameters (if present)
	poolName, dataPoolName, topology, err := util.FindPoolAndTopology(rbdVol.TopologyPools, rbdVol.TopologyRequirement)
	if err != nil {
		return err
	}
	if poolName != "" {
		rbdVol.Pool = poolName
		rbdVol.DataPool = dataPoolName
		rbdVol.Topology = topology
	}

	return nil
}

// reserveVol is a helper routine to request a rbdVolume name reservation and generate the
// volume ID for the generated name
func reserveVol(ctx context.Context, rbdVol *rbdVolume, rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	var (
		imageUUID string
		err       error
	)

	err = updateTopologyConstraints(rbdVol, rbdSnap)
	if err != nil {
		return err
	}

	journalPoolID, imagePoolID, err := util.GetPoolIDs(ctx, rbdVol.Monitors, rbdVol.JournalPool, rbdVol.Pool, cr)
	if err != nil {
		return err
	}

	kmsID := ""
	if rbdVol.Encrypted {
		kmsID = rbdVol.KMS.GetID()
	}

	imageUUID, rbdVol.RbdImageName, err = volJournal.ReserveName(ctx, rbdVol.Monitors, cr, rbdVol.JournalPool, journalPoolID,
		rbdVol.Pool, imagePoolID, rbdVol.RequestName, rbdVol.NamePrefix, "", kmsID)
	if err != nil {
		return err
	}

	rbdVol.VolID, err = util.GenerateVolID(ctx, rbdVol.Monitors, cr, imagePoolID, rbdVol.Pool,
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
	err := snapJournal.UndoReservation(ctx, rbdSnap.Monitors, cr, rbdSnap.JournalPool, rbdSnap.Pool,
		rbdSnap.RbdSnapName, rbdSnap.RequestName)

	return err
}

// undoVolReservation is a helper routine to undo a name reservation for rbdVolume
func undoVolReservation(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) error {
	err := volJournal.UndoReservation(ctx, rbdVol.Monitors, cr, rbdVol.JournalPool, rbdVol.Pool,
		rbdVol.RbdImageName, rbdVol.RequestName)

	return err
}
