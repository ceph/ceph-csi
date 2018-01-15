/*
Copyright 2017 The Kubernetes Authors.

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

package mount

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"
	utilexec "k8s.io/utils/exec"

	"github.com/golang/glog"
)

const (
	probeVolumeDuration = 1 * time.Second
	probeVolumeTimeout  = 60 * time.Second
	instanceIDFile      = "/var/lib/cloud/data/instance-id"
)

type IMount interface {
	ScanForAttach(devicePath string) error
	IsLikelyNotMountPointAttach(targetpath string) (bool, error)
	FormatAndMount(source string, target string, fstype string, options []string) error
	IsLikelyNotMountPointDetach(targetpath string) (bool, error)
	UnmountPath(mountPath string) error
	GetInstanceID() (string, error)
}

type Mount struct {
}

var MInstance IMount = nil

func GetMountProvider() (IMount, error) {

	if MInstance == nil {
		MInstance = &Mount{}
	}
	return MInstance, nil
}

// probeVolume probes volume in compute
func probeVolume() error {
	// rescan scsi bus
	scsi_path := "/sys/class/scsi_host/"
	if dirs, err := ioutil.ReadDir(scsi_path); err == nil {
		for _, f := range dirs {
			name := scsi_path + f.Name() + "/scan"
			data := []byte("- - -")
			ioutil.WriteFile(name, data, 0666)
		}
	}

	executor := utilexec.New()
	args := []string{"trigger"}
	cmd := executor.Command("udevadm", args...)
	_, err := cmd.CombinedOutput()
	if err != nil {
		glog.V(3).Infof("error running udevadm trigger %v\n", err)
		return err
	}
	glog.V(4).Infof("Successfully probed all attachments")
	return nil
}

// ScanForAttach
func (m *Mount) ScanForAttach(devicePath string) error {
	ticker := time.NewTicker(probeVolumeDuration)
	defer ticker.Stop()
	timer := time.NewTimer(probeVolumeTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			glog.V(5).Infof("Checking Cinder disk %q is attached.", devicePath)
			probeVolume()

			exists, err := util.PathExists(devicePath)
			if exists && err == nil {
				return nil
			} else {
				glog.V(3).Infof("Could not find attached Cinder disk %s", devicePath)
			}
		case <-timer.C:
			return fmt.Errorf("Could not find attached Cinder disk %s. Timeout waiting for mount paths to be created.", devicePath)
		}
	}
}

// FormatAndMount
func (m *Mount) FormatAndMount(source string, target string, fstype string, options []string) error {
	diskMounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: mount.NewOsExec()}
	return diskMounter.FormatAndMount(source, target, fstype, options)
}

// IsLikelyNotMountPointAttach
func (m *Mount) IsLikelyNotMountPointAttach(targetpath string) (bool, error) {
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetpath)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(targetpath, 0750)
			if err == nil {
				notMnt = true
			}
		}
	}
	return notMnt, err
}

// IsLikelyNotMountPointDetach
func (m *Mount) IsLikelyNotMountPointDetach(targetpath string) (bool, error) {
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetpath)
	if err != nil {
		if os.IsNotExist(err) {
			return notMnt, fmt.Errorf("targetpath not found")
		} else {
			return notMnt, err
		}
	}
	return notMnt, nil
}

// UnmountPath
func (m *Mount) UnmountPath(mountPath string) error {
	return util.UnmountPath(mountPath, mount.New(""))
}

// GetInstanceID from file
func (m *Mount) GetInstanceID() (string, error) {
	// Try to find instance ID on the local filesystem (created by cloud-init)
	idBytes, err := ioutil.ReadFile(instanceIDFile)
	if err == nil {
		instanceID := string(idBytes)
		instanceID = strings.TrimSpace(instanceID)
		glog.V(3).Infof("Got instance id from %s: %s", instanceIDFile, instanceID)
		if instanceID != "" {
			return instanceID, nil
		}
	}
	return "", err
}
