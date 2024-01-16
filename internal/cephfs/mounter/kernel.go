/*
Copyright 2021 The Ceph-CSI Authors.

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
	"fmt"
	"os"
	"strings"

	"github.com/ceph/ceph-csi/internal/cephfs/store"
	"github.com/ceph/ceph-csi/internal/util"
)

const (
	volumeMounterKernel = "kernel"
	netDev              = "_netdev"
	kernelModule        = "ceph"
)

// testErrorf can be set by unit test for enhanced error reporting.
var testErrorf = func(fmt string, args ...any) { /* do nothing */ }

type KernelMounter interface {
	Mount(
		ctx context.Context,
		mountPoint string,
		cr *util.Credentials,
		volOptions *store.VolumeOptions,
	) error

	Name() string
}

type kernelMounter struct {
	// needsModprobe indicates that the ceph kernel module is not loaded in
	// the kernel yet (or compiled into it)
	needsModprobe bool
}

func NewKernelMounter() KernelMounter {
	return &kernelMounter{
		needsModprobe: !filesystemSupported(kernelModule),
	}
}

func (m *kernelMounter) mountKernel(
	ctx context.Context,
	mountPoint string,
	cr *util.Credentials,
	volOptions *store.VolumeOptions,
) error {
	if m.needsModprobe {
		if err := execCommandErr(ctx, "modprobe", kernelModule); err != nil {
			return err
		}

		m.needsModprobe = false
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

	var (
		stderr string
		err    error
	)

	if volOptions.NetNamespaceFilePath != "" {
		_, stderr, err = util.ExecuteCommandWithNSEnter(ctx, volOptions.NetNamespaceFilePath, "mount", args[:]...)
	} else {
		_, stderr, err = util.ExecCommand(ctx, "mount", args[:]...)
	}
	if err != nil {
		return fmt.Errorf("%w stderr: %s", err, stderr)
	}

	return err
}

func (m *kernelMounter) Mount(
	ctx context.Context,
	mountPoint string,
	cr *util.Credentials,
	volOptions *store.VolumeOptions,
) error {
	if err := util.CreateMountPoint(mountPoint); err != nil {
		return err
	}

	return m.mountKernel(ctx, mountPoint, cr, volOptions)
}

func (m *kernelMounter) Name() string { return "Ceph kernel client" }

// filesystemSupported checks if the passed name of the filesystem is included
// in /proc/filesystems.
func filesystemSupported(fs string) bool {
	// /proc/filesystems contains a list of all supported filesystems,
	// either compiled into the kernel, or as loadable module.
	data, err := os.ReadFile("/proc/filesystems")
	if err != nil {
		testErrorf("failed to read /proc/filesystems: %v", err)

		return false
	}

	// The format of /proc/filesystems is one filesystem per line, an
	// optional keyword ("nodev") followed by a tab and the name of the
	// filesystem. Matching <tab>ceph<eol> for the ceph kernel module that
	// supports CephFS.
	return strings.Contains(string(data), "\t"+fs+"\n")
}
