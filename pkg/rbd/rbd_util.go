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
	"github.com/golang/glog"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/util/keymutex"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
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

type rbdVolumeOptions struct {
	VolName  string `json:"volName"`
	Monitors string `json:"monitors"`
	Pool     string `json:"pool"`
	AdminSecretName      string            `json:"adminSecret"`
	AdminSecretNamespace string            `json:"adminSecretNamespace"`
	AdminID              string            `json:"adminID"`
	UserID               string            `json:"userID"`
	UserSecretName       string            `json:"userSecret"`
	UserSecretNamespace  string            `json:"userSecretNamespace"`
	ImageFormat          string            `json:"imageFormat"`
	ImageFeatures        []string          `json:"imageFeatures"`
	ImageMapping         map[string]string `json:"imageMapping"`
	adminSecret          string
	userSecret           string
}

var attachdetachMutex = keymutex.NewKeyMutex()

// CreateImage creates a new ceph image with provision and volume options.
func createRBDImage(pOpts *rbdVolumeOptions, volSz int) error {
	var output []byte
	var err error

	// rbd create
	mon := pOpts.Monitors
	image := pOpts.VolName
	volSzGB := fmt.Sprintf("%dG", volSz)

	if pOpts.ImageFormat == rbdImageFormat2 {
		glog.V(4).Infof("rbd: create %s size %s format %s (features: %s) using mon %s, pool %s id %s key %s", image, volSzGB, pOpts.ImageFormat, pOpts.ImageFeatures, mon, pOpts.Pool, pOpts.AdminID, pOpts.adminSecret)
	} else {
		glog.V(4).Infof("rbd: create %s size %s format %s using mon %s, pool %s id %s key %s", image, volSzGB, pOpts.ImageFormat, mon, pOpts.Pool, pOpts.AdminID, pOpts.adminSecret)
	}
	args := []string{"create", image, "--size", volSzGB, "--pool", pOpts.Pool, "--id", pOpts.AdminID, "-m", mon, "--key=" + pOpts.adminSecret, "--image-format", pOpts.ImageFormat}
	if pOpts.ImageFormat == rbdImageFormat2 {
		// if no image features is provided, it results in empty string
		// which disable all RBD image format 2 features as we expected
		features := strings.Join(pOpts.ImageFeatures, ",")
		args = append(args, "--image-feature", features)
	}
	output, err = execCommand("rbd", args)

	if err != nil {
		return fmt.Errorf("failed to create rbd image: %v, command output: %s", err, string(output))
	}

	return nil
}

// rbdStatus checks if there is watcher on the image.
// It returns true if there is a watcher onthe image, otherwise returns false.
func rbdStatus(b *rbdVolumeOptions) (bool, string, error) {
	var err error
	var output string
	var cmd []byte

	image := b.VolName
	// If we don't have admin id/secret (e.g. attaching), fallback to user id/secret.
	id := b.AdminID
	secret := b.adminSecret
	if id == "" {
		id = b.UserID
		secret = b.userSecret
	}

	glog.V(4).Infof("rbd: status %s using mon %s, pool %s id %s key %s", image, b.Monitors, b.Pool, id, secret)
	args := []string{"status", image, "--pool", b.Pool, "-m", b.Monitors, "--id", id, "--key=" + secret}
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
func deleteRBDImage(b *rbdVolumeOptions) error {
	var output []byte
	image := b.VolName
	found, _, err := rbdStatus(b)
	if err != nil {
		return err
	}
	if found {
		glog.Info("rbd is still being used ", image)
		return fmt.Errorf("rbd %s is still being used", image)
	}
	id := b.AdminID
	secret := b.adminSecret
	if id == "" {
		id = b.UserID
		secret = b.userSecret
	}

	glog.V(4).Infof("rbd: rm %s using mon %s, pool %s id %s key %s", image, b.Monitors, b.Pool, id, secret)
	args := []string{"rm", image, "--pool", b.Pool, "--id", id, "-m", b.Monitors, "--key=" + secret}
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

func getRBDVolumeOptions(volOptions map[string]string, client *kubernetes.Clientset) (*rbdVolumeOptions, error) {
	rbdVolume := &rbdVolumeOptions{}
	var ok bool
	var err error
	rbdVolume.AdminID, ok = volOptions["adminId"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter adminId")
	}
	rbdVolume.AdminSecretName, ok = volOptions["adminSecretName"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter adminSecretName")
	}
	rbdVolume.AdminSecretNamespace, ok = volOptions["adminSecretNamespace"]
	if !ok {
		rbdVolume.AdminSecretNamespace = "default"
	}
	rbdVolume.adminSecret, err = parseStorageClassSecret(rbdVolume.AdminSecretName, rbdVolume.AdminSecretNamespace, client)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve Admin secret %v", err)
	}
	rbdVolume.Pool, ok = volOptions["pool"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter pool")
	}
	rbdVolume.Monitors, ok = volOptions["monitors"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter monitors")
	}
	if err != nil {
		return nil, err
	}
	rbdVolume.UserID, ok = volOptions["userId"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter userId")
	}
	rbdVolume.UserSecretName, ok = volOptions["userSecretName"]
	if ok {
		rbdVolume.UserSecretNamespace, ok = volOptions["userSecretNamespace"]
		if !ok {
			rbdVolume.UserSecretNamespace = "default"
		}
		rbdVolume.userSecret, err = parseStorageClassSecret(rbdVolume.UserSecretName, rbdVolume.UserSecretNamespace, client)
		if err != nil {
			glog.Errorf("failed to retrieve user's secret: %s/%s  (%v)", rbdVolume.UserSecretName, rbdVolume.UserSecretNamespace, err)
		}
	}
	rbdVolume.ImageFormat, ok = volOptions["imageFormat"]
	if !ok {
		rbdVolume.ImageFormat = "2"
	}

	return rbdVolume, nil
}

func getRBDVolumeOptionsV2(volOptions map[string]string, credentials map[string]string) (*rbdVolumeOptions, error) {
	rbdVolume := &rbdVolumeOptions{}
	var ok bool
	var err error

	if credentials != nil {
		// If credentials has more than 1 user/key pair, only 1st will be used
		for user, key := range credentials {
			rbdVolume.AdminID = user
			rbdVolume.adminSecret = key
			break
		}
	}
	rbdVolume.Pool, ok = volOptions["pool"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter pool")
	}
	rbdVolume.Monitors, ok = volOptions["monitors"]
	if !ok {
		return nil, fmt.Errorf("Missing required parameter monitors")
	}
	if err != nil {
		return nil, err
	}
	rbdVolume.ImageFormat, ok = volOptions["imageFormat"]
	if !ok {
		rbdVolume.ImageFormat = "2"
	}

	return rbdVolume, nil
}

func parseStorageClassSecret(secretName string, namespace string, client *kubernetes.Clientset) (string, error) {
	if client == nil {
		return "", fmt.Errorf("Cannot get kube client")
	}
	secrets, err := client.CoreV1().Secrets(namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	secret := ""
	for k, v := range secrets.Data {
		if k == secretName {
			return string(v), nil
		}
		secret = string(v)
	}

	return secret, nil
}

func attachRBDImage(volOptions *rbdVolumeOptions) (string, error) {
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
			used, rbdOutput, err := rbdStatus(volOptions)
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
		// If we don't have admin id/secret (e.g. attaching), fallback to user id/secret.
		id := volOptions.AdminID
		secret := volOptions.adminSecret
		if id == "" {
			id = volOptions.UserID
			secret = volOptions.userSecret
		}

		output, err = execCommand("rbd", []string{
			"map", image, "--pool", volOptions.Pool, "--id", id, "-m", volOptions.Monitors, "--key=" + secret})
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

func detachRBDImage(volOptions *rbdVolumeOptions) error {
	var err error
	var output []byte

	image := volOptions.VolName
	glog.V(1).Infof("rbd: unmap device %s", volOptions.ImageMapping[image])
	// If we don't have admin id/secret (e.g. attaching), fallback to user id/secret.
	id := volOptions.AdminID
	secret := volOptions.adminSecret
	if id == "" {
		id = volOptions.UserID
		secret = volOptions.userSecret
	}

	output, err = execCommand("rbd", []string{
		"unmap", volOptions.ImageMapping[image], "--id", id, "--key=" + secret})
	if err != nil {
		glog.V(1).Infof("rbd: unmap error %v, rbd output: %s", err, string(output))
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

func persistVolInfo(image string, persistentStoragePath string, volInfo *rbdVolumeOptions) error {
	file := path.Join(persistentStoragePath, image+".json")
	fp, err := os.Create(file)
	if err != nil {
		return fmt.Errorf("rbd: create err %s/%s", file, err)
	}
	defer fp.Close()

	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(volInfo); err != nil {
		return fmt.Errorf("rbd: encode err: %v.", err)
	}

	return nil
}

func loadVolInfo(image string, persistentStoragePath string, volInfo *rbdVolumeOptions) error {
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
	err := os.Remove(file)
	if err != nil {
		if err != os.ErrNotExist {
			return fmt.Errorf("rbd: open err %s/%s", file, err)
		}
	}
	return nil
}
