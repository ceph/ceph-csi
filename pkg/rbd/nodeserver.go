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
	"os"
	"path"

	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/util/mount"

	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	clientSet *kubernetes.Clientset
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()

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
	volOptions, err := getRBDVolumeOptions(req.VolumeAttributes, ns.clientSet)
	if err != nil {
		return nil, err
	}

	// Mapping RBD image
	devicePath, err := attachRBDImage(req.GetVolumeId(), volOptions)
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("rbd image: %s/%s was succesfully mapped at %s\n", req.GetVolumeId(), volOptions.Pool, devicePath)
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
	// Storing rbd device path

	volOptions.ImageMapping = map[string]string{req.GetVolumeId(): devicePath}
	// Storing volInfo into a persistent file
	if err := persistVolInfo(req.GetVolumeId(), path.Join(PluginFolder, "node"), volOptions); err != nil {
		glog.Warningf("rbd: failed to store volInfo with error: %v", err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	volName := req.GetVolumeId()
	volOptions := &rbdVolumeOptions{}
	if err := loadVolInfo(volName, path.Join(PluginFolder, "node"), volOptions); err != nil {
		return nil, err
	}

	// Recover rbd secret key value, for now by k8s specific call
	id := volOptions.AdminID
	secretName := volOptions.AdminSecretName
	secretNamespace := volOptions.AdminSecretNamespace
	if id == "" {
		secretName = volOptions.UserSecretName
		secretNamespace = volOptions.UserSecretNamespace
	}
	if key, err := parseStorageClassSecret(secretName, secretNamespace, ns.clientSet); err != nil {
		return nil, err
	} else {
		volOptions.adminSecret = key
	}

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
	if err := detachRBDImage(volName, volOptions); err != nil {
		glog.V(3).Infof("failed to unmap rbd device: %s with error: %v", volOptions.ImageMapping[volName], err)
		return nil, err
	}
	// Removing persistent storage file for the unmapped volume
	if err := deleteVolInfo(volName, path.Join(PluginFolder, "node")); err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}
