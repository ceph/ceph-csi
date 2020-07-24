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
	"errors"
	"fmt"
	"os/exec"

	"github.com/ceph/go-ceph/rados"
	klog "k8s.io/klog/v2"
)

// InvalidPoolID used to denote an invalid pool.
const InvalidPoolID int64 = -1

// ExecCommand executes passed in program with args and returns separate stdout
// and stderr streams. In case ctx is not set to context.TODO(), the command
// will be logged after it was executed.
func ExecCommand(ctx context.Context, program string, args ...string) (string, string, error) {
	var (
		cmd           = exec.Command(program, args...) // #nosec:G204, commands executing not vulnerable.
		sanitizedArgs = StripSecretInArgs(args)
		stdoutBuf     bytes.Buffer
		stderrBuf     bytes.Buffer
	)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if err != nil {
		err = fmt.Errorf("an error (%w) occurred while running %s args: %v", err, program, sanitizedArgs)
		if ctx != context.TODO() {
			UsefulLog(ctx, "%s", err)
		}
		return stdout, stderr, err
	}

	if ctx != context.TODO() {
		UsefulLog(ctx, "command succeeded: %s %v", program, sanitizedArgs)
	}

	return stdout, stderr, nil
}

// GetPoolID fetches the ID of the pool that matches the passed in poolName
// parameter.
func GetPoolID(monitors string, cr *Credentials, poolName string) (int64, error) {
	conn, err := connPool.Get(monitors, cr.ID, cr.KeyFile)
	if err != nil {
		return InvalidPoolID, err
	}
	defer connPool.Put(conn)

	id, err := conn.GetPoolByName(poolName)
	if errors.Is(err, rados.ErrNotFound) {
		return InvalidPoolID, fmt.Errorf("%w: pool (%s) not found in Ceph cluster",
			ErrPoolNotFound, poolName)
	} else if err != nil {
		return InvalidPoolID, err
	}

	return id, nil
}

// GetPoolName fetches the pool whose pool ID is equal to the requested poolID
// parameter.
func GetPoolName(monitors string, cr *Credentials, poolID int64) (string, error) {
	conn, err := connPool.Get(monitors, cr.ID, cr.KeyFile)
	if err != nil {
		return "", err
	}
	defer connPool.Put(conn)

	name, err := conn.GetPoolByID(poolID)
	if err != nil {
		return "", fmt.Errorf("%w: pool ID (%d) not found in Ceph cluster",
			ErrPoolNotFound, poolID)
	}
	return name, nil
}

// GetPoolIDs searches a list of pools in a cluster and returns the IDs of the pools that matches
// the passed in pools
// TODO this should take in a list and return a map[string(poolname)]int64(poolID).
func GetPoolIDs(ctx context.Context, monitors, journalPool, imagePool string, cr *Credentials) (int64, int64, error) {
	journalPoolID, err := GetPoolID(monitors, cr, journalPool)
	if err != nil {
		return InvalidPoolID, InvalidPoolID, err
	}

	imagePoolID := journalPoolID
	if imagePool != journalPool {
		imagePoolID, err = GetPoolID(monitors, cr, imagePool)
		if err != nil {
			return InvalidPoolID, InvalidPoolID, err
		}
	}

	return journalPoolID, imagePoolID, nil
}

// CreateObject creates the object name passed in and returns ErrObjectExists if the provided object
// is already present in rados.
func CreateObject(ctx context.Context, monitors string, cr *Credentials, poolName, namespace, objectName string) error {
	conn := ClusterConnection{}
	err := conn.Connect(monitors, cr)
	if err != nil {
		return err
	}
	defer conn.Destroy()

	ioctx, err := conn.GetIoctx(poolName)
	if err != nil {
		if errors.Is(err, ErrPoolNotFound) {
			err = JoinErrors(ErrObjectNotFound, err)
		}
		return err
	}
	defer ioctx.Destroy()

	if namespace != "" {
		ioctx.SetNamespace(namespace)
	}

	err = ioctx.Create(objectName, rados.CreateExclusive)
	if errors.Is(err, rados.ErrObjectExists) {
		return JoinErrors(ErrObjectExists, err)
	} else if err != nil {
		klog.Errorf(Log(ctx, "failed creating omap (%s) in pool (%s): (%v)"), objectName, poolName, err)
		return err
	}

	return nil
}

// RemoveObject removes the entire omap name passed in and returns ErrObjectNotFound is provided omap
// is not found in rados.
func RemoveObject(ctx context.Context, monitors string, cr *Credentials, poolName, namespace, oMapName string) error {
	conn := ClusterConnection{}
	err := conn.Connect(monitors, cr)
	if err != nil {
		return err
	}
	defer conn.Destroy()

	ioctx, err := conn.GetIoctx(poolName)
	if err != nil {
		if errors.Is(err, ErrPoolNotFound) {
			err = JoinErrors(ErrObjectNotFound, err)
		}
		return err
	}
	defer ioctx.Destroy()

	if namespace != "" {
		ioctx.SetNamespace(namespace)
	}

	err = ioctx.Delete(oMapName)
	if errors.Is(err, rados.ErrNotFound) {
		return JoinErrors(ErrObjectNotFound, err)
	} else if err != nil {
		klog.Errorf(Log(ctx, "failed removing omap (%s) in pool (%s): (%v)"), oMapName, poolName, err)
		return err
	}

	return nil
}
