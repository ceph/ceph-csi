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

package journal

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/google/uuid"
)

// Length of string representation of uuid, xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx is 36 bytes.
const uuidEncodedLength = 36

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
  - stores the key named "csi.volname", that has the value of the CO generated VolName that
  this volume refers to (referred to using csiNameKey value)
	- stores the key named "csi.imagename", that has the value of the Ceph RBD image name
  this volume refers to (referred to using csiImageKey value)

- A per snapshot omap named "rbd.csi.snap."+[RBD snapshot uuid], (referred to as CephUUIDDirectory)
  - stores a key named "csi.snapname", that has the value of the CO generated SnapName that this
  snapshot refers to (referred to using csiNameKey value)
	- stores the key named "csi.imagename", that has the value of the Ceph RBD image name
  this snapshot refers to (referred to using csiImageKey value)
  - stores a key named "csi.source", that has the value of the volume name that is the
  source of the snapshot (referred to using cephSnapSourceKey value)

Creation of omaps:
When a volume create request is received (or a snapshot create, the snapshot is not detailed in this
	comment further as the process is similar),
- The csiDirectory is consulted to find if there is already a key with the CO VolName, and if present,
it is used to read its references to reach the UUID that backs this VolName, to check if the
UUID based volume can satisfy the requirements for the request
  - If during the process of checking the same, it is found that some linking information is stale
  or missing, the corresponding keys up to the key in the csiDirectory is cleaned up, to start afresh

- If the key with the CO VolName is not found, or was cleaned up, the request is treated as a
new create request, and an CephUUIDDirectory is created first with a generated uuid, this ensures
that we do not use a uuid that is already in use

- Next, a key with the VolName is created in the csiDirectory, and its value is updated to store the
generated uuid

- This is followed by updating the CephUUIDDirectory with the VolName in the csiNameKey and the RBD image
name in the csiImageKey

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

const (
	defaultVolumeNamingPrefix   string = "csi-vol-"
	defaultSnapshotNamingPrefix string = "csi-snap-"
)

// CSIJournal defines the interface and the required key names for the above RADOS based OMaps.
type Config struct {
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

	// CSI ImageID in per Ceph volume object map, containing image ID
	// of this Ceph volume
	csiImageIDKey string

	// CSI image-name key in per Ceph volume object map, containing RBD image-name
	// of this Ceph volume
	csiImageKey string

	// pool ID where csiDirectory is maintained, as it can be different from where the ceph volume
	// object map is maintained, during topology based provisioning
	csiJournalPool string

	// source volume name key in per Ceph snapshot object map, containing Ceph source volume uuid
	// for which the snapshot was created
	cephSnapSourceKey string

	// namespace in which the RADOS objects are stored, default is no namespace
	namespace string

	// encryptKMS in which encryption passphrase was saved, default is no encryption
	encryptKMSKey string

	// encryptKMS in which encryption passphrase was saved, default is no encryption
	encryptionType string

	// ownerKey is used to identify the owner of the volume, can be used with some KMS configurations
	ownerKey string

	// backingSnapshotIDKey ID of the snapshot on which the CephFS snapshot-backed volume is based
	backingSnapshotIDKey string

	// commonPrefix is the prefix common to all omap keys for this Config
	commonPrefix string
}

// NewCSIVolumeJournal returns an instance of CSIJournal for volumes.
func NewCSIVolumeJournal(suffix string) *Config {
	return &Config{
		csiDirectory:            "csi.volumes." + suffix,
		csiNameKeyPrefix:        "csi.volume.",
		cephUUIDDirectoryPrefix: "csi.volume.",
		csiNameKey:              "csi.volname",
		csiImageKey:             "csi.imagename",
		csiJournalPool:          "csi.journalpool",
		cephSnapSourceKey:       "",
		namespace:               "",
		csiImageIDKey:           "csi.imageid",
		encryptKMSKey:           "csi.volume.encryptKMS",
		encryptionType:          "csi.volume.encryptionType",
		ownerKey:                "csi.volume.owner",
		backingSnapshotIDKey:    "csi.volume.backingsnapshotid",
		commonPrefix:            "csi.",
	}
}

// NewCSISnapshotJournal returns an instance of CSIJournal for snapshots.
func NewCSISnapshotJournal(suffix string) *Config {
	return &Config{
		csiDirectory:            "csi.snaps." + suffix,
		csiNameKeyPrefix:        "csi.snap.",
		cephUUIDDirectoryPrefix: "csi.snap.",
		csiNameKey:              "csi.snapname",
		csiImageKey:             "csi.imagename",
		csiJournalPool:          "csi.journalpool",
		cephSnapSourceKey:       "csi.source",
		namespace:               "",
		csiImageIDKey:           "csi.imageid",
		encryptKMSKey:           "csi.volume.encryptKMS",
		encryptionType:          "csi.volume.encryptionType",
		ownerKey:                "csi.volume.owner",
		commonPrefix:            "csi.",
	}
}

// NewCSIVolumeJournalWithNamespace returns an instance of CSIJournal for
// volumes using a predetermined namespace value.
func NewCSIVolumeJournalWithNamespace(suffix, ns string) *Config {
	j := NewCSIVolumeJournal(suffix)
	j.namespace = ns

	return j
}

// NewCSISnapshotJournalWithNamespace returns an instance of CSIJournal for
// snapshots using a predetermined namespace value.
func NewCSISnapshotJournalWithNamespace(suffix, ns string) *Config {
	j := NewCSISnapshotJournal(suffix)
	j.namespace = ns

	return j
}

// GetNameForUUID returns volume name.
func (cj *Config) GetNameForUUID(prefix, uid string, isSnapshot bool) string {
	if prefix == "" {
		if isSnapshot {
			prefix = defaultSnapshotNamingPrefix
		} else {
			prefix = defaultVolumeNamingPrefix
		}
	}

	return prefix + uid
}

// ImageData contains image name and stored CSI properties.
type ImageData struct {
	ImageUUID       string
	ImagePool       string
	ImagePoolID     int64
	ImageAttributes *ImageAttributes
}

// Connection represents a real or virtual connection to a ceph cluster
// that can make changes to the journal(s).
type Connection struct {
	config *Config
	// connection metadata
	monitors string
	cr       *util.Credentials
	// cached cluster connection (required by go-ceph)
	conn *util.ClusterConnection
}

// Connect establishes a new connection to a ceph cluster for journal metadata.
func (cj *Config) Connect(monitors, namespace string, cr *util.Credentials) (*Connection, error) {
	cj.namespace = namespace
	cc := &util.ClusterConnection{}
	if err := cc.Connect(monitors, cr); err != nil {
		return nil, fmt.Errorf("failed to establish the connection: %w", err)
	}
	conn := &Connection{
		config:   cj,
		monitors: monitors,
		cr:       cr,
		conn:     cc,
	}

	return conn, nil
}

/*
CheckReservation checks if given request name contains a valid reservation
- If there is a valid reservation, then the corresponding ImageData for the volume/snapshot is returned
- If there is a reservation that is stale (or not fully cleaned up), it is garbage collected using
the UndoReservation call, as appropriate
- If a snapshot is being checked, then its source is matched to the snapParentName that is provided

NOTE: As the function manipulates omaps, it should be called with a lock against the request name
held, to prevent parallel operations from modifying the state of the omaps for this request name.

Return values:
  - ImageData: which contains the UUID,Pool,PoolID and ImageAttributes that were reserved for the
    passed in reqName, empty if there was no reservation found
  - error: non-nil in case of any errors
*/
func (conn *Connection) CheckReservation(ctx context.Context,
	journalPool, reqName, namePrefix, snapParentName, kmsConfig string,
	encryptionType util.EncryptionType,
) (*ImageData, error) {
	var (
		snapSource       bool
		objUUID          string
		savedImagePool   string
		savedImagePoolID = util.InvalidPoolID
		cj               = conn.config
	)

	if snapParentName != "" {
		if cj.cephSnapSourceKey == "" {
			err := errors.New("invalid request, cephSnapSourceKey is nil")

			return nil, err
		}
		snapSource = true
	}

	// check if request name is already part of the directory omap
	fetchKeys := []string{
		cj.csiNameKeyPrefix + reqName,
	}
	values, err := getOMapValues(
		ctx, conn, journalPool, cj.namespace, cj.csiDirectory,
		cj.commonPrefix, fetchKeys)
	if err != nil {
		if errors.Is(err, util.ErrKeyNotFound) || errors.Is(err, util.ErrPoolNotFound) {
			// pool or omap (oid) was not present
			// stop processing but without an error for no reservation exists
			return nil, nil
		}

		return nil, err
	}
	objUUIDAndPool, found := values[cj.csiNameKeyPrefix+reqName]
	if !found {
		// omap was read but was missing the desired key-value pair
		// stop processing but without an error for no reservation exists
		return nil, nil
	}

	// check UUID only encoded value
	if len(objUUIDAndPool) == uuidEncodedLength {
		objUUID = objUUIDAndPool
		savedImagePool = journalPool
	} else { // check poolID/UUID encoding; extract the vol UUID and pool name
		var buf64 []byte
		components := strings.Split(objUUIDAndPool, "/")
		objUUID = components[1]
		savedImagePoolIDStr := components[0]

		buf64, err = hex.DecodeString(savedImagePoolIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode string: %w", err)
		}
		savedImagePoolID = int64(binary.BigEndian.Uint64(buf64))

		savedImagePool, err = util.GetPoolName(conn.monitors, conn.cr, savedImagePoolID)
		if err != nil {
			if errors.Is(err, util.ErrPoolNotFound) {
				err = conn.UndoReservation(ctx, journalPool, "", "", reqName)
			}

			return nil, err
		}
	}

	savedImageAttributes, err := conn.GetImageAttributes(ctx, savedImagePool,
		objUUID, snapSource)
	if err != nil {
		// error should specifically be not found, for image to be absent, any other error
		// is not conclusive, and we should not proceed
		if errors.Is(err, util.ErrKeyNotFound) {
			err = conn.UndoReservation(ctx, journalPool, savedImagePool,
				cj.GetNameForUUID(namePrefix, objUUID, snapSource), reqName)
		}

		return nil, err
	}

	// check if UUID key points back to the request name
	if savedImageAttributes.RequestName != reqName {
		// NOTE: This should never be possible, hence no cleanup, but log error
		// and return, as cleanup may need to occur manually!
		return nil, fmt.Errorf("internal state inconsistent, omap names mismatch,"+
			" request name (%s) volume UUID (%s) volume omap name (%s)",
			reqName, objUUID, savedImageAttributes.RequestName)
	}

	if kmsConfig != "" {
		if savedImageAttributes.KmsID != kmsConfig {
			return nil, fmt.Errorf("internal state inconsistent, omap encryption KMS"+
				" mismatch, request KMS (%s) volume UUID (%s) volume omap KMS (%s)",
				kmsConfig, objUUID, savedImageAttributes.KmsID)
		}
	}

	if encryptionType != util.EncryptionTypeNone {
		if savedImageAttributes.EncryptionType != encryptionType {
			return nil, fmt.Errorf("internal state inconsistent, omap encryption type"+
				" mismatch, request type %q(%d) volume UUID (%s) volume omap encryption type %q (%d)",
				util.EncryptionTypeString(encryptionType), encryptionType,
				objUUID, util.EncryptionTypeString(savedImageAttributes.EncryptionType),
				savedImageAttributes.EncryptionType)
		}
	}

	// TODO: skipping due to excessive poolID to poolname call, also this should never happen!
	// check if journal pool points back to the passed in journal pool
	// if savedJournalPoolID != journalPoolID {

	if snapSource {
		// check if source UUID key points back to the parent volume passed in
		if savedImageAttributes.SourceName != snapParentName {
			// NOTE: This can happen if there is a snapname conflict, and we already have a snapshot
			// with the same name pointing to a different UUID as the source
			err = fmt.Errorf("%w: snapname points to different volume, request name (%s)"+
				" source name (%s) : saved source name (%s)", util.ErrSnapNameConflict,
				reqName, snapParentName, savedImageAttributes.SourceName)

			return nil, err
		}
	}

	imageData := &ImageData{
		ImageUUID:       objUUID,
		ImagePool:       savedImagePool,
		ImagePoolID:     savedImagePoolID,
		ImageAttributes: savedImageAttributes,
	}

	return imageData, nil
}

/*
UndoReservation undoes a reservation, in the reverse order of ReserveName
- The UUID directory is cleaned up before the VolName key in the csiDirectory is cleaned up

NOTE: Ensure that the Ceph volume (image or FS subvolume) backing the reservation is cleaned up
prior to cleaning up the reservation

NOTE: As the function manipulates omaps, it should be called with a lock against the request name
held, to prevent parallel operations from modifying the state of the omaps for this request name.

Input arguments:
  - csiJournalPool: Pool name that holds the CSI request name based journal
  - volJournalPool: Pool name that holds the image/subvolume and the per-image journal (may be
    different if image is created in a topology constrained pool)
*/
func (conn *Connection) UndoReservation(ctx context.Context,
	csiJournalPool, volJournalPool, volName, reqName string,
) error {
	// delete volume UUID omap (first, inverse of create order)

	cj := conn.config
	if volName != "" {
		if len(volName) < uuidEncodedLength {
			return fmt.Errorf("unable to parse UUID from %s, too short", volName)
		}

		imageUUID := volName[len(volName)-36:]
		if _, err := uuid.Parse(imageUUID); err != nil {
			return fmt.Errorf("failed parsing UUID in %s: %w", volName, err)
		}

		err := util.RemoveObject(
			ctx,
			conn.monitors,
			conn.cr,
			volJournalPool,
			cj.namespace,
			cj.cephUUIDDirectoryPrefix+imageUUID)
		if err != nil {
			if !errors.Is(err, util.ErrObjectNotFound) {
				log.ErrorLog(ctx, "failed removing oMap %s (%s)", cj.cephUUIDDirectoryPrefix+imageUUID, err)

				return err
			}
		}
	}

	// delete the request name key (last, inverse of create order)
	err := removeMapKeys(ctx, conn, csiJournalPool, cj.namespace, cj.csiDirectory,
		[]string{cj.csiNameKeyPrefix + reqName})
	if err != nil {
		log.ErrorLog(ctx, "failed removing oMap key %s (%s)", cj.csiNameKeyPrefix+reqName, err)

		return err
	}

	return err
}

// reserveOMapName creates an omap with passed in oMapNamePrefix and a
// generated <uuid>. If the passed volUUID is not empty it will use it instead
// of generating its own UUID and it will return an error immediately if omap
// already exists. If the passed volUUID is empty, it ensures generated omap name
// does not already exist and if conflicts are detected, a set number of
// retries with newer uuids are attempted before returning an error.
func reserveOMapName(
	ctx context.Context,
	monitors string,
	cr *util.Credentials,
	pool, namespace, oMapNamePrefix, volUUID string,
) (string, error) {
	var iterUUID string

	maxAttempts := 5
	attempt := 1
	for attempt <= maxAttempts {
		if volUUID != "" {
			iterUUID = volUUID
		} else {
			// generate a uuid for the image name
			iterUUID = uuid.New().String()
		}

		err := util.CreateObject(ctx, monitors, cr, pool, namespace, oMapNamePrefix+iterUUID)
		if err != nil {
			// if the volUUID is empty continue with retry as consumer of this
			// function didn't request to create object with specific value.
			if volUUID == "" && errors.Is(err, util.ErrObjectExists) {
				attempt++
				// try again with a different uuid, for maxAttempts tries
				log.DebugLog(ctx, "uuid (%s) conflict detected, retrying (attempt %d of %d)",
					iterUUID, attempt, maxAttempts)

				continue
			}

			return "", fmt.Errorf(
				"failed to create omap object for oMapNamePrefix+iterUUID=%s: %w",
				oMapNamePrefix+iterUUID,
				err)
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

Input arguments:
  - journalPool: Pool where the CSI journal is stored (maybe different than the pool where the
    image/subvolume is created due to topology constraints)
  - journalPoolID: pool ID of the journalPool
  - imagePool: Pool where the image/subvolume is created
  - imagePoolID: pool ID of the imagePool
  - reqName: Name of the volume request received
  - namePrefix: Prefix to use when generating the image/subvolume name (suffix is an auto-generated UUID)
  - parentName: Name of the parent image/subvolume if reservation is for a snapshot (optional)
  - kmsConf: Name of the key management service used to encrypt the image (optional)
  - encryptionType: Type of encryption used when kmsConf is set (optional)
  - volUUID: UUID need to be reserved instead of auto-generating one (this is useful for mirroring and metro-DR)
  - owner: the owner of the volume (optional)
  - backingSnapshotID: ID of the snapshot on which the CephFS snapshot-backed volume is based (optional)

Return values:
  - string: Contains the UUID that was reserved for the passed in reqName
  - string: Contains the image name that was reserved for the passed in reqName
  - error: non-nil in case of any errors
*/
func (conn *Connection) ReserveName(ctx context.Context,
	journalPool string, journalPoolID int64,
	imagePool string, imagePoolID int64,
	reqName, namePrefix, parentName, kmsConf, volUUID, owner,
	backingSnapshotID string,
	encryptionType util.EncryptionType,
) (string, string, error) {
	// TODO: Take in-arg as ImageAttributes?
	var (
		snapSource bool
		nameKeyVal string
		cj         = conn.config
		err        error
	)

	if parentName != "" {
		if cj.cephSnapSourceKey == "" {
			err = errors.New("invalid request, cephSnapSourceKey is nil")

			return "", "", err
		}
		snapSource = true
	}

	// Create the UUID based omap first, to reserve the same and avoid conflicts
	// NOTE: If any service loss occurs post creation of the UUID directory, and before
	// setting the request name key (csiNameKey) to point back to the UUID directory, the
	// UUID directory key will be leaked
	volUUID, err = reserveOMapName(
		ctx,
		conn.monitors,
		conn.cr,
		imagePool,
		cj.namespace,
		cj.cephUUIDDirectoryPrefix,
		volUUID)
	if err != nil {
		return "", "", err
	}

	imageName := cj.GetNameForUUID(namePrefix, volUUID, snapSource)

	// Create request name (csiNameKey) key in csiDirectory and store the UUID based
	// volume name and optionally the image pool location into it
	if journalPool != imagePool && imagePoolID != util.InvalidPoolID {
		buf64 := make([]byte, 8)
		binary.BigEndian.PutUint64(buf64, uint64(imagePoolID))
		poolIDEncodedHex := hex.EncodeToString(buf64)
		nameKeyVal = poolIDEncodedHex + "/" + volUUID
	} else {
		nameKeyVal = volUUID
	}

	// After generating the UUID Directory omap, we populate the csiDirectory
	// omap with a key-value entry to map the request to the backend volume:
	// `csiNameKeyPrefix + reqName: nameKeyVal`
	err = setOMapKeys(ctx, conn, journalPool, cj.namespace, cj.csiDirectory,
		map[string]string{cj.csiNameKeyPrefix + reqName: nameKeyVal})
	if err != nil {
		return "", "", err
	}
	defer func() {
		if err != nil {
			log.WarningLog(ctx, "reservation failed for volume: %s", reqName)
			errDefer := conn.UndoReservation(ctx, imagePool, journalPool, imageName, reqName)
			if errDefer != nil {
				log.WarningLog(ctx, "failed undoing reservation of volume: %s (%v)", reqName, errDefer)
			}
		}
	}()

	oid := cj.cephUUIDDirectoryPrefix + volUUID
	omapValues := map[string]string{}

	// NOTE: UUID directory is stored on the same pool as the image, helps determine image attributes
	// 	and also CSI journal pool, when only the VolumeID is passed in (e.g DeleteVolume/DeleteSnapshot,
	// 	VolID during CreateSnapshot).
	// Update UUID directory to store CSI request name
	omapValues[cj.csiNameKey] = reqName

	// Update UUID directory to store image name
	omapValues[cj.csiImageKey] = imageName

	// Update UUID directory to store encryption values
	if kmsConf != "" {
		omapValues[cj.encryptKMSKey] = kmsConf
		omapValues[cj.encryptionType] = util.EncryptionTypeString(encryptionType)
	}

	// if owner is passed, set it in the UUID directory too
	if owner != "" {
		omapValues[cj.ownerKey] = owner
	}

	if journalPool != imagePool && journalPoolID != util.InvalidPoolID {
		buf64 := make([]byte, 8)
		binary.BigEndian.PutUint64(buf64, uint64(journalPoolID))
		journalPoolIDStr := hex.EncodeToString(buf64)

		// Update UUID directory to store CSI journal pool name (prefer ID instead of name to be pool rename proof)
		omapValues[cj.csiJournalPool] = journalPoolIDStr
	}

	if snapSource {
		// Update UUID directory to store source volume UUID in case of snapshots
		omapValues[cj.cephSnapSourceKey] = parentName
	}

	// Update backing snapshot ID for snapshot-backed CephFS volume
	if backingSnapshotID != "" {
		omapValues[cj.backingSnapshotIDKey] = backingSnapshotID
	}

	err = setOMapKeys(ctx, conn, journalPool, cj.namespace, oid, omapValues)
	if err != nil {
		return "", "", err
	}

	return volUUID, imageName, nil
}

// ImageAttributes contains all CSI stored image attributes, typically as OMap keys.
type ImageAttributes struct {
	RequestName       string              // Contains the request name for the passed in UUID
	SourceName        string              // Contains the parent image name for the passed in UUID, if it is a snapshot
	ImageName         string              // Contains the image or subvolume name for the passed in UUID
	KmsID             string              // Contains encryption KMS, if it is an encrypted image
	EncryptionType    util.EncryptionType // Type of encryption used, if image encrypted
	Owner             string              // Contains the owner to be used in combination with KmsID (for some KMS)
	ImageID           string              // Contains the image id
	JournalPoolID     int64               // Pool ID of the CSI journal pool, stored in big endian format (on-disk data)
	BackingSnapshotID string              // ID of the snapshot on which the CephFS snapshot-backed volume is based
}

// GetImageAttributes fetches all keys and their values, from a UUID directory, returning ImageAttributes structure.
func (conn *Connection) GetImageAttributes(
	ctx context.Context,
	pool, objectUUID string,
	snapSource bool,
) (*ImageAttributes, error) {
	var (
		err             error
		imageAttributes = &ImageAttributes{}
		cj              = conn.config
	)

	if snapSource && cj.cephSnapSourceKey == "" {
		err = errors.New("invalid request, cephSnapSourceKey is nil")

		return nil, err
	}

	fetchKeys := []string{
		cj.csiNameKey,
		cj.csiImageKey,
		cj.encryptKMSKey,
		cj.encryptionType,
		cj.csiJournalPool,
		cj.cephSnapSourceKey,
		cj.csiImageIDKey,
		cj.ownerKey,
		cj.backingSnapshotIDKey,
	}
	values, err := getOMapValues(
		ctx, conn, pool, cj.namespace, cj.cephUUIDDirectoryPrefix+objectUUID,
		cj.commonPrefix, fetchKeys)
	if err != nil {
		if !errors.Is(err, util.ErrKeyNotFound) && !errors.Is(err, util.ErrPoolNotFound) {
			return nil, err
		}
		log.WarningLog(ctx, "unable to read omap keys: pool or key missing: %v", err)
	}

	var found bool
	imageAttributes.RequestName = values[cj.csiNameKey]
	imageAttributes.KmsID = values[cj.encryptKMSKey]
	imageAttributes.EncryptionType = util.ParseEncryptionType(values[cj.encryptionType])
	imageAttributes.Owner = values[cj.ownerKey]
	imageAttributes.ImageID = values[cj.csiImageIDKey]
	imageAttributes.BackingSnapshotID = values[cj.backingSnapshotIDKey]

	// image key was added at a later point, so not all volumes will have this
	// key set when ceph-csi was upgraded
	imageAttributes.ImageName, found = values[cj.csiImageKey]
	if !found {
		// if the key was not found, assume the default key + UUID
		if snapSource {
			imageAttributes.ImageName = defaultSnapshotNamingPrefix + objectUUID
		} else {
			imageAttributes.ImageName = defaultVolumeNamingPrefix + objectUUID
		}
	}

	journalPoolIDStr, found := values[cj.csiJournalPool]
	if !found {
		imageAttributes.JournalPoolID = util.InvalidPoolID
	} else {
		var buf64 []byte
		buf64, err = hex.DecodeString(journalPoolIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode string: %w", err)
		}
		imageAttributes.JournalPoolID = int64(binary.BigEndian.Uint64(buf64))
	}

	if snapSource {
		imageAttributes.SourceName, found = values[cj.cephSnapSourceKey]
		if !found {
			return nil, fmt.Errorf("%w: no snap source in omap for %q",
				util.ErrKeyNotFound, cj.cephUUIDDirectoryPrefix+objectUUID)
		}
	}

	return imageAttributes, nil
}

// StoreImageID stores the image ID in omap.
func (conn *Connection) StoreImageID(ctx context.Context, pool, reservedUUID, imageID string) error {
	err := setOMapKeys(ctx, conn, pool, conn.config.namespace, conn.config.cephUUIDDirectoryPrefix+reservedUUID,
		map[string]string{conn.config.csiImageIDKey: imageID})
	if err != nil {
		return err
	}

	return nil
}

// StoreAttribute stores an attribute (key/value) in omap.
func (conn *Connection) StoreAttribute(ctx context.Context, pool, reservedUUID, attribute, value string) error {
	key := conn.config.commonPrefix + attribute
	err := setOMapKeys(ctx, conn, pool, conn.config.namespace, conn.config.cephUUIDDirectoryPrefix+reservedUUID,
		map[string]string{key: value})
	if err != nil {
		return fmt.Errorf("failed to set key %q to %q: %w", key, value, err)
	}

	return nil
}

// FetchAttribute fetches an attribute (key) in omap.
func (conn *Connection) FetchAttribute(ctx context.Context, pool, reservedUUID, attribute string) (string, error) {
	key := conn.config.commonPrefix + attribute
	values, err := getOMapValues(
		ctx, conn, pool, conn.config.namespace, conn.config.cephUUIDDirectoryPrefix+reservedUUID,
		conn.config.commonPrefix, []string{key})
	if err != nil {
		return "", fmt.Errorf("failed to get values for key %q from OMAP: %w", key, err)
	}

	value, ok := values[key]
	if !ok {
		return "", fmt.Errorf("failed to find key %q in returned map: %v", key, values)
	}

	return value, nil
}

// Destroy frees any resources and invalidates the journal connection.
func (conn *Connection) Destroy() {
	// invalidate cluster connection metadata
	conn.monitors = ""
	conn.cr = nil
}

// CheckNewUUIDMapping checks is there any UUID mapping between old
// volumeHandle and the newly generated volumeHandle.
func (conn *Connection) CheckNewUUIDMapping(ctx context.Context,
	journalPool, volumeHandle string,
) (string, error) {
	cj := conn.config

	// check if request name is already part of the directory omap
	fetchKeys := []string{
		cj.csiNameKeyPrefix + volumeHandle,
	}
	values, err := getOMapValues(
		ctx, conn, journalPool, cj.namespace, cj.csiDirectory,
		cj.commonPrefix, fetchKeys)
	if err != nil {
		if errors.Is(err, util.ErrKeyNotFound) || errors.Is(err, util.ErrPoolNotFound) {
			// pool or omap (oid) was not present
			// stop processing but without an error for no reservation exists
			return "", nil
		}

		return "", err
	}

	return values[cj.csiNameKeyPrefix+volumeHandle], nil
}

// ReserveNewUUIDMapping creates the omap mapping between the oldVolumeHandle
// and the newVolumeHandle. In case of Async Mirroring the PV is statically
// created it will have oldVolumeHandle,the volumeHandle is composed of
// clusterID,PoolID etc. as the poolID and clusterID might be different at the
// secondary cluster cephcsi will generate the new mapping and keep it for
// internal reference.
func (conn *Connection) ReserveNewUUIDMapping(ctx context.Context,
	journalPool, oldVolumeHandle, newVolumeHandle string,
) error {
	cj := conn.config

	setKeys := map[string]string{
		cj.csiNameKeyPrefix + oldVolumeHandle: newVolumeHandle,
	}

	return setOMapKeys(ctx, conn, journalPool, cj.namespace, cj.csiDirectory, setKeys)
}

// ResetVolumeOwner updates the owner in the rados object.
func (conn *Connection) ResetVolumeOwner(ctx context.Context, pool, reservedUUID, owner string) error {
	return setOMapKeys(ctx, conn, pool, conn.config.namespace, conn.config.cephUUIDDirectoryPrefix+reservedUUID,
		map[string]string{conn.config.ownerKey: owner})
}
