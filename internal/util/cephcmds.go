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
	"os"
	"os/exec"
	"time"

	"github.com/ceph/go-ceph/rados"

	"github.com/ceph/ceph-csi/internal/util/log"
	"github.com/ceph/ceph-csi/internal/util/stripsecrets"
)

const (
	// InvalidPoolID used to denote an invalid pool.
	InvalidPoolID     int64 = -1
	AutoBlocklistTime       = time.Second * 94670856  // 3 year in seconds
	MaxBlocklistTime        = time.Second * 157784760 // 5 years in seconds
)

// ExecuteCommandWithNSEnter executes passed in program with args with nsenter
// and returns separate stdout and stderr streams. In case ctx is not set to
// context.TODO(), the command will be logged after it was executed.
func ExecuteCommandWithNSEnter(ctx context.Context, netPath, program string, args ...string) (string, string, error) {
	var (
		stdoutBuf bytes.Buffer
		stderrBuf bytes.Buffer
		nsenter   = "nsenter"
	)

	// check netPath exists
	if _, err := os.Stat(netPath); err != nil {
		return "", "", fmt.Errorf("failed to get stat for %s %w", netPath, err)
	}
	//  nsenter --net=%s -- <program> <args>
	args = append([]string{"--net=" + netPath, "--", program}, args...)
	sanitizedArgs := stripsecrets.InArgs(args)
	cmd := exec.Command(nsenter, args...) // #nosec:G204, commands executing not vulnerable.
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if err != nil {
		err = fmt.Errorf("an error (%w) occurred while running %s args: %v", err, nsenter, sanitizedArgs)
		if ctx != context.TODO() {
			log.UsefulLog(ctx, "%s", err)
		}

		return stdout, stderr, err
	}

	if ctx != context.TODO() {
		log.UsefulLog(ctx, "command succeeded: %s %v", nsenter, sanitizedArgs)
	}

	return stdout, stderr, nil
}

// ExecCommand executes passed in program with args and returns separate stdout
// and stderr streams. In case ctx is not set to context.TODO(), the command
// will be logged after it was executed.
func ExecCommand(ctx context.Context, program string, args ...string) (string, string, error) {
	var (
		cmd           = exec.Command(program, args...) // #nosec:G204, commands executing not vulnerable.
		sanitizedArgs = stripsecrets.InArgs(args)
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
			log.UsefulLog(ctx, "%s", err)
		}

		return stdout, stderr, err
	}

	if ctx != context.TODO() {
		log.UsefulLog(ctx, "command succeeded: %s %v", program, sanitizedArgs)
	}

	return stdout, stderr, nil
}

// ExecCommandWithTimeout executes passed in program with args, timeout and
// returns separate stdout and stderr streams. If the command is not executed
// within given timeout, the process will be killed. In case ctx is not set to
// context.TODO(), the command will be logged after it was executed.
func ExecCommandWithTimeout(
	ctx context.Context,
	timeout time.Duration,
	program string,
	args ...string) (
	string,
	string,
	error,
) {
	var (
		sanitizedArgs = stripsecrets.InArgs(args)
		stdoutBuf     bytes.Buffer
		stderrBuf     bytes.Buffer
	)

	cctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, program, args...) // #nosec:G204, commands executing not vulnerable.
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()
	if err != nil {
		// if its a timeout log return context deadline exceeded error message
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("timeout: %w", cctx.Err())
		}
		err = fmt.Errorf("an error (%w) and stderror (%s) occurred while running %s args: %v",
			err,
			stderr,
			program,
			sanitizedArgs)

		if ctx != context.TODO() {
			log.ErrorLog(ctx, "%s", err)
		}

		return stdout, stderr, err
	}

	if ctx != context.TODO() {
		log.UsefulLog(ctx, "command succeeded: %s %v", program, sanitizedArgs)
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
	if errors.Is(err, rados.ErrNotFound) {
		return "", fmt.Errorf("%w: pool ID(%d) not found in Ceph cluster",
			ErrPoolNotFound, poolID)
	} else if err != nil {
		return "", fmt.Errorf("failed to get pool ID %d: %w", poolID, err)
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
			err = fmt.Errorf("Failed as %w (internal %w)", ErrObjectNotFound, err)
		}

		return err
	}
	defer ioctx.Destroy()

	if namespace != "" {
		ioctx.SetNamespace(namespace)
	}

	err = ioctx.Create(objectName, rados.CreateExclusive)
	if errors.Is(err, rados.ErrObjectExists) {
		return fmt.Errorf("Failed as %w (internal %w)", ErrObjectExists, err)
	} else if err != nil {
		log.ErrorLog(ctx, "failed creating omap (%s) in pool (%s): (%v)", objectName, poolName, err)

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
			err = fmt.Errorf("Failed as %w (internal %w)", ErrObjectNotFound, err)
		}

		return err
	}
	defer ioctx.Destroy()

	if namespace != "" {
		ioctx.SetNamespace(namespace)
	}

	err = ioctx.Delete(oMapName)
	if errors.Is(err, rados.ErrNotFound) {
		return fmt.Errorf("Failed as %w (internal %w)", ErrObjectNotFound, err)
	} else if err != nil {
		log.ErrorLog(ctx, "failed removing omap (%s) in pool (%s): (%v)", oMapName, poolName, err)

		return err
	}

	return nil
}

// AddCephBlocklist adds the IP to the Ceph blocklist.
func AddCephBlocklist(ctx context.Context, monitors string, cr *Credentials, ip string, useRange bool) error {
	arg := []string{
		"--id=" + cr.ID,
		"--keyfile=" + cr.KeyFile,
		"-m=" + monitors,
	}

	// TODO: add blocklist till infinity.
	// Currently, ceph does not provide the functionality to blocklist IPs
	// for infinite time. As a workaround, add a blocklist for 5 YEARS to
	// represent infinity from ceph-csi side.
	// At any point in this time, the IPs can be unblocked by an UnfenceClusterReq.
	// This needs to be updated once ceph provides functionality for the same.
	cmd := []string{"osd", "blocklist"}
	if useRange {
		cmd = append(cmd, "range")
	}
	cmd = append(cmd, "add", ip, MaxBlocklistTime.String())
	cmd = append(cmd, arg...)
	_, stderr, err := ExecCommand(ctx, "ceph", cmd...)
	if err != nil {
		return fmt.Errorf("failed to blocklist IP %q: %w stderr: %q", ip, err, stderr)
	}
	log.DebugLog(ctx, "blocklisted IP %q successfully", ip)

	return nil
}

// RemoveCephBlocklist removes the IP from the Ceph blocklist.
// The value of nonce is ignored if useRange is true.
func RemoveCephBlocklist(ctx context.Context, monitors string, cr *Credentials, ip, nonce string, useRange bool) error {
	arg := []string{
		"--id=" + cr.ID,
		"--keyfile=" + cr.KeyFile,
		"-m=" + monitors,
	}

	cmd := []string{"osd", "blocklist"}
	if useRange {
		cmd = append(cmd, "range")
	}

	// If nonce is not empty and we are not using
	// range based blocks, we need to add the nonce
	if nonce != "" && !useRange {
		cmd = append(cmd, "rm", fmt.Sprintf("%s:0/%s", ip, nonce))
	} else {
		cmd = append(cmd, "rm", ip)
	}

	cmd = append(cmd, arg...)

	_, stdErr, err := ExecCommand(ctx, "ceph", cmd...)
	if err != nil {
		return fmt.Errorf("failed to unblock IP %q: %v %w", ip, stdErr, err)
	}
	log.DebugLog(ctx, "unblocked IP %q successfully", ip)

	return nil
}
