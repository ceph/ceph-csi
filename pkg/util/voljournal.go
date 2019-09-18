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

package util

import (
	"context"
	"fmt"
	"strings"

	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

/*
RADOS omaps usage:

This note details how we preserve idempotent nature of create requests and retain the relationship
between orchestrator (CO) generated names and plugin generated names for volumes and snapshots.

NOTE: volume denotes an rbd image or a CephFS subvolume

The implementation uses Ceph RADOS omaps to preserve the relationship between request name and
generated volume (or snapshot) name. There are 4 types of omaps in use,
- A "csi.volumes.[csi-id]" (or "csi.volumes"+.+CSIInstanceID), (referred to using csiDirectory variable)
  - stores keys named using the CO generated names for volume requests (prefixed with csiNameKeyPrefix)
  - keys are named "csi.volume."+[CO generated VolName]
  - Key value contains the volume uuid that is created, for the CO provided name

- A "csi.snaps.[csi-id]" (or "csi.snaps"+.+CSIInstanceID), (referred to using csiDirectory variable)
  - stores keys named using the CO generated names for snapshot requests (prefixed with csiNameKeyPrefix)
  - keys are named "csi.snap."+[CO generated SnapName]
  - Key value contains the snapshot uuid that is created, for the CO provided name

- A per volume omap named "csi.volume."+[volume uuid], (referred to as CephUUIDDirectory)
  - stores a single key named "csi.volname", that has the value of the CO generated VolName that
  this volume refers to (referred to using csiNameKey value)

- A per snapshot omap named "rbd.csi.snap."+[RBD snapshot uuid], (referred to as CephUUIDDirectory)
  - stores a key named "csi.snapname", that has the value of the CO generated SnapName that this
  snapshot refers to (referred to using csiNameKey value)
  - also stores another key named "csi.source", that has the value of the volume name that is the
  source of the snapshot (referred to using cephSnapSourceKey value)

Creation of omaps:
When a volume create request is received (or a snapshot create, the snapshot is not detailed in this
	comment further as the process is similar),
- The csiDirectory is consulted to find if there is already a key with the CO VolName, and if present,
it is used to read its references to reach the UUID that backs this VolName, to check if the
UUID based volume can satisfy the requirements for the request
  - If during the process of checking the same, it is found that some linking information is stale
  or missing, the corresponding keys upto the key in the csiDirectory is cleaned up, to start afresh

- If the key with the CO VolName is not found, or was cleaned up, the request is treated as a
new create request, and an CephUUIDDirectory is created first with a generated uuid, this ensures
that we do not use a uuid that is already in use

- Next, a key with the VolName is created in the csiDirectory, and its value is updated to store the
generated uuid

- This is followed by updating the CephUUIDDirectory with the VolName in the csiNameKey

- Finally, the volume is created (or promoted from a snapshot, if content source was provided),
using the uuid and a corresponding name prefix (namingPrefix) as the volume name

The entire operation is locked based on VolName hash, to ensure there is only ever a single entity
modifying the related omaps for a given VolName.

This ensures idempotent nature of creates, as the same CO generated VolName would attempt to use
the same volume uuid to serve the request, as the relations are saved in the respective omaps.

Deletion of omaps:
Delete requests would not contain the VolName, hence deletion uses the volume ID, which is encoded
with the volume uuid in it, to find the volume and the CephUUIDDirectory. The CephUUIDDirectory is
read to get the VolName that this image points to. This VolName can be further used to read and
delete the key from the csiDirectory.

As we trace back and find the VolName, we also take a hash based lock on the VolName before
proceeding with deleting the volume and the related omap entries, to ensure there is only ever a
single entity modifying the related omaps for a given VolName.
*/

type CSIJournal struct {
	// csiDirectory is the name of the CSI volumes object map that contains CSI volume-name (or
	// snapshot name) based keys
	csiDirectory string

	// CSI volume-name keyname prefix, for key in csiDirectory, suffix is the CSI passed volume name
	csiNameKeyPrefix string

	// Per Ceph volume (RBD/FS-subvolume) object map name prefix, suffix is the generated volume uuid
	cephUUIDDirectoryPrefix string

	// CSI volume-name key in per Ceph volume object map, containing CSI volume-name for which the
	// Ceph volume was created
	csiNameKey string

	// source volume name key in per Ceph snapshot object map, containing Ceph source volume uuid
	// for which the snapshot was created
	cephSnapSourceKey string

	// volume name prefix for naming on Ceph rbd or FS, suffix is a uuid generated per volume
	namingPrefix string

	// namespace in which the RADOS objects are stored, default is no namespace
	namespace string
}

// CSIVolumeJournal returns an instance of volume keys
func NewCSIVolumeJournal() *CSIJournal {
	return &CSIJournal{
		csiDirectory:            "csi.volumes",
		csiNameKeyPrefix:        "csi.volume.",
		cephUUIDDirectoryPrefix: "csi.volume.",
		csiNameKey:              "csi.volname",
		namingPrefix:            "csi-vol-",
		cephSnapSourceKey:       "",
		namespace:               "",
	}
}

// CSISnapshotSnapshot returns an instance of snapshot keys
func NewCSISnapshotJournal() *CSIJournal {
	return &CSIJournal{
		csiDirectory:            "csi.snaps",
		csiNameKeyPrefix:        "csi.snap.",
		cephUUIDDirectoryPrefix: "csi.snap.",
		csiNameKey:              "csi.snapname",
		namingPrefix:            "csi-snap-",
		cephSnapSourceKey:       "csi.source",
		namespace:               "",
	}
}

// NamingPrefix returns the value of naming prefix from the journal keys
func (cj *CSIJournal) NamingPrefix() string {
	return cj.namingPrefix
}

// SetCSIDirectorySuffix sets the given suffix for the csiDirectory omap
func (cj *CSIJournal) SetCSIDirectorySuffix(suffix string) {
	cj.csiDirectory = cj.csiDirectory + "." + suffix
}

// SetNamespace sets the namespace in which all RADOS objects would be created
func (cj *CSIJournal) SetNamespace(ns string) {
	cj.namespace = ns
}

/*
CheckReservation checks if given request name contains a valid reservation
- If there is a valid reservation, then the corresponding UUID for the volume/snapshot is returned
- If there is a reservation that is stale (or not fully cleaned up), it is garbage collected using
the UndoReservation call, as appropriate
- If a snapshot is being checked, then its source is matched to the parentName that is provided

NOTE: As the function manipulates omaps, it should be called with a lock against the request name
held, to prevent parallel operations from modifying the state of the omaps for this request name.

Return values:
	- string: Contains the UUID that was reserved for the passed in reqName, empty if
	there was no reservation found
	- error: non-nil in case of any errors
*/
func (cj *CSIJournal) CheckReservation(ctx context.Context, monitors string, cr *Credentials, pool, reqName, parentName string) (string, error) {
	var snapSource bool

	if parentName != "" {
		if cj.cephSnapSourceKey == "" {
			err := errors.New("invalid request, cephSnapSourceKey is nil")
			return "", err
		}
		snapSource = true
	}

	// check if request name is already part of the directory omap
	objUUID, err := GetOMapValue(ctx, monitors, cr, pool, cj.namespace, cj.csiDirectory,
		cj.csiNameKeyPrefix+reqName)
	if err != nil {
		// error should specifically be not found, for volume to be absent, any other error
		// is not conclusive, and we should not proceed
		if _, ok := err.(ErrKeyNotFound); ok {
			return "", nil
		}
		return "", err
	}

	savedReqName, savedReqParentName, err := cj.GetObjectUUIDData(ctx, monitors, cr, pool,
		objUUID, snapSource)
	if err != nil {
		// error should specifically be not found, for image to be absent, any other error
		// is not conclusive, and we should not proceed
		if _, ok := err.(ErrKeyNotFound); ok {
			err = cj.UndoReservation(ctx, monitors, cr, pool, cj.namingPrefix+objUUID, reqName)
		}
		return "", err
	}

	// check if UUID key points back to the request name
	if savedReqName != reqName {
		// NOTE: This should never be possible, hence no cleanup, but log error
		// and return, as cleanup may need to occur manually!
		return "", fmt.Errorf("internal state inconsistent, omap names mismatch,"+
			" request name (%s) volume UUID (%s) volume omap name (%s)",
			reqName, objUUID, savedReqName)
	}

	if snapSource {
		// check if source UUID key points back to the parent volume passed in
		if savedReqParentName != parentName {
			// NOTE: This can happen if there is a snapname conflict, and we already have a snapshot
			// with the same name pointing to a different UUID as the source
			err = fmt.Errorf("snapname points to different volume, request name (%s)"+
				" source name (%s) saved source name (%s)",
				reqName, parentName, savedReqParentName)
			return "", ErrSnapNameConflict{reqName, err}
		}
	}

	return objUUID, nil
}

/*
UndoReservation undoes a reservation, in the reverse order of ReserveName
- The UUID directory is cleaned up before the VolName key in the csiDirectory is cleaned up

NOTE: Ensure that the Ceph volume (image or FS subvolume) backing the reservation is cleaned up
prior to cleaning up the reservation

NOTE: As the function manipulates omaps, it should be called with a lock against the request name
held, to prevent parallel operations from modifying the state of the omaps for this request name.
*/
func (cj *CSIJournal) UndoReservation(ctx context.Context, monitors string, cr *Credentials, pool, volName, reqName string) error {
	// delete volume UUID omap (first, inverse of create order)
	// TODO: Check cases where volName can be empty, and we need to just cleanup the reqName
	imageUUID := strings.TrimPrefix(volName, cj.namingPrefix)
	err := RemoveObject(ctx, monitors, cr, pool, cj.namespace, cj.cephUUIDDirectoryPrefix+imageUUID)
	if err != nil {
		if _, ok := err.(ErrObjectNotFound); !ok {
			klog.Errorf(Log(ctx, "failed removing oMap %s (%s)"), cj.cephUUIDDirectoryPrefix+imageUUID, err)
			return err
		}
	}

	// delete the request name key (last, inverse of create order)
	err = RemoveOMapKey(ctx, monitors, cr, pool, cj.namespace, cj.csiDirectory,
		cj.csiNameKeyPrefix+reqName)
	if err != nil {
		klog.Errorf(Log(ctx, "failed removing oMap key %s (%s)"), cj.csiNameKeyPrefix+reqName, err)
		return err
	}

	return err
}

// reserveOMapName creates an omap with passed in oMapNamePrefix and a generated <uuid>.
// It ensures generated omap name does not already exist and if conflicts are detected, a set
// number of retires with newer uuids are attempted before returning an error
func reserveOMapName(ctx context.Context, monitors string, cr *Credentials, pool, namespace, oMapNamePrefix string) (string, error) {
	var iterUUID string

	maxAttempts := 5
	attempt := 1
	for attempt <= maxAttempts {
		// generate a uuid for the image name
		iterUUID = uuid.NewUUID().String()

		err := CreateObject(ctx, monitors, cr, pool, namespace, oMapNamePrefix+iterUUID)
		if err != nil {
			if _, ok := err.(ErrObjectExists); ok {
				attempt++
				// try again with a different uuid, for maxAttempts tries
				klog.V(4).Infof(Log(ctx, "uuid (%s) conflict detected, retrying (attempt %d of %d)"),
					iterUUID, attempt, maxAttempts)
				continue
			}

			return "", err
		}

		return iterUUID, nil
	}

	return "", errors.New("uuid conflicts exceeds retry threshold")

}

/*
ReserveName adds respective entries to the csiDirectory omaps, post generating a target
UUIDDirectory for use. Further, these functions update the UUIDDirectory omaps, to store back
pointers to the CSI generated request names.

NOTE: As the function manipulates omaps, it should be called with a lock against the request name
held, to prevent parallel operations from modifying the state of the omaps for this request name.

Return values:
	- string: Contains the UUID that was reserved for the passed in reqName
	- error: non-nil in case of any errors
*/
func (cj *CSIJournal) ReserveName(ctx context.Context, monitors string, cr *Credentials, pool, reqName, parentName string) (string, error) {
	var snapSource bool

	if parentName != "" {
		if cj.cephSnapSourceKey == "" {
			err := errors.New("invalid request, cephSnapSourceKey is nil")
			return "", err
		}
		snapSource = true
	}

	// Create the UUID based omap first, to reserve the same and avoid conflicts
	// NOTE: If any service loss occurs post creation of the UUID directory, and before
	// setting the request name key (csiNameKey) to point back to the UUID directory, the
	// UUID directory key will be leaked
	volUUID, err := reserveOMapName(ctx, monitors, cr, pool, cj.namespace, cj.cephUUIDDirectoryPrefix)
	if err != nil {
		return "", err
	}

	// Create request name (csiNameKey) key in csiDirectory and store the UUId based
	// volume name into it
	err = SetOMapKeyValue(ctx, monitors, cr, pool, cj.namespace, cj.csiDirectory,
		cj.csiNameKeyPrefix+reqName, volUUID)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			klog.Warningf(Log(ctx, "reservation failed for volume: %s"), reqName)
			errDefer := cj.UndoReservation(ctx, monitors, cr, pool, cj.namingPrefix+volUUID,
				reqName)
			if errDefer != nil {
				klog.Warningf(Log(ctx, "failed undoing reservation of volume: %s (%v)"), reqName, errDefer)
			}
		}
	}()

	// Update UUID directory to store CSI request name
	err = SetOMapKeyValue(ctx, monitors, cr, pool, cj.namespace, cj.cephUUIDDirectoryPrefix+volUUID,
		cj.csiNameKey, reqName)
	if err != nil {
		return "", err
	}

	if snapSource {
		// Update UUID directory to store source volume UUID in case of snapshots
		err = SetOMapKeyValue(ctx, monitors, cr, pool, cj.namespace, cj.cephUUIDDirectoryPrefix+volUUID,
			cj.cephSnapSourceKey, parentName)
		if err != nil {
			return "", err
		}
	}

	return volUUID, nil
}

/*
GetObjectUUIDData fetches all keys from a UUID directory
Return values:
	- string: Contains the request name for the passed in UUID
	- string: Contains the parent image name for the passed in UUID, if it is a snapshot
	- error: non-nil in case of any errors
*/
func (cj *CSIJournal) GetObjectUUIDData(ctx context.Context, monitors string, cr *Credentials, pool, objectUUID string, snapSource bool) (string, string, error) {
	var sourceName string

	if snapSource && cj.cephSnapSourceKey == "" {
		err := errors.New("invalid request, cephSnapSourceKey is nil")
		return "", "", err
	}

	// TODO: fetch all omap vals in one call, than make multiple listomapvals
	requestName, err := GetOMapValue(ctx, monitors, cr, pool, cj.namespace,
		cj.cephUUIDDirectoryPrefix+objectUUID, cj.csiNameKey)
	if err != nil {
		return "", "", err
	}

	if snapSource {
		sourceName, err = GetOMapValue(ctx, monitors, cr, pool, cj.namespace,
			cj.cephUUIDDirectoryPrefix+objectUUID, cj.cephSnapSourceKey)
		if err != nil {
			return "", "", err
		}
	}

	return requestName, sourceName, nil
}
