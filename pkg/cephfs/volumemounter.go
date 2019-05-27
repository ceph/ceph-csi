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

package cephfs

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync"

	"github.com/ceph/ceph-csi/pkg/util"

	"k8s.io/klog"
)

const (
	volumeMounterFuse   = "fuse"
	volumeMounterKernel = "kernel"
)

var (
	availableMounters []string

	// maps a mountpoint to PID of its FUSE daemon
	fusePidMap    = make(map[string]int)
	fusePidMapMtx sync.Mutex

	fusePidRx = regexp.MustCompile(`(?m)^ceph-fuse\[(.+)\]: starting fuse$`)
)

// Load available ceph mounters installed on system into availableMounters
// Called from driver.go's Run()
func loadAvailableMounters() error {
	// #nosec
	fuseMounterProbe := exec.Command("ceph-fuse", "--version")
	// #nosec
	kernelMounterProbe := exec.Command("mount.ceph")

	if fuseMounterProbe.Run() == nil {
		availableMounters = append(availableMounters, volumeMounterFuse)
	}

	if kernelMounterProbe.Run() == nil {
		availableMounters = append(availableMounters, volumeMounterKernel)
	}

	if len(availableMounters) == 0 {
		return errors.New("no ceph mounters found on system")
	}

	return nil
}

type volumeMounter interface {
	mount(mountPoint string, cr *credentials, volOptions *volumeOptions) error
	name() string
}

func newMounter(volOptions *volumeOptions) (volumeMounter, error) {
	// Get the mounter from the configuration

	wantMounter := volOptions.Mounter

	if wantMounter == "" {
		wantMounter = DefaultVolumeMounter
	}

	// Verify that it's available

	var chosenMounter string

	for _, availMounter := range availableMounters {
		if chosenMounter == "" {
			if availMounter == wantMounter {
				chosenMounter = wantMounter
			}
		}
	}

	if chosenMounter == "" {
		// Otherwise pick whatever is left
		chosenMounter = availableMounters[0]
	}

	// Create the mounter

	switch chosenMounter {
	case volumeMounterFuse:
		return &fuseMounter{}, nil
	case volumeMounterKernel:
		return &kernelMounter{}, nil
	}

	return nil, fmt.Errorf("unknown mounter '%s'", chosenMounter)
}

type fuseMounter struct{}

func mountFuse(mountPoint string, cr *credentials, volOptions *volumeOptions) error {
	args := [...]string{
		mountPoint,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.id, "--key=" + cr.key,
		"-r", volOptions.RootPath,
		"-o", "nonempty",
	}

	_, stderr, err := execCommand("ceph-fuse", args[:]...)
	if err != nil {
		return err
	}

	// Parse the output:
	// We need "starting fuse" meaning the mount is ok
	// and PID of the ceph-fuse daemon for unmount

	match := fusePidRx.FindSubmatch(stderr)
	if len(match) != 2 {
		return fmt.Errorf("ceph-fuse failed: %s", stderr)
	}

	pid, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return fmt.Errorf("failed to parse FUSE daemon PID: %v", err)
	}

	fusePidMapMtx.Lock()
	fusePidMap[mountPoint] = pid
	fusePidMapMtx.Unlock()

	return nil
}

func (m *fuseMounter) mount(mountPoint string, cr *credentials, volOptions *volumeOptions) error {
	if err := createMountPoint(mountPoint); err != nil {
		return err
	}

	return mountFuse(mountPoint, cr, volOptions)
}

func (m *fuseMounter) name() string { return "Ceph FUSE driver" }

type kernelMounter struct{}

func mountKernel(mountPoint string, cr *credentials, volOptions *volumeOptions) error {
	if err := execCommandErr("modprobe", "ceph"); err != nil {
		return err
	}

	return execCommandErr("mount",
		"-t", "ceph",
		fmt.Sprintf("%s:%s", volOptions.Monitors, volOptions.RootPath),
		mountPoint,
		"-o", fmt.Sprintf("name=%s,secret=%s", cr.id, cr.key),
	)
}

func (m *kernelMounter) mount(mountPoint string, cr *credentials, volOptions *volumeOptions) error {
	if err := createMountPoint(mountPoint); err != nil {
		return err
	}

	return mountKernel(mountPoint, cr, volOptions)
}

func (m *kernelMounter) name() string { return "Ceph kernel client" }

func bindMount(from, to string, readOnly bool) error {
	if err := execCommandErr("mount", "--bind", from, to); err != nil {
		return fmt.Errorf("failed to bind-mount %s to %s: %v", from, to, err)
	}

	if readOnly {
		if err := execCommandErr("mount", "-o", "remount,ro,bind", to); err != nil {
			return fmt.Errorf("failed read-only remount of %s: %v", to, err)
		}
	}

	return nil
}

func unmountVolume(mountPoint string) error {
	if err := execCommandErr("umount", mountPoint); err != nil {
		return err
	}

	fusePidMapMtx.Lock()
	pid, ok := fusePidMap[mountPoint]
	if ok {
		delete(fusePidMap, mountPoint)
	}
	fusePidMapMtx.Unlock()

	if ok {
		p, err := os.FindProcess(pid)
		if err != nil {
			klog.Warningf("failed to find process %d: %v", pid, err)
		} else {
			if _, err = p.Wait(); err != nil {
				klog.Warningf("%d is not a child process: %v", pid, err)
			}
		}
	}

	return nil
}

func createMountPoint(root string) error {
	return os.MkdirAll(root, 0750)
}
