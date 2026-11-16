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

package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func TestMountinfoAtomicReplacement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "record.json")
	for _, key := range []string{"initial-test-key", "rotated-test-key"} {
		info := &NodeStageMountinfo{
			StagingTargetPath: "/stage", VolumeContext: map[string]string{"mounter": "kernel"},
			Secrets: map[string]string{"userKey": key},
			VolumeCapability: &csi.VolumeCapability{AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{MountFlags: []string{"noatime"}},
			}},
		}
		record, err := info.toNodeStageMountinfoRecord()
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err = writeMountinfo(file, data); err != nil {
			t.Fatal(err)
		}
		data, err = os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved nodeStageMountinfoRecord
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		restored, err := saved.toNodeStageMountinfo()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restored.Secrets, info.Secrets) ||
			!reflect.DeepEqual(restored.VolumeContext, info.VolumeContext) ||
			restored.StagingTargetPath != info.StagingTargetPath ||
			restored.VolumeCapability.GetMount().GetMountFlags()[0] != "noatime" {
			t.Fatal("Stage inputs did not survive persistence")
		}
		stat, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if stat.Mode().Perm() != 0o600 {
			t.Fatalf("credentials permissions: %v", stat.Mode())
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal("temporary record leaked")
	}
}

func TestMountinfoConcurrentReaders(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "record.json")
	payloads := [][]byte{
		[]byte(`{"Secrets":{"userKey":"old"}}`),
		[]byte(fmt.Sprintf(`{"Secrets":{"userKey":%q}}`, bytes.Repeat([]byte("a"), 32768))),
	}
	if err := writeMountinfo(file, payloads[0]); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	errs := make(chan error, 1)
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(file)
			if err == nil && !bytes.Equal(data, payloads[0]) && !bytes.Equal(data, payloads[1]) {
				err = fmt.Errorf("reader observed an incomplete record (%d bytes)", len(data))
			}
			if err != nil {
				errs <- err

				return
			}
		}
	})
	for i := range 50 {
		if err := writeMountinfo(file, payloads[i%len(payloads)]); err != nil {
			t.Error(err)

			break
		}
	}
	close(stop)
	wg.Wait()
	select {
	case err := <-errs:
		t.Fatal(err)
	default:
	}
}

func TestMountinfoReplacementRestrictsPermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "record.json")
	prior := filepath.Join(dir, "prior.json")
	if err := os.WriteFile(file, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, prior); err != nil {
		t.Fatal(err)
	}
	if err := writeMountinfo(file, []byte("new")); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("replaced credentials permissions: %v", stat.Mode())
	}
	data, err := os.ReadFile(prior)
	if err != nil || string(data) != "old" {
		t.Fatalf("replacement modified the prior inode: %q, %v", data, err)
	}
}

func TestMountinfoFailedReplacement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "record.json")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "keep")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeMountinfo(target, []byte("new")); err == nil {
		t.Fatal("expected failure")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatal("failed replacement removed prior data")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("failed replacement leaked temporary record")
	}
	if err := writeMountinfo(filepath.Join(dir, "missing", "record"), nil); err == nil {
		t.Fatal("missing directory accepted")
	}
}

func TestLegacyFuseMountinfo(t *testing.T) {
	t.Parallel()
	record := &nodeStageMountinfoRecord{
		VolumeCapabilityProtoJSON: `{ "mount": {} }`, Secrets: map[string]string{"userKey": "test"},
	}
	info, err := record.toNodeStageMountinfo()
	if err != nil || info.VolumeCapability.GetMount() == nil || info.StagingTargetPath != "" ||
		info.Secrets["userKey"] != "test" {
		t.Fatalf("legacy FUSE record rejected: %v", err)
	}
}

func TestRemoveMountinfoCleansOnlyOwnRecords(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "nodestage-volume.json")
	removed := []string{filepath.Base(file), ".nodestage-volume.json.tmp-crash"}
	preserved := []string{
		"nodestage-other.json", ".nodestage-other.json.tmp-crash", ".nodestage-volume.json-other.tmp-crash",
	}
	for _, name := range append(append([]string{}, removed...), preserved...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test-credentials"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeMountinfo(file); err != nil {
		t.Fatal(err)
	}
	for _, name := range removed {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("own record %s remains: %v", name, err)
		}
	}
	for _, name := range preserved {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != "test-credentials" {
			t.Fatalf("other record %s changed: %v", name, err)
		}
	}
	if err := removeMountinfo(file); err != nil {
		t.Fatalf("repeated removal failed: %v", err)
	}
	if err := removeMountinfo(filepath.Join(dir, "missing-directory", "record")); err != nil {
		t.Fatalf("missing directory removal failed: %v", err)
	}
}
