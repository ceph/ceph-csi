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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"k8s.io/klog"
)

// ExecCommand executes passed in program with args and returns separate stdout and stderr streams
func ExecCommand(program string, args ...string) (stdout, stderr []byte, err error) {
	var (
		cmd           = exec.Command(program, args...) // nolint: gosec
		sanitizedArgs = StripSecretInArgs(args)
		stdoutBuf     bytes.Buffer
		stderrBuf     bytes.Buffer
	)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), fmt.Errorf("an error (%v)"+
			" occurred while running %s args: %v", err, program, sanitizedArgs)
	}

	return stdoutBuf.Bytes(), nil, nil
}

// cephStoragePoolSummary strongly typed JSON spec for osd ls pools output
type cephStoragePoolSummary struct {
	Name   string `json:"poolname"`
	Number int64  `json:"poolnum"`
}

// GetPools fetches a list of pools from a cluster
func getPools(ctx context.Context, monitors string, cr *Credentials) ([]cephStoragePoolSummary, error) {
	// ceph <options> -f json osd lspools
	// JSON out: [{"poolnum":<int64>,"poolname":<string>}]

	stdout, _, err := ExecCommand(
		"ceph",
		"-m", monitors,
		"--id", cr.ID,
		"--keyfile="+cr.KeyFile,
		"-c", CephConfigPath,
		"-f", "json",
		"osd", "lspools")
	if err != nil {
		klog.Errorf(Log(ctx, "failed getting pool list from cluster (%s)"), err)
		return nil, err
	}

	var pools []cephStoragePoolSummary
	err = json.Unmarshal(stdout, &pools)
	if err != nil {
		klog.Errorf(Log(ctx, "failed to parse JSON output of pool list from cluster (%s)"), err)
		return nil, fmt.Errorf("unmarshal of pool list failed: %+v.  raw buffer response: %s", err, string(stdout))
	}

	return pools, nil
}

// GetPoolID searches a list of pools in a cluster and returns the ID of the pool that matches
// the passed in poolName parameter
func GetPoolID(ctx context.Context, monitors string, cr *Credentials, poolName string) (int64, error) {
	pools, err := getPools(ctx, monitors, cr)
	if err != nil {
		return 0, err
	}

	for _, p := range pools {
		if poolName == p.Name {
			return p.Number, nil
		}
	}

	return 0, fmt.Errorf("pool (%s) not found in Ceph cluster", poolName)
}

// GetPoolName lists all pools in a ceph cluster, and matches the pool whose pool ID is equal to
// the requested poolID parameter
func GetPoolName(ctx context.Context, monitors string, cr *Credentials, poolID int64) (string, error) {
	pools, err := getPools(ctx, monitors, cr)
	if err != nil {
		return "", err
	}

	for _, p := range pools {
		if poolID == p.Number {
			return p.Name, nil
		}
	}

	return "", fmt.Errorf("pool ID (%d) not found in Ceph cluster", poolID)
}

// SetOMapKeyValue sets the given key and value into the provided Ceph omap name
func SetOMapKeyValue(ctx context.Context, monitors string, cr *Credentials, poolName, namespace, oMapName, oMapKey, keyValue string) error {
	// Command: "rados <options> setomapval oMapName oMapKey keyValue"
	args := []string{
		"-m", monitors,
		"--id", cr.ID,
		"--keyfile=" + cr.KeyFile,
		"-c", CephConfigPath,
		"-p", poolName,
		"setomapval", oMapName, oMapKey, keyValue,
	}

	if namespace != "" {
		args = append(args, "--namespace="+namespace)
	}

	_, _, err := ExecCommand("rados", args[:]...)
	if err != nil {
		klog.Errorf(Log(ctx, "failed adding key (%s with value %s), to omap (%s) in "+
			"pool (%s): (%v)"), oMapKey, keyValue, oMapName, poolName, err)
		return err
	}

	return nil
}

// GetOMapValue gets the value for the given key from the named omap
func GetOMapValue(ctx context.Context, monitors string, cr *Credentials, poolName, namespace, oMapName, oMapKey string) (string, error) {
	// Command: "rados <options> getomapval oMapName oMapKey <outfile>"
	// No such key: replicapool/csi.volumes.directory.default/csi.volname
	tmpFile, err := ioutil.TempFile("", "omap-get-")
	if err != nil {
		klog.Errorf(Log(ctx, "failed creating a temporary file for key contents"))
		return "", err
	}
	defer tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	args := []string{
		"-m", monitors,
		"--id", cr.ID,
		"--keyfile=" + cr.KeyFile,
		"-c", CephConfigPath,
		"-p", poolName,
		"getomapval", oMapName, oMapKey, tmpFile.Name(),
	}

	if namespace != "" {
		args = append(args, "--namespace="+namespace)
	}

	stdout, stderr, err := ExecCommand("rados", args[:]...)
	if err != nil {
		// no logs, as attempting to check for non-existent key/value is done even on
		// regular call sequences
		stdoutanderr := strings.Join([]string{string(stdout), string(stderr)}, " ")
		if strings.Contains(stdoutanderr, "No such key: "+poolName+"/"+oMapName+"/"+oMapKey) {
			return "", ErrKeyNotFound{poolName + "/" + oMapName + "/" + oMapKey, err}
		}

		if strings.Contains(stdoutanderr, "error getting omap value "+
			poolName+"/"+oMapName+"/"+oMapKey+": (2) No such file or directory") {
			return "", ErrKeyNotFound{poolName + "/" + oMapName + "/" + oMapKey, err}
		}

		// log other errors for troubleshooting assistance
		klog.Errorf(Log(ctx, "failed getting omap value for key (%s) from omap (%s) in pool (%s): (%v)"),
			oMapKey, oMapName, poolName, err)

		return "", fmt.Errorf("error (%v) occurred, command output streams is (%s)",
			err.Error(), stdoutanderr)
	}

	keyValue, err := ioutil.ReadAll(tmpFile)
	return string(keyValue), err
}

// RemoveOMapKey removes the omap key from the given omap name
func RemoveOMapKey(ctx context.Context, monitors string, cr *Credentials, poolName, namespace, oMapName, oMapKey string) error {
	// Command: "rados <options> rmomapkey oMapName oMapKey"
	args := []string{
		"-m", monitors,
		"--id", cr.ID,
		"--keyfile=" + cr.KeyFile,
		"-c", CephConfigPath,
		"-p", poolName,
		"rmomapkey", oMapName, oMapKey,
	}

	if namespace != "" {
		args = append(args, "--namespace="+namespace)
	}

	_, _, err := ExecCommand("rados", args[:]...)
	if err != nil {
		// NOTE: Missing omap key removal does not return an error
		klog.Errorf(Log(ctx, "failed removing key (%s), from omap (%s) in "+
			"pool (%s): (%v)"), oMapKey, oMapName, poolName, err)
		return err
	}

	return nil
}

// CreateObject creates the object name passed in and returns ErrObjectExists if the provided object
// is already present in rados
func CreateObject(ctx context.Context, monitors string, cr *Credentials, poolName, namespace, objectName string) error {
	// Command: "rados <options> create objectName"
	args := []string{
		"-m", monitors,
		"--id", cr.ID,
		"--keyfile=" + cr.KeyFile,
		"-c", CephConfigPath,
		"-p", poolName,
		"create", objectName,
	}

	if namespace != "" {
		args = append(args, "--namespace="+namespace)
	}

	_, stderr, err := ExecCommand("rados", args[:]...)
	if err != nil {
		klog.Errorf(Log(ctx, "failed creating omap (%s) in pool (%s): (%v)"), objectName, poolName, err)
		if strings.Contains(string(stderr), "error creating "+poolName+"/"+objectName+
			": (17) File exists") {
			return ErrObjectExists{objectName, err}
		}
		return err
	}

	return nil
}

// RemoveObject removes the entire omap name passed in and returns ErrObjectNotFound is provided omap
// is not found in rados
func RemoveObject(ctx context.Context, monitors string, cr *Credentials, poolName, namespace, oMapName string) error {
	// Command: "rados <options> rm oMapName"
	args := []string{
		"-m", monitors,
		"--id", cr.ID,
		"--keyfile=" + cr.KeyFile,
		"-c", CephConfigPath,
		"-p", poolName,
		"rm", oMapName,
	}

	if namespace != "" {
		args = append(args, "--namespace="+namespace)
	}

	_, stderr, err := ExecCommand("rados", args[:]...)
	if err != nil {
		klog.Errorf(Log(ctx, "failed removing omap (%s) in pool (%s): (%v)"), oMapName, poolName, err)
		if strings.Contains(string(stderr), "error removing "+poolName+">"+oMapName+
			": (2) No such file or directory") {
			return ErrObjectNotFound{oMapName, err}
		}
		return err
	}

	return nil
}
