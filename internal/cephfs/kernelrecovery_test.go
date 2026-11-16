/*
Copyright 2026 The Ceph-CSI Authors.

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
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/moby/sys/mountinfo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mount "k8s.io/mount-utils"

	"github.com/ceph/ceph-csi/internal/cephfs/mounter"
	fsutil "github.com/ceph/ceph-csi/internal/cephfs/util"
	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	hc "github.com/ceph/ceph-csi/internal/health-checker"
	"github.com/ceph/ceph-csi/internal/util"
)

func TestKernelStagingMissing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, fs string
		checkErr error
		missing  bool
		wantErr  bool
	}{
		{name: "absent", missing: true},
		{name: "directory removed", checkErr: os.ErrNotExist, missing: true},
		{name: "healthy", fs: "ceph"},
		{name: "wrong filesystem", fs: "xfs", wantErr: true},
		{name: "fuse conflict", fs: "fuse.ceph-fuse", wantErr: true},
		{name: "corrupt mount", checkErr: syscall.ENOTCONN, wantErr: true},
		{name: "inspection failed", checkErr: errors.New("unreadable"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := t.TempDir()
			fm := mount.NewFakeMounter(nil)
			if tc.fs != "" {
				fm.MountPoints = []mount.MountPoint{{Path: path, Type: tc.fs}}
			}
			if tc.checkErr != nil {
				fm.MountCheckErrors = map[string]error{path: tc.checkErr}
			}
			ns := recoveryNodeWithMountInfo(fm, mountInfoTable(path, tc.fs))
			missing, err := ns.kernelStagingMissing(path)
			if missing != tc.missing || (err != nil) != tc.wantErr {
				t.Fatalf("missing=%v err=%v", missing, err)
			}
			if len(fm.GetLog()) != 0 {
				t.Fatal("inspection changed mounts")
			}
		})
	}
}

func TestKernelStagingMissingEquivalentPaths(t *testing.T) {
	t.Parallel()
	for _, form := range []string{"trailing slash", "symlink"} {
		for _, fs := range []string{"ceph", "xfs"} {
			t.Run(form+"/"+fs, func(t *testing.T) {
				t.Parallel()
				dir, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "stage")
				if err = os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
				requestPath := path + "/"
				if form == "symlink" {
					requestPath = filepath.Join(dir, "alias")
					if err = os.Symlink(path, requestPath); err != nil {
						t.Fatal(err)
					}
				}
				fm := mount.NewFakeMounter([]mount.MountPoint{{Path: path, Type: fs}})
				ns := recoveryNodeWithMountInfo(fm, mountInfoTable(path, fs))
				missing, err := ns.kernelStagingMissing(requestPath)
				if missing {
					t.Fatal("existing mount reported missing")
				}
				if fs == "ceph" && err != nil {
					t.Fatalf("healthy staging rejected: %v", err)
				}
				if fs == "xfs" && status.Code(err) != codes.FailedPrecondition {
					t.Fatalf("conflicting filesystem accepted: %v", err)
				}
				if len(fm.GetLog()) != 0 {
					t.Fatal("inspection changed mounts")
				}
			})
		}
	}
}

// mountInfoTable builds a stack at the supplied (mountinfo-encoded) path.
func mountInfoTable(path string, filesystems ...string) string {
	var table strings.Builder
	for i, fs := range filesystems {
		fmt.Fprintf(&table, "%d %d 0:1 / %s rw - %s source rw\n", i+2, i+1, path, fs)
	}

	return table.String()
}

func recoveryNodeWithMountInfo(fm *mount.FakeMounter, table string) *cephfsNodeServer {
	return &cephfsNodeServer{
		DefaultNodeServer: &csicommon.DefaultNodeServer{Mounter: fm},
		getMounts: func(filter mountinfo.FilterFunc) ([]*mountinfo.Info, error) {
			return mountinfo.GetMountsFromReader(strings.NewReader(table), filter)
		},
	}
}

func TestKernelStagingMissingEscapedMountPaths(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, tablePath string
	}{
		{"space", "stage dir", `stage\040dir`},
		{"backslash", `stage\040dir`, `stage\134040dir`},
		{"tab and newline", "stage\tdir\n", `stage\011dir\012`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, tc.path)
			if err = os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			tablePath := filepath.Join(dir, tc.tablePath)
			fm := mount.NewFakeMounter([]mount.MountPoint{{Path: path, Type: "ceph"}})
			ns := recoveryNodeWithMountInfo(fm, mountInfoTable(tablePath, "ceph"))
			if missing, err := ns.kernelStagingMissing(path); missing || err != nil {
				t.Fatalf("healthy staging rejected: missing=%v err=%v", missing, err)
			}
			ns = recoveryNodeWithMountInfo(fm, mountInfoTable(tablePath, "xfs"))
			if _, err := ns.kernelStagingMissing(path); status.Code(err) != codes.FailedPrecondition ||
				status.Convert(err).Message() != "staging path contains a conflicting filesystem" {
				t.Fatalf("expected filesystem conflict, got %v", err)
			}
		})
	}
}

func TestKernelStagingMountInfo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		filesystems []string
		malformed   bool
		wantCode    codes.Code
	}{
		{name: "mount disappeared", wantCode: codes.FailedPrecondition},
		{name: "invalid mountinfo", malformed: true, wantCode: codes.Internal},
		{name: "stacked CephFS", filesystems: []string{"ceph", "ceph"}},
		{name: "XFS over CephFS", filesystems: []string{"ceph", "xfs"}, wantCode: codes.FailedPrecondition},
		{name: "CephFS over XFS", filesystems: []string{"xfs", "ceph"}, wantCode: codes.FailedPrecondition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := t.TempDir()
			table := mountInfoTable(path, tc.filesystems...) + fmt.Sprintf("99 2 1:1 / %s/child rw - xfs dev rw\n", path)
			if tc.malformed {
				table += "incomplete mountinfo entry"
			}
			fm := mount.NewFakeMounter([]mount.MountPoint{{Path: path, Type: "ceph"}})
			ns := recoveryNodeWithMountInfo(fm, table)
			missing, err := ns.kernelStagingMissing(path)
			if missing || status.Code(err) != tc.wantCode {
				t.Fatalf("missing=%v err=%v, want missing=false code=%v", missing, err, tc.wantCode)
			}
			if len(tc.filesystems) > 0 && tc.wantCode == codes.FailedPrecondition &&
				status.Convert(err).Message() != "staging path contains a conflicting filesystem" {
				t.Fatalf("expected a filesystem conflict, got %v", err)
			}
			if len(fm.GetLog()) != 0 {
				t.Fatal("inspection changed mounts")
			}
		})
	}
}

func TestKernelRecoveryRequest(t *testing.T) {
	t.Parallel()
	newRecord := func() *fsutil.NodeStageMountinfo {
		return &fsutil.NodeStageMountinfo{
			StagingTargetPath: "/stage", VolumeContext: map[string]string{"clusterID": "cluster", "mounter": "kernel"},
			Secrets: map[string]string{"userID": "test", "userKey": "test-only-key"},
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{MountFlags: []string{"noatime"}}},
				AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
			},
		}
	}
	for _, tc := range []struct {
		name    string
		change  func(*fsutil.NodeStageMountinfo)
		wantErr bool
	}{
		{name: "saved stage inputs"},
		{name: "legacy FUSE record", change: func(i *fsutil.NodeStageMountinfo) { i.StagingTargetPath = "" }, wantErr: true},
		{name: "different path", change: func(i *fsutil.NodeStageMountinfo) {
			i.StagingTargetPath = "/other"
		}, wantErr: true},
		{name: "missing capability", change: func(i *fsutil.NodeStageMountinfo) { i.VolumeCapability = nil }, wantErr: true},
		{name: "missing credentials", change: func(i *fsutil.NodeStageMountinfo) { i.Secrets = nil }, wantErr: true},
		{name: "changed cluster", change: func(i *fsutil.NodeStageMountinfo) {
			i.VolumeContext["clusterID"] = "old"
		}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info := newRecord()
			if tc.change != nil {
				tc.change(info)
			}
			req := &csi.NodePublishVolumeRequest{
				VolumeId: "volume", StagingTargetPath: "/stage", TargetPath: "/pod",
				VolumeCapability: newRecord().VolumeCapability,
				VolumeContext: map[string]string{
					"clusterID": "cluster", "mounter": "kernel", "csi.storage.k8s.io/pod.name": "new-pod",
				},
			}
			stage, err := kernelRecoveryRequest(req, info)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr {
				return
			}
			if stage.GetVolumeId() != req.GetVolumeId() || stage.GetStagingTargetPath() != "/stage" ||
				stage.GetSecrets()["userKey"] != "test-only-key" ||
				stage.GetVolumeCapability().GetMount().GetMountFlags()[0] != "noatime" {
				t.Fatal("recovery lost original Stage inputs")
			}
			if _, found := stage.GetVolumeContext()["csi.storage.k8s.io/pod.name"]; found {
				t.Fatal("Pod context leaked into Stage")
			}
		})
	}
	if _, err := kernelRecoveryRequest(&csi.NodePublishVolumeRequest{}, nil); err == nil {
		t.Fatal("accepted missing record")
	}
}

func TestKernelRecoveryPreservesPublishedTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stage, target := filepath.Join(dir, "stage"), filepath.Join(dir, "pod")
	for _, p := range []string{stage, target} {
		if err := os.Mkdir(p, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	fm := mount.NewFakeMounter([]mount.MountPoint{{Path: target, Type: "ceph", Device: "existing"}})
	ns := &cephfsNodeServer{DefaultNodeServer: &csicommon.DefaultNodeServer{Mounter: fm}}
	err := ns.tryRestoreKernelMountInNodePublish(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId: "volume", StagingTargetPath: stage, TargetPath: target,
	}, func(context.Context, *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
		t.Fatal("attempted to stage for an existing consumer")

		return nil, nil
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected rejection, got %v", err)
	}
	if len(fm.GetLog()) != 0 || len(fm.MountPoints) != 1 {
		t.Fatal("existing consumer changed")
	}
}

func recoveryInputs(stage, target string) (*csi.NodePublishVolumeRequest, *fsutil.NodeStageMountinfo) {
	capability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{
			FsType: "ceph", MountFlags: []string{"noatime"},
		}},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}
	volContext := map[string]string{"mounter": "kernel", "staticVolume": "true", "rootPath": "/volume"}

	return &csi.NodePublishVolumeRequest{
		VolumeId: "volume", StagingTargetPath: stage, TargetPath: target,
		VolumeCapability: capability, VolumeContext: volContext,
	}, &fsutil.NodeStageMountinfo{
		StagingTargetPath: stage, VolumeCapability: capability, VolumeContext: volContext,
		Secrets: map[string]string{"userID": "test", "userKey": "test-only-key"},
	}
}

func TestKernelRecoveryRejectsChangedInputs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*csi.NodePublishVolumeRequest)
	}{
		{"missing capability", func(r *csi.NodePublishVolumeRequest) { r.VolumeCapability = nil }},
		{"different filesystem", func(r *csi.NodePublishVolumeRequest) {
			r.GetVolumeCapability().GetMount().FsType = "ext4"
		}},
		{"different mount flags", func(r *csi.NodePublishVolumeRequest) {
			r.GetVolumeCapability().GetMount().MountFlags = []string{"ro"}
		}},
		{"different access mode", func(r *csi.NodePublishVolumeRequest) {
			r.GetVolumeCapability().AccessMode.Mode = csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER
		}},
		{"different root", func(r *csi.NodePublishVolumeRequest) { r.GetVolumeContext()["rootPath"] = "/other" }},
		{"removed option", func(r *csi.NodePublishVolumeRequest) { delete(r.GetVolumeContext(), "rootPath") }},
		{"added option", func(r *csi.NodePublishVolumeRequest) { r.GetVolumeContext()["encrypted"] = "true" }},
		{"unknown kubelet option", func(r *csi.NodePublishVolumeRequest) {
			r.GetVolumeContext()["csi.storage.k8s.io/unknown"] = "true"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, _ := recoveryInputs("/stage", "/target")
			_, info := recoveryInputs("/stage", "/target")
			tc.change(req)
			if _, err := kernelRecoveryRequest(req, info); status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("changed input accepted: %v", err)
			}
		})
	}
}

func TestKernelRecoveryAllowsPodContext(t *testing.T) {
	t.Parallel()
	req, _ := recoveryInputs("/stage", "/target")
	_, info := recoveryInputs("/stage", "/target")
	for _, key := range []string{
		"pod.name", "pod.namespace", "pod.uid", "serviceAccount.name", "ephemeral", "serviceAccount.tokens",
	} {
		req.GetVolumeContext()["csi.storage.k8s.io/"+key] = "new-pod-value"
	}
	stage, err := kernelRecoveryRequest(req, info)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stage.GetVolumeContext(), info.VolumeContext) {
		t.Fatal("recovery did not use original Stage context")
	}
}

func persistRecoveryInputs(t *testing.T, req *csi.NodePublishVolumeRequest, info *fsutil.NodeStageMountinfo) {
	t.Helper()
	if runtime.GOOS != "linux" || os.Getenv("CEPH_CSI_RUN_ALL_TESTS") == "" {
		t.Skip("set CEPH_CSI_RUN_ALL_TESTS to enable Linux tests requiring writable /csi/mountinfo")
	}
	if err := os.MkdirAll("/csi/mountinfo", 0o700); err != nil {
		t.Fatal(err)
	}
	req.VolumeId = "recovery-" + filepath.Base(filepath.Dir(t.TempDir()))
	if err := fsutil.WriteNodeStageMountinfo(fsutil.VolumeID(req.GetVolumeId()), info); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fsutil.RemoveNodeStageMountinfo(fsutil.VolumeID(req.GetVolumeId())); err != nil {
			t.Error(err)
		}
	})
}

func TestKernelRecoveryRetryPreservesOtherConsumers(t *testing.T) {
	t.Parallel()
	for _, uncertainMount := range []bool{false, true} {
		t.Run(map[bool]string{false: "mount failed", true: "mount result uncertain"}[uncertainMount], func(t *testing.T) {
			t.Parallel()
			testKernelRecoveryRetry(t, uncertainMount)
		})
	}
}

func testKernelRecoveryRetry(t *testing.T, uncertainMount bool) {
	t.Helper()
	dir := t.TempDir()
	req, info := recoveryInputs(filepath.Join(dir, "stage"), filepath.Join(dir, "new-pod"))
	for _, path := range []string{req.GetStagingTargetPath(), req.GetTargetPath()} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	persistRecoveryInputs(t, req, info)
	other := mount.MountPoint{Path: filepath.Join(dir, "existing-pod"), Type: "ceph", Device: "existing"}
	fm := mount.NewFakeMounter([]mount.MountPoint{other})
	ns := recoveryNodeWithMountInfo(fm, mountInfoTable(req.GetStagingTargetPath(), "ceph"))
	attempts := 0
	stage := func(_ context.Context, got *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
		attempts++
		if got.GetVolumeId() != req.GetVolumeId() || !reflect.DeepEqual(got.GetSecrets(), info.Secrets) ||
			got.GetStagingTargetPath() != req.GetStagingTargetPath() {
			t.Fatal("recovery did not reuse Stage inputs")
		}
		if uncertainMount || attempts > 1 {
			if err := fm.Mount("ceph", req.GetStagingTargetPath(), "ceph", nil); err != nil {
				t.Fatal(err)
			}
		}
		if attempts == 1 {
			return nil, status.Error(codes.Unavailable, "test mount failure")
		}

		return &csi.NodeStageVolumeResponse{}, nil
	}
	err := ns.tryRestoreKernelMountInNodePublish(context.Background(), req, stage)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("unexpected first attempt: %v", err)
	}
	assertRecoveryRecord(t, req.GetVolumeId())
	if err := ns.tryRestoreKernelMountInNodePublish(context.Background(), req, stage); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	wantAttempts := 2
	if uncertainMount {
		wantAttempts = 1
	}
	if attempts != wantAttempts {
		t.Fatalf("Stage attempts = %d, want %d", attempts, wantAttempts)
	}
	for _, action := range fm.GetLog() {
		if action.Action == mount.FakeActionUnmount {
			t.Fatalf("recovery unmounted a consumer: %+v", action)
		}
	}
	mounts, err := fm.List()
	if err != nil || len(mounts) != 2 || !reflect.DeepEqual(mounts[0], other) {
		t.Fatalf("other consumer changed: %+v, %v", mounts, err)
	}
}

func assertRecoveryRecord(t *testing.T, volumeID string) {
	t.Helper()
	if saved, err := fsutil.GetNodeStageMountinfo(fsutil.VolumeID(volumeID)); err != nil || saved == nil {
		t.Fatalf("failure lost recovery record: %v", err)
	}
}

func TestKernelRecoveryRecreatesStagingDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	req, info := recoveryInputs(filepath.Join(dir, "deleted-stage"), filepath.Join(dir, "pod"))
	if err := os.Mkdir(req.GetTargetPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	persistRecoveryInputs(t, req, info)
	fm := mount.NewFakeMounter(nil)
	ns := &cephfsNodeServer{DefaultNodeServer: &csicommon.DefaultNodeServer{Mounter: fm}}
	called := false
	err := ns.tryRestoreKernelMountInNodePublish(context.Background(), req,
		func(_ context.Context, stage *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
			called = true
			stat, err := os.Stat(stage.GetStagingTargetPath())
			if err != nil || !stat.IsDir() {
				t.Fatalf("staging directory not recreated before Stage: %v", err)
			}

			return &csi.NodeStageVolumeResponse{}, nil
		})
	if err != nil || !called {
		t.Fatalf("recovery skipped deleted staging directory: %v", err)
	}
}

func TestFuseRecoveryRejectsKernelRecord(t *testing.T) {
	t.Parallel()
	for _, mountedTarget := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty target", true: "existing consumer"}[mountedTarget], func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			req, info := recoveryInputs(filepath.Join(dir, "stage"), filepath.Join(dir, "pod"))
			for _, path := range []string{req.GetStagingTargetPath(), req.GetTargetPath()} {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			persistRecoveryInputs(t, req, info)
			var mounts []mount.MountPoint
			if mountedTarget {
				mounts = []mount.MountPoint{{Path: req.GetTargetPath(), Type: "fuse.ceph-fuse"}}
			}
			fm := mount.NewFakeMounter(mounts)
			ns := &cephfsNodeServer{DefaultNodeServer: &csicommon.DefaultNodeServer{Mounter: fm}}
			err := ns.tryRestoreFuseMountsInNodePublish(context.Background(), fsutil.VolumeID(req.GetVolumeId()),
				req.GetStagingTargetPath(), req.GetTargetPath(), req.GetVolumeContext())
			if err == nil {
				t.Fatal("FUSE recovery accepted a kernel record")
			}
			if len(fm.GetLog()) != 0 {
				t.Fatal("FUSE recovery changed mounts before rejecting the kernel record")
			}
		})
	}
}

func TestKernelPublishVolumeLock(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("CEPH_CSI_RUN_ALL_TESTS") == "" {
		t.Skip("set CEPH_CSI_RUN_ALL_TESTS to enable Linux mounter initialization")
	}
	probes := t.TempDir()
	for _, name := range []string{"mount.ceph", "ceph-fuse"} {
		if err := os.WriteFile(filepath.Join(probes, name), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", probes+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := mounter.LoadAvailableMounters(&util.Config{ForceKernelCephFS: true}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	req, info := recoveryInputs(filepath.Join(dir, "stage"), filepath.Join(dir, "pod"))
	if err := os.Mkdir(req.GetStagingTargetPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	persistRecoveryInputs(t, req, info)
	locks := util.NewIDLocker()
	ns := &cephfsNodeServer{
		DefaultNodeServer: &csicommon.DefaultNodeServer{Mounter: mount.NewFakeMounter(nil)},
		VolumeLocks:       locks, healthChecker: hc.NewHealthCheckManager(),
	}
	if !locks.TryAcquire(req.GetVolumeId()) {
		t.Fatal("cannot acquire test volume lock")
	}
	defer locks.Release(req.GetVolumeId())
	ctx := context.Background()
	if _, err := ns.NodePublishVolume(ctx, req); status.Code(err) != codes.Aborted {
		t.Fatalf("Publish bypassed volume lock: %v", err)
	}
	if _, err := ns.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
		VolumeId: req.GetVolumeId(), StagingTargetPath: req.GetStagingTargetPath(),
		VolumeContext: req.GetVolumeContext(), VolumeCapability: req.GetVolumeCapability(), Secrets: info.Secrets,
	}); status.Code(err) != codes.Aborted {
		t.Fatalf("Stage bypassed volume lock: %v", err)
	}
	if _, err := ns.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
		VolumeId: req.GetVolumeId(), StagingTargetPath: req.GetStagingTargetPath(),
	}); status.Code(err) != codes.Aborted {
		t.Fatalf("Unstage bypassed volume lock: %v", err)
	}
	if saved, err := fsutil.GetNodeStageMountinfo(fsutil.VolumeID(req.GetVolumeId())); err != nil || saved == nil {
		t.Fatalf("aborted Unstage removed recovery record: %v", err)
	}
	if !locks.TryAcquire(req.GetTargetPath()) {
		t.Fatal("Publish leaked target path lock")
	}
	locks.Release(req.GetTargetPath())
}
