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

package mounter

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ceph/ceph-csi/internal/cephfs/store"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

var (
	availableMounters []string

	//nolint:gomnd // numbers specify Kernel versions.
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

func execCommandErr(ctx context.Context, program string, args ...string) error {
	_, _, err := util.ExecCommand(ctx, program, args...)

	return err
}

// Load available ceph mounters installed on system into availableMounters
// Called from driver.go's Run().
func LoadAvailableMounters(conf *util.Config) error {
	// #nosec
	fuseMounterProbe := exec.Command("ceph-fuse", "--version")
	// #nosec
	kernelMounterProbe := exec.Command("mount.ceph")

	err := kernelMounterProbe.Run()
	if err != nil {
		log.ErrorLogMsg("failed to run mount.ceph %v", err)
	} else {
		// fetch the current running kernel info
		release, kvErr := util.GetKernelVersion()
		if kvErr != nil {
			return kvErr
		}

		if conf.ForceKernelCephFS || util.CheckKernelSupport(release, quotaSupport) {
			log.DefaultLog("loaded mounter: %s", volumeMounterKernel)
			availableMounters = append(availableMounters, volumeMounterKernel)
		} else {
			log.DefaultLog("kernel version < 4.17 might not support quota feature, hence not loading kernel client")
		}
	}

	err = fuseMounterProbe.Run()
	if err != nil {
		log.ErrorLogMsg("failed to run ceph-fuse %v", err)
	} else {
		log.DefaultLog("loaded mounter: %s", volumeMounterFuse)
		availableMounters = append(availableMounters, volumeMounterFuse)
	}

	if len(availableMounters) == 0 {
		return errors.New("no ceph mounters found on system")
	}

	return nil
}

type VolumeMounter interface {
	Mount(ctx context.Context, mountPoint string, cr *util.Credentials, volOptions *store.VolumeOptions) error
	Name() string
}

func New(volOptions *store.VolumeOptions) (VolumeMounter, error) {
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
		log.DebugLogMsg("requested mounter: %s, chosen mounter: %s", wantMounter, chosenMounter)
	}

	// Create the mounter
	switch chosenMounter {
	case volumeMounterFuse:
		return &FuseMounter{}, nil
	case volumeMounterKernel:
		return &KernelMounter{}, nil
	}

	return nil, fmt.Errorf("unknown mounter '%s'", chosenMounter)
}

func BindMount(ctx context.Context, from, to string, readOnly bool, mntOptions []string) error {
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
