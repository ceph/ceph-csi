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
	"runtime/debug"
	"strings"
	"sync/atomic"

	"github.com/ceph/ceph-csi/internal/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	klog "k8s.io/klog/v2"
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

// NewDefaultIdentityServer initializes default identity servier.
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

// RunNodePublishServer starts node server.
func RunNodePublishServer(endpoint, hstOption string, d *CSIDriver, ns csi.NodeServer, m bool) {
	ids := NewDefaultIdentityServer(d)

	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, hstOption, ids, nil, ns, m)
	s.Wait()
}

// RunControllerPublishServer starts controller server.
func RunControllerPublishServer(endpoint, hstOption string, d *CSIDriver, cs csi.ControllerServer, m bool) {
	ids := NewDefaultIdentityServer(d)

	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, hstOption, ids, cs, nil, m)
	s.Wait()
}

// RunControllerandNodePublishServer starts both controller and node server.
func RunControllerandNodePublishServer(endpoint, hstOption string, d *CSIDriver, cs csi.ControllerServer, ns csi.NodeServer, m bool) {
	ids := NewDefaultIdentityServer(d)

	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, hstOption, ids, cs, ns, m)
	s.Wait()
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
	}
	return reqID
}

var id uint64

func contextIDInjector(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	atomic.AddUint64(&id, 1)
	ctx = context.WithValue(ctx, util.CtxKey, id)
	reqID := getReqID(req)
	if reqID != "" {
		ctx = context.WithValue(ctx, util.ReqID, reqID)
	}
	return handler(ctx, req)
}

func logGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	util.ExtendedLog(ctx, "GRPC call: %s", info.FullMethod)
	util.TraceLog(ctx, "GRPC request: %s", protosanitizer.StripSecrets(req))
	resp, err := handler(ctx, req)
	if err != nil {
		klog.Errorf(util.Log(ctx, "GRPC error: %v"), err)
	} else {
		util.TraceLog(ctx, "GRPC response: %s", protosanitizer.StripSecrets(resp))
	}
	return resp, err
}

func panicHandler(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("panic occurred: %v", r)
			debug.PrintStack()
			err = status.Errorf(codes.Internal, "panic %v", r)
		}
	}()
	return handler(ctx, req)
}
