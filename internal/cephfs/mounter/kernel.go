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

	"github.com/ceph/ceph-csi/internal/cephfs/store"
	"github.com/ceph/ceph-csi/internal/util"
)

const (
	volumeMounterKernel = "kernel"
	netDev              = "_netdev"
)

type KernelMounter struct{}

func mountKernel(ctx context.Context, mountPoint string, cr *util.Credentials, volOptions *store.VolumeOptions) error {
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

func (m *KernelMounter) Mount(
	ctx context.Context,
	mountPoint string,
	cr *util.Credentials,
	volOptions *store.VolumeOptions,
) error {
	if err := util.CreateMountPoint(mountPoint); err != nil {
		return err
	}

	return mountKernel(ctx, mountPoint, cr, volOptions)
}

func (m *KernelMounter) Name() string { return "Ceph kernel client" }
