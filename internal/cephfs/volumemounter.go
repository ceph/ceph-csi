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

	"github.com/ceph/ceph-csi/internal/util"
)

const (
	volumeMounterFuse   = "fuse"
	volumeMounterKernel = "kernel"
	netDev              = "_netdev"
)

var (
	availableMounters []string

	// maps a mountpoint to PID of its FUSE daemon
	fusePidMap    = make(map[string]int)
	fusePidMapMtx sync.Mutex

	fusePidRx = regexp.MustCompile(`(?m)^ceph-fuse\[(.+)\]: starting fuse$`)

	// nolint:gomnd // numbers specify Kernel versions.
	quotaSupport = []util.KernelVersion{
		{
			Version:      4,
			PatchLevel:   17,
			SubLevel:     0,
			ExtraVersion: 0, Distribution: "",
			Backport: false,
		}, // standard 4.17+ versions
		{
			Version:      3,
			PatchLevel:   10,
			SubLevel:     0,
			ExtraVersion: 1062,
			Distribution: ".el7",
			Backport:     true,
		}, // RHEL-7.7
	}
)

// Load available ceph mounters installed on system into availableMounters
// Called from driver.go's Run().
func loadAvailableMounters(conf *util.Config) error {
	// #nosec
	fuseMounterProbe := exec.Command("ceph-fuse", "--version")
	// #nosec
	kernelMounterProbe := exec.Command("mount.ceph")

	err := kernelMounterProbe.Run()
	if err != nil {
		util.ErrorLogMsg("failed to run mount.ceph %v", err)
	} else {
		// fetch the current running kernel info
		release, kvErr := util.GetKernelVersion()
		if kvErr != nil {
			return kvErr
		}

		if conf.ForceKernelCephFS || util.CheckKernelSupport(release, quotaSupport) {
			util.DefaultLog("loaded mounter: %s", volumeMounterKernel)
			availableMounters = append(availableMounters, volumeMounterKernel)
		} else {
			util.DefaultLog("kernel version < 4.17 might not support quota feature, hence not loading kernel client")
		}
	}

	err = fuseMounterProbe.Run()
	if err != nil {
		util.ErrorLogMsg("failed to run ceph-fuse %v", err)
	} else {
		util.DefaultLog("loaded mounter: %s", volumeMounterFuse)
		availableMounters = append(availableMounters, volumeMounterFuse)
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

	// Verify that it's available

	var chosenMounter string

	for _, availMounter := range availableMounters {
		if availMounter == wantMounter {
			chosenMounter = wantMounter
			break
		}
	}

	if chosenMounter == "" {
		// Otherwise pick whatever is left
		chosenMounter = availableMounters[0]
		util.DebugLogMsg("requested mounter: %s, chosen mounter: %s", wantMounter, chosenMounter)
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
	}

	fmo := "nonempty"
	if volOptions.FuseMountOptions != "" {
		fmo += "," + strings.TrimSpace(volOptions.FuseMountOptions)
	}
	args = append(args, "-o", fmo)

	if volOptions.FsName != "" {
		args = append(args, "--client_mds_namespace="+volOptions.FsName)
	}

	_, stderr, err := util.ExecCommand(ctx, "ceph-fuse", args[:]...)
	if err != nil {
		return err
	}

	// Parse the output:
	// We need "starting fuse" meaning the mount is ok
	// and PID of the ceph-fuse daemon for unmount

	match := fusePidRx.FindSubmatch([]byte(stderr))
	// validMatchLength is set to 2 as match is expected
	// to have 2 items, starting fuse and PID of the fuse daemon
	const validMatchLength = 2
	if len(match) != validMatchLength {
		return fmt.Errorf("ceph-fuse failed: %s", stderr)
	}

	pid, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return fmt.Errorf("failed to parse FUSE daemon PID: %w", err)
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
	mdsNamespace := ""
	if volOptions.FsName != "" {
		mdsNamespace = fmt.Sprintf("mds_namespace=%s", volOptions.FsName)
	}
	optionsStr = util.MountOptionsAdd(optionsStr, mdsNamespace, volOptions.KernelMountOptions, netDev)

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
		return fmt.Errorf("failed to bind-mount %s to %s: %w", from, to, err)
	}

	if readOnly {
		mntOptionSli = util.MountOptionsAdd(mntOptionSli, "remount")
		if err := execCommandErr(ctx, "mount", "-o", mntOptionSli, to); err != nil {
			return fmt.Errorf("failed read-only remount of %s: %w", to, err)
		}
	}

	return nil
}

func unmountVolume(ctx context.Context, mountPoint string) error {
	if err := execCommandErr(ctx, "umount", mountPoint); err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf("exit status 32: umount: %s: not mounted", mountPoint)) ||
			strings.Contains(err.Error(), "No such file or directory") {
			return nil
		}
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
			util.WarningLog(ctx, "failed to find process %d: %v", pid, err)
		} else {
			if _, err = p.Wait(); err != nil {
				util.WarningLog(ctx, "%d is not a child process: %v", pid, err)
			}
		}
	}

	return nil
}
