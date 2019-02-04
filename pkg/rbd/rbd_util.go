/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
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

type rbdVolume struct {
	VolName            string `json:"volName"`
	VolID              string `json:"volID"`
	Monitors           string `json:"monitors"`
	MonValueFromSecret string `json:"monValueFromSecret"`
	Pool               string `json:"pool"`
	ImageFormat        string `json:"imageFormat"`
	ImageFeatures      string `json:"imageFeatures"`
	VolSize            int64  `json:"volSize"`
	AdminID            string `json:"adminId"`
	UserID             string `json:"userId"`
	Mounter            string `json:"mounter"`
}

type rbdSnapshot struct {
	SourceVolumeID     string `json:"sourceVolumeID"`
	VolName            string `json:"volName"`
	SnapName           string `json:"snapName"`
	SnapID             string `json:"sanpID"`
	Monitors           string `json:"monitors"`
	MonValueFromSecret string `json:"monValueFromSecret"`
	Pool               string `json:"pool"`
	CreatedAt          int64  `json:"createdAt"`
	SizeBytes          int64  `json:"sizeBytes"`
	AdminID            string `json:"adminId"`
	UserID             string `json:"userId"`
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

func getRBDKey(id string, credentials map[string]string) (string, error) {

	if key, ok := credentials[id]; ok {
		return key, nil
	}
	return "", fmt.Errorf("RBD key for ID: %s not found", id)
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

// CreateImage creates a new ceph image with provision and volume options.
func createRBDImage(pOpts *rbdVolume, volSz int, adminID string, credentials map[string]string) error {
	var output []byte

	mon, err := getMon(pOpts, credentials)
	if err != nil {
		return err
	}

	image := pOpts.VolName
	volSzGB := fmt.Sprintf("%dG", volSz)

	key, err := getRBDKey(adminID, credentials)
	if err != nil {
		return err
	}
	if pOpts.ImageFormat == rbdImageFormat2 {
		glog.V(4).Infof("rbd: create %s size %s format %s (features: %s) using mon %s, pool %s id %s key %s", image, volSzGB, pOpts.ImageFormat, pOpts.ImageFeatures, mon, pOpts.Pool, adminID, key)
	} else {
		glog.V(4).Infof("rbd: create %s size %s format %s using mon %s, pool %s id %s key %s", image, volSzGB, pOpts.ImageFormat, mon, pOpts.Pool, adminID, key)
	}
	args := []string{"create", image, "--size", volSzGB, "--pool", pOpts.Pool, "--id", adminID, "-m", mon, "--key=" + key, "--image-format", pOpts.ImageFormat}
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

	image := pOpts.VolName
	// If we don't have admin id/secret (e.g. attaching), fallback to user id/secret.

	key, err := getRBDKey(userID, credentials)
	if err != nil {
		return false, "", err
	}

	mon, err := getMon(pOpts, credentials)
	if err != nil {
		return false, "", err
	}

	glog.V(4).Infof("rbd: status %s using mon %s, pool %s id %s key %s", image, mon, pOpts.Pool, userID, key)
	args := []string{"status", image, "--pool", pOpts.Pool, "-m", mon, "--id", userID, "--key=" + key}
	cmd, err = execCommand("rbd", args)
	output = string(cmd)

	if err, ok := err.(*exec.Error); ok {
		if err.Err == exec.ErrNotFound {
			glog.Errorf("rbd cmd not found")
			// fail fast if command not found
			return false, output, err
		}
	}

	// If command never succeed, returns its last error.
	if err != nil {
		return false, output, err
	}

	if strings.Contains(output, imageWatcherStr) {
		glog.V(4).Infof("rbd: watchers on %s: %s", image, output)
		return true, output, nil
	}
	glog.Warningf("rbd: no watchers on %s", image)
	return false, output, nil
}

// DeleteImage deletes a ceph image with provision and volume options.
func deleteRBDImage(pOpts *rbdVolume, adminID string, credentials map[string]string) error {
	var output []byte
	image := pOpts.VolName
	found, _, err := rbdStatus(pOpts, adminID, credentials)
	if err != nil {
		return err
	}
	if found {
		glog.Info("rbd is still being used ", image)
		return fmt.Errorf("rbd %s is still being used", image)
	}
	key, err := getRBDKey(adminID, credentials)
	if err != nil {
		return err
	}
	mon, err := getMon(pOpts, credentials)
	if err != nil {
		return err
	}

	glog.V(4).Infof("rbd: rm %s using mon %s, pool %s id %s key %s", image, mon, pOpts.Pool, adminID, key)
	args := []string{"rm", image, "--pool", pOpts.Pool, "--id", adminID, "-m", mon, "--key=" + key}
	output, err = execCommand("rbd", args)
	if err == nil {
		return nil
	}
	glog.Errorf("failed to delete rbd image: %v, command output: %s", err, string(output))
	return err
}

func execCommand(command string, args []string) ([]byte, error) {
	// #nosec
	cmd := exec.Command(command, args...)
	return cmd.CombinedOutput()
}

func getRBDVolumeOptions(volOptions map[string]string) (*rbdVolume, error) {
	var ok bool
	rbdVol := &rbdVolume{}
	rbdVol.Pool, ok = volOptions["pool"]
	if !ok {
		return nil, fmt.Errorf("missing required parameter pool")
	}
	rbdVol.Monitors, ok = volOptions["monitors"]
	if !ok {
		// if mons are not set in options, check if they are set in secret
		if rbdVol.MonValueFromSecret, ok = volOptions["monValueFromSecret"]; !ok {
			return nil, fmt.Errorf("either monitors or monValueFromSecret must be set")
		}
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
					return nil, fmt.Errorf("invalid feature %q for volume csi-rbdplugin, supported features are: %v", f, supportedFeatures)
				}
			}
			rbdVol.ImageFeatures = imageFeatures
		}

	}
	getCredsFromVol(rbdVol, volOptions)
	return rbdVol, nil
}

func getCredsFromVol(rbdVol *rbdVolume, volOptions map[string]string) {
	var ok bool
	rbdVol.AdminID, ok = volOptions["adminid"]
	if !ok {
		rbdVol.AdminID = rbdDefaultAdminID
	}
	rbdVol.UserID, ok = volOptions["userid"]
	if !ok {
		rbdVol.UserID = rbdDefaultUserID
	}
	rbdVol.Mounter, ok = volOptions["mounter"]
	if !ok {
		rbdVol.Mounter = rbdDefaultMounter
	}
}
func getRBDSnapshotOptions(snapOptions map[string]string) (*rbdSnapshot, error) {
	var ok bool
	rbdSnap := &rbdSnapshot{}
	rbdSnap.Pool, ok = snapOptions["pool"]
	if !ok {
		return nil, fmt.Errorf("missing required parameter pool")
	}
	rbdSnap.Monitors, ok = snapOptions["monitors"]
	if !ok {
		// if mons are not set in options, check if they are set in secret
		if rbdSnap.MonValueFromSecret, ok = snapOptions["monValueFromSecret"]; !ok {
			return nil, fmt.Errorf("either monitors or monValueFromSecret must be set")
		}
	}
	rbdSnap.AdminID, ok = snapOptions["adminid"]
	if !ok {
		rbdSnap.AdminID = rbdDefaultAdminID
	}
	rbdSnap.UserID, ok = snapOptions["userid"]
	if !ok {
		rbdSnap.UserID = rbdDefaultUserID
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

func getRBDVolumeByID(volumeID string) (*rbdVolume, error) {
	if rbdVol, ok := rbdVolumes[volumeID]; ok {
		return rbdVol, nil
	}
	return nil, fmt.Errorf("volume id %s does not exit in the volumes list", volumeID)
}

func getRBDVolumeByName(volName string) (*rbdVolume, error) {
	for _, rbdVol := range rbdVolumes {
		if rbdVol.VolName == volName {
			return rbdVol, nil
		}
	}
	return nil, fmt.Errorf("volume name %s does not exit in the volumes list", volName)
}

func getRBDSnapshotByName(snapName string) (*rbdSnapshot, error) {
	for _, rbdSnap := range rbdSnapshots {
		if rbdSnap.SnapName == snapName {
			return rbdSnap, nil
		}
	}
	return nil, fmt.Errorf("snapshot name %s does not exit in the snapshots list", snapName)
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

	image := pOpts.VolName
	snapID := pOpts.SnapID

	key, err := getRBDKey(adminID, credentials)
	if err != nil {
		return err
	}
	mon, err := getSnapMon(pOpts, credentials)
	if err != nil {
		return err
	}

	glog.V(4).Infof("rbd: snap protect %s using mon %s, pool %s id %s key %s", image, mon, pOpts.Pool, adminID, key)
	args := []string{"snap", "protect", "--pool", pOpts.Pool, "--snap", snapID, image, "--id", adminID, "-m", mon, "--key=" + key}

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

	image := pOpts.VolName
	snapID := pOpts.SnapID

	key, err := getRBDKey(adminID, credentials)
	if err != nil {
		return err
	}
	glog.V(4).Infof("rbd: snap create %s using mon %s, pool %s id %s key %s", image, mon, pOpts.Pool, adminID, key)
	args := []string{"snap", "create", "--pool", pOpts.Pool, "--snap", snapID, image, "--id", adminID, "-m", mon, "--key=" + key}

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

	image := pOpts.VolName
	snapID := pOpts.SnapID

	key, err := getRBDKey(adminID, credentials)
	if err != nil {
		return err
	}
	glog.V(4).Infof("rbd: snap unprotect %s using mon %s, pool %s id %s key %s", image, mon, pOpts.Pool, adminID, key)
	args := []string{"snap", "unprotect", "--pool", pOpts.Pool, "--snap", snapID, image, "--id", adminID, "-m", mon, "--key=" + key}

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

	image := pOpts.VolName
	snapID := pOpts.SnapID

	key, err := getRBDKey(adminID, credentials)
	if err != nil {
		return err
	}
	glog.V(4).Infof("rbd: snap rm %s using mon %s, pool %s id %s key %s", image, mon, pOpts.Pool, adminID, key)
	args := []string{"snap", "rm", "--pool", pOpts.Pool, "--snap", snapID, image, "--id", adminID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to delete snapshot, command output: %s", string(output))
	}

	return nil
}

func restoreSnapshot(pVolOpts *rbdVolume, pSnapOpts *rbdSnapshot, adminID string, credentials map[string]string) error {
	var output []byte

	mon, err := getMon(pVolOpts, credentials)
	if err != nil {
		return err
	}

	image := pVolOpts.VolName
	snapID := pSnapOpts.SnapID

	key, err := getRBDKey(adminID, credentials)
	if err != nil {
		return err
	}
	glog.V(4).Infof("rbd: clone %s using mon %s, pool %s id %s key %s", image, mon, pVolOpts.Pool, adminID, key)
	args := []string{"clone", pSnapOpts.Pool + "/" + pSnapOpts.VolName + "@" + snapID, pVolOpts.Pool + "/" + image, "--id", adminID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to restore snapshot, command output: %s", string(output))
	}

	return nil
}
