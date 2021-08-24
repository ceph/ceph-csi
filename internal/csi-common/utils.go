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

package csicommon

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync/atomic"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	rp "github.com/csi-addons/replication-lib-utils/protosanitizer"
	"github.com/csi-addons/spec/lib/go/replication"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/volume"
)

func parseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
	}

	return "", "", fmt.Errorf("invalid endpoint: %v", ep)
}

// NewVolumeCapabilityAccessMode returns volume access mode.
func NewVolumeCapabilityAccessMode(mode csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability_AccessMode {
	return &csi.VolumeCapability_AccessMode{Mode: mode}
}

// NewDefaultNodeServer initializes default node server.
func NewDefaultNodeServer(d *CSIDriver, t string, topology map[string]string) *DefaultNodeServer {
	d.topology = topology

	return &DefaultNodeServer{
		Driver: d,
		Type:   t,
	}
}

// NewDefaultIdentityServer initializes default identity server.
func NewDefaultIdentityServer(d *CSIDriver) *DefaultIdentityServer {
	return &DefaultIdentityServer{
		Driver: d,
	}
}

// NewDefaultControllerServer initializes default controller server.
func NewDefaultControllerServer(d *CSIDriver) *DefaultControllerServer {
	return &DefaultControllerServer{
		Driver: d,
	}
}

// NewControllerServiceCapability returns controller capabilities.
func NewControllerServiceCapability(ctrlCap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: ctrlCap,
			},
		},
	}
}

// Add replication request names to the list when we implement more API's.
func isReplicationRequest(req interface{}) bool {
	isReplicationRequest := true
	switch req.(type) {
	case *replication.EnableVolumeReplicationRequest:
	case *replication.DisableVolumeReplicationRequest:
	case *replication.PromoteVolumeRequest:
	case *replication.DemoteVolumeRequest:
	case *replication.ResyncVolumeRequest:
	default:
		isReplicationRequest = false
	}

	return isReplicationRequest
}

func getReqID(req interface{}) string {
	// if req is nil empty string will be returned
	reqID := ""
	switch r := req.(type) {
	case *csi.CreateVolumeRequest:
		reqID = r.Name

	case *csi.DeleteVolumeRequest:
		reqID = r.VolumeId

	case *csi.CreateSnapshotRequest:
		reqID = r.Name
	case *csi.DeleteSnapshotRequest:
		reqID = r.SnapshotId

	case *csi.ControllerExpandVolumeRequest:
		reqID = r.VolumeId

	case *csi.NodeStageVolumeRequest:
		reqID = r.VolumeId
	case *csi.NodeUnstageVolumeRequest:
		reqID = r.VolumeId

	case *csi.NodePublishVolumeRequest:
		reqID = r.VolumeId
	case *csi.NodeUnpublishVolumeRequest:
		reqID = r.VolumeId

	case *csi.NodeExpandVolumeRequest:
		reqID = r.VolumeId

	case *replication.EnableVolumeReplicationRequest:
		reqID = r.VolumeId
	case *replication.DisableVolumeReplicationRequest:
		reqID = r.VolumeId

	case *replication.PromoteVolumeRequest:
		reqID = r.VolumeId
	case *replication.DemoteVolumeRequest:
		reqID = r.VolumeId

	case *replication.ResyncVolumeRequest:
		reqID = r.VolumeId
	}

	return reqID
}

var id uint64

func contextIDInjector(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (resp interface{}, err error) {
	atomic.AddUint64(&id, 1)
	ctx = context.WithValue(ctx, log.CtxKey, id)
	if reqID := getReqID(req); reqID != "" {
		ctx = context.WithValue(ctx, log.ReqID, reqID)
	}

	return handler(ctx, req)
}

func logGRPC(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (interface{}, error) {
	log.ExtendedLog(ctx, "GRPC call: %s", info.FullMethod)
	if isReplicationRequest(req) {
		log.TraceLog(ctx, "GRPC request: %s", rp.StripReplicationSecrets(req))
	} else {
		log.TraceLog(ctx, "GRPC request: %s", protosanitizer.StripSecrets(req))
	}

	resp, err := handler(ctx, req)
	if err != nil {
		klog.Errorf(log.Log(ctx, "GRPC error: %v"), err)
	} else {
		log.TraceLog(ctx, "GRPC response: %s", protosanitizer.StripSecrets(resp))
	}

	return resp, err
}

func panicHandler(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("panic occurred: %v", r)
			debug.PrintStack()
			err = status.Errorf(codes.Internal, "panic %v", r)
		}
	}()

	return handler(ctx, req)
}

// FilesystemNodeGetVolumeStats can be used for getting the metrics as
// requested by the NodeGetVolumeStats CSI procedure.
// It is shared for FileMode volumes, both the CephFS and RBD NodeServers call
// this.
func FilesystemNodeGetVolumeStats(ctx context.Context, targetPath string) (*csi.NodeGetVolumeStatsResponse, error) {
	isMnt, err := util.IsMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.InvalidArgument, "targetpath %s does not exist", targetPath)
		}

		return nil, status.Error(codes.Internal, err.Error())
	}
	if !isMnt {
		return nil, status.Errorf(codes.InvalidArgument, "targetpath %s is not mounted", targetPath)
	}

	cephMetricsProvider := volume.NewMetricsStatFS(targetPath)
	volMetrics, volMetErr := cephMetricsProvider.GetMetrics()
	if volMetErr != nil {
		return nil, status.Error(codes.Internal, volMetErr.Error())
	}

	available, ok := (*(volMetrics.Available)).AsInt64()
	if !ok {
		log.ErrorLog(ctx, "failed to fetch available bytes")
	}
	capacity, ok := (*(volMetrics.Capacity)).AsInt64()
	if !ok {
		log.ErrorLog(ctx, "failed to fetch capacity bytes")

		return nil, status.Error(codes.Unknown, "failed to fetch capacity bytes")
	}
	used, ok := (*(volMetrics.Used)).AsInt64()
	if !ok {
		log.ErrorLog(ctx, "failed to fetch used bytes")
	}
	inodes, ok := (*(volMetrics.Inodes)).AsInt64()
	if !ok {
		log.ErrorLog(ctx, "failed to fetch available inodes")

		return nil, status.Error(codes.Unknown, "failed to fetch available inodes")
	}
	inodesFree, ok := (*(volMetrics.InodesFree)).AsInt64()
	if !ok {
		log.ErrorLog(ctx, "failed to fetch free inodes")
	}

	inodesUsed, ok := (*(volMetrics.InodesUsed)).AsInt64()
	if !ok {
		log.ErrorLog(ctx, "failed to fetch used inodes")
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: available,
				Total:     capacity,
				Used:      used,
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: inodesFree,
				Total:     inodes,
				Used:      inodesUsed,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}
