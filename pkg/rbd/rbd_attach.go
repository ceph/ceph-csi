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
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	envHostRootFS = "HOST_ROOTFS"
	rbdTonbd      = "rbd-nbd"
	rbd           = "rbd"
	nbd           = "nbd"
)

var (
	hostRootFS = "/"
	hasNBD     = false
)

func init() {
	host := os.Getenv(envHostRootFS)
	if len(host) > 0 {
		hostRootFS = host
	}
	hasNBD = checkRbdNbdTools()
}

// Search /sys/bus for rbd device that matches given pool and image.
func getRbdDevFromImageAndPool(pool string, image string) (string, bool) {
	// /sys/bus/rbd/devices/X/name and /sys/bus/rbd/devices/X/pool
	sysPath := "/sys/bus/rbd/devices"
	if dirs, err := ioutil.ReadDir(sysPath); err == nil {
		for _, f := range dirs {
			// Pool and name format:
			// see rbd_pool_show() and rbd_name_show() at
			// https://github.com/torvalds/linux/blob/master/drivers/block/rbd.c
			name := f.Name()
			// First match pool, then match name.
			poolFile := path.Join(sysPath, name, "pool")
			poolBytes, err := ioutil.ReadFile(poolFile)
			if err != nil {
				glog.V(4).Infof("error reading %s: %v", poolFile, err)
				continue
			}
			if strings.TrimSpace(string(poolBytes)) != pool {
				glog.V(4).Infof("device %s is not %q: %q", name, pool, string(poolBytes))
				continue
			}
			imgFile := path.Join(sysPath, name, "name")
			imgBytes, err := ioutil.ReadFile(imgFile)
			if err != nil {
				glog.V(4).Infof("error reading %s: %v", imgFile, err)
				continue
			}
			if strings.TrimSpace(string(imgBytes)) != image {
				glog.V(4).Infof("device %s is not %q: %q", name, image, string(imgBytes))
				continue
			}
			// Found a match, check if device exists.
			devicePath := "/dev/rbd" + name
			if _, err := os.Lstat(devicePath); err == nil {
				return devicePath, true
			}
		}
	}
	return "", false
}

func getMaxNbds() (int, error) {

	// the max number of nbd devices may be found in maxNbdsPath
	// we will check sysfs for possible nbd devices even if this is not available
	maxNbdsPath := "/sys/module/nbd/parameters/nbds_max"
	_, err := os.Lstat(maxNbdsPath)
	if err != nil {
		return 0, fmt.Errorf("rbd-nbd: failed to retrieve max_nbds from %s err: %q", maxNbdsPath, err)
	}

	glog.V(4).Infof("found nbds max parameters file at %s", maxNbdsPath)

	maxNbdBytes, err := ioutil.ReadFile(maxNbdsPath)
	if err != nil {
		return 0, fmt.Errorf("rbd-nbd: failed to read max_nbds from %s err: %q", maxNbdsPath, err)
	}

	maxNbds, err := strconv.Atoi(strings.TrimSpace(string(maxNbdBytes)))
	if err != nil {
		return 0, fmt.Errorf("rbd-nbd: failed to read max_nbds err: %q", err)
	}

	glog.V(4).Infof("rbd-nbd: max_nbds: %d", maxNbds)
	return maxNbds, nil
}

// Locate any existing rbd-nbd process mapping given a <pool, image>.
// Recent versions of rbd-nbd tool can correctly provide this info using list-mapped
// but older versions of list-mapped don't.
// The implementation below peeks at the command line of nbd bound processes
// to figure out any mapped images.
func getNbdDevFromImageAndPool(pool string, image string) (string, bool) {
	// nbd module exports the pid of serving process in sysfs
	basePath := "/sys/block/nbd"
	// Do not change imgPath format - some tools like rbd-nbd are strict about it.
	imgPath := fmt.Sprintf("%s/%s", pool, image)

	maxNbds, maxNbdsErr := getMaxNbds()
	if maxNbdsErr != nil {
		glog.V(4).Infof("error reading nbds_max %v", maxNbdsErr)
		return "", false
	}

	for i := 0; i < maxNbds; i++ {
		nbdPath := basePath + strconv.Itoa(i)
		_, err := os.Lstat(nbdPath)
		if err != nil {
			glog.V(4).Infof("error reading nbd info directory %s: %v", nbdPath, err)
			continue
		}
		pidBytes, err := ioutil.ReadFile(path.Join(nbdPath, "pid"))
		if err != nil {
			glog.V(5).Infof("did not find valid pid file in dir %s: %v", nbdPath, err)
			continue
		}
		cmdlineFileName := path.Join(hostRootFS, "/proc", strings.TrimSpace(string(pidBytes)), "cmdline")
		rawCmdline, err := ioutil.ReadFile(cmdlineFileName)
		if err != nil {
			glog.V(4).Infof("failed to read cmdline file %s: %v", cmdlineFileName, err)
			continue
		}
		cmdlineArgs := strings.FieldsFunc(string(rawCmdline), func(r rune) bool {
			return r == '\u0000'
		})
		// Check if this process is mapping a rbd device.
		// Only accepted pattern of cmdline is from execRbdMap:
		// rbd-nbd map pool/image ...
		if len(cmdlineArgs) < 3 || cmdlineArgs[0] != rbdTonbd || cmdlineArgs[1] != "map" {
			glog.V(4).Infof("nbd device %s is not used by rbd", nbdPath)
			continue
		}
		if cmdlineArgs[2] != imgPath {
			glog.V(4).Infof("rbd-nbd device %s did not match expected image path: %s with path found: %s",
				nbdPath, imgPath, cmdlineArgs[2])
			continue
		}
		devicePath := path.Join("/dev", "nbd"+strconv.Itoa(i))
		if _, err := os.Lstat(devicePath); err != nil {
			glog.Warningf("Stat device %s for imgpath %s failed %v", devicePath, imgPath, err)
			continue
		}
		return devicePath, true
	}
	return "", false
}

// Stat a path, if it doesn't exist, retry maxRetries times.
func waitForPath(pool, image string, maxRetries int, useNbdDriver bool) (string, bool) {
	for i := 0; i < maxRetries; i++ {
		if i != 0 {
			time.Sleep(time.Second)
		}
		if useNbdDriver {
			if devicePath, found := getNbdDevFromImageAndPool(pool, image); found {
				return devicePath, true
			}
		} else {
			if devicePath, found := getRbdDevFromImageAndPool(pool, image); found {
				return devicePath, true
			}
		}
	}
	return "", false
}

// Check if rbd-nbd tools are installed.
func checkRbdNbdTools() bool {
	_, err := execCommand("modprobe", []string{"nbd"})
	if err != nil {
		glog.V(3).Infof("rbd-nbd: nbd modprobe failed with error %v", err)
		return false
	}
	if _, err := execCommand(rbdTonbd, []string{"--version"}); err != nil {
		glog.V(3).Infof("rbd-nbd: running rbd-nbd --version failed with error %v", err)
		return false
	}
	glog.V(3).Infof("rbd-nbd tools were found.")
	return true
}

func attachRBDImage(volOptions *rbdVolume, userId string, credentials map[string]string) (string, error) {
	var err error
	var output []byte

	image := volOptions.VolName
	imagePath := fmt.Sprintf("%s/%s", volOptions.Pool, image)

	useNBD := false
	cmdName := rbd
	moduleName := rbd
	if volOptions.Mounter == rbdTonbd && hasNBD {
		useNBD = true
		cmdName = rbdTonbd
		moduleName = nbd
	}

	devicePath, found := waitForPath(volOptions.Pool, image, 1, useNBD)
	if !found {
		attachdetachMutex.LockKey(imagePath)
		defer attachdetachMutex.UnlockKey(imagePath)

		_, err = execCommand("modprobe", []string{moduleName})
		if err != nil {
			glog.Warningf("rbd: failed to load rbd kernel module:%v", err)
		}

		backoff := wait.Backoff{
			Duration: rbdImageWatcherInitDelay,
			Factor:   rbdImageWatcherFactor,
			Steps:    rbdImageWatcherSteps,
		}
		err := wait.ExponentialBackoff(backoff, func() (bool, error) {
			used, rbdOutput, err := rbdStatus(volOptions, userId, credentials)
			if err != nil {
				return false, fmt.Errorf("fail to check rbd image status with: (%v), rbd output: (%s)", err, rbdOutput)
			}
			return !used, nil
		})
		// return error if rbd image has not become available for the specified timeout
		if err == wait.ErrWaitTimeout {
			return "", fmt.Errorf("rbd image %s is still being used", imagePath)
		}
		// return error if any other errors were encountered during wating for the image to become available
		if err != nil {
			return "", err
		}

		mon, err := getMon(volOptions, credentials)
		if err != nil {
			return "", err
		}

		glog.V(5).Infof("rbd: map mon %s", mon)
		key, err := getRBDKey(userId, credentials)
		if err != nil {
			return "", err
		}
		output, err = execCommand(cmdName, []string{
			"map", imagePath, "--id", userId, "-m", mon, "--key=" + key})
		if err != nil {
			glog.Warningf("rbd: map error %v, rbd output: %s", err, string(output))
			return "", fmt.Errorf("rbd: map failed %v, rbd output: %s", err, string(output))
		}
		devicePath, found = waitForPath(volOptions.Pool, image, 10, useNBD)
		if !found {
			return "", fmt.Errorf("Could not map image %s, Timeout after 10s", imagePath)
		}
	}

	return devicePath, nil
}

func detachRBDDevice(devicePath string) error {
	var err error
	var output []byte

	glog.V(3).Infof("rbd: unmap device %s", devicePath)

	cmdName := rbd
	if strings.HasPrefix(devicePath, "/dev/nbd") {
		cmdName = rbdTonbd
	}

	output, err = execCommand(cmdName, []string{"unmap", devicePath})
	if err != nil {
		return fmt.Errorf("rbd: unmap failed %v, rbd output: %s", err, string(output))
	}

	return nil
}
