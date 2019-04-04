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
	"fmt"
	"os/exec"
	"strings"
	"time"

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
	DisableInUseChecks bool   `json:"disableInUseChecks"`
	ClusterID          string `json:"clusterId"`
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
	ClusterID          string `json:"clusterId"`
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

func getRBDKey(clusterid, id string, credentials map[string]string) (string, error) {
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

// CreateImage creates a new ceph image with provision and volume options.
func createRBDImage(pOpts *rbdVolume, volSz int, adminID string, credentials map[string]string) error {
	var output []byte

	mon, err := getMon(pOpts, credentials)
	if err != nil {
		return err
	}

	image := pOpts.VolName
	volSzMiB := fmt.Sprintf("%dM", volSz)

	key, err := getRBDKey(pOpts.ClusterID, adminID, credentials)
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

	image := pOpts.VolName
	// If we don't have admin id/secret (e.g. attaching), fallback to user id/secret.

	key, err := getRBDKey(pOpts.ClusterID, userID, credentials)
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

// DeleteImage deletes a ceph image with provision and volume options.
func deleteRBDImage(pOpts *rbdVolume, adminID string, credentials map[string]string) error {
	var output []byte
	image := pOpts.VolName
	found, _, err := rbdStatus(pOpts, adminID, credentials)
	if err != nil {
		return err
	}
	if found {
		klog.Info("rbd is still being used ", image)
		return fmt.Errorf("rbd %s is still being used", image)
	}
	key, err := getRBDKey(pOpts.ClusterID, adminID, credentials)
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
	if err == nil {
		return nil
	}
	klog.Errorf("failed to delete rbd image: %v, command output: %s", err, string(output))
	return err
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

func getRBDVolumeOptions(volOptions map[string]string, disableInUseChecks bool) (*rbdVolume, error) {
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
					return nil, fmt.Errorf("invalid feature %q for volume csi-rbdplugin, supported features are: %v", f, supportedFeatures)
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

func getRBDSnapshotOptions(snapOptions map[string]string) (*rbdSnapshot, error) {
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

	key, err := getRBDKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	mon, err := getSnapMon(pOpts, credentials)
	if err != nil {
		return err
	}

	klog.V(4).Infof("rbd: snap protect %s using mon %s, pool %s ", image, mon, pOpts.Pool)
	args := []string{"snap", "protect", "--pool", pOpts.Pool, "--snap", snapID, image, "--id", adminID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to protect snapshot, command output: %s", string(output))
	}

	return nil
}

func extractStoredVolOpt(r *rbdVolume) map[string]string {
	volOptions := make(map[string]string)
	volOptions["pool"] = r.Pool

	if len(r.Monitors) > 0 {
		volOptions["monitors"] = r.Monitors
	}

	if len(r.MonValueFromSecret) > 0 {
		volOptions["monValueFromSecret"] = r.MonValueFromSecret
	}

	volOptions["imageFormat"] = r.ImageFormat

	if len(r.ImageFeatures) > 0 {
		volOptions["imageFeatures"] = r.ImageFeatures
	}

	if len(r.AdminID) > 0 {
		volOptions["adminId"] = r.AdminID
	}

	if len(r.UserID) > 0 {
		volOptions["userId"] = r.UserID
	}
	if len(r.Mounter) > 0 {
		volOptions["mounter"] = r.Mounter
	}
	return volOptions
}

func createSnapshot(pOpts *rbdSnapshot, adminID string, credentials map[string]string) error {
	var output []byte

	mon, err := getSnapMon(pOpts, credentials)
	if err != nil {
		return err
	}

	image := pOpts.VolName
	snapID := pOpts.SnapID

	key, err := getRBDKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	klog.V(4).Infof("rbd: snap create %s using mon %s, pool %s", image, mon, pOpts.Pool)
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

	key, err := getRBDKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	klog.V(4).Infof("rbd: snap unprotect %s using mon %s, pool %s", image, mon, pOpts.Pool)
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

	key, err := getRBDKey(pOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	klog.V(4).Infof("rbd: snap rm %s using mon %s, pool %s", image, mon, pOpts.Pool)
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

	key, err := getRBDKey(pVolOpts.ClusterID, adminID, credentials)
	if err != nil {
		return err
	}
	klog.V(4).Infof("rbd: clone %s using mon %s, pool %s", image, mon, pVolOpts.Pool)
	args := []string{"clone", pSnapOpts.Pool + "/" + pSnapOpts.VolName + "@" + snapID, pVolOpts.Pool + "/" + image, "--id", adminID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return errors.Wrapf(err, "failed to restore snapshot, command output: %s", string(output))
	}

	return nil
}
