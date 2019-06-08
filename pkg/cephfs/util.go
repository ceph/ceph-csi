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
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"

	"github.com/ceph/ceph-csi/pkg/util"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/utils/keymutex"
)

type volumeID string

func mustUnlock(m keymutex.KeyMutex, key string) {
	if err := m.UnlockKey(key); err != nil {
		klog.Fatalf("failed to unlock mutex for %s: %v", key, err)
	}
}

func execCommand(program string, args ...string) (stdout, stderr []byte, err error) {
	var (
		cmd           = exec.Command(program, args...) // nolint: gosec
		sanitizedArgs = util.StripSecretInArgs(args)
		stdoutBuf     bytes.Buffer
		stderrBuf     bytes.Buffer
	)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	klog.V(4).Infof("cephfs: EXEC %s %s", program, sanitizedArgs)

	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("an error occurred while running (%d) %s %v: %v: %s",
			cmd.Process.Pid, program, sanitizedArgs, err, stderrBuf.Bytes())
	}

	return stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
}

func execCommandErr(program string, args ...string) error {
	_, _, err := execCommand(program, args...)
	return err
}

//nolint: unparam
func execCommandJSON(v interface{}, program string, args ...string) error {
	stdout, _, err := execCommand(program, args...)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(stdout, v); err != nil {
		return fmt.Errorf("failed to unmarshal JSON for %s %v: %s: %v", program, util.StripSecretInArgs(args), stdout, err)
	}

	return nil
}

// Used in isMountPoint()
var dummyMount = mount.New("")

func isMountPoint(p string) (bool, error) {
	notMnt, err := dummyMount.IsLikelyNotMountPoint(p)
	if err != nil {
		return false, status.Error(codes.Internal, err.Error())
	}

	return !notMnt, nil
}

// Controller service request validation
func (cs *ControllerServer) validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("invalid CreateVolumeRequest: %v", err)
	}

	if req.GetName() == "" {
		return status.Error(codes.InvalidArgument, "Volume Name cannot be empty")
	}

	reqCaps := req.GetVolumeCapabilities()
	if reqCaps == nil {
		return status.Error(codes.InvalidArgument, "Volume Capabilities cannot be empty")
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
		return fmt.Errorf("invalid DeleteVolumeRequest: %v", err)
	}

	return nil
}

// Node service request validation
func validateNodeStageVolumeRequest(req *csi.NodeStageVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return errors.New("volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return errors.New("volume ID missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return errors.New("staging target path missing in request")
	}

	if req.GetSecrets() == nil || len(req.GetSecrets()) == 0 {
		return errors.New("stage secrets cannot be nil or empty")
	}

	return nil
}

func validateNodeUnstageVolumeRequest(req *csi.NodeUnstageVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return errors.New("volume ID missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return errors.New("staging target path missing in request")
	}

	return nil
}

func validateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return errors.New("volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return errors.New("volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return errors.New("target path missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return errors.New("staging target path missing in request")
	}

	return nil
}

func validateNodeUnpublishVolumeRequest(req *csi.NodeUnpublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return errors.New("volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return errors.New("target path missing in request")
	}

	return nil
}

// Controller expand volume request validation
func (cs *ControllerServer) validateExpandVolumeRequest(req *csi.ControllerExpandVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_EXPAND_VOLUME); err != nil {
		return fmt.Errorf("invalid ExpandVolumeRequest: %v", err)
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
