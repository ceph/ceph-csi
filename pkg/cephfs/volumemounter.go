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

	"golang.org/x/sys/unix"
	"k8s.io/klog"
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
)

// Version checking the running kernel and comparing it to known versions that
// have support for quota. Distributors of enterprise Linux have backported
// quota support to previous versions. This function checks if the running
// kernel is one of the versions that have the feature/fixes backported.
//
// `uname -r` (or Uname().Utsname.Release has a format like 1.2.3-rc.vendor
// This can be slit up in the following components:
// - version (1)
// - patchlevel (2)
// - sublevel (3) - optional, defaults to 0
// - extraversion (rc) - optional, matching integers only
// - distribution (.vendor) - optional, match against whole `uname -r` string
//
// For matching multiple versions, the kernelSupport type contains a backport
// bool, which will cause matching
// version+patchlevel+sublevel+(>=extraversion)+(~distribution)
//
// In case the backport bool is false, a simple check for higher versions than
// version+patchlevel+sublevel is done.
func kernelSupportsQuota(release string) bool {
	type kernelSupport struct {
		version      int
		patchlevel   int
		sublevel     int
		extraversion int    // prefix of the part after the first "-"
		distribution string // component of full extraversion
		backport     bool   // backports have a fixed version/patchlevel/sublevel
	}

	quotaSupport := []kernelSupport{
		{4, 17, 0, 0, "", false},       // standard 4.17+ versions
		{3, 10, 0, 1062, ".el7", true}, // RHEL-7.7
	}

	vers := strings.Split(strings.SplitN(release, "-", 2)[0], ".")
	version, err := strconv.Atoi(vers[0])
	if err != nil {
		klog.Errorf("failed to parse version from %s: %v", release, err)
		return false
	}
	patchlevel, err := strconv.Atoi(vers[1])
	if err != nil {
		klog.Errorf("failed to parse patchlevel from %s: %v", release, err)
		return false
	}
	sublevel := 0
	if len(vers) >= 3 {
		sublevel, err = strconv.Atoi(vers[2])
		if err != nil {
			klog.Errorf("failed to parse sublevel from %s: %v", release, err)
			return false
		}
	}
	extra := strings.SplitN(release, "-", 2)
	extraversion := 0
	if len(extra) == 2 {
		// ignore errors, 1st component of extraversion does not need to be an int
		extraversion, err = strconv.Atoi(strings.Split(extra[1], ".")[0])
		if err != nil {
			// "go lint" wants err to be checked...
			extraversion = 0
		}
	}

	// compare running kernel against known versions
	for _, kernel := range quotaSupport {
		if !kernel.backport {
			// deal with the default case(s), find >= match for version, patchlevel, sublevel
			if version > kernel.version || (version == kernel.version && patchlevel > kernel.patchlevel) ||
				(version == kernel.version && patchlevel == kernel.patchlevel && sublevel >= kernel.sublevel) {
				return true
			}
		} else {
			// specific backport, match distribution initially
			if !strings.Contains(release, kernel.distribution) {
				continue
			}

			// strict match version, patchlevel, sublevel, and >= match extraversion
			if version == kernel.version && patchlevel == kernel.patchlevel &&
				sublevel == kernel.sublevel && extraversion >= kernel.extraversion {
				return true
			}
		}
	}
	klog.Errorf("kernel %s does not support quota", release)
	return false
}

// Load available ceph mounters installed on system into availableMounters
// Called from driver.go's Run()
func loadAvailableMounters(conf *util.Config) error {
	// #nosec
	fuseMounterProbe := exec.Command("ceph-fuse", "--version")
	// #nosec
	kernelMounterProbe := exec.Command("mount.ceph")

	err := kernelMounterProbe.Run()
	if err == nil {
		// fetch the current running kernel info
		utsname := unix.Utsname{}
		err := unix.Uname(&utsname)
		if err != nil {
			return err
		}
		release := string(utsname.Release[:64])

		if conf.ForceKernelCephFS || kernelSupportsQuota(release) {
			klog.Infof("loaded mounter: %s", volumeMounterKernel)
			availableMounters = append(availableMounters, volumeMounterKernel)
		} else {
			klog.Infof("kernel version < 4.17 might not support quota feature, hence not loading kernel client")
		}
	}

	if fuseMounterProbe.Run() == nil {
		klog.Infof("loaded mounter: %s", volumeMounterFuse)
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
		klog.Infof("requested mounter: %s, chosen mounter: %s", wantMounter, chosenMounter)
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

	if volOptions.FuseMountOptions != "" {
		args = append(args, ","+volOptions.FuseMountOptions)
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
	if volOptions.KernelMountOptions != "" {
		optionsStr += fmt.Sprintf(",%s", volOptions.KernelMountOptions)
	}

	if !strings.Contains(volOptions.KernelMountOptions, netDev) {
		optionsStr += fmt.Sprintf(",%s", netDev)
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
			klog.Warningf(util.Log(ctx, "failed to find process %d: %v"), pid, err)
		} else {
			if _, err = p.Wait(); err != nil {
				klog.Warningf(util.Log(ctx, "%d is not a child process: %v"), pid, err)
			}
		}
	}

	return nil
}
