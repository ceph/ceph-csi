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

package rbd

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"k8s.io/kubernetes/pkg/util/mount"

	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
}

var (
	pendingVols    = make(map[string]byte)
	pendingVolsMtx sync.Mutex
)

func isVolumePending(volID string) bool {
	pendingVolsMtx.Lock()
	defer pendingVolsMtx.Unlock()

	_, found := pendingVols[volID]
	return found
}

func markPending(volID string) {
	pendingVolsMtx.Lock()
	defer pendingVolsMtx.Unlock()

	pendingVols[volID] = 0
}

func unmarkPending(volID string) {
	pendingVolsMtx.Lock()
	defer pendingVolsMtx.Unlock()

	delete(pendingVols, volID)
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()

	if isVolumePending(req) {
		return nil, fmt.Errorf("rbd: NodePublishVolume for %s is pending", volumeID)
	}

	markPending(volumeID)
	defer unmarkPending(volumeID)

	if !strings.HasSuffix(targetPath, "/mount") {
		return nil, fmt.Errorf("rbd: malformed the value of target path: %s", targetPath)
	}
	s := strings.Split(strings.TrimSuffix(targetPath, "/mount"), "/")
	volName := s[len(s)-1]

	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(targetPath, 0750); err != nil {
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
	volOptions, err := getRBDVolumeOptions(req.VolumeAttributes)
	if err != nil {
		return nil, err
	}
	volOptions.VolName = volName
	// Mapping RBD image
	devicePath, err := attachRBDImage(volOptions)
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("rbd image: %s/%s was succesfully mapped at %s\n", volumeID, volOptions.Pool, devicePath)
	fsType := req.GetVolumeCapability().GetMount().GetFsType()

	readOnly := req.GetReadonly()
	attrib := req.GetVolumeAttributes()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()

	glog.V(4).Infof("target %v\nfstype %v\ndevice %v\nreadonly %v\nattributes %v\n mountflags %v\n",
		targetPath, fsType, devicePath, readOnly, attrib, mountFlags)

	options := []string{}
	if readOnly {
		options = append(options, "ro")
	}

	diskMounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: mount.NewOsExec()}
	if err := diskMounter.FormatAndMount(devicePath, targetPath, fsType, options); err != nil {
		return nil, err
	}
	// Storing volInfo into a persistent file
	if err := persistVolInfo(volumeID, path.Join(PluginFolder, "node"), volOptions); err != nil {
		glog.Warningf("rbd: failed to store volInfo with error: %v", err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()
	volOptions := &rbdVolumeOptions{}

	if isVolumePending(volumeID) {
		return nil, fmt.Errorf("rbd: NodeUnpublishVolume for %s is pending", volumeID)
	}

	markPending(volumeID)
	defer unmarkPending(volumeID)

	if err := loadVolInfo(volumeID, path.Join(PluginFolder, "node"), volOptions); err != nil {
		return nil, err
	}
	volName := volOptions.VolName

	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if notMnt {
		return nil, status.Error(codes.NotFound, "Volume not mounted")
	}
	// Unmounting the image
	err = mount.New("").Unmount(req.GetTargetPath())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	// Unmapping rbd device
	glog.V(4).Infof("deleting volume %s", volName)
	if err := detachRBDImage(volOptions); err != nil {
		glog.V(3).Infof("failed to unmap rbd device: %s with error: %v", volOptions.VolName, err)
		return nil, err
	}
	// Removing persistent storage file for the unmapped volume
	if err := deleteVolInfo(volumeID, path.Join(PluginFolder, "node")); err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}
