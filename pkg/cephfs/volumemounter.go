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
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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
	mount(ctx context.Context, mountPoint string, cr *util.Credentials, volOptions *volumeOptions) error
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

func mountFuse(ctx context.Context, mountPoint string, cr *util.Credentials, volOptions *volumeOptions) error {
	args := []string{
		mountPoint,
		"-m", volOptions.Monitors,
		"-c", util.CephConfigPath,
		"-n", cephEntityClientPrefix + cr.ID, "--keyfile=" + cr.KeyFile,
		"-r", volOptions.RootPath,
		"-o", "nonempty",
	}

	if volOptions.FsName != "" {
		args = append(args, "--client_mds_namespace="+volOptions.FsName)
	}

	_, stderr, err := execCommand(ctx, "ceph-fuse", args[:]...)
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

func (m *fuseMounter) mount(ctx context.Context, mountPoint string, cr *util.Credentials, volOptions *volumeOptions) error {
	if err := util.CreateMountPoint(mountPoint); err != nil {
		return err
	}

	return mountFuse(ctx, mountPoint, cr, volOptions)
}

func (m *fuseMounter) name() string { return "Ceph FUSE driver" }

type kernelMounter struct{}

func mountKernel(ctx context.Context, mountPoint string, cr *util.Credentials, volOptions *volumeOptions) error {
	if err := execCommandErr(ctx, "modprobe", "ceph"); err != nil {
		return err
	}

	args := []string{
		"-t", "ceph",
		fmt.Sprintf("%s:%s", volOptions.Monitors, volOptions.RootPath),
		mountPoint,
	}
	optionsStr := fmt.Sprintf("name=%s,secretfile=%s", cr.ID, cr.KeyFile)
	if volOptions.FsName != "" {
		optionsStr += fmt.Sprintf(",mds_namespace=%s", volOptions.FsName)
	}
	args = append(args, "-o", optionsStr)

	return execCommandErr(ctx, "mount", args[:]...)
}

func (m *kernelMounter) mount(ctx context.Context, mountPoint string, cr *util.Credentials, volOptions *volumeOptions) error {
	if err := util.CreateMountPoint(mountPoint); err != nil {
		return err
	}

	return mountKernel(ctx, mountPoint, cr, volOptions)
}

func (m *kernelMounter) name() string { return "Ceph kernel client" }

func bindMount(ctx context.Context, from, to string, readOnly bool, mntOptions []string) error {
	mntOptionSli := strings.Join(mntOptions, ",")
	if err := execCommandErr(ctx, "mount", "-o", mntOptionSli, from, to); err != nil {
		return fmt.Errorf("failed to bind-mount %s to %s: %v", from, to, err)
	}

	if readOnly {
		mntOptionSli += ",remount"
		if err := execCommandErr(ctx, "mount", "-o", mntOptionSli, to); err != nil {
			return fmt.Errorf("failed read-only remount of %s: %v", to, err)
		}
	}

	return nil
}

func unmountVolume(ctx context.Context, mountPoint string) error {
	if err := execCommandErr(ctx, "umount", mountPoint); err != nil {
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
			klog.Warningf(util.Log(ctx, "failed to find process %d: %v"), pid, err)
		} else {
			if _, err = p.Wait(); err != nil {
				klog.Warningf(util.Log(ctx, "%d is not a child process: %v"), pid, err)
			}
		}
	}

	return nil
}
