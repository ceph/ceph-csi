/*
Copyright 2019 The Ceph-CSI Authors.

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
	"io/ioutil"
	"k8s.io/klog"
	"os"
	"os/exec"
	"strings"
)

func execCommand(program string, args ...string) (stdout, stderr []byte, err error) {
	var (
		cmd       = exec.Command(program, args...) // nolint: gosec
		stdoutBuf bytes.Buffer
		stderrBuf bytes.Buffer
	)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), fmt.Errorf("an error (%v)"+
			" occurred while running %s", err, program)
	}

	return stdoutBuf.Bytes(), nil, nil
}

// CephStoragePoolSummary strongly typed JSON spec for osd ls pools output
type CephStoragePoolSummary struct {
	Name   string `json:"poolname"`
	Number int    `json:"poolnum"`
}

// GetPoolID searches a list of pools in a cluster and returns the ID of the pool that matches
// the passed in poolName parameter
func GetPoolID(monitors string, adminID string, key string, poolName string) (uint32, error) {
	// ceph <options> -f json osd lspools
	// JSON out: [{"poolnum":<int>,"poolname":<string>}]

	stdout, _, err := execCommand(
		"ceph",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", CephConfigPath,
		"-f", "json",
		"osd", "lspools")
	if err != nil {
		klog.Errorf("failed getting pool list from cluster (%s)", err)
		return 0, err
	}

	var pools []CephStoragePoolSummary
	err = json.Unmarshal(stdout, &pools)
	if err != nil {
		klog.Errorf("failed to parse JSON output of pool list from cluster (%s)", err)
		return 0, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, string(stdout))
	}

	for _, p := range pools {
		if poolName == p.Name {
			return uint32(p.Number), nil
		}
	}

	return 0, fmt.Errorf("pool (%s) not found in Ceph cluster", poolName)
}

// GetPoolName lists all pools in a ceph cluster, and matches the pool whose pool ID is equal to
// the requested poolID parameter
func GetPoolName(monitors string, adminID string, key string, poolID uint32) (string, error) {
	// ceph <options> -f json osd lspools
	// [{"poolnum":1,"poolname":"replicapool"}]

	stdout, _, err := execCommand(
		"ceph",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", CephConfigPath,
		"-f", "json",
		"osd", "lspools")
	if err != nil {
		klog.Errorf("failed getting pool list from cluster (%s)", err)
		return "", err
	}

	var pools []CephStoragePoolSummary
	err = json.Unmarshal(stdout, &pools)
	if err != nil {
		klog.Errorf("failed to parse JSON output of pool list from cluster (%s)", err)
		return "", fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, string(stdout))
	}

	for _, p := range pools {
		if poolID == uint32(p.Number) {
			return p.Name, nil
		}
	}

	return "", fmt.Errorf("pool ID (%d) not found in Ceph cluster", poolID)
}

// SetOMapKeyValue sets the given key and value into the provided Ceph omap name
func SetOMapKeyValue(monitors, adminID, key, poolName, oMapName, oMapKey, keyValue string) error {
	// Command: "rados <options> setomapval oMapName oMapKey keyValue"

	_, _, err := execCommand(
		"rados",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", CephConfigPath,
		"-p", poolName,
		"setomapval", oMapName, oMapKey, keyValue)
	if err != nil {
		klog.Errorf("failed adding key (%s with value %s), to omap (%s) in "+
			"pool (%s): (%v)", oMapKey, keyValue, oMapName, poolName, err)
		return err
	}

	return nil
}

// ErrKeyNotFound is returned when requested key in omap is not found
type ErrKeyNotFound struct {
	keyName string
	err     error
}

func (e ErrKeyNotFound) Error() string {
	return e.err.Error()
}

// GetOMapValue gets the value for the given key from the named omap
func GetOMapValue(monitors, adminID, key, poolName, oMapName, oMapKey string) (string, error) {
	// Command: "rados <options> getomapval oMapName oMapKey <outfile>"
	// No such key: replicapool/csi.volumes.directory.default/csi.volname
	tmpFile, err := ioutil.TempFile("", "omap-get-")
	if err != nil {
		klog.Errorf("failed creating a temporary file for key contents")
		return "", err
	}
	defer tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	stdout, _, err := execCommand(
		"rados",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", CephConfigPath,
		"-p", poolName,
		"getomapval", oMapName, oMapKey, tmpFile.Name())
	if err != nil {
		// no logs, as attempting to check for key/value is done even on regular call sequences
		if strings.Contains(string(stdout), "No such key: "+poolName+"/"+oMapName+"/"+oMapKey) {
			return "", ErrKeyNotFound{poolName + "/" + oMapName + "/" + oMapKey, err}
		}
		return "", err
	}

	keyValue, err := ioutil.ReadAll(tmpFile)
	return string(keyValue), err
}

// RemoveOMapKey removes the omap key from the given omap name
func RemoveOMapKey(monitors, adminID, key, poolName, oMapName, oMapKey string) error {
	// Command: "rados <options> rmomapkey oMapName oMapKey"

	_, _, err := execCommand(
		"rados",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", CephConfigPath,
		"-p", poolName,
		"rmomapkey", oMapName, oMapKey)
	if err != nil {
		// NOTE: Missing omap key removal does not return an error
		klog.Errorf("failed removing key (%s), from omap (%s) in "+
			"pool (%s): (%v)", oMapKey, oMapName, poolName, err)
		return err
	}

	return nil
}

// ErrOMapNotFound is returned when named omap is not found in rados
type ErrOMapNotFound struct {
	oMapName string
	err      error
}

func (e ErrOMapNotFound) Error() string {
	return e.err.Error()
}

// RemoveOMap removes the entire omap name passed in and returns ErrOMapNotFound is provided omap
// is not found in rados
func RemoveOMap(monitors, adminID, key, poolName, oMapName string) error {
	// Command: "rados <options> rm oMapName"

	stdout, _, err := execCommand(
		"rados",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", CephConfigPath,
		"-p", poolName,
		"rm", oMapName)
	if err != nil {
		klog.Errorf("failed removing omap (%s) in pool (%s): (%v)", oMapName, poolName, err)
		if strings.Contains(string(stdout), "error removing "+poolName+">"+oMapName+
			": (2) No such file or directory") {
			return ErrOMapNotFound{oMapName, err}
		}
		return err
	}

	return nil
}

// RBDImageInfo strongly typed JSON spec for image info
type RBDImageInfo struct {
	ImageName string   `json:"name"`
	Size      int64    `json:"size"`
	Format    int64    `json:"format"`
	Features  []string `json:"features"`
	CreatedAt string   `json:"create_timestamp"`
}

// ErrImageNotFound is returned when image name is not found in the cluster on the given pool
type ErrImageNotFound struct {
	imageName string
	err       error
}

func (e ErrImageNotFound) Error() string {
	return e.err.Error()
}

// GetImageInfo queries rbd about the given image and returns its metadata, and returns
// ErrImageNotFound if provided image is not found
func GetImageInfo(monitors, adminID, key, poolName, imageName string) (RBDImageInfo, error) {
	// rbd --format=json info [image-spec | snap-spec]

	var imageInfo RBDImageInfo

	stdout, _, err := execCommand(
		"rbd",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", CephConfigPath,
		"--format="+"json",
		"info", poolName+"/"+imageName)
	if err != nil {
		klog.Errorf("failed getting information for image (%s): (%s)", poolName+"/"+imageName, err)
		if strings.Contains(string(stdout), "rbd: error opening image "+imageName+
			": (2) No such file or directory") {
			return imageInfo, ErrImageNotFound{imageName, err}
		}
		return imageInfo, err
	}

	err = json.Unmarshal(stdout, &imageInfo)
	if err != nil {
		klog.Errorf("failed to parse JSON output of image info (%s): (%s)",
			poolName+"/"+imageName, err)
		return imageInfo, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s",
			err, string(stdout))
	}

	return imageInfo, nil
}

// RBDSnapInfo strongly typed JSON spec for snap ls rbd output
type RBDSnapInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	TimeStamp string `json:"timestamp"`
}

// ErrSnapNotFound is returned when snap name passed is not found in the list of snapshots for the
// given image
type ErrSnapNotFound struct {
	snapName string
	err      error
}

func (e ErrSnapNotFound) Error() string {
	return e.err.Error()
}

/*
GetSnapInfo queries rbd about the snapshots of the given image and returns its metadata, and
returns ErrImageNotFound if provided image is not found, and ErrSnapNotFound if povided snap
is not found in the images snapshot list
*/
func GetSnapInfo(monitors, adminID, key, poolName, imageName, snapName string) (RBDSnapInfo, error) {
	// rbd --format=json snap ls [image-spec]

	var (
		snapInfo RBDSnapInfo
		snaps    []RBDSnapInfo
	)

	stdout, _, err := execCommand(
		"rbd",
		"-m", monitors,
		"--id", adminID,
		"--key="+key,
		"-c", CephConfigPath,
		"--format="+"json",
		"snap", "ls", poolName+"/"+imageName)
	if err != nil {
		klog.Errorf("failed getting snap (%s) information from image (%s): (%s)",
			snapName, poolName+"/"+imageName, err)
		if strings.Contains(string(stdout), "rbd: error opening image "+imageName+
			": (2) No such file or directory") {
			return snapInfo, ErrImageNotFound{imageName, err}
		}
		return snapInfo, err
	}

	err = json.Unmarshal(stdout, &snaps)
	if err != nil {
		klog.Errorf("failed to parse JSON output of image snap list (%s): (%s)",
			poolName+"/"+imageName, err)
		return snapInfo, fmt.Errorf("unmarshal failed: %+v. raw buffer response: %s",
			err, string(stdout))
	}

	for _, snap := range snaps {
		if snap.Name == snapName {
			return snap, nil
		}
	}

	return snapInfo, ErrSnapNotFound{snapName, fmt.Errorf("snap (%s) for image (%s) not found",
		snapName, poolName+"/"+imageName)}
}
