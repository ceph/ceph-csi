/*
Copyright 2022 The Ceph-CSI Authors.

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

package nodeserver

import (
	"errors"
	"fmt"
	"os"
	"strings"

	csicommon "github.com/ceph/ceph-csi/internal/csi-common"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mount "k8s.io/mount-utils"
	netutil "k8s.io/utils/net"
)

const (
	defaultMountPermission = os.FileMode(0o777)
	// Address of the NFS server.
	paramServer    = "server"
	paramShare     = "share"
	paramClusterID = "clusterID"
)

// NodeServer struct of ceph CSI driver with supported methods of CSI
// node server spec.
type NodeServer struct {
	csicommon.DefaultNodeServer
}

// NewNodeServer initialize a node server for ceph CSI driver.
func NewNodeServer(
	d *csicommon.CSIDriver,
	t string,
) *NodeServer {
	return &NodeServer{
		DefaultNodeServer: *csicommon.NewDefaultNodeServer(d, t, map[string]string{}),
	}
}

// NodePublishVolume mount the volume.
func (ns *NodeServer) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	err := validateNodePublishVolumeRequest(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volumeID := req.GetVolumeId()
	volCap := req.GetVolumeCapability()
	targetPath := req.GetTargetPath()
	mountOptions := volCap.GetMount().GetMountFlags()
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	source, err := getSource(req.GetVolumeContext())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	clusterID := req.GetVolumeContext()[paramClusterID]
	netNamespaceFilePath := ""
	if clusterID != "" {
		netNamespaceFilePath, err = util.GetNFSNetNamespaceFilePath(
			util.CsiConfigFile,
			clusterID)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	err = ns.mountNFS(ctx,
		volumeID,
		source,
		targetPath,
		netNamespaceFilePath,
		mountOptions)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		return nil, status.Error(codes.Internal, err.Error())
	}
	log.DebugLog(ctx, "nfs: successfully mounted volume %q mount %q to %q succeeded",
		volumeID, source, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmount the volume.
func (ns *NodeServer) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {
	err := util.ValidateNodeUnpublishVolumeRequest(req)
	if err != nil {
		return nil, err
	}

	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()
	log.DebugLog(ctx, "nfs: unmounting volume %s on %s", volumeID, targetPath)
	err = mount.CleanupMountPoint(targetPath, ns.Mounter, true)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount target %q: %v",
			targetPath, err)
	}
	log.DebugLog(ctx, "nfs: successfully unbounded volume %q from %q",
		volumeID, targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server.
func (ns *NodeServer) NodeGetCapabilities(
	ctx context.Context,
	req *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
					},
				},
			},
		},
	}, nil
}

// NodeGetVolumeStats get volume stats.
func (ns *NodeServer) NodeGetVolumeStats(
	ctx context.Context,
	req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {
	var err error
	targetPath := req.GetVolumePath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument,
			fmt.Sprintf("targetpath %v is empty", targetPath))
	}

	stat, err := os.Stat(targetPath)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument,
			"failed to get stat for targetpath %q: %v", targetPath, err)
	}

	if stat.Mode().IsDir() {
		return csicommon.FilesystemNodeGetVolumeStats(ctx, ns.Mounter, targetPath, false)
	}

	return nil, status.Errorf(codes.InvalidArgument,
		"targetpath %q is not a directory or device", targetPath)
}

// mountNFS mounts nfs volumes.
func (ns *NodeServer) mountNFS(
	ctx context.Context,
	volumeID, source, mountPoint, netNamespaceFilePath string,
	mountOptions []string,
) error {
	var (
		stderr string
		err    error
	)

	notMnt, err := ns.Mounter.IsLikelyNotMountPoint(mountPoint)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(mountPoint, defaultMountPermission)
			if err != nil {
				return err
			}
			notMnt = true
		} else {
			return err
		}
	}
	if !notMnt {
		log.DebugLog(ctx, "nfs: volume is already mounted to %s", mountPoint)

		return nil
	}

	args := []string{
		"-t", "nfs",
		source,
		mountPoint,
	}

	if len(mountOptions) > 0 {
		args = append(append(args, "-o"), mountOptions...)
	}

	log.DefaultLog("nfs: mounting volumeID(%v) source(%s) targetPath(%s) mountflags(%v)",
		volumeID, source, mountPoint, mountOptions)
	if netNamespaceFilePath != "" {
		_, stderr, err = util.ExecuteCommandWithNSEnter(
			ctx, netNamespaceFilePath, "mount", args...)
	} else {
		err = ns.Mounter.Mount(source, mountPoint, "nfs", mountOptions)
	}
	if err != nil {
		return fmt.Errorf("nfs: failed to mount %q to %q : %w stderr: %q",
			source, mountPoint, err, stderr)
	}
	if stderr != "" {
		return fmt.Errorf("nfs: failed to mount %q to %q : stderr %q",
			source, mountPoint, stderr)
	}

	return err
}

// validateNodePublishVolumeRequest validates node publish volume request.
func validateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	switch {
	case req.GetVolumeId() == "":
		return errors.New("volume ID missing in request")
	case req.GetVolumeCapability() == nil:
		return errors.New("volume capability missing in request")
	case req.GetTargetPath() == "":
		return errors.New("target path missing in request")
	}

	return nil
}

// getSource validates volume context, extracts and returns source.
// This function expects `server` and `share` parameters to be set
// and validates for the same.
func getSource(volContext map[string]string) (string, error) {
	server := volContext[paramServer]
	if server == "" {
		return "", fmt.Errorf("%v missing in request", paramServer)
	}
	baseDir := volContext[paramShare]
	if baseDir == "" {
		return "", fmt.Errorf("%v missing in request", paramShare)
	}

	if netutil.IsIPv6String(server) {
		// if server is IPv6, format to [IPv6].
		server = fmt.Sprintf("[%s]", server)
	}

	return fmt.Sprintf("%s:%s", server, baseDir), nil
}
