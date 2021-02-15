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

	// Output strings returned during invocation of "ceph rbd task add remove <imagespec>" when
	// command is not supported by ceph manager. Used to check errors and recover when the command
	// is unsupported.
	rbdTaskRemoveCmdInvalidString1      = "no valid command found"
	rbdTaskRemoveCmdInvalidString2      = "Error EINVAL: invalid command"
	rbdTaskRemoveCmdAccessDeniedMessage = "Error EACCES:"

	// image metadata key for thick-provisioning
	thickProvisionMetaKey = ".rbd.csi.ceph.com/thick-provisioned"
)

// rbdVolume represents a CSI volume and its RBD image specifics.
type rbdVolume struct {
	// RbdImageName is the name of the RBD image backing this rbdVolume. This does not have a
	//   JSON tag as it is not stashed in JSON encoded config maps in v1.0.0
	// VolID is the volume ID that is exchanged with CSI drivers, identifying this rbdVol
	// RequestName is the CSI generated volume name for the rbdVolume.  This does not have a
	//   JSON tag as it is not stashed in JSON encoded config maps in v1.0.0
	// VolName and MonValueFromSecret are retained from older plugin versions (<= 1.0.0)
	//   for backward compatibility reasons
	// JournalPool is the ceph pool in which the CSI Journal is stored
	// Pool is where the image journal and image is stored, and could be the same as `JournalPool`
	//   (retained as Pool instead of renaming to ImagePool or such, as this is referenced in the code extensively)
	// DataPool is where the data for images in `Pool` are stored, this is used as the `--data-pool`
	// 	 argument when the pool is created, and is not used anywhere else
	TopologyPools       *[]util.TopologyConstrainedPool
	TopologyRequirement *csi.TopologyRequirement
	Topology            map[string]string
	RbdImageName        string
	NamePrefix          string
	VolID               string `json:"volID"`
	Monitors            string `json:"monitors"`
	JournalPool         string
	Pool                string `json:"pool"`
	DataPool            string
	RadosNamespace      string
	ImageID             string
	ParentName          string
	imageFeatureSet     librbd.FeatureSet
	AdminID             string `json:"adminId"`
	UserID              string `json:"userId"`
	Mounter             string `json:"mounter"`
	ClusterID           string `json:"clusterId"`
	RequestName         string
	ReservedID          string
	MapOptions          string
	UnmapOptions        string
	VolName             string `json:"volName"`
	MonValueFromSecret  string `json:"monValueFromSecret"`
	VolSize             int64  `json:"volSize"`
	DisableInUseChecks  bool   `json:"disableInUseChecks"`
	Encrypted           bool
	readOnly            bool
	Primary             bool
	ThickProvision      bool
	KMS                 util.EncryptionKMS
	// Owner is the creator (tenant, Kubernetes Namespace) of the volume.
	Owner     string
	CreatedAt *timestamp.Timestamp
	// conn is a connection to the Ceph cluster obtained from a ConnPool
	conn *util.ClusterConnection
	// an opened IOContext, call .openIoctx() before using
	ioctx *rados.IOContext
}

// rbdSnapshot represents a CSI snapshot and its RBD snapshot specifics.
type rbdSnapshot struct {
	// SourceVolumeID is the volume ID of RbdImageName, that is exchanged with CSI drivers
	// RbdImageName is the name of the RBD image, that is this rbdSnapshot's source image
	// RbdSnapName is the name of the RBD snapshot backing this rbdSnapshot
	// SnapID is the snapshot ID that is exchanged with CSI drivers, identifying this rbdSnapshot
	// RequestName is the CSI generated snapshot name for the rbdSnapshot
	// JournalPool is the ceph pool in which the CSI snapshot Journal is stored
	// Pool is where the image snapshot journal and snapshot is stored, and could be the same as `JournalPool`
	// ImageID contains the image id of cloned image
	SourceVolumeID string
	RbdImageName   string
	ReservedID     string
	NamePrefix     string
	RbdSnapName    string
	SnapID         string
	ImageID        string
	Monitors       string
	JournalPool    string
	Pool           string
	RadosNamespace string
	CreatedAt      *timestamp.Timestamp
	SizeBytes      int64
	ClusterID      string
	RequestName    string
}

var (
	supportedFeatures = sets.NewString(librbd.FeatureNameLayering)
)

// Connect an rbdVolume to the Ceph cluster.
func (rv *rbdVolume) Connect(cr *util.Credentials) error {
	if rv.conn != nil {
		return nil
	}

	conn := &util.ClusterConnection{}
	if err := conn.Connect(rv.Monitors, cr); err != nil {
		return err
	}

	rv.conn = conn
	return nil
}

// Destroy cleans up the rbdVolume and closes the connection to the Ceph
// cluster in case one was setup.
func (rv *rbdVolume) Destroy() {
	if rv.ioctx != nil {
		rv.ioctx.Destroy()
	}
	if rv.conn != nil {
		rv.conn.Destroy()
	}
	if rv.KMS != nil {
		rv.KMS.Destroy()
	}
}

// String returns the image-spec (pool/{namespace/}image) format of the image.
func (rv *rbdVolume) String() string {
	if rv.RadosNamespace != "" {
		return fmt.Sprintf("%s/%s/%s", rv.Pool, rv.RadosNamespace, rv.RbdImageName)
	}
	return fmt.Sprintf("%s/%s", rv.Pool, rv.RbdImageName)
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

func (rv *rbdVolume) openIoctx() error {
	if rv.ioctx != nil {
		return nil
	}

	ioctx, err := rv.conn.GetIoctx(rv.Pool)
	if err != nil {
		// GetIoctx() can return util.ErrPoolNotFound
		return err
	}

	ioctx.SetNamespace(rv.RadosNamespace)
	rv.ioctx = ioctx

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

// open the rbdVolume after it has been connected.
// ErrPoolNotFound or ErrImageNotFound are returned in case the pool or image
// can not be found, other errors will contain more details about other issues
// (permission denied, ...) and are expected to relate to configuration issues.
func (rv *rbdVolume) open() (*librbd.Image, error) {
	err := rv.openIoctx()
	if err != nil {
		return nil, err
	}

	image, err := librbd.OpenImage(rv.ioctx, rv.RbdImageName, librbd.NoSnapshot)
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

	// zeroBlock is the stripe-period: size of the object-size multiplied
	// by the stripe-count
	zeroBlock := make([]byte, sc*(1<<st.Order))

	// the actual size of the image as available in the pool, can be
	// marginally different from the requested image size
	_, err = image.WriteSame(offset, st.Size-offset, zeroBlock, rados.OpFlagNone)

	return err
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
		// a watcher will be added by the rbd mirror daemon if the image is primary
		defaultWatchers++
	}
	return len(watchers) != defaultWatchers, nil
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
	// Support deleting the older rbd images whose imageID is not stored in omap
	err := pOpts.getImageID()
	if err != nil {
		return err
	}

	util.DebugLog(ctx, "rbd: delete %s using mon %s, pool %s", image, pOpts.Monitors, pOpts.Pool)

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
	vol := rbdVolume{
		Pool:         rv.Pool,
		Monitors:     rv.Monitors,
		RbdImageName: rv.RbdImageName,
		conn:         rv.conn,
	}

	err := vol.openIoctx()
	if err != nil {
		return depth, err
	}

	defer func() {
		vol.ioctx.Destroy()
	}()
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
	rv := &rbdVolume{
		Monitors:     monitors,
		Pool:         pool,
		RbdImageName: rbdImageName,
	}
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

	if forceFlatten || (depth >= hardlimit) || (depth >= softlimit) {
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
	vol := rbdVolume{
		Pool:           rv.Pool,
		RadosNamespace: rv.RadosNamespace,
		Monitors:       rv.Monitors,
		RbdImageName:   rv.RbdImageName,
		conn:           rv.conn,
	}
	err := vol.openIoctx()
	if err != nil {
		return false, err
	}
	defer vol.ioctx.Destroy()

	for {
		if vol.RbdImageName == "" {
			return false, nil
		}
		err = vol.getImageInfo()
		if err != nil {
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
func genSnapFromSnapID(ctx context.Context, rbdSnap *rbdSnapshot, snapshotID string, cr *util.Credentials) error {
	var (
		options map[string]string
		vi      util.CSIIdentifier
	)
	options = make(map[string]string)

	rbdSnap.SnapID = snapshotID

	err := vi.DecomposeCSIID(rbdSnap.SnapID)
	if err != nil {
		util.ErrorLog(ctx, "error decoding snapshot ID (%s) (%s)", err, rbdSnap.SnapID)
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
	// convert the journal pool ID to name, for use in DeleteSnapshot cases
	if imageAttributes.JournalPoolID != util.InvalidPoolID {
		rbdSnap.JournalPool, err = util.GetPoolName(rbdSnap.Monitors, cr, imageAttributes.JournalPoolID)
		if err != nil {
			// TODO: If pool is not found we may leak the image (as DeleteSnapshot will return success)
			return err
		}
	}

	return err
}

// genVolFromVolID generates a rbdVolume structure from the provided identifier, updating
// the structure with elements from on-disk image metadata as well.
func genVolFromVolID(ctx context.Context, volumeID string, cr *util.Credentials, secrets map[string]string) (*rbdVolume, error) {
	var (
		options map[string]string
		vi      util.CSIIdentifier
		rbdVol  *rbdVolume
		err     error
	)
	options = make(map[string]string)

	// rbdVolume fields that are not filled up in this function are:
	//              Mounter, MultiNodeWritable
	rbdVol = &rbdVolume{VolID: volumeID}

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

	// check is there any volumeID mapping exists.
	id, err := j.CheckNewUUIDMapping(ctx, rbdVol.JournalPool, volumeID)
	if err != nil {
		return rbdVol, fmt.Errorf("failed to get volume id %s mapping %w",
			volumeID, err)
	}
	if id != "" {
		rbdVol.VolID = id
		err = vi.DecomposeCSIID(rbdVol.VolID)
		if err != nil {
			return rbdVol, fmt.Errorf("%w: error decoding volume ID (%s) (%s)",
				ErrInvalidVolID, err, rbdVol.VolID)
		}
	}
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
		rbdVol.Encrypted = true
		rbdVol.KMS, err = util.GetKMS(rbdVol.Owner, imageAttributes.KmsID, secrets)
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
	// if no image features is provided, it results in empty string
	// which disable all RBD image features as we expected

	imageFeatures, found := volOptions["imageFeatures"]
	if found {
		arr := strings.Split(imageFeatures, ",")
		for _, f := range arr {
			if !supportedFeatures.Has(f) {
				return nil, fmt.Errorf("invalid feature %q for volume csi-rbdplugin, supported"+
					" features are: %v", f, supportedFeatures)
			}
		}
		rbdVol.imageFeatureSet = librbd.FeatureSetFromNames(arr)
	}

	util.ExtendedLog(ctx, "setting disableInUseChecks on rbd volume to: %v", disableInUseChecks)
	rbdVol.DisableInUseChecks = disableInUseChecks

	rbdVol.Mounter, ok = volOptions["mounter"]
	if !ok {
		rbdVol.Mounter = rbdDefaultMounter
	}

	err = rbdVol.initKMS(ctx, volOptions, credentials)
	if err != nil {
		return nil, err
	}

	return rbdVol, nil
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
		Encrypted:      volOptions.Encrypted,
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

func (rv *rbdVolume) GetMetadata(key string) (string, error) {
	image, err := rv.open()
	if err != nil {
		return "", err
	}
	defer image.Close()

	return image.GetMetadata(key)
}

func (rv *rbdVolume) SetMetadata(key, value string) error {
	image, err := rv.open()
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
