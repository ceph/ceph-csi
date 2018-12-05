/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"os/exec"

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/kubernetes/pkg/util/mount"
)

type volumeID string

func makeVolumeID(volName string) volumeID {
	return volumeID("csi-cephfs-" + volName)
}

func execCommand(command string, args ...string) ([]byte, error) {
	glog.V(4).Infof("cephfs: EXEC %s %s", command, args)

	cmd := exec.Command(command, args...)
	return cmd.CombinedOutput()
}

func execCommandAndValidate(program string, args ...string) error {
	out, err := execCommand(program, args...)
	if err != nil {
		return fmt.Errorf("cephfs: %s failed with following error: %s\ncephfs: %s output: %s", program, err, program, out)
	}

	return nil
}

func execCommandJson(v interface{}, program string, args ...string) error {
	out, err := execCommand(program, args...)

	if err != nil {
		return fmt.Errorf("cephfs: %s failed with following error: %s\ncephfs: %s output: %s", program, err, program, out)
	}

	return json.NewDecoder(bytes.NewReader(out)).Decode(v)
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

func storeCephCredentials(volId volumeID, cr *credentials) error {
	keyringData := cephKeyringData{
		UserId:   cr.id,
		Key:      cr.key,
		VolumeID: volId,
	}

	if err := keyringData.writeToFile(); err != nil {
		return err
	}

	secret := cephSecretData{
		UserId:   cr.id,
		Key:      cr.key,
		VolumeID: volId,
	}

	if err := secret.writeToFile(); err != nil {
		return err
	}

	return nil
}

//
// Controller service request validation
//

func (cs *controllerServer) validateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("invalid CreateVolumeRequest: %v", err)
	}

	if req.GetName() == "" {
		return status.Error(codes.InvalidArgument, "Volume Name cannot be empty")
	}

	if req.GetVolumeCapabilities() == nil {
		return status.Error(codes.InvalidArgument, "Volume Capabilities cannot be empty")
	}

	return nil
}

func (cs *controllerServer) validateDeleteVolumeRequest(req *csi.DeleteVolumeRequest) error {
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return fmt.Errorf("invalid DeleteVolumeRequest: %v", err)
	}

	return nil
}

//
// Node service request validation
//

func validateNodeStageVolumeRequest(req *csi.NodeStageVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return fmt.Errorf("volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return fmt.Errorf("volume ID missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return fmt.Errorf("staging target path missing in request")
	}

	if req.GetSecrets() == nil || len(req.GetSecrets()) == 0 {
		return fmt.Errorf("stage secrets cannot be nil or empty")
	}

	return nil
}

func validateNodeUnstageVolumeRequest(req *csi.NodeUnstageVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return fmt.Errorf("volume ID missing in request")
	}

	if req.GetStagingTargetPath() == "" {
		return fmt.Errorf("staging target path missing in request")
	}

	return nil
}

func validateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return fmt.Errorf("volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return fmt.Errorf("volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return fmt.Errorf("varget path missing in request")
	}

	return nil
}

func validateNodeUnpublishVolumeRequest(req *csi.NodeUnpublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return fmt.Errorf("volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return fmt.Errorf("target path missing in request")
	}

	return nil
}
