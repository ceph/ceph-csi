/*
Copyright 2018 The Ceph-CSI Authors.

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
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/keymutex"
)

const (
	imageWatcherStr = "watcher="
	rbdImageFormat2 = "2"
	// The following three values are used for 30 seconds timeout
	// while waiting for RBD Watcher to expire.
	rbdImageWatcherInitDelay = 1 * time.Second
	rbdImageWatcherFactor    = 1.4
	rbdImageWatcherSteps     = 10
	rbdDefaultMounter        = "rbd"
)

// rbdVolume represents a CSI volume and its RBD image specifics
type rbdVolume struct {
	// RbdImageName is the name of the RBD image backing this rbdVolume
	// VolID is the volume ID that is exchanged with CSI drivers, identifying this rbdVol
	// RequestName is the CSI generated volume name for the rbdVolume
	RbdImageName       string
	VolID              string
	Monitors           string
	MonValueFromSecret string
	Pool               string
	ImageFormat        string
	ImageFeatures      string
	VolSize            int64
	AdminID            string
	UserID             string
	Mounter            string
	DisableInUseChecks bool
	ClusterID          string
	RequestName        string
}

// rbdSnapshot represents a CSI snapshot and its RBD snapshot specifics
type rbdSnapshot struct {
	// SourceVolumeID is the volume ID of RbdImageName, that is exchanged with CSI drivers
	// RbdImageName is the name of the RBD image, that is this rbdSnapshot's source image
	// RbdSnapName is the name of the RBD snapshot backing this rbdSnapshot
	// SnapID is the snapshot ID that is exchanged with CSI drivers, identifying this rbdSnapshot
	// RequestName is the CSI generated snapshot name for the rbdSnapshot
	SourceVolumeID     string
	RbdImageName       string
	RbdSnapName        string
	SnapID             string
	Monitors           string
	MonValueFromSecret string
	Pool               string
	CreatedAt          *timestamp.Timestamp
	SizeBytes          int64
	AdminID            string
	UserID             string
	ClusterID          string
	RequestName        string
}

var (
	// serializes operations based on "<rbd pool>/<rbd image>" as key
	attachdetachMutex = keymutex.NewHashed(0)
	// serializes operations based on "volume name" as key
	volumeNameMutex = keymutex.NewHashed(0)
	// serializes operations based on "volume id" as key
	volumeIDMutex = keymutex.NewHashed(0)
	// serializes operations based on "snapshot name" as key
	snapshotNameMutex = keymutex.NewHashed(0)
	// serializes operations based on "snapshot id" as key
	snapshotIDMutex = keymutex.NewHashed(0)
	// serializes operations based on "mount target path" as key
	targetPathMutex = keymutex.NewHashed(0)

	supportedFeatures = sets.NewString("layering")
)

func getKey(clusterid, id string, credentials map[string]string) (string, error) {
	var (
		ok  bool
		err error
		key string
	)

	if key, ok = credentials[id]; !ok {
		if clusterid != "" {
			key, err = confStore.KeyForUser(clusterid, id)
			if err != nil {
				return "", fmt.Errorf("RBD key for ID: %s not found in config store of clusterID (%s)", id, clusterid)
			}
		} else {
			return "", fmt.Errorf("RBD key for ID: %s not found", id)
		}
	}

	return key, nil
}

func getMon(pOpts *rbdVolume, credentials map[string]string) (string, error) {
	mon := pOpts.Monitors
	if len(mon) == 0 {
		// if mons are set in secret, retrieve them
		if len(pOpts.MonValueFromSecret) == 0 {
			// yet another sanity check
			return "", errors.New("either monitors or monValueFromSecret must be set")
		}
		val, ok := credentials[pOpts.MonValueFromSecret]
		if !ok {
			return "", fmt.Errorf("mon data %s is not set in secret", pOpts.MonValueFromSecret)
		}
		mon = val

	}
	return mon, nil
}

// createImage creates a new ceph image with provision and volume options.
func createImage(pOpts *rbdVolume, volSz int64, adminID string, credentials map[string]string) error {
	var output []byte

	mon, err := getMon(pOpts, credentials)
	if err != nil {
		return err
	}

	image := pOpts.RbdImageName
	volSzMiB := fmt.Sprintf("%dM", volSz)

	key, err := getKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	if pOpts.ImageFormat == rbdImageFormat2 {
		klog.V(4).Infof("rbd: create %s size %s format %s (features: %s) using mon %s, pool %s ", image, volSzMiB, pOpts.ImageFormat, pOpts.ImageFeatures, mon, pOpts.Pool)
	} else {
		klog.V(4).Infof("rbd: create %s size %s format %s using mon %s, pool %s", image, volSzMiB, pOpts.ImageFormat, mon, pOpts.Pool)
	}
	args := []string{"create", image, "--size", volSzMiB, "--pool", pOpts.Pool, "--id", adminID, "-m", mon, "--key=" + key, "--image-format", pOpts.ImageFormat}
	if pOpts.ImageFormat == rbdImageFormat2 {
		args = append(args, "--image-feature", pOpts.ImageFeatures)
	}
	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to create rbd image, command output: %s", string(output))
	}

	return nil
}

// rbdStatus checks if there is watcher on the image.
// It returns true if there is a watcher on the image, otherwise returns false.
func rbdStatus(pOpts *rbdVolume, userID string, credentials map[string]string) (bool, string, error) {
	var output string
	var cmd []byte

	image := pOpts.RbdImageName
	// If we don't have admin id/secret (e.g. attaching), fallback to user id/secret.

	key, err := getKey(pOpts.ClusterID, userID, credentials)
	if err != nil {
		return false, "", err
	}

	mon, err := getMon(pOpts, credentials)
	if err != nil {
		return false, "", err
	}

	klog.V(4).Infof("rbd: status %s using mon %s, pool %s", image, mon, pOpts.Pool)
	args := []string{"status", image, "--pool", pOpts.Pool, "-m", mon, "--id", userID, "--key=" + key}
	cmd, err = execCommand("rbd", args)
	output = string(cmd)

	if err, ok := err.(*exec.Error); ok {
		if err.Err == exec.ErrNotFound {
			klog.Errorf("rbd cmd not found")
			// fail fast if command not found
			return false, output, err
		}
	}

	// If command never succeed, returns its last error.
	if err != nil {
		return false, output, err
	}

	if strings.Contains(output, imageWatcherStr) {
		klog.V(4).Infof("rbd: watchers on %s: %s", image, output)
		return true, output, nil
	}
	klog.Warningf("rbd: no watchers on %s", image)
	return false, output, nil
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
	// Structure members used to determine if provided rbdSnapshot exists, are checked here, to
	// provide an easy way to check when this function can be reused
	if rbdSnap.RequestName == "" || rbdSnap.Monitors == "" || rbdSnap.AdminID == "" ||
		rbdSnap.Pool == "" || rbdSnap.RbdImageName == "" || rbdSnap.ClusterID == "" {
		return false, errors.New("missing information in rbdSnapshot to check for existence")
	}

	key, err := getKey(rbdSnap.ClusterID, rbdSnap.AdminID, credentials)
	if err != nil {
		return false, err
	}

	// check if request name is already part of the snaps omap
	snapUUID, err := util.GetOMapValue(rbdSnap.Monitors, rbdSnap.AdminID,
		key, rbdSnap.Pool, csiSnapsDirectory, csiSnapNameKeyPrefix+rbdSnap.RequestName)
	if err != nil {
		return false, nil
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
		// NOTE: This should never be possible, hence no cleanup, but log error
		// and return, as cleanup may need to occur manually!
		return false, fmt.Errorf("internal state inconsistent, omap volume"+
			" names disagree, request name (%s) image name (%s) image omap"+
			" volume name (%s)", rbdSnap.RequestName, rbdSnap.RbdImageName, savedVolName)
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
		klog.Errorf("internal error fetching pool ID (%s)", err)
		return false, err
	}

	vi := util.CsiIdentifier{
		PoolID:          poolID,
		EncodingVersion: volIDVersion,
		ClusterID:       rbdSnap.ClusterID,
		ObjectUUID:      snapUUID,
	}
	rbdSnap.SnapID, err = vi.ComposeCsiID()
	if err != nil {
		return false, err
	}

	klog.V(4).Infof("Found existing snap (%s) with snap name (%s) for request (%s)",
		rbdSnap.SnapID, rbdSnap.RbdSnapName, rbdSnap.RequestName)

	return true, nil
}

// ErrVolNameConflict is generated when a requested CSI volume name already exists on RBD but with
// different properties, and hence is in conflict with the passed in CSI volume name
type ErrVolNameConflict struct {
	requestName string
	err         error
}

func (e ErrVolNameConflict) Error() string {
	return e.err.Error()
}

/*
Check comment on checkSnapExists, to understand how this function behaves

**NOTE:** These functions manipulate the rados omaps that hold information regarding
volume names as requested by the CSI drivers. Hence, these need to be invoked only when the
respective CSI snapshot or volume name based locks are held, as otherwise racy access to these
omaps may end up leaving the ompas in an inconsistent state.
*/
func checkVolExists(rbdVol *rbdVolume, credentials map[string]string) (found bool, err error) {
	var vi util.CsiIdentifier

	// Structure members used to determine if provided rbdVolume exists, are checked here, to
	// provide an easy way to check when this function can be reused
	if rbdVol.RequestName == "" || rbdVol.Monitors == "" || rbdVol.AdminID == "" ||
		rbdVol.Pool == "" || rbdVol.ClusterID == "" || rbdVol.VolSize == 0 {
		return false, errors.New("missing information in rbdVolume to check for existence")
	}

	key, err := getKey(rbdVol.ClusterID, rbdVol.AdminID, credentials)
	if err != nil {
		return false, err
	}

	// check if request name is already part of the volumes omap
	imageUUID, err := util.GetOMapValue(rbdVol.Monitors, rbdVol.AdminID,
		key, rbdVol.Pool, csiVolsDirectory, csiVolNameKeyPrefix+rbdVol.RequestName)
	if err != nil {
		return false, nil
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
		klog.Errorf("internal error fetching pool ID (%s)", err)
		return false, err
	}

	vi = util.CsiIdentifier{
		PoolID:          poolID,
		EncodingVersion: volIDVersion,
		ClusterID:       rbdVol.ClusterID,
		ObjectUUID:      imageUUID,
	}
	rbdVol.VolID, err = vi.ComposeCsiID()
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
	key, err := getKey(rbdSnap.ClusterID, rbdSnap.AdminID, credentials)
	if err != nil {
		return err
	}

	// delete snap image omap (first, inverse of create order)
	snapUUID := strings.TrimPrefix(rbdSnap.RbdSnapName, rbdSnapNamePrefix)
	err = util.RemoveObject(rbdSnap.Monitors, rbdSnap.AdminID, key, rbdSnap.Pool, rbdSnapOMapPrefix+snapUUID)
	if err != nil {
		if _, ok := err.(util.ErrObjectNotFound); !ok {
			klog.V(4).Infof("failed removing oMap %s (%s)", rbdSnapOMapPrefix+snapUUID, err)
			return err
		}
	}

	// delete the request name omap key (last, inverse of create order)
	err = util.RemoveOMapKey(rbdSnap.Monitors, rbdSnap.AdminID, key, rbdSnap.Pool,
		csiSnapsDirectory, csiSnapNameKeyPrefix+rbdSnap.RequestName)
	if err != nil {
		klog.V(4).Infof("failed removing oMap key %s (%s)", csiSnapNameKeyPrefix+rbdSnap.RequestName, err)
		return err
	}

	return nil
}

func unreserveVol(rbdVol *rbdVolume, credentials map[string]string) error {
	key, err := getKey(rbdVol.ClusterID, rbdVol.AdminID, credentials)
	if err != nil {
		return err
	}

	// delete image omap (first, inverse of create order)
	imageUUID := strings.TrimPrefix(rbdVol.RbdImageName, rbdImgNamePrefix)
	err = util.RemoveObject(rbdVol.Monitors, rbdVol.AdminID, key, rbdVol.Pool, rbdImageOMapPrefix+imageUUID)
	if err != nil {
		if _, ok := err.(util.ErrObjectNotFound); !ok {
			klog.V(4).Infof("failed removing oMap %s (%s)", rbdImageOMapPrefix+imageUUID, err)
			return err
		}
	}

	// delete the request name omap key (last, inverse of create order)
	err = util.RemoveOMapKey(rbdVol.Monitors, rbdVol.AdminID, key, rbdVol.Pool,
		csiVolsDirectory, csiVolNameKeyPrefix+rbdVol.RequestName)
	if err != nil {
		klog.V(4).Infof("failed removing oMap key %s (%s)", csiVolNameKeyPrefix+rbdVol.RequestName, err)
		return err
	}

	return nil
}

// reserveOMapName creates an omap with passed in oMapNameSuffix and a generated <uuid>.
// It ensures generated omap name does not already exist and if conflicts are detected, a set
// number of retires with newer uuids are attempted before returning an error
func reserveOMapName(monitors, adminID, key, poolName, oMapNameSuffix string) (string, error) {
	var iterUUID string

	maxAttempts := 5
	attempt := 1
	for attempt <= maxAttempts {
		// generate a uuid for the image name
		iterUUID = uuid.NewUUID().String()

		err := util.CreateObject(monitors, adminID, key, poolName, oMapNameSuffix+iterUUID)
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
	var vi util.CsiIdentifier

	key, err := getKey(rbdSnap.ClusterID, rbdSnap.AdminID, credentials)
	if err != nil {
		return err
	}

	poolID, err := util.GetPoolID(rbdSnap.Monitors, rbdSnap.AdminID, key,
		rbdSnap.Pool)
	if err != nil {
		klog.Errorf("Error fetching pool ID (%s)", err)
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
				klog.Warningf("failed undoing reservation of snapshot: %s", rbdSnap.RequestName)
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
	vi = util.CsiIdentifier{
		PoolID:          poolID,
		EncodingVersion: volIDVersion,
		ClusterID:       rbdSnap.ClusterID,
		ObjectUUID:      snapUUID,
	}
	rbdSnap.SnapID, err = vi.ComposeCsiID()
	if err != nil {
		return err
	}
	rbdSnap.RbdSnapName = rbdSnapNamePrefix + snapUUID
	klog.V(4).Infof("Generated Volume ID (%s) and image name (%s) for request name (%s)",
		rbdSnap.SnapID, rbdSnap.RbdImageName, rbdSnap.RequestName)

	return nil
}

func reserveVol(rbdVol *rbdVolume, credentials map[string]string) error {
	var vi util.CsiIdentifier

	key, err := getKey(rbdVol.ClusterID, rbdVol.AdminID, credentials)
	if err != nil {
		return err
	}

	poolID, err := util.GetPoolID(rbdVol.Monitors, rbdVol.AdminID, key,
		rbdVol.Pool)
	if err != nil {
		klog.Errorf("Error fetching pool ID (%s)", err)
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
				klog.Warningf("failed undoing reservation of volume: %s", rbdVol.RequestName)
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
	vi = util.CsiIdentifier{
		PoolID:          poolID,
		EncodingVersion: volIDVersion,
		ClusterID:       rbdVol.ClusterID,
		ObjectUUID:      imageUUID,
	}
	rbdVol.VolID, err = vi.ComposeCsiID()
	if err != nil {
		return err
	}
	rbdVol.RbdImageName = rbdImgNamePrefix + imageUUID
	klog.V(4).Infof("Generated Volume ID (%s) and image name (%s) for request name (%s)",
		rbdVol.VolID, rbdVol.RbdImageName, rbdVol.RequestName)

	return nil
}

// deleteImage deletes a ceph image with provision and volume options.
func deleteImage(pOpts *rbdVolume, adminID string, credentials map[string]string) error {
	var output []byte

	image := pOpts.RbdImageName
	found, _, err := rbdStatus(pOpts, adminID, credentials)
	if err != nil {
		return err
	}
	if found {
		klog.Info("rbd is still being used ", image)
		return fmt.Errorf("rbd %s is still being used", image)
	}
	key, err := getKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	mon, err := getMon(pOpts, credentials)
	if err != nil {
		return err
	}

	klog.V(4).Infof("rbd: rm %s using mon %s, pool %s", image, mon, pOpts.Pool)
	args := []string{"rm", image, "--pool", pOpts.Pool, "--id", adminID, "-m", mon, "--key=" + key}
	output, err = execCommand("rbd", args)
	if err != nil {
		klog.Errorf("failed to delete rbd image: %v, command output: %s", err, string(output))
		return err
	}

	err = unreserveVol(pOpts, credentials)
	if err != nil {
		klog.Errorf("failed to remove reservation for volume (%s) with backing image (%s) (%s)",
			pOpts.RequestName, pOpts.RbdImageName, err)
		err = nil
	}

	return err
}

// updateSnapWithImageInfo updates provided rbdSnapshot with information from on-disk data
// regarding the same
func updateSnapWithImageInfo(rbdSnap *rbdSnapshot, credentials map[string]string) error {
	key, err := getKey(rbdSnap.ClusterID, rbdSnap.AdminID, credentials)
	if err != nil {
		return err
	}

	snapInfo, err := getSnapInfo(rbdSnap.Monitors, rbdSnap.AdminID, key,
		rbdSnap.Pool, rbdSnap.RbdImageName, rbdSnap.RbdSnapName)
	if err != nil {
		return err
	}

	rbdSnap.SizeBytes = snapInfo.Size

	tm, err := time.Parse(time.ANSIC, snapInfo.TimeStamp)
	if err != nil {
		return err
	}

	rbdSnap.CreatedAt, err = ptypes.TimestampProto(tm)
	if err != nil {
		return err
	}

	return nil
}

// updateVolWithImageInfo updates provided rbdVolume with information from on-disk data
// regarding the same
func updateVolWithImageInfo(rbdVol *rbdVolume, credentials map[string]string) error {
	key, err := getKey(rbdVol.ClusterID, rbdVol.AdminID, credentials)
	if err != nil {
		return err
	}

	imageInfo, err := getImageInfo(rbdVol.Monitors, rbdVol.AdminID, key,
		rbdVol.Pool, rbdVol.RbdImageName)
	if err != nil {
		return err
	}

	switch imageInfo.Format {
	case 2:
		rbdVol.ImageFormat = rbdImageFormat2
	default:
		return fmt.Errorf("unknown or unsupported image format (%d) returned for image (%s)",
			imageInfo.Format, rbdVol.RbdImageName)
	}

	rbdVol.VolSize = imageInfo.Size
	rbdVol.ImageFeatures = strings.Join(imageInfo.Features, ",")

	return nil
}

// genSnapFromSnapID generates a rbdSnapshot structure from the provided identifier, updating
// the structure with elements from on-disk snapshot metadata as well
func genSnapFromSnapID(rbdSnap *rbdSnapshot, snapshotID string, credentials map[string]string) error {
	var (
		options map[string]string
		vi      util.CsiIdentifier
	)
	options = make(map[string]string)

	rbdSnap.SnapID = snapshotID

	err := vi.DecomposeCsiID(rbdSnap.SnapID)
	if err != nil {
		klog.V(4).Infof("error decoding snapshot ID (%s) (%s)", err, rbdSnap.SnapID)
		return err
	}

	rbdSnap.ClusterID = vi.ClusterID
	options["clusterID"] = rbdSnap.ClusterID
	rbdSnap.RbdSnapName = rbdSnapNamePrefix + vi.ObjectUUID

	rbdSnap.Monitors, _, _, err = getMonsAndClusterID(options)
	if err != nil {
		return err
	}

	rbdSnap.AdminID, rbdSnap.UserID, err = getIDs(options, rbdSnap.ClusterID)
	if err != nil {
		return err
	}

	key, err := getKey(rbdSnap.ClusterID, rbdSnap.AdminID, credentials)
	if err != nil {
		return err
	}

	rbdSnap.Pool, err = util.GetPoolName(rbdSnap.Monitors, rbdSnap.AdminID, key, vi.PoolID)
	if err != nil {
		return err
	}

	// TODO: fetch all omap vals in one call, than make multiple listomapvals
	snapUUID := strings.TrimPrefix(rbdSnap.RbdSnapName, rbdSnapNamePrefix)
	rbdSnap.RequestName, err = util.GetOMapValue(rbdSnap.Monitors, rbdSnap.AdminID,
		key, rbdSnap.Pool, rbdSnapOMapPrefix+snapUUID, rbdSnapCSISnapNameKey)
	if err != nil {
		return err
	}

	rbdSnap.RbdImageName, err = util.GetOMapValue(rbdSnap.Monitors, rbdSnap.AdminID,
		key, rbdSnap.Pool, rbdSnapOMapPrefix+snapUUID, rbdSnapSourceImageKey)
	if err != nil {
		return err
	}

	err = updateSnapWithImageInfo(rbdSnap, credentials)
	if err != nil {
		return err
	}

	return nil
}

// genVolFromVolID generates a rbdVolume structure from the provided identifier, updating
// the structure with elements from on-disk image metadata as well
func genVolFromVolID(rbdVol *rbdVolume, volumeID string, credentials map[string]string) error {
	var (
		options map[string]string
		vi      util.CsiIdentifier
	)
	options = make(map[string]string)

	// rbdVolume fields that are not filled up in this function are:
	//		Mounter, MultiNodeWritable
	rbdVol.VolID = volumeID

	err := vi.DecomposeCsiID(rbdVol.VolID)
	if err != nil {
		klog.V(4).Infof("error decoding volume ID (%s) (%s)", err, rbdVol.VolID)
		return err
	}

	rbdVol.ClusterID = vi.ClusterID
	options["clusterID"] = rbdVol.ClusterID
	rbdVol.RbdImageName = rbdImgNamePrefix + vi.ObjectUUID

	rbdVol.Monitors, _, _, err = getMonsAndClusterID(options)
	if err != nil {
		return err
	}

	rbdVol.AdminID, rbdVol.UserID, err = getIDs(options, rbdVol.ClusterID)
	if err != nil {
		return err
	}

	key, err := getKey(rbdVol.ClusterID, rbdVol.AdminID, credentials)
	if err != nil {
		return err
	}

	rbdVol.Pool, err = util.GetPoolName(rbdVol.Monitors, rbdVol.AdminID, key,
		vi.PoolID)
	if err != nil {
		return err
	}

	imageUUID := strings.TrimPrefix(rbdVol.RbdImageName, rbdImgNamePrefix)
	rbdVol.RequestName, err = util.GetOMapValue(rbdVol.Monitors, rbdVol.AdminID,
		key, rbdVol.Pool, rbdImageOMapPrefix+imageUUID, rbdImageCSIVolNameKey)
	if err != nil {
		return err
	}

	err = updateVolWithImageInfo(rbdVol, credentials)
	if err != nil {
		return err
	}

	return nil
}

func execCommand(command string, args []string) ([]byte, error) {
	// #nosec
	cmd := exec.Command(command, args...)
	return cmd.CombinedOutput()
}

func getMonsAndClusterID(options map[string]string) (monitors, clusterID, monInSecret string, err error) {
	var ok bool

	monitors, ok = options["monitors"]
	if !ok {
		// if mons are not set in options, check if they are set in secret
		if monInSecret, ok = options["monValueFromSecret"]; !ok {
			// if mons are not in secret, check if we have a cluster-id
			if clusterID, ok = options["clusterID"]; !ok {
				err = errors.New("either monitors or monValueFromSecret or clusterID must be set")
				return
			}

			if monitors, err = confStore.Mons(clusterID); err != nil {
				klog.Errorf("failed getting mons (%s)", err)
				err = fmt.Errorf("failed to fetch monitor list using clusterID (%s)", clusterID)
				return
			}
		}
	}

	return
}

func getIDs(options map[string]string, clusterID string) (adminID, userID string, err error) {
	var ok bool

	adminID, ok = options["adminid"]
	switch {
	case ok:
	case clusterID != "":
		if adminID, err = confStore.AdminID(clusterID); err != nil {
			klog.Errorf("failed getting adminID (%s)", err)
			return "", "", fmt.Errorf("failed to fetch adminID for clusterID (%s)", clusterID)
		}
	default:
		adminID = rbdDefaultAdminID
	}

	userID, ok = options["userid"]
	switch {
	case ok:
	case clusterID != "":
		if userID, err = confStore.UserID(clusterID); err != nil {
			klog.Errorf("failed getting userID (%s)", err)
			return "", "", fmt.Errorf("failed to fetch userID using clusterID (%s)", clusterID)
		}
	default:
		userID = rbdDefaultUserID
	}

	return adminID, userID, err
}

func genVolFromVolumeOptions(volOptions map[string]string, disableInUseChecks bool) (*rbdVolume, error) {
	var (
		ok  bool
		err error
	)

	rbdVol := &rbdVolume{}
	rbdVol.Pool, ok = volOptions["pool"]
	if !ok {
		return nil, errors.New("missing required parameter pool")
	}

	rbdVol.Monitors, rbdVol.ClusterID, rbdVol.MonValueFromSecret, err = getMonsAndClusterID(volOptions)
	if err != nil {
		return nil, err
	}

	rbdVol.ImageFormat, ok = volOptions["imageFormat"]
	if !ok {
		rbdVol.ImageFormat = rbdImageFormat2
	}

	if rbdVol.ImageFormat == rbdImageFormat2 {
		// if no image features is provided, it results in empty string
		// which disable all RBD image format 2 features as we expected
		imageFeatures, found := volOptions["imageFeatures"]
		if found {
			arr := strings.Split(imageFeatures, ",")
			for _, f := range arr {
				if !supportedFeatures.Has(f) {
					return nil, fmt.Errorf("invalid feature %q for volume csi-rbdplugin, supported"+
						" features are: %v", f, supportedFeatures)
				}
			}
			rbdVol.ImageFeatures = imageFeatures
		}

	}

	klog.V(3).Infof("setting disableInUseChecks on rbd volume to: %v", disableInUseChecks)
	rbdVol.DisableInUseChecks = disableInUseChecks

	err = getCredsFromVol(rbdVol, volOptions)
	if err != nil {
		return nil, err
	}

	return rbdVol, nil
}

func getCredsFromVol(rbdVol *rbdVolume, volOptions map[string]string) error {
	var (
		ok  bool
		err error
	)

	rbdVol.AdminID, rbdVol.UserID, err = getIDs(volOptions, rbdVol.ClusterID)
	if err != nil {
		return err
	}

	rbdVol.Mounter, ok = volOptions["mounter"]
	if !ok {
		rbdVol.Mounter = rbdDefaultMounter
	}

	return err
}

func genSnapFromOptions(snapOptions map[string]string) (*rbdSnapshot, error) {
	var (
		ok  bool
		err error
	)

	rbdSnap := &rbdSnapshot{}
	rbdSnap.Pool, ok = snapOptions["pool"]
	if !ok {
		return nil, errors.New("missing required parameter pool")
	}

	rbdSnap.Monitors, rbdSnap.ClusterID, rbdSnap.MonValueFromSecret, err = getMonsAndClusterID(snapOptions)
	if err != nil {
		return nil, err
	}

	rbdSnap.AdminID, rbdSnap.UserID, err = getIDs(snapOptions, rbdSnap.ClusterID)
	if err != nil {
		return nil, err
	}
	return rbdSnap, nil
}

func hasSnapshotFeature(imageFeatures string) bool {
	arr := strings.Split(imageFeatures, ",")
	for _, f := range arr {
		if f == "layering" {
			return true
		}
	}
	return false
}

func getSnapMon(pOpts *rbdSnapshot, credentials map[string]string) (string, error) {
	mon := pOpts.Monitors
	if len(mon) == 0 {
		// if mons are set in secret, retrieve them
		if len(pOpts.MonValueFromSecret) == 0 {
			// yet another sanity check
			return "", errors.New("either monitors or monValueFromSecret must be set")
		}
		val, ok := credentials[pOpts.MonValueFromSecret]
		if !ok {
			return "", fmt.Errorf("mon data %s is not set in secret", pOpts.MonValueFromSecret)
		}
		mon = val
	}
	return mon, nil
}

func protectSnapshot(pOpts *rbdSnapshot, adminID string, credentials map[string]string) error {
	var output []byte

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	key, err := getKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	mon, err := getSnapMon(pOpts, credentials)
	if err != nil {
		return err
	}

	klog.V(4).Infof("rbd: snap protect %s using mon %s, pool %s ", image, mon, pOpts.Pool)
	args := []string{"snap", "protect", "--pool", pOpts.Pool, "--snap", snapName, image, "--id", adminID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to protect snapshot, command output: %s", string(output))
	}

	return nil
}

func createSnapshot(pOpts *rbdSnapshot, adminID string, credentials map[string]string) error {
	var output []byte

	mon, err := getSnapMon(pOpts, credentials)
	if err != nil {
		return err
	}

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	key, err := getKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	klog.V(4).Infof("rbd: snap create %s using mon %s, pool %s", image, mon, pOpts.Pool)
	args := []string{"snap", "create", "--pool", pOpts.Pool, "--snap", snapName, image, "--id", adminID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to create snapshot, command output: %s", string(output))
	}

	return nil
}

func unprotectSnapshot(pOpts *rbdSnapshot, adminID string, credentials map[string]string) error {
	var output []byte

	mon, err := getSnapMon(pOpts, credentials)
	if err != nil {
		return err
	}

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	key, err := getKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	klog.V(4).Infof("rbd: snap unprotect %s using mon %s, pool %s", image, mon, pOpts.Pool)
	args := []string{"snap", "unprotect", "--pool", pOpts.Pool, "--snap", snapName, image, "--id", adminID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to unprotect snapshot, command output: %s", string(output))
	}

	return nil
}

func deleteSnapshot(pOpts *rbdSnapshot, adminID string, credentials map[string]string) error {
	var output []byte

	mon, err := getSnapMon(pOpts, credentials)
	if err != nil {
		return err
	}

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	key, err := getKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	klog.V(4).Infof("rbd: snap rm %s using mon %s, pool %s", image, mon, pOpts.Pool)
	args := []string{"snap", "rm", "--pool", pOpts.Pool, "--snap", snapName, image, "--id", adminID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to delete snapshot, command output: %s", string(output))
	}

	if err := unreserveSnap(pOpts, credentials); err != nil {
		klog.Errorf("failed to remove reservation for snapname (%s) with backing snap (%s) on image (%s) (%s)",
			pOpts.RequestName, pOpts.RbdSnapName, pOpts.RbdImageName, err)
	}

	return nil
}

func restoreSnapshot(pVolOpts *rbdVolume, pSnapOpts *rbdSnapshot, adminID string, credentials map[string]string) error {
	var output []byte

	mon, err := getMon(pVolOpts, credentials)
	if err != nil {
		return err
	}

	image := pVolOpts.RbdImageName
	snapName := pSnapOpts.RbdSnapName

	key, err := getKey(pVolOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	klog.V(4).Infof("rbd: clone %s using mon %s, pool %s", image, mon, pVolOpts.Pool)
	args := []string{"clone", pSnapOpts.Pool + "/" + pSnapOpts.RbdImageName + "@" + snapName,
		pVolOpts.Pool + "/" + image, "--id", adminID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to restore snapshot, command output: %s", string(output))
	}

	return nil
}

// getSnapshotMetadata fetches on-disk metadata about the snapshot and populates the passed in
// rbdSnapshot structure
func getSnapshotMetadata(pSnapOpts *rbdSnapshot, adminID string, credentials map[string]string) error {
	mon, err := getSnapMon(pSnapOpts, credentials)
	if err != nil {
		return err
	}

	imageName := pSnapOpts.RbdImageName
	snapName := pSnapOpts.RbdSnapName

	key, err := getKey(pSnapOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}

	snapInfo, err := getSnapInfo(mon, adminID, key, pSnapOpts.Pool, imageName, snapName)
	if err != nil {
		return err
	}

	pSnapOpts.SizeBytes = snapInfo.Size

	tm, err := time.Parse(time.ANSIC, snapInfo.TimeStamp)
	if err != nil {
		return err
	}

	pSnapOpts.CreatedAt, err = ptypes.TimestampProto(tm)
	if err != nil {
		return err
	}

	return nil
}

// imageInfo strongly typed JSON spec for image info
type imageInfo struct {
	ObjectUUID string   `json:"name"`
	Size       int64    `json:"size"`
	Format     int64    `json:"format"`
	Features   []string `json:"features"`
	CreatedAt  string   `json:"create_timestamp"`
}

// ErrImageNotFound is returned when image name is not found in the cluster on the given pool
type ErrImageNotFound struct {
	imageName string
	err       error
}

func (e ErrImageNotFound) Error() string {
	return e.err.Error()
}

// getImageInfo queries rbd about the given image and returns its metadata, and returns
// ErrImageNotFound if provided image is not found
func getImageInfo(monitors, adminID, key, poolName, imageName string) (imageInfo, error) {
	// rbd --format=json info [image-spec | snap-spec]

	var imgInfo imageInfo

	stdout, _, err := util.ExecCommand(
		"rbd",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", util.CephConfigPath,
		"--format="+"json",
		"info", poolName+"/"+imageName)
	if err != nil {
		klog.Errorf("failed getting information for image (%s): (%s)", poolName+"/"+imageName, err)
		if strings.Contains(string(stdout), "rbd: error opening image "+imageName+
			": (2) No such file or directory") {
			return imgInfo, ErrImageNotFound{imageName, err}
		}
		return imgInfo, err
	}

	err = json.Unmarshal(stdout, &imgInfo)
	if err != nil {
		klog.Errorf("failed to parse JSON output of image info (%s): (%s)",
			poolName+"/"+imageName, err)
		return imgInfo, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s",
			err, string(stdout))
	}

	return imgInfo, nil
}

// snapInfo strongly typed JSON spec for snap ls rbd output
type snapInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	TimeStamp string `json:"timestamp"`
}

// ErrSnapNotFound is returned when snap name passed is not found in the list of snapshots for the
// given image
type ErrSnapNotFound struct {
	snapName string
	err      error
}

func (e ErrSnapNotFound) Error() string {
	return e.err.Error()
}

/*
getSnapInfo queries rbd about the snapshots of the given image and returns its metadata, and
returns ErrImageNotFound if provided image is not found, and ErrSnapNotFound if povided snap
is not found in the images snapshot list
*/
func getSnapInfo(monitors, adminID, key, poolName, imageName, snapName string) (snapInfo, error) {
	// rbd --format=json snap ls [image-spec]

	var (
		snpInfo snapInfo
		snaps   []snapInfo
	)

	stdout, _, err := util.ExecCommand(
		"rbd",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", util.CephConfigPath,
		"--format="+"json",
		"snap", "ls", poolName+"/"+imageName)
	if err != nil {
		klog.Errorf("failed getting snap (%s) information from image (%s): (%s)",
			snapName, poolName+"/"+imageName, err)
		if strings.Contains(string(stdout), "rbd: error opening image "+imageName+
			": (2) No such file or directory") {
			return snpInfo, ErrImageNotFound{imageName, err}
		}
		return snpInfo, err
	}

	err = json.Unmarshal(stdout, &snaps)
	if err != nil {
		klog.Errorf("failed to parse JSON output of image snap list (%s): (%s)",
			poolName+"/"+imageName, err)
		return snpInfo, fmt.Errorf("unmarshal failed: %+v. raw buffer response: %s",
			err, string(stdout))
	}

	for _, snap := range snaps {
		if snap.Name == snapName {
			return snap, nil
		}
	}

	return snpInfo, ErrSnapNotFound{snapName, fmt.Errorf("snap (%s) for image (%s) not found",
		snapName, poolName+"/"+imageName)}
}
