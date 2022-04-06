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
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ceph/ceph-csi/internal/cephfs/store"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	volumeMounterFuse = "fuse"

	cephEntityClientPrefix = "client."
)

var (

	// maps a mountpoint to PID of its FUSE daemon.
	fusePidMap    = make(map[string]int)
	fusePidMapMtx sync.Mutex

	fusePidRx = regexp.MustCompile(`(?m)^ceph-fuse\[(.+)\]: starting fuse$`)
)

type FuseMounter struct{}

func mountFuse(ctx context.Context, mountPoint string, cr *util.Credentials, volOptions *store.VolumeOptions) error {
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
	var (
		stderr string
		err    error
	)

	if volOptions.NetNamespaceFilePath != "" {
		_, stderr, err = util.ExecuteCommandWithNSEnter(ctx, volOptions.NetNamespaceFilePath, "ceph-fuse", args[:]...)
	} else {
		_, stderr, err = util.ExecCommand(ctx, "ceph-fuse", args[:]...)
	}

	if err != nil {
		return fmt.Errorf("%w stderr: %s", err, stderr)
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

func (m *FuseMounter) Mount(
	ctx context.Context,
	mountPoint string,
	cr *util.Credentials,
	volOptions *store.VolumeOptions,
) error {
	if err := util.CreateMountPoint(mountPoint); err != nil {
		return err
	}

	return mountFuse(ctx, mountPoint, cr, volOptions)
}

func (m *FuseMounter) Name() string { return "Ceph FUSE driver" }

func UnmountVolume(ctx context.Context, mountPoint string, opts ...string) error {
	if _, stderr, err := util.ExecCommand(ctx, "umount", append([]string{mountPoint}, opts...)...); err != nil {
		err = fmt.Errorf("%w stderr: %s", err, stderr)
		if strings.Contains(err.Error(), fmt.Sprintf("umount: %s: not mounted", mountPoint)) ||
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
			log.WarningLog(ctx, "failed to find process %d: %v", pid, err)
		} else {
			if _, err = p.Wait(); err != nil {
				log.WarningLog(ctx, "%d is not a child process: %v", pid, err)
			}
		}
	}

	return nil
}

func UnmountAll(ctx context.Context, mountPoint string) error {
	return UnmountVolume(ctx, mountPoint, "--all-targets")
}
