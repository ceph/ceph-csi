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
	"errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/journal"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/k8s"
	"github.com/ceph/ceph-csi/internal/util/log"
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

func getEncryptionConfig(rbdVol *rbdVolume) (string, util.EncryptionType) {
	switch {
	case rbdVol.isBlockEncrypted():
		return rbdVol.blockEncryption.GetID(), util.EncryptionTypeBlock
	case rbdVol.isFileEncrypted():
		return rbdVol.fileEncryption.GetID(), util.EncryptionTypeFile
	default:
		return "", util.EncryptionTypeNone
	}
}

/*
checkSnapCloneExists, and its counterpart checkVolExists, function checks if
the passed in rbdSnapshot or rbdVolume exists on the backend.

**NOTE:** These functions manipulate the rados omaps that hold information
regarding volume names as requested by the CSI drivers. Hence, these need to be
invoked only when the respective CSI driver generated snapshot or volume name
based locks are held, as otherwise racy access to these omaps may end up
leaving them in an inconsistent state.

These functions need enough information about cluster and pool (ie, Monitors,
Pool, IDs filled in) to operate. They further require that the RequestName
element of the structure have a valid value to operate on and determine if the
said RequestName already exists on the backend.

These functions populate the snapshot or the image name, its attributes and the
CSI snapshot/volume ID for the same when successful.

These functions also cleanup omap reservations that are stale. I.e when omap
entries exist and backing images or snapshots are missing, or one of the omaps
exist and the next is missing. This is because, the order of omap creation and
deletion are inverse of each other, and protected by the request name lock, and
hence any stale omaps are leftovers from incomplete transactions and are hence
safe to garbage collect.
*/
func checkSnapCloneExists(
	ctx context.Context,
	parentVol *rbdVolume,
	rbdSnap *rbdSnapshot,
	cr *util.Credentials,
) (bool, error) {
	err := validateRbdSnap(rbdSnap)
	if err != nil {
		return false, err
	}

	j, err := snapJournal.Connect(rbdSnap.Monitors, rbdSnap.RadosNamespace, cr)
	if err != nil {
		return false, err
	}
	defer j.Destroy()

	snapData, err := j.CheckReservation(ctx, rbdSnap.JournalPool,
		rbdSnap.RequestName, rbdSnap.NamePrefix, rbdSnap.RbdImageName, "", util.EncryptionTypeNone)
	if err != nil {
		return false, err
	}
	if snapData == nil {
		return false, nil
	}
	snapUUID := snapData.ImageUUID
	rbdSnap.RbdSnapName = snapData.ImageAttributes.ImageName
	rbdSnap.ImageID = snapData.ImageAttributes.ImageID

	// it should never happen that this disagrees, but check
	if rbdSnap.Pool != snapData.ImagePool {
		return false, fmt.Errorf("stored snapshot pool (%s) and expected snapshot pool (%s) mismatch",
			snapData.ImagePool, rbdSnap.Pool)
	}

	vol := generateVolFromSnap(rbdSnap)
	defer vol.Destroy()
	err = vol.Connect(cr)
	if err != nil {
		return false, err
	}
	vol.ReservedID = snapUUID
	// Fetch on-disk image attributes
	err = vol.getImageInfo()
	if err != nil {
		if errors.Is(err, ErrImageNotFound) {
			err = parentVol.deleteSnapshot(ctx, rbdSnap)
			if err != nil {
				if !errors.Is(err, ErrSnapNotFound) {
					log.ErrorLog(ctx, "failed to delete snapshot %s: %v", rbdSnap, err)

					return false, err
				}
			}
			err = undoSnapshotCloning(ctx, parentVol, rbdSnap, vol, cr)
		}

		return false, err
	}

	// Snapshot creation transaction is rolled forward if rbd clone image
	// representing the snapshot is found. Any failures till finding the image
	// causes a roll back of the snapshot creation transaction.
	// Code from here on, rolls the transaction forward.

	rbdSnap.CreatedAt = vol.CreatedAt
	rbdSnap.VolSize = vol.VolSize
	// found a snapshot already available, process and return its information
	rbdSnap.VolID, err = util.GenerateVolID(ctx, rbdSnap.Monitors, cr, snapData.ImagePoolID, rbdSnap.Pool,
		rbdSnap.ClusterID, snapUUID, volIDVersion)
	if err != nil {
		return false, err
	}

	// check snapshot exists if not create it
	err = vol.checkSnapExists(rbdSnap)
	if errors.Is(err, ErrSnapNotFound) {
		// create snapshot
		sErr := vol.createSnapshot(ctx, rbdSnap)
		if sErr != nil {
			log.ErrorLog(ctx, "failed to create snapshot %s: %v", rbdSnap, sErr)
			err = undoSnapshotCloning(ctx, parentVol, rbdSnap, vol, cr)

			return false, err
		}
	}
	if err != nil {
		return false, err
	}

	if vol.ImageID == "" {
		sErr := vol.getImageID()
		if sErr != nil {
			log.ErrorLog(ctx, "failed to get image id %s: %v", vol, sErr)
			err = undoSnapshotCloning(ctx, parentVol, rbdSnap, vol, cr)

			return false, err
		}
		sErr = j.StoreImageID(ctx, vol.JournalPool, vol.ReservedID, vol.ImageID)
		if sErr != nil {
			log.ErrorLog(ctx, "failed to store volume id %s: %v", vol, sErr)
			err = undoSnapshotCloning(ctx, parentVol, rbdSnap, vol, cr)

			return false, err
		}
	}

	log.DebugLog(ctx, "found existing image (%s) with name (%s) for request (%s)",
		rbdSnap.VolID, rbdSnap.RbdSnapName, rbdSnap.RequestName)

	return true, nil
}

/*
Check comment on checkSnapExists, to understand how this function behaves

**NOTE:** These functions manipulate the rados omaps that hold information
regarding volume names as requested by the CSI drivers. Hence, these need to be
invoked only when the respective CSI snapshot or volume name based locks are
held, as otherwise racy access to these omaps may end up leaving the omaps in
an inconsistent state.

parentVol is required to check the clone is created from the requested parent
image or not, if temporary snapshots and clones created for the volume when the
content source is volume we need to recover from the stale entries or complete
the pending operations.
*/
func (rv *rbdVolume) Exists(ctx context.Context, parentVol *rbdVolume) (bool, error) {
	err := validateRbdVol(rv)
	if err != nil {
		return false, err
	}

	kmsID, encryptionType := getEncryptionConfig(rv)

	j, err := volJournal.Connect(rv.Monitors, rv.RadosNamespace, rv.conn.Creds)
	if err != nil {
		return false, err
	}
	defer j.Destroy()

	imageData, err := j.CheckReservation(
		ctx, rv.JournalPool, rv.RequestName, rv.NamePrefix, "", kmsID, encryptionType)
	if err != nil {
		return false, err
	}
	if imageData == nil {
		return false, nil
	}

	rv.ReservedID = imageData.ImageUUID
	rv.RbdImageName = imageData.ImageAttributes.ImageName
	rv.ImageID = imageData.ImageAttributes.ImageID
	// check if topology constraints match what is found
	_, _, rv.Topology, err = util.MatchPoolAndTopology(rv.TopologyPools, rv.TopologyRequirement,
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
	requestSize := rv.VolSize //nolint:ifshort // FIXME: rename and split function into helpers
	// Fetch on-disk image attributes and compare against request
	err = rv.getImageInfo()
	if err != nil {
		if errors.Is(err, ErrImageNotFound) {
			// Need to check cloned info here not on createvolume,
			if parentVol != nil {
				found, cErr := rv.checkCloneImage(ctx, parentVol)
				switch {
				case found && cErr == nil:
					return true, nil
				case cErr != nil:
					return false, cErr
				}
			}
			err = j.UndoReservation(ctx, rv.JournalPool, rv.Pool,
				rv.RbdImageName, rv.RequestName)

			return false, err
		}

		return false, err
	}

	err = rv.repairImageID(ctx, j, false)
	if err != nil {
		return false, err
	}

	// size checks
	if rv.VolSize < requestSize {
		return false, fmt.Errorf("%w: image with the same name (%s) but with different size already exists",
			ErrVolNameConflict, rv.RbdImageName)
	}
	// TODO: We should also ensure image features and format is the same

	// found a volume already available, process and return it!
	rv.VolID, err = util.GenerateVolID(ctx, rv.Monitors, rv.conn.Creds, imageData.ImagePoolID, rv.Pool,
		rv.ClusterID, rv.ReservedID, volIDVersion)
	if err != nil {
		return false, err
	}

	if parentVol != nil {
		err = parentVol.copyEncryptionConfig(&rv.rbdImage, true)
		if err != nil {
			log.ErrorLog(ctx, err.Error())

			return false, err
		}
	}

	log.DebugLog(ctx, "found existing volume (%s) with image name (%s) for request (%s)",
		rv.VolID, rv.RbdImageName, rv.RequestName)

	return true, nil
}

// repairImageID checks if rv.ImageID is already available (if so, it was
// fetched from the journal), in case it is missing, the imageID is obtained
// and stored in the journal.
// if the force is set to true, the latest imageID will get added/updated in OMAP.
func (rv *rbdVolume) repairImageID(ctx context.Context, j *journal.Connection, force bool) error {
	if force {
		// reset the imageID so that we can fetch latest imageID from ceph cluster.
		rv.ImageID = ""
	}

	if rv.ImageID != "" {
		return nil
	}

	err := rv.getImageID()
	if err != nil {
		log.ErrorLog(ctx, "failed to get image id %s: %v", rv, err)

		return err
	}
	err = j.StoreImageID(ctx, rv.JournalPool, rv.ReservedID, rv.ImageID)
	if err != nil {
		log.ErrorLog(ctx, "failed to store volume id %s: %v", rv, err)

		return err
	}

	return nil
}

// reserveSnap is a helper routine to request a rbdSnapshot name reservation and generate the
// volume ID for the generated name.
func reserveSnap(ctx context.Context, rbdSnap *rbdSnapshot, rbdVol *rbdVolume, cr *util.Credentials) error {
	var err error

	journalPoolID, imagePoolID, err := util.GetPoolIDs(ctx, rbdSnap.Monitors, rbdSnap.JournalPool, rbdSnap.Pool, cr)
	if err != nil {
		return err
	}

	j, err := snapJournal.Connect(rbdSnap.Monitors, rbdSnap.RadosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	kmsID, encryptionType := getEncryptionConfig(rbdVol)

	rbdSnap.ReservedID, rbdSnap.RbdSnapName, err = j.ReserveName(
		ctx, rbdSnap.JournalPool, journalPoolID, rbdSnap.Pool, imagePoolID,
		rbdSnap.RequestName, rbdSnap.NamePrefix, rbdVol.RbdImageName, kmsID, rbdSnap.ReservedID, rbdVol.Owner,
		"", encryptionType)
	if err != nil {
		return err
	}

	rbdSnap.VolID, err = util.GenerateVolID(ctx, rbdSnap.Monitors, cr, imagePoolID, rbdSnap.Pool,
		rbdSnap.ClusterID, rbdSnap.ReservedID, volIDVersion)
	if err != nil {
		return err
	}

	log.DebugLog(ctx, "generated Volume ID (%s) and image name (%s) for request name (%s)",
		rbdSnap.VolID, rbdSnap.RbdSnapName, rbdSnap.RequestName)

	return nil
}

func updateTopologyConstraints(rbdVol *rbdVolume, rbdSnap *rbdSnapshot) error {
	var err error
	if rbdSnap != nil {
		// check if topology constraints matches snapshot pool
		var poolName string
		var dataPoolName string

		poolName, dataPoolName, rbdVol.Topology, err = util.MatchPoolAndTopology(rbdVol.TopologyPools,
			rbdVol.TopologyRequirement, rbdSnap.Pool)
		if err != nil {
			return err
		}

		// update Pool, if it was topology constrained
		if rbdVol.Topology != nil {
			rbdVol.Pool = poolName
			rbdVol.DataPool = dataPoolName
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
// volume ID for the generated name.
func reserveVol(ctx context.Context, rbdVol *rbdVolume, rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	var err error

	err = updateTopologyConstraints(rbdVol, rbdSnap)
	if err != nil {
		return err
	}

	journalPoolID, imagePoolID, err := util.GetPoolIDs(ctx, rbdVol.Monitors, rbdVol.JournalPool, rbdVol.Pool, cr)
	if err != nil {
		return err
	}

	kmsID, encryptionType := getEncryptionConfig(rbdVol)

	j, err := volJournal.Connect(rbdVol.Monitors, rbdVol.RadosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	rbdVol.ReservedID, rbdVol.RbdImageName, err = j.ReserveName(
		ctx, rbdVol.JournalPool, journalPoolID, rbdVol.Pool, imagePoolID,
		rbdVol.RequestName, rbdVol.NamePrefix, "", kmsID, rbdVol.ReservedID, rbdVol.Owner, "", encryptionType)
	if err != nil {
		return err
	}

	rbdVol.VolID, err = util.GenerateVolID(ctx, rbdVol.Monitors, cr, imagePoolID, rbdVol.Pool,
		rbdVol.ClusterID, rbdVol.ReservedID, volIDVersion)
	if err != nil {
		return err
	}

	log.DebugLog(ctx, "generated Volume ID (%s) and image name (%s) for request name (%s)",
		rbdVol.VolID, rbdVol.RbdImageName, rbdVol.RequestName)

	return nil
}

// undoSnapReservation is a helper routine to undo a name reservation for rbdSnapshot.
func undoSnapReservation(ctx context.Context, rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	j, err := snapJournal.Connect(rbdSnap.Monitors, rbdSnap.RadosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	err = j.UndoReservation(
		ctx, rbdSnap.JournalPool, rbdSnap.Pool, rbdSnap.RbdSnapName,
		rbdSnap.RequestName)

	return err
}

// undoVolReservation is a helper routine to undo a name reservation for rbdVolume.
func undoVolReservation(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) error {
	j, err := volJournal.Connect(rbdVol.Monitors, rbdVol.RadosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	err = j.UndoReservation(ctx, rbdVol.JournalPool, rbdVol.Pool,
		rbdVol.RbdImageName, rbdVol.RequestName)

	return err
}

// RegenerateJournal regenerates the omap data for the static volumes, the
// input parameters imageName, volumeID, pool, journalPool, requestName will be
// present in the PV.Spec.CSI object based on that we can regenerate the
// complete omap mapping between imageName and volumeID.

// RegenerateJournal performs below operations
// Extract clusterID, Mons after checking clusterID mapping
// Extract parameters journalPool, pool from volumeAttributes
// Extract optional parameters volumeNamePrefix, kmsID, owner from volumeAttributes
// Extract information from volumeID
// Get pool ID from pool name
// Extract uuid from volumeID
// Reserve omap data
// Generate new volume Handler
// The volume handler won't remain same as its contains poolID,clusterID etc
// which are not same across clusters.
//
//nolint:gocyclo,cyclop,nestif // TODO: reduce complexity
func RegenerateJournal(
	volumeAttributes map[string]string,
	claimName,
	volumeID,
	requestName,
	owner,
	clusterName string,
	setMetadata bool,
	cr *util.Credentials,
) (string, error) {
	ctx := context.Background()
	var (
		vi             util.CSIIdentifier
		rbdVol         *rbdVolume
		kmsID          string
		encryptionType util.EncryptionType
		err            error
		ok             bool
	)

	rbdVol = &rbdVolume{}
	rbdVol.VolID = volumeID
	rbdVol.ClusterName = clusterName
	rbdVol.EnableMetadata = setMetadata

	err = vi.DecomposeCSIID(rbdVol.VolID)
	if err != nil {
		return "", fmt.Errorf("%w: error decoding volume ID (%w) (%s)",
			ErrInvalidVolID, err, rbdVol.VolID)
	}

	rbdVol.Owner = owner

	kmsID, encryptionType, err = ParseEncryptionOpts(volumeAttributes, util.EncryptionTypeNone)
	if err != nil {
		return "", err
	}

	rbdVol.Monitors, rbdVol.ClusterID, err = util.FetchMappedClusterIDAndMons(ctx, vi.ClusterID)
	if err != nil {
		return "", err
	}

	if rbdVol.Pool, ok = volumeAttributes["pool"]; !ok {
		return "", errors.New("required 'pool' parameter missing in volume attributes")
	}
	err = rbdVol.Connect(cr)
	if err != nil {
		return "", err
	}
	rbdVol.JournalPool = volumeAttributes["journalPool"]
	if rbdVol.JournalPool == "" {
		rbdVol.JournalPool = rbdVol.Pool
	}
	volJournal = journal.NewCSIVolumeJournal(CSIInstanceID)
	j, err := volJournal.Connect(rbdVol.Monitors, rbdVol.RadosNamespace, cr)
	if err != nil {
		return "", err
	}
	defer j.Destroy()

	journalPoolID, imagePoolID, err := util.GetPoolIDs(ctx, rbdVol.Monitors, rbdVol.JournalPool, rbdVol.Pool, cr)
	if err != nil {
		return "", err
	}

	rbdVol.RequestName = requestName
	rbdVol.NamePrefix = volumeAttributes["volumeNamePrefix"]

	imageData, err := j.CheckReservation(
		ctx, rbdVol.JournalPool, rbdVol.RequestName, rbdVol.NamePrefix, "", kmsID, encryptionType)
	if err != nil {
		return "", err
	}

	if imageData != nil {
		rbdVol.ReservedID = imageData.ImageUUID
		rbdVol.ImageID = imageData.ImageAttributes.ImageID
		rbdVol.Owner = imageData.ImageAttributes.Owner
		rbdVol.RbdImageName = imageData.ImageAttributes.ImageName
		if rbdVol.ImageID == "" {
			err = rbdVol.storeImageID(ctx, j)
			if err != nil {
				return "", err
			}
		}
		if rbdVol.Owner != owner {
			err = j.ResetVolumeOwner(ctx, rbdVol.JournalPool, rbdVol.ReservedID, owner)
			if err != nil {
				return "", err
			}
		}
		// Update Metadata on reattach of the same old PV
		parameters := k8s.PrepareVolumeMetadata(claimName, owner, "")
		err = rbdVol.setAllMetadata(parameters)
		if err != nil {
			return "", fmt.Errorf("failed to set volume metadata: %w", err)
		}
		// As the omap already exists for this image ID return nil.
		rbdVol.VolID, err = util.GenerateVolID(ctx, rbdVol.Monitors, cr, imagePoolID, rbdVol.Pool,
			rbdVol.ClusterID, rbdVol.ReservedID, volIDVersion)
		if err != nil {
			return "", err
		}

		return rbdVol.VolID, nil
	}

	rbdVol.ReservedID, rbdVol.RbdImageName, err = j.ReserveName(
		ctx, rbdVol.JournalPool, journalPoolID, rbdVol.Pool, imagePoolID,
		rbdVol.RequestName, rbdVol.NamePrefix, "", kmsID, vi.ObjectUUID, rbdVol.Owner, "", encryptionType)
	if err != nil {
		return "", err
	}

	defer func() {
		if err != nil {
			undoErr := j.UndoReservation(ctx, rbdVol.JournalPool, rbdVol.Pool,
				rbdVol.RbdImageName, rbdVol.RequestName)
			if undoErr != nil {
				log.ErrorLog(ctx, "failed to undo reservation %s: %v", rbdVol, undoErr)
			}
		}
	}()
	rbdVol.VolID, err = util.GenerateVolID(ctx, rbdVol.Monitors, cr, imagePoolID, rbdVol.Pool,
		rbdVol.ClusterID, rbdVol.ReservedID, volIDVersion)
	if err != nil {
		return "", err
	}

	log.DebugLog(ctx, "re-generated Volume ID (%s) and image name (%s) for request name (%s)",
		rbdVol.VolID, rbdVol.RbdImageName, rbdVol.RequestName)
	if rbdVol.ImageID == "" {
		err = rbdVol.storeImageID(ctx, j)
		if err != nil {
			return "", err
		}
	}

	return rbdVol.VolID, nil
}

// storeImageID retrieves the image ID and stores it in OMAP.
func (rv *rbdVolume) storeImageID(ctx context.Context, j *journal.Connection) error {
	err := rv.getImageID()
	if err != nil {
		log.ErrorLog(ctx, "failed to get image id %s: %v", rv, err)

		return err
	}
	err = j.StoreImageID(ctx, rv.JournalPool, rv.ReservedID, rv.ImageID)
	if err != nil {
		log.ErrorLog(ctx, "failed to store volume id %s: %v", rv, err)

		return err
	}

	return nil
}
