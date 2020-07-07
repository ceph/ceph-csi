/*
Copyright 2018 The Ceph-CSI Authors.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

type volumeID string

func execCommand(ctx context.Context, program string, args ...string) (stdout, stderr []byte, err error) {
	var (
		cmd           = exec.Command(program, args...) // nolint: gosec, #nosec
		sanitizedArgs = util.StripSecretInArgs(args)
		stdoutBuf     bytes.Buffer
		stderrBuf     bytes.Buffer
	)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	klog.V(4).Infof(util.Log(ctx, "cephfs: EXEC %s %s"), program, sanitizedArgs) // nolint:gomnd // number specifies log level

	if err := cmd.Run(); err != nil {
		if cmd.Process == nil {
			return nil, nil, fmt.Errorf("cannot get process pid while running %s %v: %w: %s",
				program, sanitizedArgs, err, stderrBuf.Bytes())
		}
		return nil, nil, fmt.Errorf("an error occurred while running (%d) %s %v: %w: %s",
			cmd.Process.Pid, program, sanitizedArgs, err, stderrBuf.Bytes())
	}

	return stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
}

func execCommandErr(ctx context.Context, program string, args ...string) error {
	_, _, err := execCommand(ctx, program, args...)
	return err
}

//nolint: unparam
func execCommandJSON(ctx context.Context, v interface{}, program string, args ...string) error {
	stdout, _, err := execCommand(ctx, program, args...)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(stdout, v); err != nil {
		return fmt.Errorf("failed to unmarshal JSON for %s %v: %s: %w", program, util.StripSecretInArgs(args), stdout, err)
	}

	return nil
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Controller service request validation.
func (cs *ControllerServer) validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("invalid CreateVolumeRequest: %w", err)
	}

	if req.GetName() == "" {
		return status.Error(codes.InvalidArgument, "volume Name cannot be empty")
	}

	reqCaps := req.GetVolumeCapabilities()
	if reqCaps == nil {
		return status.Error(codes.InvalidArgument, "volume Capabilities cannot be empty")
	}

	for _, cap := range reqCaps {
		if cap.GetBlock() != nil {
			return status.Error(codes.Unimplemented, "block volume not supported")
		}
	}

	return nil
}

func (cs *ControllerServer) validateDeleteVolumeRequest() error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("invalid DeleteVolumeRequest: %w", err)
	}

	return nil
}

// Controller expand volume request validation.
func (cs *ControllerServer) validateExpandVolumeRequest(req *csi.ControllerExpandVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_EXPAND_VOLUME); err != nil {
		return fmt.Errorf("invalid ExpandVolumeRequest: %w", err)
	}

	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return status.Error(codes.InvalidArgument, "CapacityRange cannot be empty")
	}

	return nil
}
