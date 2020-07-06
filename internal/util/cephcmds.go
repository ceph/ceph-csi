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
	"k8s.io/klog"
)

// InvalidPoolID used to denote an invalid pool
const InvalidPoolID int64 = -1

// ExecCommand executes passed in program with args and returns separate stdout and stderr streams
func ExecCommand(program string, args ...string) (stdout, stderr []byte, err error) {
	var (
		cmd           = exec.Command(program, args...) // nolint: gosec, #nosec
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

// GetPoolID fetches the ID of the pool that matches the passed in poolName
// parameter
func GetPoolID(monitors string, cr *Credentials, poolName string) (int64, error) {
	conn, err := connPool.Get(monitors, cr.ID, cr.KeyFile)
	if err != nil {
		return InvalidPoolID, err
	}
	defer connPool.Put(conn)

	id, err := conn.GetPoolByName(poolName)
	if errors.Is(err, rados.ErrNotFound) {
		return InvalidPoolID, ErrPoolNotFound{poolName, fmt.Errorf("pool (%s) not found in Ceph cluster", poolName)}
	} else if err != nil {
		return InvalidPoolID, err
	}

	return id, nil
}

// GetPoolName fetches the pool whose pool ID is equal to the requested poolID
// parameter
func GetPoolName(monitors string, cr *Credentials, poolID int64) (string, error) {
	conn, err := connPool.Get(monitors, cr.ID, cr.KeyFile)
	if err != nil {
		return "", err
	}
	defer connPool.Put(conn)

	name, err := conn.GetPoolByID(poolID)
	if err != nil {
		return "", ErrPoolNotFound{string(poolID), fmt.Errorf("pool ID (%d) not found in Ceph cluster", poolID)}
	}
	return name, nil
}

// GetPoolIDs searches a list of pools in a cluster and returns the IDs of the pools that matches
// the passed in pools
// TODO this should take in a list and return a map[string(poolname)]int64(poolID)
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
// is already present in rados
func CreateObject(ctx context.Context, monitors string, cr *Credentials, poolName, namespace, objectName string) error {
	conn := ClusterConnection{}
	err := conn.Connect(monitors, cr)
	if err != nil {
		return err
	}
	defer conn.Destroy()

	ioctx, err := conn.GetIoctx(poolName)
	if err != nil {
		var epnf ErrPoolNotFound
		if errors.As(err, &epnf) {
			err = ErrObjectNotFound{poolName, err}
		}
		return err
	}
	defer ioctx.Destroy()

	if namespace != "" {
		ioctx.SetNamespace(namespace)
	}

	err = ioctx.Create(objectName, rados.CreateExclusive)
	if errors.Is(err, rados.ErrObjectExists) {
		return ErrObjectExists{objectName, err}
	} else if err != nil {
		klog.Errorf(Log(ctx, "failed creating omap (%s) in pool (%s): (%v)"), objectName, poolName, err)
		return err
	}

	return nil
}

// RemoveObject removes the entire omap name passed in and returns ErrObjectNotFound is provided omap
// is not found in rados
func RemoveObject(ctx context.Context, monitors string, cr *Credentials, poolName, namespace, oMapName string) error {
	conn := ClusterConnection{}
	err := conn.Connect(monitors, cr)
	if err != nil {
		return err
	}
	defer conn.Destroy()

	ioctx, err := conn.GetIoctx(poolName)
	if err != nil {
		var epnf ErrPoolNotFound
		if errors.As(err, &epnf) {
			err = ErrObjectNotFound{poolName, err}
		}
		return err
	}
	defer ioctx.Destroy()

	if namespace != "" {
		ioctx.SetNamespace(namespace)
	}

	err = ioctx.Delete(oMapName)
	if errors.Is(err, rados.ErrNotFound) {
		return ErrObjectNotFound{oMapName, err}
	} else if err != nil {
		klog.Errorf(Log(ctx, "failed removing omap (%s) in pool (%s): (%v)"), oMapName, poolName, err)
		return err
	}

	return nil
}
