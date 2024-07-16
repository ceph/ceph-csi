/*
Copyright 2024 The Ceph-CSI Authors.

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

package journal

import (
	"context"
	"errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/google/uuid"
)

const (
	defaultVolumeGroupNamingPrefix string = "csi-vol-group-"
)

type VolumeGroupJournal interface {
	Destroy()
	CheckReservation(
		ctx context.Context,
		journalPool,
		reqName,
		namePrefix string) (*VolumeGroupData, error)
	UndoReservation(
		ctx context.Context,
		csiJournalPool,
		groupName,
		reqName string) error
	// GetGroupAttributes fetches all keys and their values, from a UUID directory,
	// returning VolumeGroupAttributes structure.
	GetVolumeGroupAttributes(
		ctx context.Context,
		pool,
		objectUUID string) (*VolumeGroupAttributes, error)
	ReserveName(
		ctx context.Context,
		journalPool,
		reqName,
		namePrefix string) (string, string, error)
	// AddVolumesMapping adds a volumeMap map which contains volumeID's and its
	// corresponding values mapping which need to be added to the UUID
	// directory. value can be anything which needs mapping, in case of
	// volumegroupsnapshot its a snapshotID and its empty in case of
	// volumegroup.
	AddVolumesMapping(
		ctx context.Context,
		pool,
		reservedUUID string,
		volumeMap map[string]string) error
	// RemoveVolumesMapping removes volumeIDs mapping from the UUID directory.
	RemoveVolumesMapping(
		ctx context.Context,
		pool,
		reservedUUID string,
		volumeIDs []string) error
}

// VolumeGroupJournalConfig contains the configuration.
type VolumeGroupJournalConfig struct {
	Config
}

type volumeGroupJournalConnection struct {
	config     *VolumeGroupJournalConfig
	connection *Connection
}

// assert that volumeGroupJournalConnection implements the VolumeGroupJournal
// interface.
var _ VolumeGroupJournal = &volumeGroupJournalConnection{}

// NewCSIVolumeGroupJournal returns an instance of VolumeGroupJournal for groups.
func NewCSIVolumeGroupJournal(suffix string) VolumeGroupJournalConfig {
	return VolumeGroupJournalConfig{
		Config: Config{
			csiDirectory:            "csi.groups." + suffix,
			csiNameKeyPrefix:        "csi.volume.group.",
			cephUUIDDirectoryPrefix: "csi.volume.group.",
			csiImageKey:             "csi.groupname",
			csiNameKey:              "csi.volname",
			namespace:               "",
		},
	}
}

// SetNamespace sets the namespace for the journal.
func (vgc *VolumeGroupJournalConfig) SetNamespace(ns string) {
	vgc.Config.namespace = ns
}

// NewCSIVolumeGroupJournalWithNamespace returns an instance of VolumeGroupJournal for
// volume groups using a predetermined namespace value.
func NewCSIVolumeGroupJournalWithNamespace(suffix, ns string) VolumeGroupJournalConfig {
	j := NewCSIVolumeGroupJournal(suffix)
	j.SetNamespace(ns)

	return j
}

// Connect establishes a new connection to a ceph cluster for journal metadata.
func (vgc *VolumeGroupJournalConfig) Connect(
	monitors,
	namespace string,
	cr *util.Credentials,
) (VolumeGroupJournal, error) {
	vgjc := &volumeGroupJournalConnection{}
	vgjc.config = &VolumeGroupJournalConfig{
		Config: vgc.Config,
	}
	conn, err := vgc.Config.Connect(monitors, namespace, cr)
	if err != nil {
		return nil, err
	}
	vgjc.connection = conn

	return vgjc, nil
}

// Destroy frees any resources and invalidates the journal connection.
func (vgjc *volumeGroupJournalConnection) Destroy() {
	vgjc.connection.Destroy()
}

// VolumeGroupData contains the GroupUUID and VolumeGroupAttributes for a
// volume group.
type VolumeGroupData struct {
	GroupUUID             string
	GroupName             string
	VolumeGroupAttributes *VolumeGroupAttributes
}

func generateVolumeGroupName(namePrefix, groupUUID string) string {
	if namePrefix == "" {
		namePrefix = defaultVolumeGroupNamingPrefix
	}

	return namePrefix + groupUUID
}

/*
CheckReservation checks if given request name contains a valid reservation
  - If there is a valid reservation, then the corresponding VolumeGroupData for
    the snapshot group is returned
  - If there is a reservation that is stale (or not fully cleaned up), it is
    garbage collected using the UndoReservation call, as appropriate

NOTE: As the function manipulates omaps, it should be called with a lock
against the request name held, to prevent parallel operations from modifying
the state of the omaps for this request name.

Return values:
  - VolumeGroupData: which contains the GroupUUID and GroupSnapshotAttributes
    that were reserved for the passed in reqName, empty if there was no
    reservation found.
  - error: non-nil in case of any errors.
*/
func (vgjc *volumeGroupJournalConnection) CheckReservation(ctx context.Context,
	journalPool, reqName, namePrefix string,
) (*VolumeGroupData, error) {
	var (
		cj           = vgjc.config
		volGroupData = &VolumeGroupData{}
	)

	// check if request name is already part of the directory omap
	fetchKeys := []string{
		cj.csiNameKeyPrefix + reqName,
	}
	values, err := getOMapValues(
		ctx, vgjc.connection, journalPool, cj.namespace, cj.csiDirectory,
		cj.commonPrefix, fetchKeys)
	if err != nil {
		if errors.Is(err, util.ErrKeyNotFound) || errors.Is(err, util.ErrPoolNotFound) {
			// pool or omap (oid) was not present
			// stop processing but without an error for no reservation exists
			return nil, nil
		}

		return nil, err
	}

	objUUID, found := values[cj.csiNameKeyPrefix+reqName]
	if !found {
		// omap was read but was missing the desired key-value pair
		// stop processing but without an error for no reservation exists
		return nil, nil
	}
	volGroupData.GroupUUID = objUUID

	savedVolumeGroupAttributes, err := vgjc.GetVolumeGroupAttributes(ctx, journalPool,
		objUUID)
	if err != nil {
		// error should specifically be not found, for image to be absent, any other error
		// is not conclusive, and we should not proceed
		if errors.Is(err, util.ErrKeyNotFound) {
			err = vgjc.UndoReservation(ctx, journalPool,
				generateVolumeGroupName(namePrefix, objUUID), reqName)
		}

		return nil, err
	}

	// check if the request name in the omap matches the passed in request name
	if savedVolumeGroupAttributes.RequestName != reqName {
		// NOTE: This should never be possible, hence no cleanup, but log error
		// and return, as cleanup may need to occur manually!
		return nil, fmt.Errorf("internal state inconsistent, omap names mismatch,"+
			" request name (%s) volume group UUID (%s) volume group omap name (%s)",
			reqName, objUUID, savedVolumeGroupAttributes.RequestName)
	}
	volGroupData.GroupName = savedVolumeGroupAttributes.GroupName
	volGroupData.VolumeGroupAttributes = &VolumeGroupAttributes{}
	volGroupData.VolumeGroupAttributes.RequestName = savedVolumeGroupAttributes.RequestName
	volGroupData.VolumeGroupAttributes.VolumeMap = savedVolumeGroupAttributes.VolumeMap

	return volGroupData, nil
}

/*
UndoReservation undoes a reservation, in the reverse order of ReserveName
- The UUID directory is cleaned up before the GroupName key in the csiDirectory is cleaned up

NOTE: Ensure that the Ceph volume snapshots backing the reservation is cleaned up
prior to cleaning up the reservation

NOTE: As the function manipulates omaps, it should be called with a lock against the request name
held, to prevent parallel operations from modifying the state of the omaps for this request name.

Input arguments:
  - csiJournalPool: Pool name that holds the CSI request name based journal
  - groupID: ID of the volume group, generated from the UUID
  - reqName: Request name for the volume group
*/
func (vgjc *volumeGroupJournalConnection) UndoReservation(ctx context.Context,
	csiJournalPool, groupID, reqName string,
) error {
	// delete volume UUID omap (first, inverse of create order)
	cj := vgjc.config
	if groupID != "" {
		if len(groupID) < uuidEncodedLength {
			return fmt.Errorf("unable to parse UUID from %s, too short", groupID)
		}

		groupUUID := groupID[len(groupID)-36:]
		if _, err := uuid.Parse(groupUUID); err != nil {
			return fmt.Errorf("failed parsing UUID in %s: %w", groupUUID, err)
		}

		err := util.RemoveObject(
			ctx,
			vgjc.connection.monitors,
			vgjc.connection.cr,
			csiJournalPool,
			cj.namespace,
			cj.cephUUIDDirectoryPrefix+groupUUID)
		if err != nil {
			if !errors.Is(err, util.ErrObjectNotFound) {
				log.ErrorLog(ctx, "failed removing oMap %s (%s)", cj.cephUUIDDirectoryPrefix+groupUUID, err)

				return err
			}
		}
	}

	// delete the request name key (last, inverse of create order)
	err := removeMapKeys(ctx, vgjc.connection, csiJournalPool, cj.namespace, cj.csiDirectory,
		[]string{cj.csiNameKeyPrefix + reqName})
	if err != nil {
		log.ErrorLog(ctx, "failed removing oMap key %s (%s)", cj.csiNameKeyPrefix+reqName, err)
	}

	return err
}

/*
ReserveName adds respective entries to the csiDirectory omaps, post generating a target
UUIDDirectory for use. Further, these functions update the UUIDDirectory omaps, to store back
pointers to the CSI generated request names.

NOTE: As the function manipulates omaps, it should be called with a lock against the request name
held, to prevent parallel operations from modifying the state of the omaps for this request name.

Input arguments:
  - journalPool: Pool where the CSI journal is stored
  - reqName: Name of the volumeGroupSnapshot request received
  - namePrefix: Prefix to use when generating the volumeGroupName name (suffix is an auto-generated UUID)

Return values:
  - string: Contains the UUID that was reserved for the passed in reqName
  - string: Contains the VolumeGroup name that was reserved for the passed in reqName
  - error: non-nil in case of any errors
*/
func (vgjc *volumeGroupJournalConnection) ReserveName(ctx context.Context,
	journalPool, reqName, namePrefix string,
) (string, string, error) {
	cj := vgjc.config

	// Create the UUID based omap first, to reserve the same and avoid conflicts
	// NOTE: If any service loss occurs post creation of the UUID directory, and before
	// setting the request name key to point back to the UUID directory, the
	// UUID directory key will be leaked
	objUUID, err := reserveOMapName(
		ctx,
		vgjc.connection.monitors,
		vgjc.connection.cr,
		journalPool,
		cj.namespace,
		cj.cephUUIDDirectoryPrefix,
		"")
	if err != nil {
		return "", "", err
	}
	groupName := generateVolumeGroupName(namePrefix, objUUID)
	nameKeyVal := objUUID
	// After generating the UUID Directory omap, we populate the csiDirectory
	// omap with a key-value entry to map the request to the backend volume group:
	// `csiNameKeyPrefix + reqName: nameKeyVal`
	err = setOMapKeys(ctx, vgjc.connection, journalPool, cj.namespace, cj.csiDirectory,
		map[string]string{cj.csiNameKeyPrefix + reqName: nameKeyVal})
	if err != nil {
		return "", "", err
	}
	defer func() {
		if err != nil {
			log.WarningLog(ctx, "reservation failed for volume group: %s", reqName)
			errDefer := vgjc.UndoReservation(ctx, journalPool, groupName, reqName)
			if errDefer != nil {
				log.WarningLog(ctx, "failed undoing reservation of volume group: %s (%v)", reqName, errDefer)
			}
		}
	}()

	oid := cj.cephUUIDDirectoryPrefix + objUUID
	omapValues := map[string]string{}

	// Update UUID directory to store CSI request name
	omapValues[cj.csiNameKey] = reqName
	omapValues[cj.csiImageKey] = groupName

	err = setOMapKeys(ctx, vgjc.connection, journalPool, cj.namespace, oid, omapValues)
	if err != nil {
		return "", "", err
	}

	return objUUID, groupName, nil
}

// VolumeGroupAttributes contains the request name and the volumeID's and
// the corresponding snapshotID's.
type VolumeGroupAttributes struct {
	RequestName string            // Contains the request name for the passed in UUID
	GroupName   string            // Contains the group name
	VolumeMap   map[string]string // Contains the volumeID and the corresponding value mapping
}

func (vgjc *volumeGroupJournalConnection) GetVolumeGroupAttributes(
	ctx context.Context,
	pool, objectUUID string,
) (*VolumeGroupAttributes, error) {
	var (
		err             error
		groupAttributes = &VolumeGroupAttributes{}
		cj              = vgjc.config
	)

	values, err := listOMapValues(
		ctx, vgjc.connection, pool, cj.namespace, cj.cephUUIDDirectoryPrefix+objectUUID,
		cj.commonPrefix)
	if err != nil {
		if !errors.Is(err, util.ErrKeyNotFound) && !errors.Is(err, util.ErrPoolNotFound) {
			return nil, err
		}
		log.WarningLog(ctx, "unable to read omap values: pool missing: %v", err)
	}

	groupAttributes.RequestName = values[cj.csiNameKey]
	groupAttributes.GroupName = values[cj.csiImageKey]

	// Remove request name key and group name key from the omap, as we are
	// looking for volumeID/snapshotID mapping
	delete(values, cj.csiNameKey)
	delete(values, cj.csiImageKey)
	groupAttributes.VolumeMap = map[string]string{}
	for k, v := range values {
		groupAttributes.VolumeMap[k] = v
	}

	return groupAttributes, nil
}

func (vgjc *volumeGroupJournalConnection) AddVolumesMapping(
	ctx context.Context,
	pool,
	reservedUUID string,
	volumeMap map[string]string,
) error {
	err := setOMapKeys(ctx, vgjc.connection, pool, vgjc.config.namespace, vgjc.config.cephUUIDDirectoryPrefix+reservedUUID,
		volumeMap)
	if err != nil {
		log.ErrorLog(ctx, "failed to add volumeMap %v: %w ", volumeMap, err)

		return err
	}

	return nil
}

func (vgjc *volumeGroupJournalConnection) RemoveVolumesMapping(
	ctx context.Context,
	pool,
	reservedUUID string,
	volumeIDs []string,
) error {
	err := removeMapKeys(ctx, vgjc.connection, pool, vgjc.config.namespace,
		vgjc.config.cephUUIDDirectoryPrefix+reservedUUID,
		volumeIDs)
	if err != nil {
		log.ErrorLog(ctx, "failed removing volume mapping from group: key: %q %v", volumeIDs, err)

		return err
	}

	return nil
}
