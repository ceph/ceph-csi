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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/util/keymutex"
)

const (
	imageWatcherStr = "watcher="
	rbdImageFormat1 = "1"
	rbdImageFormat2 = "2"
	imageSizeStr    = "size "
	sizeDivStr      = " MB in"
	kubeLockMagic   = "kubelet_lock_magic_"
	// The following three values are used for 30 seconds timeout
	// while waiting for RBD Watcher to expire.
	rbdImageWatcherInitDelay = 1 * time.Second
	rbdImageWatcherFactor    = 1.4
	rbdImageWatcherSteps     = 10
)

type rbdVolume struct {
	VolName       string `json:"volName"`
	VolID         string `json:"volID"`
	Monitors      string `json:"monitors"`
	Pool          string `json:"pool"`
	ImageFormat   string `json:"imageFormat"`
	ImageFeatures string `json:"imageFeatures"`
	VolSize       int64  `json:"volSize"`
}

type rbdSnapshot struct {
	SourceVolumeID string `json:"sourceVolumeID"`
	VolName        string `json:"volName"`
	SnapName       string `json:"snapName"`
	SnapID         string `json:"sanpID"`
	Monitors       string `json:"monitors"`
	Pool           string `json:"pool"`
	CreatedAt      int64  `json:"createdAt"`
	SizeBytes      int64  `json:"sizeBytes"`
}

var (
	attachdetachMutex = keymutex.NewKeyMutex()
	supportedFeatures = sets.NewString("layering")
)

func getRBDKey(id string, credentials map[string]string) (string, error) {

	if key, ok := credentials[id]; ok {
		return key, nil
	}
	return "", fmt.Errorf("RBD key for ID: %s not found", id)
}

// CreateImage creates a new ceph image with provision and volume options.
func createRBDImage(pOpts *rbdVolume, volSz int, credentials map[string]string) error {
	var output []byte
	var err error

	// rbd create
	mon := pOpts.Monitors
	image := pOpts.VolName
	volSzGB := fmt.Sprintf("%dG", volSz)

	key, err := getRBDKey(RBDUserID, credentials)
	if err != nil {
		return err
	}
	if pOpts.ImageFormat == rbdImageFormat2 {
		glog.V(4).Infof("rbd: create %s size %s format %s (features: %s) using mon %s, pool %s id %s key %s", image, volSzGB, pOpts.ImageFormat, pOpts.ImageFeatures, mon, pOpts.Pool, RBDUserID, key)
	} else {
		glog.V(4).Infof("rbd: create %s size %s format %s using mon %s, pool %s id %s key %s", image, volSzGB, pOpts.ImageFormat, mon, pOpts.Pool, RBDUserID, key)
	}
	args := []string{"create", image, "--size", volSzGB, "--pool", pOpts.Pool, "--id", RBDUserID, "-m", mon, "--key=" + key, "--image-format", pOpts.ImageFormat}
	if pOpts.ImageFormat == rbdImageFormat2 {
		args = append(args, "--image-feature", pOpts.ImageFeatures)
	}
	output, err = execCommand("rbd", args)

	if err != nil {
		return fmt.Errorf("failed to create rbd image: %v, command output: %s", err, string(output))
	}

	return nil
}

// rbdStatus checks if there is watcher on the image.
// It returns true if there is a watcher onthe image, otherwise returns false.
func rbdStatus(pOpts *rbdVolume, credentials map[string]string) (bool, string, error) {
	var err error
	var output string
	var cmd []byte

	image := pOpts.VolName
	// If we don't have admin id/secret (e.g. attaching), fallback to user id/secret.
	key, err := getRBDKey(RBDUserID, credentials)
	if err != nil {
		return false, "", err
	}

	glog.V(4).Infof("rbd: status %s using mon %s, pool %s id %s key %s", image, pOpts.Monitors, pOpts.Pool, RBDUserID, key)
	args := []string{"status", image, "--pool", pOpts.Pool, "-m", pOpts.Monitors, "--id", RBDUserID, "--key=" + key}
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
	} else {
		glog.Warningf("rbd: no watchers on %s", image)
		return false, output, nil
	}
}

// DeleteImage deletes a ceph image with provision and volume options.
func deleteRBDImage(pOpts *rbdVolume, credentials map[string]string) error {
	var output []byte
	image := pOpts.VolName
	found, _, err := rbdStatus(pOpts, credentials)
	if err != nil {
		return err
	}
	if found {
		glog.Info("rbd is still being used ", image)
		return fmt.Errorf("rbd %s is still being used", image)
	}
	key, err := getRBDKey(RBDUserID, credentials)
	if err != nil {
		return err
	}

	glog.V(4).Infof("rbd: rm %s using mon %s, pool %s id %s key %s", image, pOpts.Monitors, pOpts.Pool, RBDUserID, key)
	args := []string{"rm", image, "--pool", pOpts.Pool, "--id", RBDUserID, "-m", pOpts.Monitors, "--key=" + key}
	output, err = execCommand("rbd", args)
	if err == nil {
		return nil
	}
	glog.Errorf("failed to delete rbd image: %v, command output: %s", err, string(output))
	return err
}

func execCommand(command string, args []string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	return cmd.CombinedOutput()
}

func getRBDVolumeOptions(volOptions map[string]string) (*rbdVolume, error) {
	var ok bool
	rbdVol := &rbdVolume{}
	rbdVol.Pool, ok = volOptions["pool"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter pool")
	}
	rbdVol.Monitors, ok = volOptions["monitors"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter monitors")
	}
	rbdVol.ImageFormat, ok = volOptions["imageFormat"]
	if !ok {
		rbdVol.ImageFormat = rbdImageFormat2
	}
	if rbdVol.ImageFormat == rbdImageFormat2 {
		// if no image features is provided, it results in empty string
		// which disable all RBD image format 2 features as we expected
		imageFeatures, ok := volOptions["imageFeatures"]
		if ok {
			arr := strings.Split(imageFeatures, ",")
			for _, f := range arr {
				if !supportedFeatures.Has(f) {
					return nil, fmt.Errorf("invalid feature %q for volume csi-rbdplugin, supported features are: %v", f, supportedFeatures)
				}
			}
			rbdVol.ImageFeatures = imageFeatures
		}

	}

	return rbdVol, nil
}

func getRBDSnapshotOptions(snapOptions map[string]string) (*rbdSnapshot, error) {
	var ok bool
	rbdSnap := &rbdSnapshot{}
	rbdSnap.Pool, ok = snapOptions["pool"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter pool")
	}
	rbdSnap.Monitors, ok = snapOptions["monitors"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter monitors")
	}

	return rbdSnap, nil
}

func attachRBDImage(volOptions *rbdVolume, credentials map[string]string) (string, error) {
	var err error
	var output []byte

	image := volOptions.VolName
	devicePath, found := waitForPath(volOptions.Pool, image, 1)
	if !found {
		attachdetachMutex.LockKey(string(volOptions.Pool + image))
		defer attachdetachMutex.UnlockKey(string(volOptions.Pool + image))

		_, err = execCommand("modprobe", []string{"rbd"})
		if err != nil {
			glog.Warningf("rbd: failed to load rbd kernel module:%v", err)
		}

		backoff := wait.Backoff{
			Duration: rbdImageWatcherInitDelay,
			Factor:   rbdImageWatcherFactor,
			Steps:    rbdImageWatcherSteps,
		}
		err := wait.ExponentialBackoff(backoff, func() (bool, error) {
			used, rbdOutput, err := rbdStatus(volOptions, credentials)
			if err != nil {
				return false, fmt.Errorf("fail to check rbd image status with: (%v), rbd output: (%s)", err, rbdOutput)
			}
			return !used, nil
		})
		// return error if rbd image has not become available for the specified timeout
		if err == wait.ErrWaitTimeout {
			return "", fmt.Errorf("rbd image %s/%s is still being used", volOptions.Pool, image)
		}
		// return error if any other errors were encountered during wating for the image to becme avialble
		if err != nil {
			return "", err
		}

		glog.V(1).Infof("rbd: map mon %s", volOptions.Monitors)
		key, err := getRBDKey(RBDUserID, credentials)
		if err != nil {
			return "", err
		}

		output, err = execCommand("rbd", []string{
			"map", image, "--pool", volOptions.Pool, "--id", RBDUserID, "-m", volOptions.Monitors, "--key=" + key})
		if err != nil {
			glog.V(1).Infof("rbd: map error %v, rbd output: %s", err, string(output))
			return "", fmt.Errorf("rbd: map failed %v, rbd output: %s", err, string(output))
		}
		devicePath, found = waitForPath(volOptions.Pool, image, 10)
		if !found {
			return "", fmt.Errorf("Could not map image %s/%s, Timeout after 10s", volOptions.Pool, image)
		}
	}

	return devicePath, nil
}

func detachRBDDevice(devicePath string) error {
	var err error
	var output []byte

	glog.V(3).Infof("rbd: unmap device %s", devicePath)

	output, err = execCommand("rbd", []string{"unmap", devicePath})
	if err != nil {
		return fmt.Errorf("rbd: unmap failed %v, rbd output: %s", err, string(output))
	}

	return nil
}

func getDevFromImageAndPool(pool, image string) (string, bool) {
	// /sys/bus/rbd/devices/X/name and /sys/bus/rbd/devices/X/pool
	sys_path := "/sys/bus/rbd/devices"
	if dirs, err := ioutil.ReadDir(sys_path); err == nil {
		for _, f := range dirs {
			name := f.Name()
			// first match pool, then match name
			poolFile := path.Join(sys_path, name, "pool")
			poolBytes, err := ioutil.ReadFile(poolFile)
			if err != nil {
				glog.V(4).Infof("Error reading %s: %v", poolFile, err)
				continue
			}
			if strings.TrimSpace(string(poolBytes)) != pool {
				glog.V(4).Infof("Device %s is not %q: %q", name, pool, string(poolBytes))
				continue
			}
			imgFile := path.Join(sys_path, name, "name")
			imgBytes, err := ioutil.ReadFile(imgFile)
			if err != nil {
				glog.V(4).Infof("Error reading %s: %v", imgFile, err)
				continue
			}
			if strings.TrimSpace(string(imgBytes)) != image {
				glog.V(4).Infof("Device %s is not %q: %q", name, image, string(imgBytes))
				continue
			}
			// found a match, check if device exists
			devicePath := "/dev/rbd" + name
			if _, err := os.Lstat(devicePath); err == nil {
				return devicePath, true
			}
		}
	}
	return "", false
}

// stat a path, if not exists, retry maxRetries times
func waitForPath(pool, image string, maxRetries int) (string, bool) {
	for i := 0; i < maxRetries; i++ {
		devicePath, found := getDevFromImageAndPool(pool, image)
		if found {
			return devicePath, true
		}
		if i == maxRetries-1 {
			break
		}
		time.Sleep(time.Second)
	}
	return "", false
}

func persistVolInfo(image string, persistentStoragePath string, volInfo *rbdVolume) error {
	file := path.Join(persistentStoragePath, image+".json")
	fp, err := os.Create(file)
	if err != nil {
		glog.Errorf("rbd: failed to create persistent storage file %s with error: %v\n", file, err)
		return fmt.Errorf("rbd: create err %s/%s", file, err)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(volInfo); err != nil {
		glog.Errorf("rbd: failed to encode volInfo: %+v for file: %s with error: %v\n", volInfo, file, err)
		return fmt.Errorf("rbd: encode err: %v", err)
	}
	glog.Infof("rbd: successfully saved volInfo: %+v into file: %s\n", volInfo, file)
	return nil
}

func loadVolInfo(image string, persistentStoragePath string, volInfo *rbdVolume) error {
	file := path.Join(persistentStoragePath, image+".json")
	fp, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("rbd: open err %s/%s", file, err)
	}
	defer fp.Close()

	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(volInfo); err != nil {
		return fmt.Errorf("rbd: decode err: %v.", err)
	}

	return nil
}

func deleteVolInfo(image string, persistentStoragePath string) error {
	file := path.Join(persistentStoragePath, image+".json")
	glog.Infof("rbd: Deleting file for Volume: %s at: %s resulting path: %+v\n", image, persistentStoragePath, file)
	err := os.Remove(file)
	if err != nil {
		if err != os.ErrNotExist {
			return fmt.Errorf("rbd: error removing file: %s/%s", file, err)
		}
	}
	return nil
}

func persistSnapInfo(snapshot string, persistentStoragePath string, snapInfo *rbdSnapshot) error {
	file := path.Join(persistentStoragePath, snapshot+".json")
	fp, err := os.Create(file)
	if err != nil {
		glog.Errorf("rbd: failed to create persistent storage file %s with error: %v\n", file, err)
		return fmt.Errorf("rbd: create err %s/%s", file, err)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(snapInfo); err != nil {
		glog.Errorf("rbd: failed to encode snapInfo: %+v for file: %s with error: %v\n", snapInfo, file, err)
		return fmt.Errorf("rbd: encode err: %v", err)
	}
	glog.Infof("rbd: successfully saved snapInfo: %+v into file: %s\n", snapInfo, file)
	return nil
}

func loadSnapInfo(snapshot string, persistentStoragePath string, snapInfo *rbdSnapshot) error {
	file := path.Join(persistentStoragePath, snapshot+".json")
	fp, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("rbd: open err %s/%s", file, err)
	}
	defer fp.Close()

	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(snapInfo); err != nil {
		return fmt.Errorf("rbd: decode err: %v.", err)
	}
	return nil
}

func deleteSnapInfo(snapshot string, persistentStoragePath string) error {
	file := path.Join(persistentStoragePath, snapshot+".json")
	glog.Infof("rbd: Deleting file for Snapshot: %s at: %s resulting path: %+v\n", snapshot, persistentStoragePath, file)
	err := os.Remove(file)
	if err != nil {
		if err != os.ErrNotExist {
			return fmt.Errorf("rbd: error removing file: %s/%s", file, err)
		}
	}
	return nil
}

func getRBDVolumeByID(volumeID string) (rbdVolume, error) {
	if rbdVol, ok := rbdVolumes[volumeID]; ok {
		return rbdVol, nil
	}
	return rbdVolume{}, fmt.Errorf("volume id %s does not exit in the volumes list", volumeID)
}

func getRBDVolumeByName(volName string) (rbdVolume, error) {
	for _, rbdVol := range rbdVolumes {
		if rbdVol.VolName == volName {
			return rbdVol, nil
		}
	}
	return rbdVolume{}, fmt.Errorf("volume name %s does not exit in the volumes list", volName)
}

func getRBDSnapshotByName(snapName string) (rbdSnapshot, error) {
	for _, rbdSnap := range rbdSnapshots {
		if rbdSnap.SnapName == snapName {
			return rbdSnap, nil
		}
	}
	return rbdSnapshot{}, fmt.Errorf("snapshot name %s does not exit in the snapshots list", snapName)
}

func protectSnapshot(pOpts *rbdSnapshot, credentials map[string]string) error {
	var output []byte
	var err error

	mon := pOpts.Monitors
	image := pOpts.VolName
	snapID := pOpts.SnapID

	key, err := getRBDKey(RBDUserID, credentials)
	if err != nil {
		return err
	}
	glog.V(4).Infof("rbd: snap protect %s using mon %s, pool %s id %s key %s", image, pOpts.Monitors, pOpts.Pool, RBDUserID, key)
	args := []string{"snap", "protect", "--pool", pOpts.Pool, "--snap", snapID, image, "--id", RBDUserID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return fmt.Errorf("failed to protect snapshot: %v, command output: %s", err, string(output))
	}

	return nil
}

func createSnapshot(pOpts *rbdSnapshot, credentials map[string]string) error {
	var output []byte
	var err error

	mon := pOpts.Monitors
	image := pOpts.VolName
	snapID := pOpts.SnapID

	key, err := getRBDKey(RBDUserID, credentials)
	if err != nil {
		return err
	}
	glog.V(4).Infof("rbd: snap create %s using mon %s, pool %s id %s key %s", image, pOpts.Monitors, pOpts.Pool, RBDUserID, key)
	args := []string{"snap", "create", "--pool", pOpts.Pool, "--snap", snapID, image, "--id", RBDUserID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return fmt.Errorf("failed to create snapshot: %v, command output: %s", err, string(output))
	}

	return nil
}

func unprotectSnapshot(pOpts *rbdSnapshot, credentials map[string]string) error {
	var output []byte
	var err error

	mon := pOpts.Monitors
	image := pOpts.VolName
	snapID := pOpts.SnapID

	key, err := getRBDKey(RBDUserID, credentials)
	if err != nil {
		return err
	}
	glog.V(4).Infof("rbd: snap unprotect %s using mon %s, pool %s id %s key %s", image, pOpts.Monitors, pOpts.Pool, RBDUserID, key)
	args := []string{"snap", "unprotect", "--pool", pOpts.Pool, "--snap", snapID, image, "--id", RBDUserID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return fmt.Errorf("failed to unprotect snapshot: %v, command output: %s", err, string(output))
	}

	return nil
}

func deleteSnapshot(pOpts *rbdSnapshot, credentials map[string]string) error {
	var output []byte
	var err error

	mon := pOpts.Monitors
	image := pOpts.VolName
	snapID := pOpts.SnapID

	key, err := getRBDKey(RBDUserID, credentials)
	if err != nil {
		return err
	}
	glog.V(4).Infof("rbd: snap rm %s using mon %s, pool %s id %s key %s", image, pOpts.Monitors, pOpts.Pool, RBDUserID, key)
	args := []string{"snap", "rm", "--pool", pOpts.Pool, "--snap", snapID, image, "--id", RBDUserID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %v, command output: %s", err, string(output))
	}

	return nil
}

func restoreSnapshot(pVolOpts *rbdVolume, pSnapOpts *rbdSnapshot, credentials map[string]string) error {
	var output []byte
	var err error

	mon := pVolOpts.Monitors
	image := pVolOpts.VolName
	snapID := pSnapOpts.SnapID

	key, err := getRBDKey(RBDUserID, credentials)
	if err != nil {
		return err
	}
	glog.V(4).Infof("rbd: clone %s using mon %s, pool %s id %s key %s", image, pVolOpts.Monitors, pVolOpts.Pool, RBDUserID, key)
	args := []string{"clone", pSnapOpts.Pool + "/" + pSnapOpts.VolName + "@" + snapID, pVolOpts.Pool + "/" + image, "--id", RBDUserID, "-m", mon, "--key=" + key}

	output, err = execCommand("rbd", args)

	if err != nil {
		return fmt.Errorf("failed to restore snapshot: %v, command output: %s", err, string(output))
	}

	return nil
}
