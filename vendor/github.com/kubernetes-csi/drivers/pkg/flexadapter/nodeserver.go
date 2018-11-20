/*
Copyright 2017 The Kubernetes Authors.

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

package flexadapter

import (
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"

	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type nodeServer struct {
	flexDriver *flexVolumeDriver
	*csicommon.DefaultNodeServer
}

func mountDevice(devicePath, targetPath, fsType string, readOnly bool, mountOptions []string) error {
	var options []string

	if readOnly {
		options = append(options, "ro")
	} else {
		options = append(options, "rw")
	}
	options = append(options, mountOptions...)

	diskMounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: mount.NewOsExec()}

	return diskMounter.FormatAndMount(devicePath, targetPath, fsType, options)
}

func (ns *nodeServer) waitForAttach(req *csi.NodePublishVolumeRequest, fsType string) error {

	var dID string

	if req.GetPublishContext() != nil {
		var ok bool
		dID, ok = req.GetPublishContext()[deviceID]
		if !ok {
			return status.Error(codes.InvalidArgument, "Missing device ID")
		}
	} else {
		return status.Error(codes.InvalidArgument, "Missing publish info and device ID")
	}

	call := ns.flexDriver.NewDriverCall(waitForAttachCmd)
	call.Append(dID)
	call.AppendSpec(req.GetVolumeId(), fsType, req.GetReadonly(), req.GetVolumeContext())

	_, err := call.Run()
	if isCmdNotSupportedErr(err) {
		return nil
	}

	if err != nil {
		status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	targetPath := req.GetTargetPath()
	fsType := req.GetVolumeCapability().GetMount().GetFsType()

	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	var call *DriverCall

	// Attachable driver.
	if ns.flexDriver.capabilities.Attach {
		err = ns.waitForAttach(req, fsType)
		if err != nil {
			return nil, err
		}

		call = ns.flexDriver.NewDriverCall(mountDeviceCmd)
	} else {
		call = ns.flexDriver.NewDriverCall(mountCmd)
	}

	call.Append(req.GetTargetPath())

	if req.GetPublishContext() != nil {
		call.Append(req.GetPublishContext()[deviceID])
	}

	call.AppendSpec(req.GetVolumeId(), fsType, req.GetReadonly(), req.GetVolumeContext())
	_, err = call.Run()
	if isCmdNotSupportedErr(err) {
		mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
		err := mountDevice(req.VolumeContext[deviceID], targetPath, fsType, req.GetReadonly(), mountFlags)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func unmountDevice(path string) error {
	return util.UnmountPath(path, mount.New(""))
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {

	var call *DriverCall
	if ns.flexDriver.capabilities.Attach {
		call = ns.flexDriver.NewDriverCall(unmountDeviceCmd)
	} else {
		call = ns.flexDriver.NewDriverCall(unmountCmd)
	}
	call.Append(req.GetTargetPath())

	_, err := call.Run()
	if isCmdNotSupportedErr(err) {
		err := unmountDevice(req.GetTargetPath())
		return nil, status.Error(codes.Internal, err.Error())
	} else if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// WaitForDetach is ignored in current K8S plugins
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}
