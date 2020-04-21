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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/ceph/go-ceph/rados"
	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cloud-provider/volume/helpers"
	"k8s.io/klog"
)

const (
	imageWatcherStr = "watcher="
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
)

// rbdVolume represents a CSI volume and its RBD image specifics
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
	ImageFeatures       string `json:"imageFeatures"`
	AdminID             string `json:"adminId"`
	UserID              string `json:"userId"`
	Mounter             string `json:"mounter"`
	ClusterID           string `json:"clusterId"`
	RequestName         string
	VolName             string `json:"volName"`
	MonValueFromSecret  string `json:"monValueFromSecret"`
	VolSize             int64  `json:"volSize"`
	DisableInUseChecks  bool   `json:"disableInUseChecks"`
	Encrypted           bool
	KMS                 util.EncryptionKMS

	// connection
	conn *rados.Conn
}

// rbdSnapshot represents a CSI snapshot and its RBD snapshot specifics
type rbdSnapshot struct {
	// SourceVolumeID is the volume ID of RbdImageName, that is exchanged with CSI drivers
	// RbdImageName is the name of the RBD image, that is this rbdSnapshot's source image
	// RbdSnapName is the name of the RBD snapshot backing this rbdSnapshot
	// SnapID is the snapshot ID that is exchanged with CSI drivers, identifying this rbdSnapshot
	// RequestName is the CSI generated snapshot name for the rbdSnapshot
	// JournalPool is the ceph pool in which the CSI snapshot Journal is stored
	// Pool is where the image snapshot journal and snapshot is stored, and could be the same as `JournalPool`
	SourceVolumeID string
	RbdImageName   string
	NamePrefix     string
	RbdSnapName    string
	SnapID         string
	Monitors       string
	JournalPool    string
	Pool           string
	CreatedAt      *timestamp.Timestamp
	SizeBytes      int64
	ClusterID      string
	RequestName    string
}

var (
	supportedFeatures = sets.NewString("layering")

	// large interval and timeout, it should be longer than the maximum
	// time an operation can take (until refcounting of the connections is
	// available)
	cpInterval = 15 * time.Minute
	cpExpiry   = 10 * time.Minute
	connPool   = util.NewConnPool(cpInterval, cpExpiry)
)

// createImage creates a new ceph image with provision and volume options.
func createImage(ctx context.Context, pOpts *rbdVolume, cr *util.Credentials) error {
	volSzMiB := fmt.Sprintf("%dM", util.RoundOffVolSize(pOpts.VolSize))
	options := librbd.NewRbdImageOptions()

	logMsg := "rbd: create %s size %s (features: %s) using mon %s, pool %s "
	if pOpts.DataPool != "" {
		logMsg += fmt.Sprintf("data pool %s", pOpts.DataPool)
		err := options.SetString(librbd.RbdImageOptionDataPool, pOpts.DataPool)
		if err != nil {
			return errors.Wrapf(err, "failed to set data pool")
		}
	}
	klog.V(4).Infof(util.Log(ctx, logMsg),
		pOpts.RbdImageName, volSzMiB, pOpts.ImageFeatures, pOpts.Monitors, pOpts.Pool)

	if pOpts.ImageFeatures != "" {
		features := imageFeaturesToUint64(ctx, pOpts.ImageFeatures)
		err := options.SetUint64(librbd.RbdImageOptionFeatures, features)
		if err != nil {
			return errors.Wrapf(err, "failed to set image features")
		}
	}

	ioctx, err := pOpts.getIoctx(cr)
	if err != nil {
		return errors.Wrapf(err, "failed to get IOContext")
	}
	defer ioctx.Destroy()

	err = librbd.CreateImage(ioctx, pOpts.RbdImageName,
		uint64(util.RoundOffVolSize(pOpts.VolSize)*helpers.MiB), options)
	if err != nil {
		return errors.Wrapf(err, "failed to create rbd image")
	}

	return nil
}

func (rv *rbdVolume) getIoctx(cr *util.Credentials) (*rados.IOContext, error) {
	if rv.conn == nil {
		conn, err := connPool.Get(rv.Pool, rv.Monitors, cr.ID, cr.KeyFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get connection")
		}

		rv.conn = conn
	}

	ioctx, err := rv.conn.OpenIOContext(rv.Pool)
	if err != nil {
		connPool.Put(rv.conn)
		return nil, errors.Wrapf(err, "failed to open IOContext for pool %s", rv.Pool)
	}

	return ioctx, nil
}

func (rv *rbdVolume) Destroy() {
	if rv.conn != nil {
		connPool.Put(rv.conn)
	}
}

// rbdStatus checks if there is watcher on the image.
// It returns true if there is a watcher on the image, otherwise returns false.
func rbdStatus(ctx context.Context, pOpts *rbdVolume, cr *util.Credentials) (bool, string, error) {
	var output string
	var cmd []byte

	image := pOpts.RbdImageName

	klog.V(4).Infof(util.Log(ctx, "rbd: status %s using mon %s, pool %s"), image, pOpts.Monitors, pOpts.Pool)
	args := []string{"status", image, "--pool", pOpts.Pool, "-m", pOpts.Monitors, "--id", cr.ID, "--keyfile=" + cr.KeyFile}
	cmd, err := execCommand("rbd", args)
	output = string(cmd)

	if err, ok := err.(*exec.Error); ok {
		if err.Err == exec.ErrNotFound {
			klog.Errorf(util.Log(ctx, "rbd cmd not found"))
			// fail fast if command not found
			return false, output, err
		}
	}

	// If command never succeed, returns its last error.
	if err != nil {
		return false, output, err
	}

	if strings.Contains(output, imageWatcherStr) {
		klog.V(4).Infof(util.Log(ctx, "rbd: watchers on %s: %s"), image, output)
		return true, output, nil
	}
	klog.Warningf(util.Log(ctx, "rbd: no watchers on %s"), image)
	return false, output, nil
}

// rbdManagerTaskDelete adds a ceph manager task to delete an rbd image, thus deleting
// it asynchronously. If command is not found returns a bool set to false
func rbdManagerTaskDeleteImage(ctx context.Context, pOpts *rbdVolume, cr *util.Credentials) (bool, error) {
	var output []byte

	args := []string{"rbd", "task", "add", "remove",
		pOpts.Pool + "/" + pOpts.RbdImageName,
		"--id", cr.ID,
		"--keyfile=" + cr.KeyFile,
		"-m", pOpts.Monitors,
	}

	output, err := execCommand("ceph", args)
	if err != nil {
		switch {
		case strings.Contains(string(output), rbdTaskRemoveCmdInvalidString1) &&
			strings.Contains(string(output), rbdTaskRemoveCmdInvalidString2):
			klog.Warningf(util.Log(ctx, "cluster with cluster ID (%s) does not support Ceph manager based rbd image"+
				" deletion (minimum ceph version required is v14.2.3)"), pOpts.ClusterID)
		case strings.HasPrefix(string(output), rbdTaskRemoveCmdAccessDeniedMessage):
			klog.Warningf(util.Log(ctx, "access denied to Ceph MGR-based RBD image deletion "+
				"on cluster ID (%s)"), pOpts.ClusterID)
		default:
			klog.Warningf(util.Log(ctx, "uncaught error while scheduling an image deletion task: %s"), err)
		}
		return false, err
	}

	return true, err
}

// deleteImage deletes a ceph image with provision and volume options.
func deleteImage(ctx context.Context, pOpts *rbdVolume, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	found, _, err := rbdStatus(ctx, pOpts, cr)
	if err != nil {
		return err
	}
	if found {
		klog.Errorf(util.Log(ctx, "rbd is still being used "), image)
		return fmt.Errorf("rbd %s is still being used", image)
	}

	klog.V(4).Infof(util.Log(ctx, "rbd: rm %s using mon %s, pool %s"), image, pOpts.Monitors, pOpts.Pool)

	// attempt to use Ceph manager based deletion support if available
	rbdCephMgrSupported, err := rbdManagerTaskDeleteImage(ctx, pOpts, cr)
	if rbdCephMgrSupported && err != nil {
		klog.Errorf(util.Log(ctx, "failed to add task to delete rbd image: %s/%s, %v"), pOpts.Pool, image, err)
		return err
	}

	if !rbdCephMgrSupported {
		// attempt older style deletion
		args := []string{"rm", image, "--pool", pOpts.Pool, "--id", cr.ID, "-m", pOpts.Monitors,
			"--keyfile=" + cr.KeyFile}
		output, err = execCommand("rbd", args)
		if err != nil {
			klog.Errorf(util.Log(ctx, "failed to delete rbd image: %s/%s, error: %v, command output: %s"), pOpts.Pool, image, err, string(output))
		}
	}

	return err
}

// updateSnapWithImageInfo updates provided rbdSnapshot with information from on-disk data
// regarding the same
func updateSnapWithImageInfo(ctx context.Context, rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	snapInfo, err := getSnapInfo(ctx, rbdSnap.Monitors, cr, rbdSnap.Pool,
		rbdSnap.RbdImageName, rbdSnap.RbdSnapName)
	if err != nil {
		return err
	}

	rbdSnap.SizeBytes = snapInfo.Size

	tm, err := time.Parse(time.ANSIC, snapInfo.Timestamp)
	if err != nil {
		return err
	}

	rbdSnap.CreatedAt, err = ptypes.TimestampProto(tm)

	return err
}

// updateVolWithImageInfo updates provided rbdVolume with information from on-disk data
// regarding the same
func updateVolWithImageInfo(ctx context.Context, rbdVol *rbdVolume, cr *util.Credentials) error {
	imageInfo, err := getImageInfo(ctx, rbdVol.Monitors, cr, rbdVol.Pool, rbdVol.RbdImageName)
	if err != nil {
		return err
	}

	rbdVol.VolSize = imageInfo.Size
	rbdVol.ImageFeatures = strings.Join(imageInfo.Features, ",")

	return nil
}

// genSnapFromSnapID generates a rbdSnapshot structure from the provided identifier, updating
// the structure with elements from on-disk snapshot metadata as well
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

	rbdSnap.Pool, err = util.GetPoolName(ctx, rbdSnap.Monitors, cr, vi.LocationID)
	if err != nil {
		return err
	}
	rbdSnap.JournalPool = rbdSnap.Pool

	imageAttributes, err := snapJournal.GetImageAttributes(ctx, rbdSnap.Monitors,
		cr, rbdSnap.Pool, vi.ObjectUUID, true)
	if err != nil {
		return err
	}
	rbdSnap.RequestName = imageAttributes.RequestName
	rbdSnap.RbdImageName = imageAttributes.SourceName
	rbdSnap.RbdSnapName = imageAttributes.ImageName

	// convert the journal pool ID to name, for use in DeleteSnapshot cases
	if imageAttributes.JournalPoolID != util.InvalidPoolID {
		rbdSnap.JournalPool, err = util.GetPoolName(ctx, rbdSnap.Monitors, cr, imageAttributes.JournalPoolID)
		if err != nil {
			// TODO: If pool is not found we may leak the image (as DeleteSnapshot will return success)
			return err
		}
	}

	err = updateSnapWithImageInfo(ctx, rbdSnap, cr)

	return err
}

// genVolFromVolID generates a rbdVolume structure from the provided identifier, updating
// the structure with elements from on-disk image metadata as well
func genVolFromVolID(ctx context.Context, rbdVol *rbdVolume, volumeID string, cr *util.Credentials, secrets map[string]string) error {
	var (
		options map[string]string
		vi      util.CSIIdentifier
	)
	options = make(map[string]string)

	// rbdVolume fields that are not filled up in this function are:
	//		Mounter, MultiNodeWritable
	rbdVol.VolID = volumeID

	err := vi.DecomposeCSIID(rbdVol.VolID)
	if err != nil {
		err = fmt.Errorf("error decoding volume ID (%s) (%s)", err, rbdVol.VolID)
		return ErrInvalidVolID{err}
	}

	rbdVol.ClusterID = vi.ClusterID
	options["clusterID"] = rbdVol.ClusterID

	rbdVol.Monitors, _, err = getMonsAndClusterID(ctx, options)
	if err != nil {
		return err
	}

	rbdVol.Pool, err = util.GetPoolName(ctx, rbdVol.Monitors, cr, vi.LocationID)
	if err != nil {
		return err
	}
	rbdVol.JournalPool = rbdVol.Pool

	imageAttributes, err := volJournal.GetImageAttributes(ctx, rbdVol.Monitors, cr,
		rbdVol.Pool, vi.ObjectUUID, false)
	if err != nil {
		return err
	}

	if imageAttributes.KmsID != "" {
		rbdVol.Encrypted = true
		rbdVol.KMS, err = util.GetKMS(imageAttributes.KmsID, secrets)
		if err != nil {
			return err
		}
	}
	rbdVol.RequestName = imageAttributes.RequestName
	rbdVol.RbdImageName = imageAttributes.ImageName

	// convert the journal pool ID to name, for use in DeleteVolume cases
	if imageAttributes.JournalPoolID >= 0 {
		rbdVol.JournalPool, err = util.GetPoolName(ctx, rbdVol.Monitors, cr, imageAttributes.JournalPoolID)
		if err != nil {
			// TODO: If pool is not found we may leak the image (as DeleteVolume will return success)
			return err
		}
	}

	err = updateVolWithImageInfo(ctx, rbdVol, cr)

	return err
}

func execCommand(command string, args []string) ([]byte, error) {
	// #nosec
	cmd := exec.Command(command, args...)
	return cmd.CombinedOutput()
}

func getMonsAndClusterID(ctx context.Context, options map[string]string) (monitors, clusterID string, err error) {
	var ok bool

	if clusterID, ok = options["clusterID"]; !ok {
		err = errors.New("clusterID must be set")
		return
	}

	if monitors, err = util.Mons(csiConfigFile, clusterID); err != nil {
		klog.Errorf(util.Log(ctx, "failed getting mons (%s)"), err)
		err = errors.Wrapf(err, "failed to fetch monitor list using clusterID (%s)", clusterID)
		return
	}

	return
}

// isLegacyVolumeID checks if passed in volume ID string conforms to volume ID naming scheme used
// by the version 1.0.0 (legacy) of the plugin, and returns true if found to be conforming
func isLegacyVolumeID(volumeID string) bool {
	// Version 1.0.0 volumeID format: "csi-rbd-vol-" + UUID string
	//    length: 12 ("csi-rbd-vol-") + 36 (UUID string)

	// length check
	if len(volumeID) != 48 {
		return false
	}

	// Header check
	if !strings.HasPrefix(volumeID, "csi-rbd-vol-") {
		return false
	}

	// Trailer UUID format check
	if uuid.Parse(volumeID[12:]) == nil {
		return false
	}

	return true
}

// upadateMons function is used to update the rbdVolume.Monitors for volumes that were provisioned
// using the 1.0.0 version (legacy) of the plugin.
func updateMons(rbdVol *rbdVolume, options, credentials map[string]string) error {
	var ok bool

	// read monitors and MonValueFromSecret from options, else check passed in rbdVolume for
	// MonValueFromSecret key in credentials
	monInSecret := ""
	if options != nil {
		if rbdVol.Monitors, ok = options["monitors"]; !ok {
			rbdVol.Monitors = ""
		}
		if monInSecret, ok = options["monValueFromSecret"]; !ok {
			monInSecret = ""
		}
	} else {
		monInSecret = rbdVol.MonValueFromSecret
	}

	// if monitors are present in secrets and we have the credentials, use monitors from the
	// credentials overriding monitors from other sources
	if monInSecret != "" && credentials != nil {
		monsFromSecret, ok := credentials[monInSecret]
		if ok {
			rbdVol.Monitors = monsFromSecret
		}
	}

	if rbdVol.Monitors == "" {
		return errors.New("either monitors or monValueFromSecret must be set")
	}

	return nil
}

func genVolFromVolumeOptions(ctx context.Context, volOptions, credentials map[string]string, disableInUseChecks, isLegacyVolume bool) (*rbdVolume, error) {
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

	if isLegacyVolume {
		err = updateMons(rbdVol, volOptions, credentials)
		if err != nil {
			return nil, err
		}
	} else {
		rbdVol.Monitors, rbdVol.ClusterID, err = getMonsAndClusterID(ctx, volOptions)
		if err != nil {
			return nil, err
		}
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
		rbdVol.ImageFeatures = imageFeatures
	}

	klog.V(3).Infof(util.Log(ctx, "setting disableInUseChecks on rbd volume to: %v"), disableInUseChecks)
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

func hasSnapshotFeature(imageFeatures string) bool {
	arr := strings.Split(imageFeatures, ",")
	for _, f := range arr {
		if f == "layering" {
			return true
		}
	}
	return false
}

// imageFeaturesToUint64 takes the comma separated image features and converts
// them to a RbdImageOptionFeatures value.
func imageFeaturesToUint64(ctx context.Context, imageFeatures string) uint64 {
	features := uint64(0)

	for _, f := range strings.Split(imageFeatures, ",") {
		if f == "layering" {
			features |= librbd.RbdFeatureLayering
		} else {
			klog.Warningf(util.Log(ctx, "rbd: image feature %s not recognized, skipping"), f)
		}
	}
	return features
}

func protectSnapshot(ctx context.Context, pOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	klog.V(4).Infof(util.Log(ctx, "rbd: snap protect %s using mon %s, pool %s "), image, pOpts.Monitors, pOpts.Pool)
	args := []string{"snap", "protect", "--pool", pOpts.Pool, "--snap", snapName, image, "--id",
		cr.ID, "-m", pOpts.Monitors, "--keyfile=" + cr.KeyFile}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to protect snapshot, command output: %s", string(output))
	}

	return nil
}

func createSnapshot(ctx context.Context, pOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	klog.V(4).Infof(util.Log(ctx, "rbd: snap create %s using mon %s, pool %s"), image, pOpts.Monitors, pOpts.Pool)
	args := []string{"snap", "create", "--pool", pOpts.Pool, "--snap", snapName, image,
		"--id", cr.ID, "-m", pOpts.Monitors, "--keyfile=" + cr.KeyFile}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to create snapshot, command output: %s", string(output))
	}

	return nil
}

func unprotectSnapshot(ctx context.Context, pOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	klog.V(4).Infof(util.Log(ctx, "rbd: snap unprotect %s using mon %s, pool %s"), image, pOpts.Monitors, pOpts.Pool)
	args := []string{"snap", "unprotect", "--pool", pOpts.Pool, "--snap", snapName, image, "--id",
		cr.ID, "-m", pOpts.Monitors, "--keyfile=" + cr.KeyFile}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to unprotect snapshot, command output: %s", string(output))
	}

	return nil
}

func deleteSnapshot(ctx context.Context, pOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	klog.V(4).Infof(util.Log(ctx, "rbd: snap rm %s using mon %s, pool %s"), image, pOpts.Monitors, pOpts.Pool)
	args := []string{"snap", "rm", "--pool", pOpts.Pool, "--snap", snapName, image, "--id",
		cr.ID, "-m", pOpts.Monitors, "--keyfile=" + cr.KeyFile}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to delete snapshot, command output: %s", string(output))
	}

	if err := undoSnapReservation(ctx, pOpts, cr); err != nil {
		klog.Errorf(util.Log(ctx, "failed to remove reservation for snapname (%s) with backing snap (%s) on image (%s) (%s)"),
			pOpts.RequestName, pOpts.RbdSnapName, pOpts.RbdImageName, err)
	}

	return nil
}

func restoreSnapshot(ctx context.Context, pVolOpts *rbdVolume, pSnapOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pVolOpts.RbdImageName
	snapName := pSnapOpts.RbdSnapName

	klog.V(4).Infof(util.Log(ctx, "rbd: clone %s using mon %s, pool %s"), image, pVolOpts.Monitors, pVolOpts.Pool)
	args := []string{"clone", pSnapOpts.Pool + "/" + pSnapOpts.RbdImageName + "@" + snapName,
		pVolOpts.Pool + "/" + image, "--id", cr.ID, "-m", pVolOpts.Monitors, "--keyfile=" + cr.KeyFile}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to restore snapshot, command output: %s", string(output))
	}

	return nil
}

// getSnapshotMetadata fetches on-disk metadata about the snapshot and populates the passed in
// rbdSnapshot structure
func getSnapshotMetadata(ctx context.Context, pSnapOpts *rbdSnapshot, cr *util.Credentials) error {
	imageName := pSnapOpts.RbdImageName
	snapName := pSnapOpts.RbdSnapName

	snapInfo, err := getSnapInfo(ctx, pSnapOpts.Monitors, cr, pSnapOpts.Pool, imageName, snapName)
	if err != nil {
		return err
	}

	pSnapOpts.SizeBytes = snapInfo.Size

	tm, err := time.Parse(time.ANSIC, snapInfo.Timestamp)
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
	Features   []string `json:"features"`
	CreatedAt  string   `json:"create_timestamp"`
}

// getImageInfo queries rbd about the given image and returns its metadata, and returns
// ErrImageNotFound if provided image is not found
func getImageInfo(ctx context.Context, monitors string, cr *util.Credentials, poolName, imageName string) (imageInfo, error) {
	// rbd --format=json info [image-spec | snap-spec]

	var imgInfo imageInfo

	stdout, stderr, err := util.ExecCommand(
		"rbd",
		"-m", monitors,
		"--id", cr.ID,
		"--keyfile="+cr.KeyFile,
		"-c", util.CephConfigPath,
		"--format="+"json",
		"info", poolName+"/"+imageName)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed getting information for image (%s): (%s)"), poolName+"/"+imageName, err)
		if strings.Contains(string(stderr), "rbd: error opening image "+imageName+
			": (2) No such file or directory") {
			return imgInfo, ErrImageNotFound{imageName, err}
		}
		return imgInfo, err
	}

	err = json.Unmarshal(stdout, &imgInfo)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to parse JSON output of image info (%s): (%s)"),
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
	Timestamp string `json:"timestamp"`
}

/*
getSnapInfo queries rbd about the snapshots of the given image and returns its metadata, and
returns ErrImageNotFound if provided image is not found, and ErrSnapNotFound if provided snap
is not found in the images snapshot list
*/
func getSnapInfo(ctx context.Context, monitors string, cr *util.Credentials, poolName, imageName, snapName string) (snapInfo, error) {
	// rbd --format=json snap ls [image-spec]

	var (
		snpInfo snapInfo
		snaps   []snapInfo
	)

	stdout, stderr, err := util.ExecCommand(
		"rbd",
		"-m", monitors,
		"--id", cr.ID,
		"--keyfile="+cr.KeyFile,
		"-c", util.CephConfigPath,
		"--format="+"json",
		"snap", "ls", poolName+"/"+imageName)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed getting snap (%s) information from image (%s): (%s)"),
			snapName, poolName+"/"+imageName, err)
		if strings.Contains(string(stderr), "rbd: error opening image "+imageName+
			": (2) No such file or directory") {
			return snpInfo, ErrImageNotFound{imageName, err}
		}
		return snpInfo, err
	}

	err = json.Unmarshal(stdout, &snaps)
	if err != nil {
		klog.Errorf(util.Log(ctx, "failed to parse JSON output of image snap list (%s): (%s)"),
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

// rbdImageMetadataStash strongly typed JSON spec for stashed RBD image metadata
type rbdImageMetadataStash struct {
	Version   int    `json:"Version"`
	Pool      string `json:"pool"`
	ImageName string `json:"image"`
	NbdAccess bool   `json:"accessType"`
	Encrypted bool   `json:"encrypted"`
}

// file name in which image metadata is stashed
const stashFileName = "image-meta.json"

// stashRBDImageMetadata stashes required fields into the stashFileName at the passed in path, in
// JSON format
func stashRBDImageMetadata(volOptions *rbdVolume, path string) error {
	var imgMeta = rbdImageMetadataStash{
		Version:   2, // there are no checks for this at present
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
		return fmt.Errorf("failed to marshall JSON image metadata for image (%s) in pool (%s): (%v)",
			volOptions.RbdImageName, volOptions.Pool, err)
	}

	fPath := filepath.Join(path, stashFileName)
	err = ioutil.WriteFile(fPath, encodedBytes, 0600)
	if err != nil {
		return fmt.Errorf("failed to stash JSON image metadata for image (%s) in pool (%s) at path (%s): (%v)",
			volOptions.RbdImageName, volOptions.Pool, fPath, err)
	}

	return nil
}

// lookupRBDImageMetadataStash reads and returns stashed image metadata at passed in path
func lookupRBDImageMetadataStash(path string) (rbdImageMetadataStash, error) {
	var imgMeta rbdImageMetadataStash

	fPath := filepath.Join(path, stashFileName)
	encodedBytes, err := ioutil.ReadFile(fPath) // #nosec - intended reading from fPath
	if err != nil {
		if !os.IsNotExist(err) {
			return imgMeta, fmt.Errorf("failed to read stashed JSON image metadata from path (%s): (%v)", fPath, err)
		}

		return imgMeta, ErrMissingStash{err}
	}

	err = json.Unmarshal(encodedBytes, &imgMeta)
	if err != nil {
		return imgMeta, fmt.Errorf("failed to unmarshall stashed JSON image metadata from path (%s): (%v)", fPath, err)
	}

	return imgMeta, nil
}

// cleanupRBDImageMetadataStash cleans up any stashed metadata at passed in path
func cleanupRBDImageMetadataStash(path string) error {
	fPath := filepath.Join(path, stashFileName)
	if err := os.Remove(fPath); err != nil {
		return fmt.Errorf("failed to cleanup stashed JSON data (%s): (%v)", fPath, err)
	}

	return nil
}

// resizeRBDImage resizes the given volume to new size
func resizeRBDImage(rbdVol *rbdVolume, cr *util.Credentials) error {
	var output []byte

	mon := rbdVol.Monitors
	image := rbdVol.RbdImageName
	volSzMiB := fmt.Sprintf("%dM", util.RoundOffVolSize(rbdVol.VolSize))

	args := []string{"resize", image, "--size", volSzMiB, "--pool", rbdVol.Pool, "--id", cr.ID, "-m", mon, "--keyfile=" + cr.KeyFile}
	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to resize rbd image, command output: %s", string(output))
	}

	return nil
}

func ensureEncryptionMetadataSet(ctx context.Context, cr *util.Credentials, rbdVol *rbdVolume) error {
	var vi util.CSIIdentifier

	err := vi.DecomposeCSIID(rbdVol.VolID)
	if err != nil {
		err = fmt.Errorf("error decoding volume ID (%s) (%s)", rbdVol.VolID, err)
		return ErrInvalidVolID{err}
	}

	rbdImageName := volJournal.GetNameForUUID(rbdVol.NamePrefix, vi.ObjectUUID, false)
	imageSpec := rbdVol.Pool + "/" + rbdImageName

	err = util.SaveRbdImageEncryptionStatus(ctx, cr, rbdVol.Monitors, imageSpec, rbdImageRequiresEncryption)
	if err != nil {
		return fmt.Errorf("failed to save encryption status for %s: %v", imageSpec, err)
	}

	return nil
}
