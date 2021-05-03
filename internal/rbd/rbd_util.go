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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/internal/util"

	"github.com/ceph/go-ceph/rados"
	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cloud-provider/volume/helpers"
)

const (
	// The following three values are used for 30 seconds timeout
	// while waiting for RBD Watcher to expire.
	rbdImageWatcherInitDelay = 1 * time.Second
	rbdImageWatcherFactor    = 1.4
	rbdImageWatcherSteps     = 10
	rbdDefaultMounter        = "rbd"
	rbdNbdMounter            = "rbd-nbd"

	// Output strings returned during invocation of "ceph rbd task add remove <imagespec>" when
	// command is not supported by ceph manager. Used to check errors and recover when the command
	// is unsupported.
	rbdTaskRemoveCmdInvalidString1      = "no valid command found"
	rbdTaskRemoveCmdInvalidString2      = "Error EINVAL: invalid command"
	rbdTaskRemoveCmdAccessDeniedMessage = "Error EACCES:"

	// image metadata key for thick-provisioning
	thickProvisionMetaKey = ".rbd.csi.ceph.com/thick-provisioned"
)

// rbdImage contains common attributes and methods for the rbdVolume and
// rbdSnapshot types.
type rbdImage struct {
	// RbdImageName is the name of the RBD image backing this rbdVolume.
	// This does not have a JSON tag as it is not stashed in JSON encoded
	// config maps in v1.0.0
	RbdImageName string
	// ImageID contains the image id of the image
	ImageID string
	// VolID is the volume ID that is exchanged with CSI drivers,
	// identifying this rbd image
	VolID string `json:"volID"`

	Monitors string
	// JournalPool is the ceph pool in which the CSI Journal/CSI snapshot Journal is
	// stored
	JournalPool string
	// Pool is where the image journal/image snapshot journal and image/snapshot
	// is stored, and could be the same as `JournalPool` (retained as Pool instead of
	// renaming to ImagePool or such, as this is referenced in the code
	// extensively)
	Pool           string
	RadosNamespace string
	ClusterID      string `json:"clusterId"`
	// RequestName is the CSI generated volume name for the rbdVolume.
	// This does not have a JSON tag as it is not stashed in JSON encoded
	// config maps in v1.0.0
	RequestName string
	NamePrefix  string

	// encryption provides access to optional VolumeEncryption functions
	encryption *util.VolumeEncryption
	// Owner is the creator (tenant, Kubernetes Namespace) of the volume
	Owner string

	CreatedAt *timestamp.Timestamp

	// conn is a connection to the Ceph cluster obtained from a ConnPool
	conn *util.ClusterConnection
	// an opened IOContext, call .openIoctx() before using
	ioctx *rados.IOContext
}

// rbdVolume represents a CSI volume and its RBD image specifics.
type rbdVolume struct {
	rbdImage

	// VolName and MonValueFromSecret are retained from older plugin versions (<= 1.0.0)
	//   for backward compatibility reasons
	TopologyPools       *[]util.TopologyConstrainedPool
	TopologyRequirement *csi.TopologyRequirement
	Topology            map[string]string
	// DataPool is where the data for images in `Pool` are stored, this is used as the `--data-pool`
	// 	 argument when the pool is created, and is not used anywhere else
	DataPool           string
	ParentName         string
	imageFeatureSet    librbd.FeatureSet
	AdminID            string `json:"adminId"`
	UserID             string `json:"userId"`
	Mounter            string `json:"mounter"`
	ReservedID         string
	MapOptions         string
	UnmapOptions       string
	VolName            string `json:"volName"`
	MonValueFromSecret string `json:"monValueFromSecret"`
	VolSize            int64  `json:"volSize"`
	DisableInUseChecks bool   `json:"disableInUseChecks"`
	readOnly           bool
	Primary            bool
	ThickProvision     bool
}

// rbdSnapshot represents a CSI snapshot and its RBD snapshot specifics.
type rbdSnapshot struct {
	rbdImage

	// SourceVolumeID is the volume ID of RbdImageName, that is exchanged with CSI drivers
	// RbdSnapName is the name of the RBD snapshot backing this rbdSnapshot
	SourceVolumeID string
	ReservedID     string
	RbdSnapName    string
	SizeBytes      int64
}

// imageFeature represents required image features and value.
type imageFeature struct {
	// needRbdNbd indicates whether this image feature requires an rbd-nbd mounter
	needRbdNbd bool
	// dependsOn is the image features required for this imageFeature
	dependsOn []string
}

var (
	supportedFeatures = map[string]imageFeature{
		librbd.FeatureNameLayering: {
			needRbdNbd: false,
		},
		librbd.FeatureNameExclusiveLock: {
			needRbdNbd: true,
		},
		librbd.FeatureNameJournaling: {
			needRbdNbd: true,
			dependsOn:  []string{librbd.FeatureNameExclusiveLock},
		},
	}
)

// Connect an rbdVolume to the Ceph cluster.
func (ri *rbdImage) Connect(cr *util.Credentials) error {
	if ri.conn != nil {
		return nil
	}

	conn := &util.ClusterConnection{}
	if err := conn.Connect(ri.Monitors, cr); err != nil {
		return err
	}

	ri.conn = conn
	return nil
}

// Destroy cleans up the rbdVolume and closes the connection to the Ceph
// cluster in case one was setup.
func (ri *rbdImage) Destroy() {
	if ri.ioctx != nil {
		ri.ioctx.Destroy()
	}
	if ri.conn != nil {
		ri.conn.Destroy()
	}
	if ri.isEncrypted() {
		ri.encryption.Destroy()
	}
}

// String returns the image-spec (pool/{namespace/}image) format of the image.
func (ri *rbdImage) String() string {
	if ri.RadosNamespace != "" {
		return fmt.Sprintf("%s/%s/%s", ri.Pool, ri.RadosNamespace, ri.RbdImageName)
	}
	return fmt.Sprintf("%s/%s", ri.Pool, ri.RbdImageName)
}

// String returns the snap-spec (pool/{namespace/}image@snap) format of the snapshot.
func (rs *rbdSnapshot) String() string {
	if rs.RadosNamespace != "" {
		return fmt.Sprintf("%s/%s/%s@%s", rs.Pool, rs.RadosNamespace, rs.RbdImageName, rs.RbdSnapName)
	}
	return fmt.Sprintf("%s/%s@%s", rs.Pool, rs.RbdImageName, rs.RbdSnapName)
}

// createImage creates a new ceph image with provision and volume options.
func createImage(ctx context.Context, pOpts *rbdVolume, cr *util.Credentials) error {
	volSzMiB := fmt.Sprintf("%dM", util.RoundOffVolSize(pOpts.VolSize))
	options := librbd.NewRbdImageOptions()

	logMsg := "rbd: create %s size %s (features: %s) using mon %s"
	if pOpts.DataPool != "" {
		logMsg += fmt.Sprintf(", data pool %s", pOpts.DataPool)
		err := options.SetString(librbd.RbdImageOptionDataPool, pOpts.DataPool)
		if err != nil {
			return fmt.Errorf("failed to set data pool: %w", err)
		}
	}
	util.DebugLog(ctx, logMsg,
		pOpts, volSzMiB, pOpts.imageFeatureSet.Names(), pOpts.Monitors)

	if pOpts.imageFeatureSet != 0 {
		err := options.SetUint64(librbd.RbdImageOptionFeatures, uint64(pOpts.imageFeatureSet))
		if err != nil {
			return fmt.Errorf("failed to set image features: %w", err)
		}
	}

	err := pOpts.Connect(cr)
	if err != nil {
		return err
	}

	err = pOpts.openIoctx()
	if err != nil {
		return fmt.Errorf("failed to get IOContext: %w", err)
	}

	err = librbd.CreateImage(pOpts.ioctx, pOpts.RbdImageName,
		uint64(util.RoundOffVolSize(pOpts.VolSize)*helpers.MiB), options)
	if err != nil {
		return fmt.Errorf("failed to create rbd image: %w", err)
	}

	if pOpts.isEncrypted() {
		err = pOpts.setupEncryption(ctx)
		if err != nil {
			return fmt.Errorf("failed to setup encroption for image %s: %v", pOpts, err)
		}
	}

	if pOpts.ThickProvision {
		err = pOpts.allocate(0)
		if err != nil {
			// nolint:errcheck // deleteImage() will log errors in
			// case it fails, no need to log them here again
			_ = deleteImage(ctx, pOpts, cr)
			return fmt.Errorf("failed to thick provision image: %w", err)
		}

		err = pOpts.setThickProvisioned()
		if err != nil {
			// nolint:errcheck // deleteImage() will log errors in
			// case it fails, no need to log them here again
			_ = deleteImage(ctx, pOpts, cr)
			return fmt.Errorf("failed to mark image as thick-provisioned: %w", err)
		}
	}

	return nil
}

func (ri *rbdImage) openIoctx() error {
	if ri.ioctx != nil {
		return nil
	}

	ioctx, err := ri.conn.GetIoctx(ri.Pool)
	if err != nil {
		// GetIoctx() can return util.ErrPoolNotFound
		return err
	}

	ioctx.SetNamespace(ri.RadosNamespace)
	ri.ioctx = ioctx

	return nil
}

// getImageID queries rbd about the given image and stores its id, returns
// ErrImageNotFound if provided image is not found.
func (rv *rbdVolume) getImageID() error {
	if rv.ImageID != "" {
		return nil
	}
	image, err := rv.open()
	if err != nil {
		return err
	}
	defer image.Close()

	id, err := image.GetId()
	if err != nil {
		return err
	}
	rv.ImageID = id
	return nil
}

// open the rbdImage after it has been connected.
// ErrPoolNotFound or ErrImageNotFound are returned in case the pool or image
// can not be found, other errors will contain more details about other issues
// (permission denied, ...) and are expected to relate to configuration issues.
func (ri *rbdImage) open() (*librbd.Image, error) {
	err := ri.openIoctx()
	if err != nil {
		return nil, err
	}

	image, err := librbd.OpenImage(ri.ioctx, ri.RbdImageName, librbd.NoSnapshot)
	if err != nil {
		if errors.Is(err, librbd.ErrNotFound) {
			err = util.JoinErrors(ErrImageNotFound, err)
		}
		return nil, err
	}
	return image, nil
}

// allocate uses the stripe-period of the image to fully allocate (thick
// provision) the image.
func (rv *rbdVolume) allocate(offset uint64) error {
	// We do not want to call discard, we really want to write zeros to get
	// the allocation. This sets the option for the re-used connection, and
	// all subsequent images that are opened. That is not a problem, as
	// this is the only place images get written.
	err := rv.conn.DisableDiscardOnZeroedWriteSame()
	if err != nil {
		return err
	}

	image, err := rv.open()
	if err != nil {
		return err
	}
	defer image.Close()

	st, err := image.Stat()
	if err != nil {
		return err
	}

	sc, err := image.GetStripeCount()
	if err != nil {
		return err
	}

	// blockSize is the stripe-period: size of the object-size multiplied
	// by the stripe-count
	blockSize := sc * (1 << st.Order)
	zeroBlock := make([]byte, blockSize)

	// the actual size of the image as available in the pool, can be
	// marginally different from the requested image size
	size := st.Size - offset

	// In case the remaining space on the volume is smaller than blockSize,
	// write a partial block with WriteAt() after this loop.
	for size > blockSize {
		writeSize := size
		// write a maximum of 1GB per WriteSame() call
		if size > helpers.GiB {
			writeSize = helpers.GiB
		}

		// round down to the size of a zeroBlock
		if (writeSize % blockSize) != 0 {
			writeSize = (writeSize / blockSize) * blockSize
		}

		_, err = image.WriteSame(offset, writeSize, zeroBlock,
			rados.OpFlagNone)
		if err != nil {
			return fmt.Errorf("failed to allocate %d/%d bytes at "+
				"offset %d: %w", writeSize, blockSize, offset, err)
		}

		// write succeeded
		size -= writeSize
		offset += writeSize
	}

	// write the last remaining bytes, in case the image size can not be
	// written with the optimal blockSize
	if size != 0 {
		_, err = image.WriteAt(zeroBlock[:size], int64(offset))
		if err != nil {
			return fmt.Errorf("failed to allocate %d bytes at "+
				"offset %d: %w", size, offset, err)
		}
	}

	return nil
}

// isInUse checks if there is a watcher on the image. It returns true if there
// is a watcher on the image, otherwise returns false.
func (rv *rbdVolume) isInUse() (bool, error) {
	image, err := rv.open()
	if err != nil {
		if errors.Is(err, ErrImageNotFound) || errors.Is(err, util.ErrPoolNotFound) {
			return false, err
		}
		// any error should assume something else is using the image
		return true, err
	}
	defer image.Close()

	watchers, err := image.ListWatchers()
	if err != nil {
		return false, err
	}

	// TODO replace this with logic to get mirroring information once
	// https://github.com/ceph/go-ceph/issues/379 is fixed
	err = rv.updateVolWithImageInfo()
	if err != nil {
		return false, err
	}
	// because we opened the image, there is at least one watcher
	defaultWatchers := 1
	if rv.Primary {
		// if rbd mirror daemon is running, a watcher will be added by the rbd
		// mirror daemon for mirrored images.
		defaultWatchers++
	}
	return len(watchers) > defaultWatchers, nil
}

// addRbdManagerTask adds a ceph manager task to execute command
// asynchronously. If command is not found returns a bool set to false
// example arg ["trash", "remove","pool/image"].
func addRbdManagerTask(ctx context.Context, pOpts *rbdVolume, arg []string) (bool, error) {
	args := []string{"rbd", "task", "add"}
	args = append(args, arg...)
	util.DebugLog(ctx, "executing %v for image (%s) using mon %s, pool %s", args, pOpts.RbdImageName, pOpts.Monitors, pOpts.Pool)
	supported := true
	_, stderr, err := util.ExecCommand(ctx, "ceph", args...)

	if err != nil {
		switch {
		case strings.Contains(stderr, rbdTaskRemoveCmdInvalidString1) &&
			strings.Contains(stderr, rbdTaskRemoveCmdInvalidString2):
			util.WarningLog(ctx, "cluster with cluster ID (%s) does not support Ceph manager based rbd commands (minimum ceph version required is v14.2.3)", pOpts.ClusterID)
			supported = false
		case strings.HasPrefix(stderr, rbdTaskRemoveCmdAccessDeniedMessage):
			util.WarningLog(ctx, "access denied to Ceph MGR-based rbd commands on cluster ID (%s)", pOpts.ClusterID)
			supported = false
		default:
			util.WarningLog(ctx, "uncaught error while scheduling a task (%v): %s", err, stderr)
		}
	}
	return supported, err
}

// deleteImage deletes a ceph image with provision and volume options.
func deleteImage(ctx context.Context, pOpts *rbdVolume, cr *util.Credentials) error {
	image := pOpts.RbdImageName

	util.DebugLog(ctx, "rbd: delete %s using mon %s, pool %s", image, pOpts.Monitors, pOpts.Pool)

	// Support deleting the older rbd images whose imageID is not stored in omap
	err := pOpts.getImageID()
	if err != nil {
		return err
	}

	if pOpts.isEncrypted() {
		util.DebugLog(ctx, "rbd: going to remove DEK for %q", pOpts.String())
		if err = pOpts.encryption.RemoveDEK(pOpts.VolID); err != nil {
			util.WarningLog(ctx, "failed to clean the passphrase for volume %s: %s", pOpts.VolID, err)
		}
	}

	err = pOpts.openIoctx()
	if err != nil {
		return err
	}

	rbdImage := librbd.GetImage(pOpts.ioctx, image)
	err = rbdImage.Trash(0)
	if err != nil {
		util.ErrorLog(ctx, "failed to delete rbd image: %s, error: %v", pOpts, err)
		return err
	}

	// attempt to use Ceph manager based deletion support if available
	args := []string{"trash", "remove",
		pOpts.Pool + "/" + pOpts.ImageID,
		"--id", cr.ID,
		"--keyfile=" + cr.KeyFile,
		"-m", pOpts.Monitors,
	}
	rbdCephMgrSupported, err := addRbdManagerTask(ctx, pOpts, args)
	if rbdCephMgrSupported && err != nil {
		util.ErrorLog(ctx, "failed to add task to delete rbd image: %s, %v", pOpts, err)
		return err
	}

	if !rbdCephMgrSupported {
		err = librbd.TrashRemove(pOpts.ioctx, pOpts.ImageID, true)
		if err != nil {
			util.ErrorLog(ctx, "failed to delete rbd image: %s, %v", pOpts, err)
			return err
		}
	}

	return nil
}

func (rv *rbdVolume) getCloneDepth(ctx context.Context) (uint, error) {
	var depth uint
	vol := rbdVolume{}
	defer vol.Destroy()

	vol.Pool = rv.Pool
	vol.Monitors = rv.Monitors
	vol.RbdImageName = rv.RbdImageName
	vol.conn = rv.conn.Copy()

	err := vol.openIoctx()
	if err != nil {
		return depth, err
	}

	for {
		if vol.RbdImageName == "" {
			return depth, nil
		}
		err = vol.getImageInfo()
		if err != nil {
			// if the parent image is moved to trash the name will be present
			// in rbd image info but the image will be in trash, in that case
			// return the found depth
			if errors.Is(err, ErrImageNotFound) {
				return depth, nil
			}
			util.ErrorLog(ctx, "failed to check depth on image %s: %s", vol.String(), err)
			return depth, err
		}
		if vol.ParentName != "" {
			depth++
		}
		vol.RbdImageName = vol.ParentName
	}
}

type trashSnapInfo struct {
	origSnapName string
}

func flattenClonedRbdImages(ctx context.Context, snaps []librbd.SnapInfo, pool, monitors, rbdImageName string, cr *util.Credentials) error {
	rv := &rbdVolume{}
	rv.Monitors = monitors
	rv.Pool = pool
	rv.RbdImageName = rbdImageName

	defer rv.Destroy()
	err := rv.Connect(cr)
	if err != nil {
		util.ErrorLog(ctx, "failed to open connection %s; err %v", rv, err)
		return err
	}
	var origNameList []trashSnapInfo
	for _, snapInfo := range snaps {
		// check if the snapshot belongs to trash namespace.
		isTrash, retErr := rv.isTrashSnap(snapInfo.Id)
		if retErr != nil {
			return retErr
		}

		if isTrash {
			// get original snap name for the snapshot in trash namespace
			origSnapName, retErr := rv.getOrigSnapName(snapInfo.Id)
			if retErr != nil {
				return retErr
			}
			origNameList = append(origNameList, trashSnapInfo{origSnapName})
		}
	}

	for _, snapName := range origNameList {
		rv.RbdImageName = snapName.origSnapName
		err = rv.flattenRbdImage(ctx, cr, true, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
		if err != nil {
			util.ErrorLog(ctx, "failed to flatten %s; err %v", rv, err)
			continue
		}
	}
	return nil
}

func (rv *rbdVolume) flattenRbdImage(ctx context.Context, cr *util.Credentials, forceFlatten bool, hardlimit, softlimit uint) error {
	var depth uint
	var err error

	// skip clone depth check if request is for force flatten
	if !forceFlatten {
		depth, err = rv.getCloneDepth(ctx)
		if err != nil {
			return err
		}
		util.ExtendedLog(ctx, "clone depth is (%d), configured softlimit (%d) and hardlimit (%d) for %s", depth, softlimit, hardlimit, rv)
	}

	if !forceFlatten && (depth < hardlimit) && (depth < softlimit) {
		return nil
	}
	args := []string{"flatten", rv.String(), "--id", cr.ID, "--keyfile=" + cr.KeyFile, "-m", rv.Monitors}
	supported, err := addRbdManagerTask(ctx, rv, args)
	if supported {
		if err != nil {
			// discard flattening error if the image does not have any parent
			rbdFlattenNoParent := fmt.Sprintf("Image %s/%s does not have a parent", rv.Pool, rv.RbdImageName)
			if strings.Contains(err.Error(), rbdFlattenNoParent) {
				return nil
			}
			util.ErrorLog(ctx, "failed to add task flatten for %s : %v", rv, err)
			return err
		}
		if forceFlatten || depth >= hardlimit {
			return fmt.Errorf("%w: flatten is in progress for image %s", ErrFlattenInProgress, rv.RbdImageName)
		}
	}
	if !supported {
		util.ErrorLog(ctx, "task manager does not support flatten,image will be flattened once hardlimit is reached: %v", err)
		if forceFlatten || depth >= hardlimit {
			err = rv.Connect(cr)
			if err != nil {
				return err
			}
			err := rv.flatten()
			if err != nil {
				util.ErrorLog(ctx, "rbd failed to flatten image %s %s: %v", rv.Pool, rv.RbdImageName, err)
				return err
			}
		}
	}

	return nil
}

func (rv *rbdVolume) getParentName() (string, error) {
	rbdImage, err := rv.open()
	if err != nil {
		return "", err
	}
	defer rbdImage.Close()

	parentInfo, err := rbdImage.GetParent()
	if err != nil {
		return "", err
	}
	return parentInfo.Image.ImageName, nil
}

func (rv *rbdVolume) flatten() error {
	rbdImage, err := rv.open()
	if err != nil {
		return err
	}
	defer rbdImage.Close()

	err = rbdImage.Flatten()
	if err != nil {
		// rbd image flatten will fail if the rbd image does not have a parent
		parent, pErr := rv.getParentName()
		if pErr != nil {
			return util.JoinErrors(err, pErr)
		}
		if parent == "" {
			return nil
		}
	}
	return nil
}

func (rv *rbdVolume) hasFeature(feature uint64) bool {
	return (uint64(rv.imageFeatureSet) & feature) == feature
}

func (rv *rbdVolume) checkImageChainHasFeature(ctx context.Context, feature uint64) (bool, error) {
	vol := rbdVolume{}
	defer vol.Destroy()

	vol.Pool = rv.Pool
	vol.RadosNamespace = rv.RadosNamespace
	vol.Monitors = rv.Monitors
	vol.RbdImageName = rv.RbdImageName
	vol.conn = rv.conn.Copy()

	err := vol.openIoctx()
	if err != nil {
		return false, err
	}

	for {
		if vol.RbdImageName == "" {
			return false, nil
		}
		err = vol.getImageInfo()
		if err != nil {
			// call to getImageInfo returns the parent name even if the parent
			// is in the trash, when we try to open the parent image to get its
			// information it fails because it is already in trash. We should
			// treat error as nil if the parent is not found.
			if errors.Is(err, ErrImageNotFound) {
				return false, nil
			}
			util.ErrorLog(ctx, "failed to get image info for %s: %s", vol, err)
			return false, err
		}
		if f := vol.hasFeature(feature); f {
			return true, nil
		}
		vol.RbdImageName = vol.ParentName
	}
}

// genSnapFromSnapID generates a rbdSnapshot structure from the provided identifier, updating
// the structure with elements from on-disk snapshot metadata as well.
func genSnapFromSnapID(ctx context.Context, rbdSnap *rbdSnapshot, snapshotID string, cr *util.Credentials, secrets map[string]string) error {
	var (
		options map[string]string
		vi      util.CSIIdentifier
	)
	options = make(map[string]string)

	rbdSnap.VolID = snapshotID

	err := vi.DecomposeCSIID(rbdSnap.VolID)
	if err != nil {
		util.ErrorLog(ctx, "error decoding snapshot ID (%s) (%s)", err, rbdSnap.VolID)
		return err
	}

	rbdSnap.ClusterID = vi.ClusterID
	options["clusterID"] = rbdSnap.ClusterID

	rbdSnap.Monitors, _, err = util.GetMonsAndClusterID(options)
	if err != nil {
		util.ErrorLog(ctx, "failed getting mons (%s)", err)
		return err
	}

	rbdSnap.Pool, err = util.GetPoolName(rbdSnap.Monitors, cr, vi.LocationID)
	if err != nil {
		return err
	}
	rbdSnap.JournalPool = rbdSnap.Pool

	rbdSnap.RadosNamespace, err = util.RadosNamespace(util.CsiConfigFile, rbdSnap.ClusterID)
	if err != nil {
		return err
	}

	j, err := snapJournal.Connect(rbdSnap.Monitors, rbdSnap.RadosNamespace, cr)
	if err != nil {
		return err
	}
	defer j.Destroy()

	imageAttributes, err := j.GetImageAttributes(
		ctx, rbdSnap.Pool, vi.ObjectUUID, true)
	if err != nil {
		return err
	}
	rbdSnap.ImageID = imageAttributes.ImageID
	rbdSnap.RequestName = imageAttributes.RequestName
	rbdSnap.RbdImageName = imageAttributes.SourceName
	rbdSnap.RbdSnapName = imageAttributes.ImageName
	rbdSnap.ReservedID = vi.ObjectUUID
	rbdSnap.Owner = imageAttributes.Owner
	// convert the journal pool ID to name, for use in DeleteSnapshot cases
	if imageAttributes.JournalPoolID != util.InvalidPoolID {
		rbdSnap.JournalPool, err = util.GetPoolName(rbdSnap.Monitors, cr, imageAttributes.JournalPoolID)
		if err != nil {
			// TODO: If pool is not found we may leak the image (as DeleteSnapshot will return success)
			return err
		}
	}

	err = rbdSnap.Connect(cr)
	defer func() {
		if err != nil {
			rbdSnap.Destroy()
		}
	}()
	if err != nil {
		return fmt.Errorf("failed to connect to %q: %w",
			rbdSnap.String(), err)
	}

	if imageAttributes.KmsID != "" {
		err = rbdSnap.configureEncryption(imageAttributes.KmsID, secrets)
		if err != nil {
			return fmt.Errorf("failed to configure encryption for "+
				"%q: %w", rbdSnap.String(), err)
		}
	}

	return err
}

// generateVolumeFromVolumeID generates a rbdVolume structure from the provided identifier.
func generateVolumeFromVolumeID(ctx context.Context, volumeID string, cr *util.Credentials, secrets map[string]string) (*rbdVolume, error) {
	var (
		options map[string]string
		vi      util.CSIIdentifier
		rbdVol  *rbdVolume
		err     error
	)
	options = make(map[string]string)

	// rbdVolume fields that are not filled up in this function are:
	//              Mounter, MultiNodeWritable
	rbdVol = &rbdVolume{}
	rbdVol.VolID = volumeID

	err = vi.DecomposeCSIID(rbdVol.VolID)
	if err != nil {
		return rbdVol, fmt.Errorf("%w: error decoding volume ID (%s) (%s)",
			ErrInvalidVolID, err, rbdVol.VolID)
	}

	// TODO check clusterID mapping exists

	rbdVol.ClusterID = vi.ClusterID
	options["clusterID"] = rbdVol.ClusterID

	rbdVol.Monitors, _, err = util.GetMonsAndClusterID(options)
	if err != nil {
		util.ErrorLog(ctx, "failed getting mons (%s)", err)
		return rbdVol, err
	}

	rbdVol.RadosNamespace, err = util.RadosNamespace(util.CsiConfigFile, rbdVol.ClusterID)
	if err != nil {
		return rbdVol, err
	}

	j, err := volJournal.Connect(rbdVol.Monitors, rbdVol.RadosNamespace, cr)
	if err != nil {
		return rbdVol, err
	}
	defer j.Destroy()

	rbdVol.Pool, err = util.GetPoolName(rbdVol.Monitors, cr, vi.LocationID)
	if err != nil {
		return rbdVol, err
	}

	err = rbdVol.Connect(cr)
	if err != nil {
		return rbdVol, err
	}
	rbdVol.JournalPool = rbdVol.Pool

	imageAttributes, err := j.GetImageAttributes(
		ctx, rbdVol.Pool, vi.ObjectUUID, false)
	if err != nil {
		return rbdVol, err
	}
	rbdVol.RequestName = imageAttributes.RequestName
	rbdVol.RbdImageName = imageAttributes.ImageName
	rbdVol.ReservedID = vi.ObjectUUID
	rbdVol.ImageID = imageAttributes.ImageID
	rbdVol.Owner = imageAttributes.Owner

	if imageAttributes.KmsID != "" {
		err = rbdVol.configureEncryption(imageAttributes.KmsID, secrets)
		if err != nil {
			return rbdVol, err
		}
	}
	// convert the journal pool ID to name, for use in DeleteVolume cases
	if imageAttributes.JournalPoolID >= 0 {
		rbdVol.JournalPool, err = util.GetPoolName(rbdVol.Monitors, cr, imageAttributes.JournalPoolID)
		if err != nil {
			// TODO: If pool is not found we may leak the image (as DeleteVolume will return success)
			return rbdVol, err
		}
	}

	if rbdVol.ImageID == "" {
		err = rbdVol.storeImageID(ctx, j)
		if err != nil {
			return rbdVol, err
		}
	}
	err = rbdVol.getImageInfo()
	return rbdVol, err
}

// genVolFromVolID generates a rbdVolume structure from the provided identifier, updating
// the structure with elements from on-disk image metadata as well.
func genVolFromVolID(ctx context.Context, volumeID string, cr *util.Credentials, secrets map[string]string) (*rbdVolume, error) {
	vol, err := generateVolumeFromVolumeID(ctx, volumeID, cr, secrets)
	if !errors.Is(err, util.ErrKeyNotFound) && !errors.Is(err, util.ErrPoolNotFound) && !errors.Is(err, ErrImageNotFound) {
		return vol, err
	}

	// If the volume details are not found in the OMAP it can be a mirrored RBD
	// image and the OMAP is already generated and the volumeHandle might not
	// be the same in the PV.Spec.CSI.VolumeHandle. Check the PV annotation for
	// the new volumeHandle. If the new volumeHandle is found, generate the RBD
	// volume structure from the new volumeHandle.
	c := util.NewK8sClient()
	listOpt := metav1.ListOptions{
		LabelSelector: PVReplicatedLabelKey,
	}
	pvlist, pErr := c.CoreV1().PersistentVolumes().List(context.TODO(), listOpt)
	if pErr != nil {
		return vol, pErr
	}
	for i := range pvlist.Items {
		if pvlist.Items[i].Spec.CSI != nil && pvlist.Items[i].Spec.CSI.VolumeHandle == volumeID {
			if v, ok := pvlist.Items[i].Annotations[PVVolumeHandleAnnotationKey]; ok {
				util.UsefulLog(ctx, "found new volumeID %s for existing volumeID %s", v, volumeID)
				return generateVolumeFromVolumeID(ctx, v, cr, secrets)
			}
		}
	}
	return vol, err
}

func genVolFromVolumeOptions(ctx context.Context, volOptions, credentials map[string]string, disableInUseChecks bool) (*rbdVolume, error) {
	var (
		ok         bool
		err        error
		namePrefix string
	)

	rbdVol := &rbdVolume{}
	rbdVol.Pool, ok = volOptions["pool"]
	if !ok {
		return nil, errors.New("missing required parameter pool")
	}

	rbdVol.DataPool = volOptions["dataPool"]
	if namePrefix, ok = volOptions["volumeNamePrefix"]; ok {
		rbdVol.NamePrefix = namePrefix
	}

	rbdVol.Monitors, rbdVol.ClusterID, err = util.GetMonsAndClusterID(volOptions)
	if err != nil {
		util.ErrorLog(ctx, "failed getting mons (%s)", err)
		return nil, err
	}

	rbdVol.RadosNamespace, err = util.RadosNamespace(util.CsiConfigFile, rbdVol.ClusterID)
	if err != nil {
		return nil, err
	}
	if rbdVol.Mounter, ok = volOptions["mounter"]; !ok {
		rbdVol.Mounter = rbdDefaultMounter
	}
	// if no image features is provided, it results in empty string
	// which disable all RBD image features as we expected
	if err = rbdVol.validateImageFeatures(volOptions["imageFeatures"]); err != nil {
		util.ErrorLog(ctx, "failed to validate image features %v", err)
		return nil, err
	}

	util.ExtendedLog(ctx, "setting disableInUseChecks: %t image features: %v mounter: %s", disableInUseChecks, rbdVol.imageFeatureSet.Names(), rbdVol.Mounter)
	rbdVol.DisableInUseChecks = disableInUseChecks

	err = rbdVol.initKMS(ctx, volOptions, credentials)
	if err != nil {
		return nil, err
	}

	return rbdVol, nil
}

func (rv *rbdVolume) validateImageFeatures(imageFeatures string) error {
	arr := strings.Split(imageFeatures, ",")
	featureSet := sets.NewString(arr...)
	for _, f := range arr {
		sf, found := supportedFeatures[f]
		if !found {
			return fmt.Errorf("invalid feature %s", f)
		}

		for _, r := range sf.dependsOn {
			if !featureSet.Has(r) {
				return fmt.Errorf("feature %s requires %s to be set", f, r)
			}
		}

		if sf.needRbdNbd && rv.Mounter != rbdNbdMounter {
			return fmt.Errorf("feature %s requires rbd-nbd for mounter", f)
		}
	}
	rv.imageFeatureSet = librbd.FeatureSetFromNames(arr)
	return nil
}

func genSnapFromOptions(ctx context.Context, rbdVol *rbdVolume, snapOptions map[string]string) (*rbdSnapshot, error) {
	var err error

	rbdSnap := &rbdSnapshot{}
	rbdSnap.Pool = rbdVol.Pool
	rbdSnap.JournalPool = rbdVol.JournalPool
	rbdSnap.RadosNamespace = rbdVol.RadosNamespace

	rbdSnap.Monitors, rbdSnap.ClusterID, err = util.GetMonsAndClusterID(snapOptions)
	if err != nil {
		util.ErrorLog(ctx, "failed getting mons (%s)", err)
		return nil, err
	}

	if namePrefix, ok := snapOptions["snapshotNamePrefix"]; ok {
		rbdSnap.NamePrefix = namePrefix
	}

	return rbdSnap, nil
}

// hasSnapshotFeature checks if Layering is enabled for this image.
func (rv *rbdVolume) hasSnapshotFeature() bool {
	return (uint64(rv.imageFeatureSet) & librbd.FeatureLayering) == librbd.FeatureLayering
}

func (rv *rbdVolume) createSnapshot(ctx context.Context, pOpts *rbdSnapshot) error {
	util.DebugLog(ctx, "rbd: snap create %s using mon %s", pOpts, pOpts.Monitors)
	image, err := rv.open()
	if err != nil {
		return err
	}
	defer image.Close()

	_, err = image.CreateSnapshot(pOpts.RbdSnapName)
	return err
}

func (rv *rbdVolume) deleteSnapshot(ctx context.Context, pOpts *rbdSnapshot) error {
	util.DebugLog(ctx, "rbd: snap rm %s using mon %s", pOpts, pOpts.Monitors)
	image, err := rv.open()
	if err != nil {
		return err
	}
	defer image.Close()

	snap := image.GetSnapshot(pOpts.RbdSnapName)
	if snap == nil {
		return fmt.Errorf("snapshot value is nil for %s", pOpts.RbdSnapName)
	}
	err = snap.Remove()
	if errors.Is(err, librbd.ErrNotFound) {
		return util.JoinErrors(ErrSnapNotFound, err)
	}
	return err
}

func (rv *rbdVolume) cloneRbdImageFromSnapshot(ctx context.Context, pSnapOpts *rbdSnapshot) error {
	image := rv.RbdImageName
	var err error
	logMsg := "rbd: clone %s %s (features: %s) using mon %s"

	options := librbd.NewRbdImageOptions()
	defer options.Destroy()

	if rv.DataPool != "" {
		logMsg += fmt.Sprintf(", data pool %s", rv.DataPool)
		err = options.SetString(librbd.RbdImageOptionDataPool, rv.DataPool)
		if err != nil {
			return fmt.Errorf("failed to set data pool: %w", err)
		}
	}

	util.DebugLog(ctx, logMsg,
		pSnapOpts, image, rv.imageFeatureSet.Names(), rv.Monitors)

	if rv.imageFeatureSet != 0 {
		err = options.SetUint64(librbd.RbdImageOptionFeatures, uint64(rv.imageFeatureSet))
		if err != nil {
			return fmt.Errorf("failed to set image features: %w", err)
		}
	}

	err = options.SetUint64(librbd.ImageOptionCloneFormat, 2)
	if err != nil {
		return fmt.Errorf("failed to set image features: %w", err)
	}

	err = rv.openIoctx()
	if err != nil {
		return fmt.Errorf("failed to get IOContext: %w", err)
	}

	err = librbd.CloneImage(rv.ioctx, pSnapOpts.RbdImageName, pSnapOpts.RbdSnapName, rv.ioctx, rv.RbdImageName, options)
	if err != nil {
		return fmt.Errorf("failed to create rbd clone: %w", err)
	}

	// delete the cloned image if a next step fails
	deleteClone := true
	defer func() {
		if deleteClone {
			err = librbd.RemoveImage(rv.ioctx, rv.RbdImageName)
			if err != nil {
				util.ErrorLog(ctx, "failed to delete temporary image %q: %v", rv.String(), err)
			}
		}
	}()

	if pSnapOpts.isEncrypted() {
		pSnapOpts.conn = rv.conn.Copy()

		err = pSnapOpts.copyEncryptionConfig(&rv.rbdImage)
		if err != nil {
			return fmt.Errorf("failed to clone encryption config: %w", err)
		}
	}

	// Success! Do not delete the cloned image now :)
	deleteClone = false

	return nil
}

// getImageInfo queries rbd about the given image and returns its metadata, and returns
// ErrImageNotFound if provided image is not found.
func (rv *rbdVolume) getImageInfo() error {
	image, err := rv.open()
	if err != nil {
		return err
	}
	defer image.Close()

	imageInfo, err := image.Stat()
	if err != nil {
		return err
	}
	// TODO: can rv.VolSize not be a uint64? Or initialize it to -1?
	rv.VolSize = int64(imageInfo.Size)

	features, err := image.GetFeatures()
	if err != nil {
		return err
	}
	rv.imageFeatureSet = librbd.FeatureSet(features)

	// Get parent information.
	parentInfo, err := image.GetParent()
	if err != nil {
		// Caller should decide whether not finding
		// the parent is an error or not.
		if errors.Is(err, librbd.ErrNotFound) {
			rv.ParentName = ""
		} else {
			return err
		}
	} else {
		rv.ParentName = parentInfo.Image.ImageName
	}
	// Get image creation time
	tm, err := image.GetCreateTimestamp()
	if err != nil {
		return err
	}
	t := time.Unix(tm.Sec, tm.Nsec)
	protoTime, err := ptypes.TimestampProto(t)
	if err != nil {
		return err
	}
	rv.CreatedAt = protoTime
	return nil
}

// imageInfo strongly typed JSON spec for image info.
type imageInfo struct {
	Mirroring mirroring `json:"mirroring"`
}

// parentInfo  spec for parent volume  info.
type mirroring struct {
	Primary bool `json:"primary"`
}

// updateVolWithImageInfo updates provided rbdVolume with information from on-disk data
// regarding the same.
func (rv *rbdVolume) updateVolWithImageInfo() error {
	// rbd --format=json info [image-spec | snap-spec]
	var imgInfo imageInfo

	stdout, stderr, err := util.ExecCommand(
		context.TODO(),
		"rbd",
		"-m", rv.Monitors,
		"--id", rv.conn.Creds.ID,
		"--keyfile="+rv.conn.Creds.KeyFile,
		"-c", util.CephConfigPath,
		"--format="+"json",
		"info", rv.String())
	if err != nil {
		if strings.Contains(stderr, "rbd: error opening image "+rv.RbdImageName+
			": (2) No such file or directory") {
			return util.JoinErrors(ErrImageNotFound, err)
		}
		return err
	}

	if stdout != "" {
		err = json.Unmarshal([]byte(stdout), &imgInfo)
		if err != nil {
			return fmt.Errorf("unmarshal failed (%w), raw buffer response: %s", err, stdout)
		}
		rv.Primary = imgInfo.Mirroring.Primary
	}
	return nil
}

/*
checkSnapExists queries rbd about the snapshots of the given image and returns
ErrImageNotFound if provided image is not found, and ErrSnapNotFound if
provided snap is not found in the images snapshot list.
*/
func (rv *rbdVolume) checkSnapExists(rbdSnap *rbdSnapshot) error {
	image, err := rv.open()
	if err != nil {
		return err
	}
	defer image.Close()

	snaps, err := image.GetSnapshotNames()
	if err != nil {
		return err
	}

	for _, snap := range snaps {
		if snap.Name == rbdSnap.RbdSnapName {
			return nil
		}
	}

	return fmt.Errorf("%w: snap %s not found", ErrSnapNotFound, rbdSnap.String())
}

// rbdImageMetadataStash strongly typed JSON spec for stashed RBD image metadata.
type rbdImageMetadataStash struct {
	Version        int    `json:"Version"`
	Pool           string `json:"pool"`
	RadosNamespace string `json:"radosNamespace"`
	ImageName      string `json:"image"`
	UnmapOptions   string `json:"unmapOptions"`
	NbdAccess      bool   `json:"accessType"`
	Encrypted      bool   `json:"encrypted"`
}

// file name in which image metadata is stashed.
const stashFileName = "image-meta.json"

// spec returns the image-spec (pool/{namespace/}image) format of the image.
func (ri *rbdImageMetadataStash) String() string {
	if ri.RadosNamespace != "" {
		return fmt.Sprintf("%s/%s/%s", ri.Pool, ri.RadosNamespace, ri.ImageName)
	}
	return fmt.Sprintf("%s/%s", ri.Pool, ri.ImageName)
}

// stashRBDImageMetadata stashes required fields into the stashFileName at the passed in path, in
// JSON format.
func stashRBDImageMetadata(volOptions *rbdVolume, path string) error {
	var imgMeta = rbdImageMetadataStash{
		// there are no checks for this at present
		Version:        3, // nolint:gomnd // number specifies version.
		Pool:           volOptions.Pool,
		RadosNamespace: volOptions.RadosNamespace,
		ImageName:      volOptions.RbdImageName,
		Encrypted:      volOptions.isEncrypted(),
		UnmapOptions:   volOptions.UnmapOptions,
	}

	imgMeta.NbdAccess = false
	if volOptions.Mounter == rbdTonbd && hasNBD {
		imgMeta.NbdAccess = true
	}

	encodedBytes, err := json.Marshal(imgMeta)
	if err != nil {
		return fmt.Errorf("failed to marshall JSON image metadata for image (%s): %w", volOptions, err)
	}

	fPath := filepath.Join(path, stashFileName)
	err = ioutil.WriteFile(fPath, encodedBytes, 0600)
	if err != nil {
		return fmt.Errorf("failed to stash JSON image metadata for image (%s) at path (%s): %w", volOptions, fPath, err)
	}

	return nil
}

// lookupRBDImageMetadataStash reads and returns stashed image metadata at passed in path.
func lookupRBDImageMetadataStash(path string) (rbdImageMetadataStash, error) {
	var imgMeta rbdImageMetadataStash

	fPath := filepath.Join(path, stashFileName)
	encodedBytes, err := ioutil.ReadFile(fPath) // #nosec - intended reading from fPath
	if err != nil {
		if !os.IsNotExist(err) {
			return imgMeta, fmt.Errorf("failed to read stashed JSON image metadata from path (%s): %w", fPath, err)
		}

		return imgMeta, util.JoinErrors(ErrMissingStash, err)
	}

	err = json.Unmarshal(encodedBytes, &imgMeta)
	if err != nil {
		return imgMeta, fmt.Errorf("failed to unmarshall stashed JSON image metadata from path (%s): %w", fPath, err)
	}

	return imgMeta, nil
}

// cleanupRBDImageMetadataStash cleans up any stashed metadata at passed in path.
func cleanupRBDImageMetadataStash(path string) error {
	fPath := filepath.Join(path, stashFileName)
	if err := os.Remove(fPath); err != nil {
		return fmt.Errorf("failed to cleanup stashed JSON data (%s): %w", fPath, err)
	}

	return nil
}

// resize the given volume to new size.
// updates Volsize of rbdVolume object to newSize in case of success.
func (rv *rbdVolume) resize(newSize int64) error {
	image, err := rv.open()
	if err != nil {
		return err
	}
	defer image.Close()

	thick, err := rv.isThickProvisioned()
	if err != nil {
		return err
	}

	// offset is used to track from where on the expansion is done, so that
	// the extents can be allocated in case the image is thick-provisioned
	var offset uint64
	if thick {
		st, statErr := image.Stat()
		if statErr != nil {
			return statErr
		}

		offset = st.Size
	}

	err = image.Resize(uint64(util.RoundOffVolSize(newSize) * helpers.MiB))
	if err != nil {
		return err
	}

	if thick {
		err = rv.allocate(offset)
		if err != nil {
			resizeErr := image.Resize(offset)
			if resizeErr != nil {
				err = fmt.Errorf("failed to shrink image (%v) after failed allocation: %w", resizeErr, err)
			}
			return err
		}
	}

	// update Volsize of rbdVolume object to newSize.
	rv.VolSize = newSize

	return nil
}

func (ri *rbdImage) GetMetadata(key string) (string, error) {
	image, err := ri.open()
	if err != nil {
		return "", err
	}
	defer image.Close()

	return image.GetMetadata(key)
}

func (ri *rbdImage) SetMetadata(key, value string) error {
	image, err := ri.open()
	if err != nil {
		return err
	}
	defer image.Close()

	return image.SetMetadata(key, value)
}

// setThickProvisioned records in the image metadata that it has been
// thick-provisioned.
func (rv *rbdVolume) setThickProvisioned() error {
	err := rv.SetMetadata(thickProvisionMetaKey, "true")
	if err != nil {
		return fmt.Errorf("failed to set metadata %q for %q: %w", thickProvisionMetaKey, rv.String(), err)
	}

	return nil
}

// isThickProvisioned checks in the image metadata if the image has been marked
// as thick-provisioned. This can be used while expanding the image, so that
// the expansion can be allocated too.
func (rv *rbdVolume) isThickProvisioned() (bool, error) {
	value, err := rv.GetMetadata(thickProvisionMetaKey)
	if err != nil {
		if err == librbd.ErrNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to get metadata %q for %q: %w", thickProvisionMetaKey, rv.String(), err)
	}

	thick, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("failed to convert %q=%q to a boolean: %w", thickProvisionMetaKey, value, err)
	}
	return thick, nil
}

func (rv *rbdVolume) listSnapshots() ([]librbd.SnapInfo, error) {
	image, err := rv.open()
	if err != nil {
		return nil, err
	}
	defer image.Close()

	snapInfoList, err := image.GetSnapshotNames()
	if err != nil {
		return nil, err
	}
	return snapInfoList, nil
}

// isTrashSnap returns true if the snapshot belongs to trash namespace.
func (rv *rbdVolume) isTrashSnap(snapID uint64) (bool, error) {
	image, err := rv.open()
	if err != nil {
		return false, err
	}
	defer image.Close()

	// Get namespace type for the snapshot
	nsType, err := image.GetSnapNamespaceType(snapID)
	if err != nil {
		return false, err
	}

	if nsType == librbd.SnapNamespaceTypeTrash {
		return true, nil
	}
	return false, nil
}

// getOrigSnapName returns the original snap name for
// the snapshots in Trash Namespace.
func (rv *rbdVolume) getOrigSnapName(snapID uint64) (string, error) {
	image, err := rv.open()
	if err != nil {
		return "", err
	}
	defer image.Close()

	origSnapName, err := image.GetSnapTrashNamespace(snapID)
	if err != nil {
		return "", err
	}

	return origSnapName, nil
}
