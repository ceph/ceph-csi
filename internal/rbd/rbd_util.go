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
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/ceph/go-ceph/rados"
	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/ceph/go-ceph/rbd/admin"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cloud-provider/volume/helpers"
	mount "k8s.io/mount-utils"
)

const (
	// The following three values are used for 30 seconds timeout
	// while waiting for RBD Watcher to expire.
	rbdImageWatcherInitDelay = 1 * time.Second
	rbdImageWatcherFactor    = 1.4
	rbdImageWatcherSteps     = 10
	rbdDefaultMounter        = "rbd"
	rbdNbdMounter            = "rbd-nbd"
	defaultLogDir            = "/var/log/ceph"
	defaultLogStrategy       = "remove" // supports remove, compress and preserve

	// Output strings returned during invocation of "ceph rbd task add remove <imagespec>" when
	// command is not supported by ceph manager. Used to check errors and recover when the command
	// is unsupported.
	rbdTaskRemoveCmdInvalidString       = "No handler found"
	rbdTaskRemoveCmdAccessDeniedMessage = "access denied:"

	// migration label key and value for parameters in volume context.
	intreeMigrationKey   = "migration"
	intreeMigrationLabel = "true"
	migInTreeImagePrefix = "kubernetes-dynamic-pvc-"
	// migration volume handle identifiers.
	// total length of fields in the migration volume handle.
	migVolIDTotalLength = 4
	// split boundary length of fields.
	migVolIDSplitLength = 3
	// separator for migration handle fields.
	migVolIDFieldSep = "_"
	// identifier of a migration vol handle.
	migIdentifier = "mig"
	// prefix of image field.
	migImageNamePrefix = "image-"
	// prefix in the handle for monitors field.
	migMonPrefix = "mons-"

	// krbd attribute file to check supported features.
	krbdSupportedFeaturesFile = "/sys/bus/rbd/supported_features"

	// clusterNameKey cluster Key, set on RBD image.
	clusterNameKey = "csi.ceph.com/cluster/name"
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
	VolID    string `json:"volID"`
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
	// ParentName represents the parent image name of the image.
	ParentName string
	// Parent Pool is the pool that contains the parent image.
	ParentPool string
	// Cluster name
	ClusterName string

	// Owner is the creator (tenant, Kubernetes Namespace) of the volume
	Owner string

	// VolSize is the size of the RBD image backing this rbdImage.
	VolSize int64

	// image striping configurations.
	StripeCount uint64
	StripeUnit  uint64
	ObjectSize  uint64

	ImageFeatureSet librbd.FeatureSet

	// blockEncryption provides access to optional VolumeEncryption functions (e.g LUKS)
	blockEncryption *util.VolumeEncryption
	// fileEncryption provides access to optional VolumeEncryption functions (e.g fscrypt)
	fileEncryption *util.VolumeEncryption

	CreatedAt *timestamp.Timestamp

	// conn is a connection to the Ceph cluster obtained from a ConnPool
	conn *util.ClusterConnection
	// an opened IOContext, call .openIoctx() before using
	ioctx *rados.IOContext

	// Set metadata on volume
	EnableMetadata bool
}

// rbdVolume represents a CSI volume and its RBD image specifics.
type rbdVolume struct {
	rbdImage

	// VolName and MonValueFromSecret are retained from older plugin versions (<= 1.0.0)
	// for backward compatibility reasons
	TopologyPools       *[]util.TopologyConstrainedPool
	TopologyRequirement *csi.TopologyRequirement
	Topology            map[string]string
	// DataPool is where the data for images in `Pool` are stored, this is used as the `--data-pool`
	// argument when the pool is created, and is not used anywhere else
	DataPool           string
	AdminID            string
	UserID             string
	Mounter            string
	ReservedID         string
	MapOptions         string
	UnmapOptions       string
	LogDir             string
	LogStrategy        string
	VolName            string
	MonValueFromSecret string
	// Network namespace file path to execute nsenter command
	NetNamespaceFilePath string
	// RequestedVolSize has the size of the volume requested by the user and
	// this value will not be updated when doing getImageInfo() on rbdVolume.
	RequestedVolSize   int64
	DisableInUseChecks bool
	readOnly           bool
}

// rbdSnapshot represents a CSI snapshot and its RBD snapshot specifics.
type rbdSnapshot struct {
	rbdImage

	// SourceVolumeID is the volume ID of RbdImageName, that is exchanged with CSI drivers
	// RbdSnapName is the name of the RBD snapshot backing this rbdSnapshot
	SourceVolumeID string
	ReservedID     string
	RbdSnapName    string
}

// imageFeature represents required image features and value.
type imageFeature struct {
	// needRbdNbd indicates whether this image feature requires an rbd-nbd mounter
	needRbdNbd bool
	// dependsOn is the image features required for this imageFeature
	dependsOn []string
}

// migrationvolID is a struct which consists of required fields of a rbd volume
// from migrated volumeID.
type migrationVolID struct {
	imageName string
	poolName  string
	clusterID string
}

var (
	supportedFeatures = map[string]imageFeature{
		librbd.FeatureNameLayering: {
			needRbdNbd: false,
		},
		librbd.FeatureNameExclusiveLock: {
			needRbdNbd: false,
		},
		librbd.FeatureNameObjectMap: {
			needRbdNbd: false,
			dependsOn:  []string{librbd.FeatureNameExclusiveLock},
		},
		librbd.FeatureNameFastDiff: {
			needRbdNbd: false,
			dependsOn:  []string{librbd.FeatureNameObjectMap},
		},
		librbd.FeatureNameJournaling: {
			needRbdNbd: true,
			dependsOn:  []string{librbd.FeatureNameExclusiveLock},
		},
		librbd.FeatureNameDeepFlatten: {
			needRbdNbd: false,
		},
	}

	krbdLayeringSupport = []util.KernelVersion{
		{
			Version:    3,
			PatchLevel: 8,
			SubLevel:   0,
		},
	}
	krbdStripingV2Support = []util.KernelVersion{
		{
			Version:    3,
			PatchLevel: 10,
			SubLevel:   0,
		},
	}
	krbdExclusiveLockSupport = []util.KernelVersion{
		{
			Version:    4,
			PatchLevel: 9,
			SubLevel:   0,
		},
	}
	krbdDataPoolSupport = []util.KernelVersion{
		{
			Version:    4,
			PatchLevel: 11,
			SubLevel:   0,
		},
	}
)

// prepareKrbdFeatureAttrs prepare krbd fearure set based on kernel version.
// Minimum kernel version should be 3.8, else it will return error.
func prepareKrbdFeatureAttrs() (uint64, error) {
	// fetch the current running kernel info
	release, err := util.GetKernelVersion()
	if err != nil {
		return 0, fmt.Errorf("fetching current kernel version failed: %w", err)
	}

	switch {
	case util.CheckKernelSupport(release, krbdDataPoolSupport):
		return librbd.FeatureDataPool, nil
	case util.CheckKernelSupport(release, krbdExclusiveLockSupport):
		return librbd.FeatureExclusiveLock, nil
	case util.CheckKernelSupport(release, krbdStripingV2Support):
		return librbd.FeatureStripingV2, nil
	case util.CheckKernelSupport(release, krbdLayeringSupport):
		return librbd.FeatureLayering, nil
	}
	log.ErrorLogMsg("kernel version is too old: %q", release)

	return 0, os.ErrNotExist
}

// GetKrbdSupportedFeatures load the module if needed and return supported
// features attribute as a string.
func GetKrbdSupportedFeatures() (string, error) {
	var stderr string
	// check if the module is loaded or compiled in
	_, err := os.Stat(krbdSupportedFeaturesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.ErrorLogMsg("stat on %q failed: %v", krbdSupportedFeaturesFile, err)

			return "", err
		}
		// try to load the module
		_, stderr, err = util.ExecCommand(context.TODO(), "modprobe", rbdDefaultMounter)
		if err != nil {
			log.ErrorLogMsg("modprobe failed (%v): %q", err, stderr)

			return "", err
		}
	}
	val, err := os.ReadFile(krbdSupportedFeaturesFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.ErrorLogMsg("reading file %q failed: %v", krbdSupportedFeaturesFile, err)

			return "", err
		}
		attrs, err := prepareKrbdFeatureAttrs()
		if err != nil {
			log.ErrorLogMsg("preparing krbd feature attributes failed, %v", err)

			return "", err
		}

		return strconv.FormatUint(attrs, 16), nil
	}

	return strings.TrimSuffix(string(val), "\n"), nil
}

// HexStringToInteger convert hex value to uint.
func HexStringToInteger(hexString string) (uint, error) {
	// trim 0x prefix
	numberStr := strings.TrimPrefix(strings.ToLower(hexString), "0x")

	output, err := strconv.ParseUint(numberStr, 16, 64)
	if err != nil {
		log.ErrorLogMsg("converting string %q to integer failed: %v", numberStr, err)

		return 0, err
	}

	return uint(output), nil
}

// isKrbdFeatureSupported checks if a given Image Feature is supported by krbd
// driver or not.
func isKrbdFeatureSupported(ctx context.Context, imageFeatures string) (bool, error) {
	// return false when /sys/bus/rbd/supported_features is absent and we are
	// not in a position to prepare krbd feature attributes, i.e. if kernel <= 3.8
	if krbdFeatures == 0 {
		return false, os.ErrNotExist
	}
	arr := strings.Split(imageFeatures, ",")
	log.UsefulLog(ctx, "checking for ImageFeatures: %v", arr)
	imageFeatureSet := librbd.FeatureSetFromNames(arr)

	supported := true
	for _, featureName := range imageFeatureSet.Names() {
		if (uint(librbd.FeatureSetFromNames(strings.Split(featureName, " "))) & krbdFeatures) == 0 {
			supported = false
			log.ErrorLog(ctx, "krbd feature %q not supported", featureName)

			break
		}
	}

	return supported, nil
}

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
	if ri.isBlockEncrypted() {
		ri.blockEncryption.Destroy()
	}
	if ri.isFileEncrypted() {
		ri.fileEncryption.Destroy()
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

	log.DebugLog(ctx, "rbd: create %s size %s (features: %s) using mon %s",
		pOpts, volSzMiB, pOpts.ImageFeatureSet.Names(), pOpts.Monitors)

	options := librbd.NewRbdImageOptions()
	defer options.Destroy()

	err := pOpts.setImageOptions(ctx, options)
	if err != nil {
		return err
	}

	err = pOpts.Connect(cr)
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

	if pOpts.isBlockEncrypted() {
		err = pOpts.setupBlockEncryption(ctx)
		if err != nil {
			return fmt.Errorf("failed to setup encryption for image %s: %w", pOpts, err)
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
func (ri *rbdImage) getImageID() error {
	if ri.ImageID != "" {
		return nil
	}
	image, err := ri.open()
	if err != nil {
		return err
	}
	defer image.Close()

	id, err := image.GetId()
	if err != nil {
		return err
	}
	ri.ImageID = id

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

// isInUse checks if there is a watcher on the image. It returns true if there
// is a watcher on the image, otherwise returns false.
// In case of mirroring, the image should be primary to check watchers if the
// image is secondary it returns an error.
// isInUse is called with exponential backoff to check the image is used by
// anyone else the returned bool value is discarded if its a RWX access.
func (ri *rbdImage) isInUse() (bool, error) {
	image, err := ri.open()
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

	mirrorInfo, err := image.GetMirrorImageInfo()
	if err != nil {
		return false, err
	}

	if mirrorInfo.State == librbd.MirrorImageEnabled && !mirrorInfo.Primary {
		// Mapping secondary image can cause issues.returning error as the
		// bool value is discarded if it its RWX access.
		return false, fmt.Errorf("cannot map image %s it is not primary", ri)
	}

	// because we opened the image, there is at least one watcher
	defaultWatchers := 1
	if mirrorInfo.Primary {
		// if rbd mirror daemon is running, a watcher will be added by the rbd
		// mirror daemon for mirrored images.
		defaultWatchers++
	}

	return len(watchers) > defaultWatchers, nil
}

// checkValidImageFeatures check presence of imageFeatures parameter. It returns false when
// there imageFeatures is present and empty.
func checkValidImageFeatures(imageFeatures string, ok bool) bool {
	return !(ok && imageFeatures == "")
}

// isNotMountPoint checks whether MountPoint does not exists and
// also discards error indicating mountPoint exists.
func isNotMountPoint(mounter mount.Interface, stagingTargetPath string) (bool, error) {
	isMnt, err := mounter.IsMountPoint(stagingTargetPath)
	if os.IsNotExist(err) {
		err = nil
	}

	return !isMnt, err
}

// isCephMgrSupported determines if the cluster has support for MGR based operation
// depending on the error.
func isCephMgrSupported(ctx context.Context, clusterID string, err error) bool {
	switch {
	case err == nil:
		return true
	case strings.Contains(err.Error(), rbdTaskRemoveCmdInvalidString):
		log.WarningLog(
			ctx,
			"cluster with cluster ID (%s) does not support Ceph manager based rbd commands"+
				"(minimum ceph version required is v14.2.3)",
			clusterID)

		return false
	case strings.Contains(err.Error(), rbdTaskRemoveCmdAccessDeniedMessage):
		log.WarningLog(ctx, "access denied to Ceph MGR-based rbd commands on cluster ID (%s)", clusterID)

		return false
	}

	return true
}

// ensureImageCleanup finds image in trash and if found removes it
// from trash.
func (ri *rbdImage) ensureImageCleanup(ctx context.Context) error {
	trashInfoList, err := librbd.GetTrashList(ri.ioctx)
	if err != nil {
		log.ErrorLog(ctx, "failed to list images in trash: %v", err)

		return err
	}
	for _, val := range trashInfoList {
		if val.Name == ri.RbdImageName {
			ri.ImageID = val.Id

			return ri.trashRemoveImage(ctx)
		}
	}

	return nil
}

// deleteImage deletes a ceph image with provision and volume options.
func (ri *rbdImage) deleteImage(ctx context.Context) error {
	image := ri.RbdImageName

	log.DebugLog(ctx, "rbd: delete %s using mon %s, pool %s", image, ri.Monitors, ri.Pool)

	// Support deleting the older rbd images whose imageID is not stored in omap
	err := ri.getImageID()
	if err != nil {
		return err
	}

	if ri.isBlockEncrypted() {
		log.DebugLog(ctx, "rbd: going to remove DEK for %q (block encryption)", ri)
		if err = ri.blockEncryption.RemoveDEK(ri.VolID); err != nil {
			log.WarningLog(ctx, "failed to clean the passphrase for volume %s (block encryption): %s", ri.VolID, err)
		}
	}

	if ri.isFileEncrypted() {
		log.DebugLog(ctx, "rbd: going to remove DEK for %q (file encryption)", ri)
		if err = ri.fileEncryption.RemoveDEK(ri.VolID); err != nil {
			log.WarningLog(ctx, "failed to clean the passphrase for volume %s (file encryption): %s", ri.VolID, err)
		}
	}

	err = ri.openIoctx()
	if err != nil {
		return err
	}

	rbdImage := librbd.GetImage(ri.ioctx, image)
	err = rbdImage.Trash(0)
	if err != nil {
		log.ErrorLog(ctx, "failed to delete rbd image: %s, error: %v", ri, err)

		return err
	}

	return ri.trashRemoveImage(ctx)
}

// trashRemoveImage adds a task to trash remove an image using ceph manager if supported,
// otherwise removes the image from trash.
func (ri *rbdImage) trashRemoveImage(ctx context.Context) error {
	// attempt to use Ceph manager based deletion support if available
	log.DebugLog(ctx, "rbd: adding task to remove image %q with id %q from trash", ri, ri.ImageID)

	ta, err := ri.conn.GetTaskAdmin()
	if err != nil {
		return err
	}

	_, err = ta.AddTrashRemove(admin.NewImageSpec(ri.Pool, ri.RadosNamespace, ri.ImageID))

	rbdCephMgrSupported := isCephMgrSupported(ctx, ri.ClusterID, err)
	if rbdCephMgrSupported && err != nil {
		log.ErrorLog(ctx, "failed to add task to delete rbd image: %s, %v", ri, err)

		return err
	}

	if !rbdCephMgrSupported {
		err = librbd.TrashRemove(ri.ioctx, ri.ImageID, true)
		if err != nil {
			log.ErrorLog(ctx, "failed to delete rbd image: %s, %v", ri, err)

			return err
		}
	} else {
		log.DebugLog(ctx, "rbd: successfully added task to move image %q with id %q to trash", ri, ri.ImageID)
	}

	return nil
}

func (ri *rbdImage) getCloneDepth(ctx context.Context) (uint, error) {
	var depth uint
	vol := rbdVolume{}

	vol.Pool = ri.Pool
	vol.Monitors = ri.Monitors
	vol.RbdImageName = ri.RbdImageName
	vol.RadosNamespace = ri.RadosNamespace
	vol.conn = ri.conn.Copy()

	for {
		if vol.RbdImageName == "" {
			return depth, nil
		}
		err := vol.openIoctx()
		if err != nil {
			return depth, err
		}

		err = vol.getImageInfo()
		// FIXME: create and destroy the vol inside the loop.
		// see https://github.com/ceph/ceph-csi/pull/1838#discussion_r598530807
		vol.ioctx.Destroy()
		vol.ioctx = nil
		if err != nil {
			// if the parent image is moved to trash the name will be present
			// in rbd image info but the image will be in trash, in that case
			// return the found depth
			if errors.Is(err, ErrImageNotFound) {
				return depth, nil
			}
			log.ErrorLog(ctx, "failed to check depth on image %s: %s", &vol, err)

			return depth, err
		}
		if vol.ParentName != "" {
			depth++
		}
		vol.RbdImageName = vol.ParentName
		vol.Pool = vol.ParentPool
	}
}

type trashSnapInfo struct {
	origSnapName string
}

func flattenClonedRbdImages(
	ctx context.Context,
	snaps []librbd.SnapInfo,
	pool, monitors, rbdImageName string,
	cr *util.Credentials,
) error {
	rv := &rbdVolume{}
	rv.Monitors = monitors
	rv.Pool = pool
	rv.RbdImageName = rbdImageName

	defer rv.Destroy()
	err := rv.Connect(cr)
	if err != nil {
		log.ErrorLog(ctx, "failed to open connection %s; err %v", rv, err)

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
		err = rv.flattenRbdImage(ctx, true, rbdHardMaxCloneDepth, rbdSoftMaxCloneDepth)
		if err != nil {
			log.ErrorLog(ctx, "failed to flatten %s; err %v", rv, err)

			continue
		}
	}

	return nil
}

func (ri *rbdImage) flattenRbdImage(
	ctx context.Context,
	forceFlatten bool,
	hardlimit, softlimit uint,
) error {
	var depth uint
	var err error

	// skip clone depth check if request is for force flatten
	if !forceFlatten {
		depth, err = ri.getCloneDepth(ctx)
		if err != nil {
			return err
		}
		log.ExtendedLog(
			ctx,
			"clone depth is (%d), configured softlimit (%d) and hardlimit (%d) for %s",
			depth,
			softlimit,
			hardlimit,
			ri)
	}

	if !forceFlatten && (depth < hardlimit) && (depth < softlimit) {
		return nil
	}

	log.DebugLog(ctx, "rbd: adding task to flatten image %q", ri)

	ta, err := ri.conn.GetTaskAdmin()
	if err != nil {
		return err
	}

	_, err = ta.AddFlatten(admin.NewImageSpec(ri.Pool, ri.RadosNamespace, ri.RbdImageName))
	rbdCephMgrSupported := isCephMgrSupported(ctx, ri.ClusterID, err)
	if rbdCephMgrSupported {
		if err != nil {
			// discard flattening error if the image does not have any parent
			rbdFlattenNoParent := fmt.Sprintf("Image %s/%s does not have a parent", ri.Pool, ri.RbdImageName)
			if strings.Contains(err.Error(), rbdFlattenNoParent) {
				return nil
			}
			log.ErrorLog(ctx, "failed to add task flatten for %s : %v", ri, err)

			return err
		}
		if forceFlatten || depth >= hardlimit {
			return fmt.Errorf("%w: flatten is in progress for image %s", ErrFlattenInProgress, ri.RbdImageName)
		}
		log.DebugLog(ctx, "successfully added task to flatten image %q", ri)
	}
	if !rbdCephMgrSupported {
		log.ErrorLog(
			ctx,
			"task manager does not support flatten,image will be flattened once hardlimit is reached: %v",
			err)
		if forceFlatten || depth >= hardlimit {
			err := ri.flatten()
			if err != nil {
				log.ErrorLog(ctx, "rbd failed to flatten image %s %s: %v", ri.Pool, ri.RbdImageName, err)

				return err
			}
		}
	}

	return nil
}

func (ri *rbdImage) getParentName() (string, error) {
	rbdImage, err := ri.open()
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

func (ri *rbdImage) flatten() error {
	rbdImage, err := ri.open()
	if err != nil {
		return err
	}
	defer rbdImage.Close()

	err = rbdImage.Flatten()
	if err != nil {
		// rbd image flatten will fail if the rbd image does not have a parent
		parent, pErr := ri.getParentName()
		if pErr != nil {
			return util.JoinErrors(err, pErr)
		}
		if parent == "" {
			return nil
		}
	}

	return nil
}

func (ri *rbdImage) hasFeature(feature uint64) bool {
	return (uint64(ri.ImageFeatureSet) & feature) == feature
}

func (ri *rbdImage) checkImageChainHasFeature(ctx context.Context, feature uint64) (bool, error) {
	rbdImg := rbdImage{}

	rbdImg.Pool = ri.Pool
	rbdImg.RadosNamespace = ri.RadosNamespace
	rbdImg.Monitors = ri.Monitors
	rbdImg.RbdImageName = ri.RbdImageName
	rbdImg.conn = ri.conn.Copy()

	for {
		if rbdImg.RbdImageName == "" {
			return false, nil
		}
		err := rbdImg.openIoctx()
		if err != nil {
			return false, err
		}

		err = rbdImg.getImageInfo()
		// FIXME: create and destroy the vol inside the loop.
		// see https://github.com/ceph/ceph-csi/pull/1838#discussion_r598530807
		rbdImg.ioctx.Destroy()
		rbdImg.ioctx = nil
		if err != nil {
			// call to getImageInfo returns the parent name even if the parent
			// is in the trash, when we try to open the parent image to get its
			// information it fails because it is already in trash. We should
			// treat error as nil if the parent is not found.
			if errors.Is(err, ErrImageNotFound) {
				return false, nil
			}
			log.ErrorLog(ctx, "failed to get image info for %s: %s", rbdImg.String(), err)

			return false, err
		}
		if f := rbdImg.hasFeature(feature); f {
			return true, nil
		}
		rbdImg.RbdImageName = rbdImg.ParentName
		rbdImg.Pool = rbdImg.ParentPool
	}
}

// genSnapFromSnapID generates a rbdSnapshot structure from the provided identifier, updating
// the structure with elements from on-disk snapshot metadata as well.
func genSnapFromSnapID(
	ctx context.Context,
	rbdSnap *rbdSnapshot,
	snapshotID string,
	cr *util.Credentials,
	secrets map[string]string,
) error {
	var vi util.CSIIdentifier

	rbdSnap.VolID = snapshotID

	err := vi.DecomposeCSIID(rbdSnap.VolID)
	if err != nil {
		log.ErrorLog(ctx, "error decoding snapshot ID (%s) (%s)", err, rbdSnap.VolID)

		return err
	}

	rbdSnap.ClusterID = vi.ClusterID

	rbdSnap.Monitors, _, err = util.GetMonsAndClusterID(ctx, rbdSnap.ClusterID, false)
	if err != nil {
		log.ErrorLog(ctx, "failed getting mons (%s)", err)

		return err
	}

	rbdSnap.Pool, err = util.GetPoolName(rbdSnap.Monitors, cr, vi.LocationID)
	if err != nil {
		return err
	}
	rbdSnap.JournalPool = rbdSnap.Pool

	rbdSnap.RadosNamespace, err = util.GetRadosNamespace(util.CsiConfigFile, rbdSnap.ClusterID)
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
			rbdSnap, err)
	}

	if imageAttributes.KmsID != "" && imageAttributes.EncryptionType == util.EncryptionTypeBlock {
		err = rbdSnap.configureBlockEncryption(imageAttributes.KmsID, secrets)
		if err != nil {
			return fmt.Errorf("failed to configure block encryption for "+
				"%q: %w", rbdSnap, err)
		}
	}
	if imageAttributes.KmsID != "" && imageAttributes.EncryptionType == util.EncryptionTypeFile {
		err = rbdSnap.configureFileEncryption(imageAttributes.KmsID, secrets)
		if err != nil {
			return fmt.Errorf("failed to configure file encryption for "+
				"%q: %w", rbdSnap, err)
		}
	}

	err = updateSnapshotDetails(rbdSnap)
	if err != nil {
		return fmt.Errorf("failed to update snapshot details for %q: %w", rbdSnap, err)
	}

	return err
}

// updateSnapshotDetails will copy the details from the rbdVolume to the
// rbdSnapshot. example copying size from rbdVolume to rbdSnapshot.
func updateSnapshotDetails(rbdSnap *rbdSnapshot) error {
	vol := generateVolFromSnap(rbdSnap)
	err := vol.Connect(rbdSnap.conn.Creds)
	if err != nil {
		return err
	}
	defer vol.Destroy()

	err = vol.getImageInfo()
	if err != nil {
		return err
	}
	rbdSnap.VolSize = vol.VolSize

	return nil
}

// generateVolumeFromVolumeID generates a rbdVolume structure from the provided identifier.
func generateVolumeFromVolumeID(
	ctx context.Context,
	volumeID string,
	vi util.CSIIdentifier,
	cr *util.Credentials,
	secrets map[string]string,
) (*rbdVolume, error) {
	var (
		rbdVol *rbdVolume
		err    error
	)

	// rbdVolume fields that are not filled up in this function are:
	//              Mounter, MultiNodeWritable
	rbdVol = &rbdVolume{}
	rbdVol.VolID = volumeID

	rbdVol.ClusterID = vi.ClusterID

	rbdVol.Monitors, _, err = util.GetMonsAndClusterID(ctx, rbdVol.ClusterID, false)
	if err != nil {
		log.ErrorLog(ctx, "failed getting mons (%s)", err)

		return rbdVol, err
	}

	rbdVol.RadosNamespace, err = util.GetRadosNamespace(util.CsiConfigFile, rbdVol.ClusterID)
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

	if imageAttributes.KmsID != "" && imageAttributes.EncryptionType == util.EncryptionTypeBlock {
		err = rbdVol.configureBlockEncryption(imageAttributes.KmsID, secrets)
		if err != nil {
			return rbdVol, err
		}
	}
	if imageAttributes.KmsID != "" && imageAttributes.EncryptionType == util.EncryptionTypeFile {
		err = rbdVol.configureFileEncryption(imageAttributes.KmsID, secrets)
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

// GenVolFromVolID generates a rbdVolume structure from the provided identifier, updating
// the structure with elements from on-disk image metadata as well.
//
//nolint:golint // TODO: returning unexported rbdVolume type, use an interface instead.
func GenVolFromVolID(
	ctx context.Context,
	volumeID string,
	cr *util.Credentials,
	secrets map[string]string,
) (*rbdVolume, error) {
	var (
		vi  util.CSIIdentifier
		vol *rbdVolume
	)

	err := vi.DecomposeCSIID(volumeID)
	if err != nil {
		return vol, fmt.Errorf("%w: error decoding volume ID (%w) (%s)",
			ErrInvalidVolID, err, volumeID)
	}

	vol, err = generateVolumeFromVolumeID(ctx, volumeID, vi, cr, secrets)
	if !errors.Is(err, util.ErrKeyNotFound) && !errors.Is(err, util.ErrPoolNotFound) &&
		!errors.Is(err, ErrImageNotFound) {
		return vol, err
	}

	// Check clusterID mapping exists
	mapping, mErr := util.GetClusterMappingInfo(vi.ClusterID)
	if mErr != nil {
		return vol, mErr
	}
	if mapping != nil {
		rbdVol, vErr := generateVolumeFromMapping(ctx, mapping, volumeID, vi, cr, secrets)
		if !errors.Is(vErr, util.ErrKeyNotFound) && !errors.Is(vErr, util.ErrPoolNotFound) &&
			!errors.Is(vErr, ErrImageNotFound) {
			return rbdVol, vErr
		}
	}

	return vol, err
}

// generateVolumeFromMapping checks the clusterID and poolID mapping and
// generates retrieves the OMAP information from the poolID got from the
// mapping.
func generateVolumeFromMapping(
	ctx context.Context,
	mapping *[]util.ClusterMappingInfo,
	volumeID string,
	vi util.CSIIdentifier,
	cr *util.Credentials,
	secrets map[string]string,
) (*rbdVolume, error) {
	nvi := vi
	vol := &rbdVolume{}
	// extract clusterID mapping
	for _, cm := range *mapping {
		for key, val := range cm.ClusterIDMapping {
			mappedClusterID := util.GetMappedID(key, val, vi.ClusterID)
			if mappedClusterID == "" {
				continue
			}

			log.DebugLog(ctx,
				"found new clusterID mapping %s for existing clusterID %s",
				mappedClusterID,
				vi.ClusterID)
			// Add mapping clusterID to Identifier
			nvi.ClusterID = mappedClusterID
			poolID := fmt.Sprintf("%d", (vi.LocationID))
			for _, pools := range cm.RBDpoolIDMappingInfo {
				for key, val := range pools {
					mappedPoolID := util.GetMappedID(key, val, poolID)
					if mappedPoolID == "" {
						continue
					}
					log.DebugLog(ctx,
						"found new poolID mapping %s for existing pooID %s",
						mappedPoolID,
						poolID)
					pID, err := strconv.ParseInt(mappedPoolID, 10, 64)
					if err != nil {
						return vol, err
					}
					// Add mapping poolID to Identifier
					nvi.LocationID = pID
					vol, err = generateVolumeFromVolumeID(ctx, volumeID, nvi, cr, secrets)
					if !errors.Is(err, util.ErrKeyNotFound) && !errors.Is(err, util.ErrPoolNotFound) &&
						!errors.Is(err, ErrImageNotFound) {
						return vol, err
					}
				}
			}
		}
	}

	return vol, util.ErrPoolNotFound
}

func genVolFromVolumeOptions(
	ctx context.Context,
	volOptions map[string]string,
	disableInUseChecks, checkClusterIDMapping bool,
) (*rbdVolume, error) {
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

	clusterID, err := util.GetClusterID(volOptions)
	if err != nil {
		return nil, err
	}
	rbdVol.Monitors, rbdVol.ClusterID, err = util.GetMonsAndClusterID(ctx, clusterID, checkClusterIDMapping)
	if err != nil {
		log.ErrorLog(ctx, "failed getting mons (%s)", err)

		return nil, err
	}

	rbdVol.RadosNamespace, err = util.GetRadosNamespace(util.CsiConfigFile, rbdVol.ClusterID)
	if err != nil {
		return nil, err
	}
	if rbdVol.Mounter, ok = volOptions["mounter"]; !ok {
		rbdVol.Mounter = rbdDefaultMounter
	}
	// if no image features is provided, it results in empty string
	// which disable all RBD image features as we expected
	if err = rbdVol.validateImageFeatures(volOptions["imageFeatures"]); err != nil {
		log.ErrorLog(ctx, "failed to validate image features %v", err)

		return nil, err
	}

	log.ExtendedLog(
		ctx,
		"setting disableInUseChecks: %t image features: %v mounter: %s",
		disableInUseChecks,
		rbdVol.ImageFeatureSet.Names(),
		rbdVol.Mounter)
	rbdVol.DisableInUseChecks = disableInUseChecks

	err = rbdVol.setStripeConfiguration(volOptions)
	if err != nil {
		return nil, err
	}

	return rbdVol, nil
}

func (ri *rbdImage) setStripeConfiguration(options map[string]string) error {
	var err error
	if val, ok := options["stripeUnit"]; ok {
		ri.StripeUnit, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse stripeUnit %s: %w", val, err)
		}
	}

	if val, ok := options["stripeCount"]; ok {
		ri.StripeCount, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse stripeCount %s: %w", val, err)
		}
	}

	if val, ok := options["objectSize"]; ok {
		ri.ObjectSize, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse objectSize %s: %w", val, err)
		}
	}

	return nil
}

func (rv *rbdVolume) validateImageFeatures(imageFeatures string) error {
	// It is possible for image features to be an empty string which
	// the Go split function would return a single item array with
	// an empty string, causing a failure when trying to validate
	// the features.
	if imageFeatures == "" {
		return nil
	}
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
	rv.ImageFeatureSet = librbd.FeatureSetFromNames(arr)

	return nil
}

func genSnapFromOptions(ctx context.Context, rbdVol *rbdVolume, snapOptions map[string]string) (*rbdSnapshot, error) {
	var err error

	rbdSnap := &rbdSnapshot{}
	rbdSnap.Pool = rbdVol.Pool
	rbdSnap.JournalPool = rbdVol.JournalPool
	rbdSnap.RadosNamespace = rbdVol.RadosNamespace

	clusterID, err := util.GetClusterID(snapOptions)
	if err != nil {
		return nil, err
	}
	rbdSnap.Monitors, rbdSnap.ClusterID, err = util.GetMonsAndClusterID(ctx, clusterID, false)
	if err != nil {
		log.ErrorLog(ctx, "failed getting mons (%s)", err)

		return nil, err
	}

	if namePrefix, ok := snapOptions["snapshotNamePrefix"]; ok {
		rbdSnap.NamePrefix = namePrefix
	}

	return rbdSnap, nil
}

// hasSnapshotFeature checks if Layering is enabled for this image.
func (ri *rbdImage) hasSnapshotFeature() bool {
	return (uint64(ri.ImageFeatureSet) & librbd.FeatureLayering) == librbd.FeatureLayering
}

func (ri *rbdImage) createSnapshot(ctx context.Context, pOpts *rbdSnapshot) error {
	log.DebugLog(ctx, "rbd: snap create %s using mon %s", pOpts, pOpts.Monitors)
	image, err := ri.open()
	if err != nil {
		return err
	}
	defer image.Close()

	_, err = image.CreateSnapshot(pOpts.RbdSnapName)

	return err
}

func (ri *rbdImage) deleteSnapshot(ctx context.Context, pOpts *rbdSnapshot) error {
	log.DebugLog(ctx, "rbd: snap rm %s using mon %s", pOpts, pOpts.Monitors)
	image, err := ri.open()
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

func (rv *rbdVolume) cloneRbdImageFromSnapshot(
	ctx context.Context,
	pSnapOpts *rbdSnapshot,
	parentVol *rbdVolume,
) error {
	var err error
	log.DebugLog(ctx, "rbd: clone %s %s (features: %s) using mon %s",
		pSnapOpts, rv, rv.ImageFeatureSet.Names(), rv.Monitors)

	err = parentVol.openIoctx()
	if err != nil {
		return fmt.Errorf("failed to get parent IOContext: %w", err)
	}
	defer func() {
		defer parentVol.ioctx.Destroy()
		parentVol.ioctx = nil
	}()

	options := librbd.NewRbdImageOptions()
	defer options.Destroy()
	err = rv.setImageOptions(ctx, options)
	if err != nil {
		return err
	}

	err = options.SetUint64(librbd.ImageOptionCloneFormat, 2)
	if err != nil {
		return err
	}
	// As the clone is yet to be created, open the Ioctx.
	err = rv.openIoctx()
	if err != nil {
		return fmt.Errorf("failed to get IOContext: %w", err)
	}

	err = librbd.CloneImage(
		parentVol.ioctx,
		pSnapOpts.RbdImageName,
		pSnapOpts.RbdSnapName,
		rv.ioctx,
		rv.RbdImageName,
		options)
	if err != nil {
		return fmt.Errorf("failed to create rbd clone: %w", err)
	}

	// delete the cloned image if a next step fails
	deleteClone := true
	defer func() {
		if deleteClone {
			err = librbd.RemoveImage(rv.ioctx, rv.RbdImageName)
			if err != nil {
				log.ErrorLog(ctx, "failed to delete temporary image %q: %v", rv, err)
			}
		}
	}()

	// get image latest information
	err = rv.getImageInfo()
	if err != nil {
		return fmt.Errorf("failed to get image info of %s: %w", rv, err)
	}

	// Success! Do not delete the cloned image now :)
	deleteClone = false

	return nil
}

// setImageOptions sets the image options.
func (rv *rbdVolume) setImageOptions(ctx context.Context, options *librbd.ImageOptions) error {
	var err error

	logMsg := fmt.Sprintf("setting image options on %s", rv)
	if rv.DataPool != "" {
		logMsg += fmt.Sprintf(", data pool %s", rv.DataPool)
		err = options.SetString(librbd.RbdImageOptionDataPool, rv.DataPool)
		if err != nil {
			return fmt.Errorf("failed to set data pool: %w", err)
		}
	}

	if rv.ImageFeatureSet != 0 {
		err = options.SetUint64(librbd.RbdImageOptionFeatures, uint64(rv.ImageFeatureSet))
		if err != nil {
			return fmt.Errorf("failed to set image features: %w", err)
		}
	}

	if rv.StripeCount != 0 {
		logMsg += fmt.Sprintf(", stripe count %d, stripe unit %d", rv.StripeCount, rv.StripeUnit)
		err = options.SetUint64(librbd.RbdImageOptionStripeCount, rv.StripeCount)
		if err != nil {
			return fmt.Errorf("failed to set stripe count: %w", err)
		}
		err = options.SetUint64(librbd.RbdImageOptionStripeUnit, rv.StripeUnit)
		if err != nil {
			return fmt.Errorf("failed to set stripe unit: %w", err)
		}
	}

	if rv.ObjectSize != 0 {
		order := uint64(math.Log2(float64(rv.ObjectSize)))
		logMsg += fmt.Sprintf(", object size %d, order %d", rv.ObjectSize, order)
		err = options.SetUint64(librbd.RbdImageOptionOrder, order)
		if err != nil {
			return fmt.Errorf("failed to set object size: %w", err)
		}
	}

	log.DebugLog(ctx, logMsg)

	return nil
}

// getImageInfo queries rbd about the given image and returns its metadata, and returns
// ErrImageNotFound if provided image is not found.
func (ri *rbdImage) getImageInfo() error {
	image, err := ri.open()
	if err != nil {
		return err
	}
	defer image.Close()

	imageInfo, err := image.Stat()
	if err != nil {
		return err
	}
	// TODO: can rv.VolSize not be a uint64? Or initialize it to -1?
	ri.VolSize = int64(imageInfo.Size)

	features, err := image.GetFeatures()
	if err != nil {
		return err
	}
	ri.ImageFeatureSet = librbd.FeatureSet(features)

	// Get parent information.
	parentInfo, err := image.GetParent()
	if err != nil {
		// Caller should decide whether not finding
		// the parent is an error or not.
		if errors.Is(err, librbd.ErrNotFound) {
			ri.ParentName = ""
		} else {
			return err
		}
	} else {
		ri.ParentName = parentInfo.Image.ImageName
		ri.ParentPool = parentInfo.Image.PoolName
	}
	// Get image creation time
	tm, err := image.GetCreateTimestamp()
	if err != nil {
		return err
	}
	t := time.Unix(tm.Sec, tm.Nsec)
	ri.CreatedAt = timestamppb.New(t)

	return nil
}

// getParent returns parent image if it exists.
func (ri *rbdImage) getParent() (*rbdImage, error) {
	err := ri.getImageInfo()
	if err != nil {
		return nil, err
	}
	if ri.ParentName == "" {
		return nil, nil
	}

	parentImage := rbdImage{}
	parentImage.conn = ri.conn.Copy()
	parentImage.ClusterID = ri.ClusterID
	parentImage.Monitors = ri.Monitors
	parentImage.Pool = ri.ParentPool
	parentImage.RadosNamespace = ri.RadosNamespace
	parentImage.RbdImageName = ri.ParentName

	err = parentImage.getImageInfo()
	if err != nil {
		return nil, err
	}

	return &parentImage, nil
}

// flattenParent flatten the given image's parent if it exists according to hard and soft
// limits.
func (ri *rbdImage) flattenParent(ctx context.Context, hardLimit, softLimit uint) error {
	parentImage, err := ri.getParent()
	if err != nil {
		return err
	}

	if parentImage == nil {
		return nil
	}

	return parentImage.flattenRbdImage(ctx, false, hardLimit, softLimit)
}

/*
checkSnapExists queries rbd about the snapshots of the given image and returns
ErrImageNotFound if provided image is not found, and ErrSnapNotFound if
provided snap is not found in the images snapshot list.
*/
func (ri *rbdImage) checkSnapExists(rbdSnap *rbdSnapshot) error {
	image, err := ri.open()
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

	return fmt.Errorf("%w: snap %s not found", ErrSnapNotFound, rbdSnap)
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
	DevicePath     string `json:"device"`          // holds NBD device path for now
	LogDir         string `json:"logDir"`          // holds the client log path
	LogStrategy    string `json:"logFileStrategy"` // ceph client log strategy
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
func stashRBDImageMetadata(volOptions *rbdVolume, metaDataPath string) error {
	imgMeta := rbdImageMetadataStash{
		// there are no checks for this at present
		Version:        3, //nolint:gomnd // number specifies version.
		Pool:           volOptions.Pool,
		RadosNamespace: volOptions.RadosNamespace,
		ImageName:      volOptions.RbdImageName,
		Encrypted:      volOptions.isBlockEncrypted(),
		UnmapOptions:   volOptions.UnmapOptions,
	}

	imgMeta.NbdAccess = false
	if volOptions.Mounter == rbdTonbd && hasNBD {
		imgMeta.NbdAccess = true
		imgMeta.LogDir = volOptions.LogDir
		imgMeta.LogStrategy = volOptions.LogStrategy
	}

	encodedBytes, err := json.Marshal(imgMeta)
	if err != nil {
		return fmt.Errorf("failed to marshall JSON image metadata for image (%s): %w", volOptions, err)
	}

	fPath := filepath.Join(metaDataPath, stashFileName)
	err = os.WriteFile(fPath, encodedBytes, 0o600)
	if err != nil {
		return fmt.Errorf("failed to stash JSON image metadata for image (%s) at path (%s): %w", volOptions, fPath, err)
	}

	return nil
}

// checkRBDImageMetadataStashExists checks if the stashFile exists at the passed in path.
func checkRBDImageMetadataStashExists(metaDataPath string) bool {
	imageMetaPath := filepath.Join(metaDataPath, stashFileName)
	_, err := os.Stat(imageMetaPath)

	return err == nil
}

// lookupRBDImageMetadataStash reads and returns stashed image metadata at passed in path.
func lookupRBDImageMetadataStash(metaDataPath string) (rbdImageMetadataStash, error) {
	var imgMeta rbdImageMetadataStash

	fPath := filepath.Join(metaDataPath, stashFileName)
	encodedBytes, err := os.ReadFile(fPath) // #nosec - intended reading from fPath
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

// updateRBDImageMetadataStash reads and updates stashFile with the required
// fields at the passed in path, in JSON format.
func updateRBDImageMetadataStash(metaDataPath, device string) error {
	if device == "" {
		return errors.New("device is empty")
	}
	imgMeta, err := lookupRBDImageMetadataStash(metaDataPath)
	if err != nil {
		return fmt.Errorf("failed to find image metadata: %w", err)
	}
	imgMeta.DevicePath = device

	encodedBytes, err := json.Marshal(imgMeta)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON image metadata for spec:(%s) : %w", imgMeta.String(), err)
	}

	fPath := filepath.Join(metaDataPath, stashFileName)
	err = os.WriteFile(fPath, encodedBytes, 0o600)
	if err != nil {
		return fmt.Errorf("failed to stash JSON image metadata at path: (%s) for spec:(%s) : %w",
			fPath, imgMeta.String(), err)
	}

	return nil
}

// cleanupRBDImageMetadataStash cleans up any stashed metadata at passed in path.
func cleanupRBDImageMetadataStash(metaDataPath string) error {
	fPath := filepath.Join(metaDataPath, stashFileName)
	if err := os.Remove(fPath); err != nil {
		return fmt.Errorf("failed to cleanup stashed JSON data (%s): %w", fPath, err)
	}

	return nil
}

// expand checks if the requestedVolume size and the existing image size both
// are same. If they are same, it returns nil else it resizes the image.
func (rv *rbdVolume) expand() error {
	if rv.RequestedVolSize == rv.VolSize {
		return nil
	}

	return rv.resize(rv.RequestedVolSize)
}

// resize the given volume to new size.
// updates Volsize of rbdVolume object to newSize in case of success.
func (ri *rbdImage) resize(newSize int64) error {
	image, err := ri.open()
	if err != nil {
		return err
	}
	defer image.Close()

	err = image.Resize(uint64(util.RoundOffVolSize(newSize) * helpers.MiB))
	if err != nil {
		return err
	}
	// update Volsize of rbdVolume object to newSize.
	ri.VolSize = newSize

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

// RemoveMetadata deletes the key and data from the metadata of the image.
func (ri *rbdImage) RemoveMetadata(key string) error {
	image, err := ri.open()
	if err != nil {
		return err
	}
	defer image.Close()

	return image.RemoveMetadata(key)
}

// MigrateMetadata reads the metadata contents from oldKey and stores it in
// newKey. In case oldKey was not set, the defaultValue is stored in newKey.
// Once done, oldKey will be removed as well.
func (ri *rbdImage) MigrateMetadata(oldKey, newKey, defaultValue string) (string, error) {
	value, err := ri.GetMetadata(newKey)
	if err == nil {
		return value, nil
	} else if !errors.Is(err, librbd.ErrNotFound) {
		return "", err
	}

	// migrate contents from oldKey to newKey
	removeOldKey := true
	value, err = ri.GetMetadata(oldKey)
	if errors.Is(err, librbd.ErrNotFound) {
		// in case oldKey was not set, set newKey to defaultValue
		value = defaultValue
		removeOldKey = false
	} else if err != nil {
		return "", err
	}

	// newKey was not set, set it now to prevent regular error cases for missing metadata
	err = ri.SetMetadata(newKey, value)
	if err != nil {
		return "", err
	}

	// the newKey was set with data from oldKey, oldKey is not needed anymore
	if removeOldKey {
		err = ri.RemoveMetadata(oldKey)
		if err != nil {
			return "", err
		}
	}

	return value, nil
}

// DeepCopy creates an independent image (dest) from the source image. This
// process may take some time when the image is large.
func (ri *rbdImage) DeepCopy(dest *rbdImage) error {
	opts := librbd.NewRbdImageOptions()
	defer opts.Destroy()

	// when doing DeepCopy, also flatten the new image
	err := opts.SetUint64(librbd.ImageOptionFlatten, 1)
	if err != nil {
		return err
	}

	err = dest.openIoctx()
	if err != nil {
		return err
	}

	image, err := ri.open()
	if err != nil {
		return err
	}
	defer image.Close()

	err = image.DeepCopy(dest.ioctx, dest.RbdImageName, opts)
	if err != nil {
		return err
	}

	// deep-flatten is not supported by all clients, so disable it
	return dest.DisableDeepFlatten()
}

// DisableDeepFlatten removed the deep-flatten feature from the image.
func (ri *rbdImage) DisableDeepFlatten() error {
	image, err := ri.open()
	if err != nil {
		return err
	}
	defer image.Close()

	return image.UpdateFeatures(librbd.FeatureDeepFlatten, false)
}

func (ri *rbdImage) listSnapshots() ([]librbd.SnapInfo, error) {
	image, err := ri.open()
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
func (ri *rbdImage) isTrashSnap(snapID uint64) (bool, error) {
	image, err := ri.open()
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
func (ri *rbdImage) getOrigSnapName(snapID uint64) (string, error) {
	image, err := ri.open()
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

func (ri *rbdImage) isCompatibleEncryption(dst *rbdImage) error {
	riEncrypted := ri.isBlockEncrypted() || ri.isFileEncrypted()
	dstEncrypted := dst.isBlockEncrypted() || dst.isFileEncrypted()
	switch {
	case riEncrypted && !dstEncrypted:
		return fmt.Errorf("cannot create unencrypted volume from encrypted volume %q", ri)

	case !riEncrypted && dstEncrypted:
		return fmt.Errorf("cannot create encrypted volume from unencrypted volume %q", ri)
	}

	return nil
}

func (ri *rbdImage) isCompabitableClone(dst *rbdImage) error {
	if dst.VolSize < ri.VolSize {
		return fmt.Errorf(
			"volume size %d is smaller than source volume size %d",
			dst.VolSize,
			ri.VolSize)
	}

	return nil
}

func (ri *rbdImage) AddSnapshotScheduling(
	interval admin.Interval,
	startTime admin.StartTime,
) error {
	ls := admin.NewLevelSpec(ri.Pool, ri.RadosNamespace, ri.RbdImageName)
	ra, err := ri.conn.GetRBDAdmin()
	if err != nil {
		return err
	}
	adminConn := ra.MirrorSnashotSchedule()
	err = adminConn.Add(ls, interval, startTime)
	if err != nil {
		return err
	}

	return nil
}

// getCephClientLogFileName compiles the complete log file path based on inputs.
func getCephClientLogFileName(id, logDir, prefix string) string {
	if prefix == "" {
		prefix = "ceph"
	}

	if logDir == "" {
		logDir = defaultLogDir
	}

	return fmt.Sprintf("%s/%s-%s.log", logDir, prefix, id)
}

// CheckSliceContains checks the slice for string.
func CheckSliceContains(options []string, opt string) bool {
	for _, o := range options {
		if o == opt {
			return true
		}
	}

	return false
}

// strategicActionOnLogFile act on log file based on cephLogStrategy.
func strategicActionOnLogFile(ctx context.Context, logStrategy, logFile string) {
	var err error

	switch strings.ToLower(logStrategy) {
	case "compress":
		if err = log.GzipLogFile(logFile); err != nil {
			log.ErrorLog(ctx, "failed to compress logfile %q: %v", logFile, err)
		}
	case "remove":
		if err = os.Remove(logFile); err != nil {
			log.ErrorLog(ctx, "failed to remove logfile %q: %v", logFile, err)
		}
	case "preserve":
		// do nothing
	default:
		log.ErrorLog(ctx, "unknown cephLogStrategy option %q: hint: 'remove'|'compress'|'preserve'", logStrategy)
	}
}

// genVolFromVolIDWithMigration populate a rbdVol structure based on the volID format.
func genVolFromVolIDWithMigration(
	ctx context.Context, volID string, cr *util.Credentials, secrets map[string]string,
) (*rbdVolume, error) {
	if isMigrationVolID(volID) {
		pmVolID, pErr := parseMigrationVolID(volID)
		if pErr != nil {
			return nil, pErr
		}

		return genVolFromMigVolID(ctx, pmVolID, cr)
	}
	rv, err := GenVolFromVolID(ctx, volID, cr, secrets)
	if err != nil {
		rv.Destroy()
	}

	return rv, err
}

// setAllMetadata set all the metadata from arg parameters on RBD image.
func (rv *rbdVolume) setAllMetadata(parameters map[string]string) error {
	if !rv.EnableMetadata {
		return nil
	}

	for k, v := range parameters {
		err := rv.SetMetadata(k, v)
		if err != nil {
			return fmt.Errorf("failed to set metadata key %q, value %q on image: %w", k, v, err)
		}
	}

	if rv.ClusterName != "" {
		err := rv.SetMetadata(clusterNameKey, rv.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to set metadata key %q, value %q on image: %w",
				clusterNameKey, rv.ClusterName, err)
		}
	}

	return nil
}

// unsetAllMetadata unset all the metadata from arg keys on RBD image.
func (rv *rbdVolume) unsetAllMetadata(keys []string) error {
	for _, key := range keys {
		err := rv.RemoveMetadata(key)
		// TODO: replace string comparison with errno.
		if err != nil && !strings.Contains(err.Error(), "No such file or directory") {
			return fmt.Errorf("failed to unset metadata key %q on %q: %w", key, rv, err)
		}
	}

	err := rv.RemoveMetadata(clusterNameKey)
	// TODO: replace string comparison with errno.
	if err != nil && !strings.Contains(err.Error(), "No such file or directory") {
		return fmt.Errorf("failed to unset metadata key %q on %q: %w", clusterNameKey, rv, err)
	}

	return nil
}
