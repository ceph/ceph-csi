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
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
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
	Pool               string
	ImageFormat        string
	ImageFeatures      string
	VolSize            int64
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
	SourceVolumeID string
	RbdImageName   string
	RbdSnapName    string
	SnapID         string
	Monitors       string
	Pool           string
	CreatedAt      *timestamp.Timestamp
	SizeBytes      int64
	ClusterID      string
	RequestName    string
}

var (
	// serializes operations based on "<rbd pool>/<rbd image>" as key
	attachdetachLocker = util.NewIDLocker()
	// serializes operations based on "volume name" as key
	volumeNameLocker = util.NewIDLocker()
	// serializes operations based on "snapshot name" as key
	snapshotNameLocker = util.NewIDLocker()
	// serializes operations based on "mount target path" as key
	targetPathLocker = util.NewIDLocker()

	supportedFeatures = sets.NewString("layering")
)

// createImage creates a new ceph image with provision and volume options.
func createImage(pOpts *rbdVolume, volSz int64, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	volSzMiB := fmt.Sprintf("%dM", volSz)

	if pOpts.ImageFormat == rbdImageFormat2 {
		klog.V(4).Infof("rbd: create %s size %s format %s (features: %s) using mon %s, pool %s ", image, volSzMiB, pOpts.ImageFormat, pOpts.ImageFeatures, pOpts.Monitors, pOpts.Pool)
	} else {
		klog.V(4).Infof("rbd: create %s size %s format %s using mon %s, pool %s", image, volSzMiB, pOpts.ImageFormat, pOpts.Monitors, pOpts.Pool)
	}
	args := []string{"create", image, "--size", volSzMiB, "--pool", pOpts.Pool, "--id", cr.ID, "-m", pOpts.Monitors, "--key=" + cr.Key, "--image-format", pOpts.ImageFormat}
	if pOpts.ImageFormat == rbdImageFormat2 {
		args = append(args, "--image-feature", pOpts.ImageFeatures)
	}
	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to create rbd image, command output: %s", string(output))
	}

	return nil
}

// rbdStatus checks if there is watcher on the image.
// It returns true if there is a watcher on the image, otherwise returns false.
func rbdStatus(pOpts *rbdVolume, cr *util.Credentials) (bool, string, error) {
	var output string
	var cmd []byte

	image := pOpts.RbdImageName

	klog.V(4).Infof("rbd: status %s using mon %s, pool %s", image, pOpts.Monitors, pOpts.Pool)
	args := []string{"status", image, "--pool", pOpts.Pool, "-m", pOpts.Monitors, "--id", cr.ID, "--key=" + cr.Key}
	cmd, err := execCommand("rbd", args)
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

// deleteImage deletes a ceph image with provision and volume options.
func deleteImage(pOpts *rbdVolume, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	found, _, err := rbdStatus(pOpts, cr)
	if err != nil {
		return err
	}
	if found {
		klog.Info("rbd is still being used ", image)
		return fmt.Errorf("rbd %s is still being used", image)
	}

	klog.V(4).Infof("rbd: rm %s using mon %s, pool %s", image, pOpts.Monitors, pOpts.Pool)
	args := []string{"rm", image, "--pool", pOpts.Pool, "--id", cr.ID, "-m", pOpts.Monitors,
		"--key=" + cr.Key}
	output, err = execCommand("rbd", args)
	if err != nil {
		klog.Errorf("failed to delete rbd image: %v, command output: %s", err, string(output))
		return err
	}

	err = undoVolReservation(pOpts, cr)
	if err != nil {
		klog.Errorf("failed to remove reservation for volume (%s) with backing image (%s) (%s)",
			pOpts.RequestName, pOpts.RbdImageName, err)
		err = nil
	}

	return err
}

// updateSnapWithImageInfo updates provided rbdSnapshot with information from on-disk data
// regarding the same
func updateSnapWithImageInfo(rbdSnap *rbdSnapshot, cr *util.Credentials) error {
	snapInfo, err := getSnapInfo(rbdSnap.Monitors, cr, rbdSnap.Pool,
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
func updateVolWithImageInfo(rbdVol *rbdVolume, cr *util.Credentials) error {
	imageInfo, err := getImageInfo(rbdVol.Monitors, cr, rbdVol.Pool, rbdVol.RbdImageName)
	if err != nil {
		return err
	}

	if imageInfo.Format != 2 {
		return fmt.Errorf("unknown or unsupported image format (%d) returned for image (%s)",
			imageInfo.Format, rbdVol.RbdImageName)
	}
	rbdVol.ImageFormat = rbdImageFormat2

	rbdVol.VolSize = imageInfo.Size
	rbdVol.ImageFeatures = strings.Join(imageInfo.Features, ",")

	return nil
}

// genSnapFromSnapID generates a rbdSnapshot structure from the provided identifier, updating
// the structure with elements from on-disk snapshot metadata as well
func genSnapFromSnapID(rbdSnap *rbdSnapshot, snapshotID string, cr *util.Credentials) error {
	var (
		options map[string]string
		vi      util.CSIIdentifier
	)
	options = make(map[string]string)

	rbdSnap.SnapID = snapshotID

	err := vi.DecomposeCSIID(rbdSnap.SnapID)
	if err != nil {
		klog.Errorf("error decoding snapshot ID (%s) (%s)", err, rbdSnap.SnapID)
		return err
	}

	rbdSnap.ClusterID = vi.ClusterID
	options["clusterID"] = rbdSnap.ClusterID
	rbdSnap.RbdSnapName = snapJournal.NamingPrefix() + vi.ObjectUUID

	rbdSnap.Monitors, _, err = getMonsAndClusterID(options)
	if err != nil {
		return err
	}

	rbdSnap.Pool, err = util.GetPoolName(rbdSnap.Monitors, cr, vi.LocationID)
	if err != nil {
		return err
	}

	rbdSnap.RequestName, rbdSnap.RbdImageName, err = snapJournal.GetObjectUUIDData(rbdSnap.Monitors,
		cr, rbdSnap.Pool, vi.ObjectUUID, true)
	if err != nil {
		return err
	}

	err = updateSnapWithImageInfo(rbdSnap, cr)

	return err
}

// genVolFromVolID generates a rbdVolume structure from the provided identifier, updating
// the structure with elements from on-disk image metadata as well
func genVolFromVolID(rbdVol *rbdVolume, volumeID string, cr *util.Credentials) error {
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
		klog.V(4).Infof("error decoding volume ID (%s) (%s)", err, rbdVol.VolID)
		return err

	}

	rbdVol.ClusterID = vi.ClusterID
	options["clusterID"] = rbdVol.ClusterID
	rbdVol.RbdImageName = volJournal.NamingPrefix() + vi.ObjectUUID

	rbdVol.Monitors, _, err = getMonsAndClusterID(options)
	if err != nil {
		return err
	}

	rbdVol.Pool, err = util.GetPoolName(rbdVol.Monitors, cr, vi.LocationID)
	if err != nil {
		return err
	}

	rbdVol.RequestName, _, err = volJournal.GetObjectUUIDData(rbdVol.Monitors, cr,
		rbdVol.Pool, vi.ObjectUUID, false)
	if err != nil {
		return err
	}

	err = updateVolWithImageInfo(rbdVol, cr)

	return err
}

func execCommand(command string, args []string) ([]byte, error) {
	// #nosec
	cmd := exec.Command(command, args...)
	return cmd.CombinedOutput()
}

func getMonsAndClusterID(options map[string]string) (monitors, clusterID string, err error) {
	var ok bool

	if clusterID, ok = options["clusterID"]; !ok {
		err = errors.New("clusterID must be set")
		return
	}

	if monitors, err = util.Mons(csiConfigFile, clusterID); err != nil {
		klog.Errorf("failed getting mons (%s)", err)
		err = fmt.Errorf("failed to fetch monitor list using clusterID (%s)", clusterID)
		return
	}

	return
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

	rbdVol.Monitors, rbdVol.ClusterID, err = getMonsAndClusterID(volOptions)
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

	rbdVol.Mounter, ok = volOptions["mounter"]
	if !ok {
		rbdVol.Mounter = rbdDefaultMounter
	}

	return rbdVol, nil
}

func genSnapFromOptions(rbdVol *rbdVolume, snapOptions map[string]string) *rbdSnapshot {
	var err error

	rbdSnap := &rbdSnapshot{}
	rbdSnap.Pool = rbdVol.Pool

	rbdSnap.Monitors, rbdSnap.ClusterID, err = getMonsAndClusterID(snapOptions)
	if err != nil {
		rbdSnap.Monitors = rbdVol.Monitors
		rbdSnap.ClusterID = rbdVol.ClusterID
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

func protectSnapshot(pOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	klog.V(4).Infof("rbd: snap protect %s using mon %s, pool %s ", image, pOpts.Monitors, pOpts.Pool)
	args := []string{"snap", "protect", "--pool", pOpts.Pool, "--snap", snapName, image, "--id",
		cr.ID, "-m", pOpts.Monitors, "--key=" + cr.Key}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to protect snapshot, command output: %s", string(output))
	}

	return nil
}

func createSnapshot(pOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	klog.V(4).Infof("rbd: snap create %s using mon %s, pool %s", image, pOpts.Monitors, pOpts.Pool)
	args := []string{"snap", "create", "--pool", pOpts.Pool, "--snap", snapName, image,
		"--id", cr.ID, "-m", pOpts.Monitors, "--key=" + cr.Key}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to create snapshot, command output: %s", string(output))
	}

	return nil
}

func unprotectSnapshot(pOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	klog.V(4).Infof("rbd: snap unprotect %s using mon %s, pool %s", image, pOpts.Monitors, pOpts.Pool)
	args := []string{"snap", "unprotect", "--pool", pOpts.Pool, "--snap", snapName, image, "--id",
		cr.ID, "-m", pOpts.Monitors, "--key=" + cr.Key}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to unprotect snapshot, command output: %s", string(output))
	}

	return nil
}

func deleteSnapshot(pOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pOpts.RbdImageName
	snapName := pOpts.RbdSnapName

	klog.V(4).Infof("rbd: snap rm %s using mon %s, pool %s", image, pOpts.Monitors, pOpts.Pool)
	args := []string{"snap", "rm", "--pool", pOpts.Pool, "--snap", snapName, image, "--id",
		cr.ID, "-m", pOpts.Monitors, "--key=" + cr.Key}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to delete snapshot, command output: %s", string(output))
	}

	if err := undoSnapReservation(pOpts, cr); err != nil {
		klog.Errorf("failed to remove reservation for snapname (%s) with backing snap (%s) on image (%s) (%s)",
			pOpts.RequestName, pOpts.RbdSnapName, pOpts.RbdImageName, err)
	}

	return nil
}

func restoreSnapshot(pVolOpts *rbdVolume, pSnapOpts *rbdSnapshot, cr *util.Credentials) error {
	var output []byte

	image := pVolOpts.RbdImageName
	snapName := pSnapOpts.RbdSnapName

	klog.V(4).Infof("rbd: clone %s using mon %s, pool %s", image, pVolOpts.Monitors, pVolOpts.Pool)
	args := []string{"clone", pSnapOpts.Pool + "/" + pSnapOpts.RbdImageName + "@" + snapName,
		pVolOpts.Pool + "/" + image, "--id", cr.ID, "-m", pVolOpts.Monitors, "--key=" + cr.Key}

	output, err := execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to restore snapshot, command output: %s", string(output))
	}

	return nil
}

// getSnapshotMetadata fetches on-disk metadata about the snapshot and populates the passed in
// rbdSnapshot structure
func getSnapshotMetadata(pSnapOpts *rbdSnapshot, cr *util.Credentials) error {
	imageName := pSnapOpts.RbdImageName
	snapName := pSnapOpts.RbdSnapName

	snapInfo, err := getSnapInfo(pSnapOpts.Monitors, cr, pSnapOpts.Pool, imageName, snapName)
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
	Format     int64    `json:"format"`
	Features   []string `json:"features"`
	CreatedAt  string   `json:"create_timestamp"`
}

// getImageInfo queries rbd about the given image and returns its metadata, and returns
// ErrImageNotFound if provided image is not found
func getImageInfo(monitors string, cr *util.Credentials, poolName, imageName string) (imageInfo, error) {
	// rbd --format=json info [image-spec | snap-spec]

	var imgInfo imageInfo

	stdout, _, err := util.ExecCommand(
		"rbd",
		"-m", monitors,
		"--id", cr.ID,
		"--key="+cr.Key,
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
	Timestamp string `json:"timestamp"`
}

/*
getSnapInfo queries rbd about the snapshots of the given image and returns its metadata, and
returns ErrImageNotFound if provided image is not found, and ErrSnapNotFound if provided snap
is not found in the images snapshot list
*/
func getSnapInfo(monitors string, cr *util.Credentials, poolName, imageName, snapName string) (snapInfo, error) {
	// rbd --format=json snap ls [image-spec]

	var (
		snpInfo snapInfo
		snaps   []snapInfo
	)

	stdout, _, err := util.ExecCommand(
		"rbd",
		"-m", monitors,
		"--id", cr.ID,
		"--key="+cr.Key,
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
