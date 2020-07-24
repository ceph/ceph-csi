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
	klog "k8s.io/klog/v2"
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

	// Encryption statuses for RbdImage
	rbdImageEncrypted          = "encrypted"
	rbdImageRequiresEncryption = "requiresEncryption"
	// image metadata key for encryption
	encryptionMetaKey = ".rbd.csi.ceph.com/encrypted"
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
	ImageID             string
	ParentName          string
	imageFeatureSet     librbd.FeatureSet
	AdminID             string `json:"adminId"`
	UserID              string `json:"userId"`
	Mounter             string `json:"mounter"`
	ClusterID           string `json:"clusterId"`
	RequestName         string
	ReservedID          string
	VolName             string `json:"volName"`
	MonValueFromSecret  string `json:"monValueFromSecret"`
	VolSize             int64  `json:"volSize"`
	DisableInUseChecks  bool   `json:"disableInUseChecks"`
	Encrypted           bool
	readOnly            bool
	KMS                 util.EncryptionKMS
	CreatedAt           *timestamp.Timestamp
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
}

// String returns the image-spec (pool/image) format of the image.
func (rv *rbdVolume) String() string {
	return fmt.Sprintf("%s/%s", rv.Pool, rv.RbdImageName)
}

// String returns the snap-spec (pool/image@snap) format of the snapshot.
func (rs *rbdSnapshot) String() string {
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

	// because we opened the image, there is at least one watcher
	return len(watchers) != 1, nil
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
			klog.Warningf(util.Log(ctx, "cluster with cluster ID (%s) does not support Ceph manager based rbd commands (minimum ceph version required is v14.2.3)"), pOpts.ClusterID)
			supported = false
		case strings.HasPrefix(stderr, rbdTaskRemoveCmdAccessDeniedMessage):
			klog.Warningf(util.Log(ctx, "access denied to Ceph MGR-based rbd commands on cluster ID (%s)"), pOpts.ClusterID)
			supported = false
		default:
			klog.Warningf(util.Log(ctx, "uncaught error while scheduling a task: %s"), err)
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
		klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s, error: %v"), pOpts, err)
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
		klog.Errorf(util.Log(ctx, "failed to add task to delete rbd image: %s, %v"), pOpts, err)
		return err
	}

	if !rbdCephMgrSupported {
		err = librbd.TrashRemove(pOpts.ioctx, pOpts.ImageID, true)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s, %v"), pOpts, err)
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
			klog.Errorf(util.Log(ctx, "failed to check depth on image %s: %s"), vol, err)
			return depth, err
		}
		if vol.ParentName != "" {
			depth++
		}
		vol.RbdImageName = vol.ParentName
	}
}

func flattenClonedRbdImages(ctx context.Context, snaps []snapshotInfo, pool, monitors string, cr *util.Credentials) error {
	rv := &rbdVolume{
		Monitors: monitors,
		Pool:     pool,
	}
	defer rv.Destroy()
	err := rv.Connect(cr)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to open connection %s; err %v"), rv, err)
		return err
	}
	for _, s := range snaps {
		if s.Namespace.Type == "trash" {
			rv.RbdImageName = s.Namespace.OriginalName
			err = rv.flattenRbdImage(ctx, cr, true, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
			if err != nil {
				klog.Errorf(util.Log(ctx, "failed to flatten %s; err %v"), rv, err)
				continue
			}
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
		klog.Infof(util.Log(ctx, "clone depth is (%d), configured softlimit (%d) and hardlimit (%d) for %s"), depth, softlimit, hardlimit, rv)
	}

	if forceFlatten || (depth >= hardlimit) || (depth >= softlimit) {
		args := []string{"flatten", rv.Pool + "/" + rv.RbdImageName, "--id", cr.ID, "--keyfile=" + cr.KeyFile, "-m", rv.Monitors}
		supported, err := addRbdManagerTask(ctx, rv, args)
		if supported {
			if err != nil {
				klog.Errorf(util.Log(ctx, "failed to add task flatten for %s : %v"), rv, err)
				return err
			}
			if forceFlatten || depth >= hardlimit {
				return fmt.Errorf("%w: flatten is in progress for image %s", ErrFlattenInProgress, rv.RbdImageName)
			}
		}
		if !supported {
			klog.Errorf(util.Log(ctx, "task manager does not support flatten,image will be flattened once hardlimit is reached: %v"), err)
			if forceFlatten || depth >= hardlimit {
				err = rv.Connect(cr)
				if err != nil {
					return err
				}
				rbdImage, err := rv.open()
				if err != nil {
					return err
				}
				defer rbdImage.Close()
				if err = rbdImage.Flatten(); err != nil {
					klog.Errorf(util.Log(ctx, "rbd failed to flatten image %s %s: %v"), rv.Pool, rv.RbdImageName, err)
					return err
				}
			}
		}
	}
	return nil
}

func (rv *rbdVolume) hasFeature(feature uint64) bool {
	return (uint64(rv.imageFeatureSet) & feature) == feature
}

func (rv *rbdVolume) checkImageChainHasFeature(ctx context.Context, feature uint64) (bool, error) {
	vol := rbdVolume{
		Pool:         rv.Pool,
		Monitors:     rv.Monitors,
		RbdImageName: rv.RbdImageName,
		conn:         rv.conn,
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
			klog.Errorf(util.Log(ctx, "failed to get image info for %s: %s"), vol, err)
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
		klog.Errorf(util.Log(ctx, "error decoding snapshot ID (%s) (%s)"), err, rbdSnap.SnapID)
		return err
	}

	rbdSnap.ClusterID = vi.ClusterID
	options["clusterID"] = rbdSnap.ClusterID

	rbdSnap.Monitors, _, err = getMonsAndClusterID(ctx, options)
	if err != nil {
		return err
	}

	rbdSnap.Pool, err = util.GetPoolName(rbdSnap.Monitors, cr, vi.LocationID)
	if err != nil {
		return err
	}
	rbdSnap.JournalPool = rbdSnap.Pool

	j, err := snapJournal.Connect(rbdSnap.Monitors, cr)
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
	)
	options = make(map[string]string)

	// rbdVolume fields that are not filled up in this function are:
	//              Mounter, MultiNodeWritable
	rbdVol = &rbdVolume{VolID: volumeID}

	err := vi.DecomposeCSIID(rbdVol.VolID)
	if err != nil {
		return rbdVol, fmt.Errorf("%w: error decoding volume ID (%s) (%s)",
			ErrInvalidVolID, err, rbdVol.VolID)
	}

	rbdVol.ClusterID = vi.ClusterID
	options["clusterID"] = rbdVol.ClusterID

	rbdVol.Monitors, _, err = getMonsAndClusterID(ctx, options)
	if err != nil {
		return rbdVol, err
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

	j, err := volJournal.Connect(rbdVol.Monitors, cr)
	if err != nil {
		return rbdVol, err
	}
	defer j.Destroy()

	imageAttributes, err := j.GetImageAttributes(
		ctx, rbdVol.Pool, vi.ObjectUUID, false)
	if err != nil {
		return rbdVol, err
	}

	rbdVol.RequestName = imageAttributes.RequestName
	rbdVol.RbdImageName = imageAttributes.ImageName
	rbdVol.ReservedID = vi.ObjectUUID
	rbdVol.ImageID = imageAttributes.ImageID

	if imageAttributes.KmsID != "" {
		rbdVol.Encrypted = true
		rbdVol.KMS, err = util.GetKMS(imageAttributes.KmsID, secrets)
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
		err = rbdVol.getImageID()
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to get image id %s: %v"), rbdVol, err)
			return rbdVol, err
		}
		err = j.StoreImageID(ctx, rbdVol.JournalPool, rbdVol.ReservedID, rbdVol.ImageID, cr)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to store volume id %s: %v"), rbdVol, err)
			return rbdVol, err
		}
	}
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to get stored image id: %v"), err)
		return rbdVol, err
	}

	err = rbdVol.getImageInfo()
	return rbdVol, err
}

func getMonsAndClusterID(ctx context.Context, options map[string]string) (monitors, clusterID string, err error) {
	var ok bool

	if clusterID, ok = options["clusterID"]; !ok {
		err = errors.New("clusterID must be set")
		return
	}

	if monitors, err = util.Mons(csiConfigFile, clusterID); err != nil {
		klog.Errorf(util.Log(ctx, "failed getting mons (%s)"), err)
		err = fmt.Errorf("failed to fetch monitor list using clusterID (%s): %w", clusterID, err)
		return
	}

	return
}

func genVolFromVolumeOptions(ctx context.Context, volOptions, credentials map[string]string, disableInUseChecks bool) (*rbdVolume, error) {
	var (
		ok         bool
		err        error
		namePrefix string
		encrypted  string
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

	rbdVol.Monitors, rbdVol.ClusterID, err = getMonsAndClusterID(ctx, volOptions)
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

	rbdVol.Encrypted = false
	encrypted, ok = volOptions["encrypted"]
	if ok {
		rbdVol.Encrypted, err = strconv.ParseBool(encrypted)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid value set in 'encrypted': %s (should be \"true\" or \"false\")", encrypted)
		}

		if rbdVol.Encrypted {
			// deliberately ignore if parsing failed as GetKMS will return default
			// implementation of kmsID is empty
			kmsID := volOptions["encryptionKMSID"]
			rbdVol.KMS, err = util.GetKMS(kmsID, credentials)
			if err != nil {
				return nil, fmt.Errorf("invalid encryption kms configuration: %s", err)
			}
		}
	}

	return rbdVol, nil
}

func genSnapFromOptions(ctx context.Context, rbdVol *rbdVolume, snapOptions map[string]string) *rbdSnapshot {
	var err error

	rbdSnap := &rbdSnapshot{}
	rbdSnap.Pool = rbdVol.Pool
	rbdSnap.JournalPool = rbdVol.JournalPool

	rbdSnap.Monitors, rbdSnap.ClusterID, err = getMonsAndClusterID(ctx, snapOptions)
	if err != nil {
		rbdSnap.Monitors = rbdVol.Monitors
		rbdSnap.ClusterID = rbdVol.ClusterID
	}

	if namePrefix, ok := snapOptions["snapshotNamePrefix"]; ok {
		rbdSnap.NamePrefix = namePrefix
	}

	return rbdSnap
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

// imageInfo strongly typed JSON spec for image info.
type imageInfo struct {
	ObjectUUID string     `json:"name"`
	Size       int64      `json:"size"`
	Features   []string   `json:"features"`
	CreatedAt  string     `json:"create_timestamp"`
	Parent     parentInfo `json:"parent"`
}

// parentInfo  spec for parent volume  info.
type parentInfo struct {
	Image    string `json:"image"`
	Pool     string `json:"pool"`
	Snapshot string `json:"snapshost"`
}

// updateVolWithImageInfo updates provided rbdVolume with information from on-disk data
// regarding the same.
func (rv *rbdVolume) updateVolWithImageInfo(cr *util.Credentials) error {
	// rbd --format=json info [image-spec | snap-spec]
	var imgInfo imageInfo

	stdout, stderr, err := util.ExecCommand(
		context.TODO(),
		"rbd",
		"-m", rv.Monitors,
		"--id", cr.ID,
		"--keyfile="+cr.KeyFile,
		"-c", util.CephConfigPath,
		"--format="+"json",
		"info", rv.String())
	if err != nil {
		klog.Errorf("failed getting information for image (%s): (%s)", rv, err)
		if strings.Contains(stderr, "rbd: error opening image "+rv.RbdImageName+
			": (2) No such file or directory") {
			return util.JoinErrors(ErrImageNotFound, err)
		}
		return err
	}

	err = json.Unmarshal([]byte(stdout), &imgInfo)
	if err != nil {
		klog.Errorf("failed to parse JSON output of image info (%s): (%s)", rv, err)
		return fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, stdout)
	}

	rv.VolSize = imgInfo.Size
	rv.ParentName = imgInfo.Parent.Image

	tm, err := time.Parse(time.ANSIC, imgInfo.CreatedAt)
	if err != nil {
		return err
	}

	rv.CreatedAt, err = ptypes.TimestampProto(tm)
	return err
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
	err = rv.updateVolWithImageInfo(rv.conn.Creds)
	if err != nil {
		return err
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
	Version   int    `json:"Version"`
	Pool      string `json:"pool"`
	ImageName string `json:"image"`
	NbdAccess bool   `json:"accessType"`
	Encrypted bool   `json:"encrypted"`
}

// file name in which image metadata is stashed.
const stashFileName = "image-meta.json"

// spec returns the image-spec (pool/image) format of the image.
func (ri *rbdImageMetadataStash) String() string {
	return fmt.Sprintf("%s/%s", ri.Pool, ri.ImageName)
}

// stashRBDImageMetadata stashes required fields into the stashFileName at the passed in path, in
// JSON format.
func stashRBDImageMetadata(volOptions *rbdVolume, path string) error {
	var imgMeta = rbdImageMetadataStash{
		// there are no checks for this at present
		Version:   2, // nolint:gomnd // number specifies version.
		Pool:      volOptions.Pool,
		ImageName: volOptions.RbdImageName,
		Encrypted: volOptions.Encrypted,
	}

	imgMeta.NbdAccess = false
	if volOptions.Mounter == rbdTonbd && hasNBD {
		imgMeta.NbdAccess = true
	}

	encodedBytes, err := json.Marshal(imgMeta)
	if err != nil {
		return fmt.Errorf("failed to marshall JSON image metadata for image (%s): (%v)", volOptions, err)
	}

	fPath := filepath.Join(path, stashFileName)
	err = ioutil.WriteFile(fPath, encodedBytes, 0600)
	if err != nil {
		return fmt.Errorf("failed to stash JSON image metadata for image (%s) at path (%s): (%v)", volOptions, fPath, err)
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
			return imgMeta, fmt.Errorf("failed to read stashed JSON image metadata from path (%s): (%v)", fPath, err)
		}

		return imgMeta, util.JoinErrors(ErrMissingStash, err)
	}

	err = json.Unmarshal(encodedBytes, &imgMeta)
	if err != nil {
		return imgMeta, fmt.Errorf("failed to unmarshall stashed JSON image metadata from path (%s): (%v)", fPath, err)
	}

	return imgMeta, nil
}

// cleanupRBDImageMetadataStash cleans up any stashed metadata at passed in path.
func cleanupRBDImageMetadataStash(path string) error {
	fPath := filepath.Join(path, stashFileName)
	if err := os.Remove(fPath); err != nil {
		return fmt.Errorf("failed to cleanup stashed JSON data (%s): (%v)", fPath, err)
	}

	return nil
}

// resize the given volume to new size.
func (rv *rbdVolume) resize(ctx context.Context, cr *util.Credentials) error {
	mon := rv.Monitors
	volSzMiB := fmt.Sprintf("%dM", util.RoundOffVolSize(rv.VolSize))

	args := []string{"resize", rv.String(), "--size", volSzMiB, "--id", cr.ID, "-m", mon, "--keyfile=" + cr.KeyFile}
	_, stderr, err := util.ExecCommand(ctx, "rbd", args...)

	if err != nil {
		return fmt.Errorf("failed to resize rbd image (%w), command output: %s", err, stderr)
	}

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

// checkRbdImageEncrypted verifies if rbd image was encrypted when created.
func (rv *rbdVolume) checkRbdImageEncrypted(ctx context.Context) (string, error) {
	value, err := rv.GetMetadata(encryptionMetaKey)
	if err != nil {
		klog.Errorf(util.Log(ctx, "checking image %s encrypted state metadata failed: %s"), rv, err)
		return "", err
	}

	encrypted := strings.TrimSpace(value)
	util.DebugLog(ctx, "image %s encrypted state metadata reports %q", rv, encrypted)
	return encrypted, nil
}

func (rv *rbdVolume) ensureEncryptionMetadataSet(status string) error {
	err := rv.SetMetadata(encryptionMetaKey, status)
	if err != nil {
		return fmt.Errorf("failed to save encryption status for %s: %v", rv, err)
	}

	return nil
}

// SnapshotInfo holds snapshots details.
type snapshotInfo struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Protected string `json:"protected"`
	Timestamp string `json:"timestamp"`
	Namespace struct {
		Type         string `json:"type"`
		OriginalName string `json:"original_name"`
	} `json:"namespace"`
}

// TODO: use go-ceph once https://github.com/ceph/go-ceph/issues/300 is available in a release.
func (rv *rbdVolume) listSnapshots(ctx context.Context, cr *util.Credentials) ([]snapshotInfo, error) {
	// rbd snap ls <image> --pool=<pool-name> --all --format=json
	var snapInfo []snapshotInfo
	stdout, stderr, err := util.ExecCommand(
		ctx,
		"rbd",
		"-m", rv.Monitors,
		"--id", cr.ID,
		"--keyfile="+cr.KeyFile,
		"-c", util.CephConfigPath,
		"--format="+"json",
		"snap",
		"ls",
		"--all", rv.String())
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed getting information for image (%s): (%s)"), rv, err)
		if strings.Contains(stderr, "rbd: error opening image "+rv.RbdImageName+
			": (2) No such file or directory") {
			return snapInfo, util.JoinErrors(ErrImageNotFound, err)
		}
		return snapInfo, err
	}

	err = json.Unmarshal([]byte(stdout), &snapInfo)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to parse JSON output of snapshot info (%s)"), err)
		return snapInfo, fmt.Errorf("unmarshal failed: %w. raw buffer response: %s", err, stdout)
	}
	return snapInfo, nil
}
