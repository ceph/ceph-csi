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
	"time"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/csi-addons/spec/lib/go/replication"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/volume"
	mount "k8s.io/mount-utils"
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
func NewDefaultNodeServer(
	d *CSIDriver, t, cliReadAffinityMapOptions string,
	topology, nodeLabels map[string]string,
) *DefaultNodeServer {
	d.topology = topology

	return &DefaultNodeServer{
		Driver:                 d,
		Type:                   t,
		Mounter:                mount.NewWithoutSystemd(""),
		NodeLabels:             nodeLabels,
		CLIReadAffinityOptions: cliReadAffinityMapOptions,
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

// NewGroupControllerServiceCapability returns group controller capabilities.
func NewGroupControllerServiceCapability(ctrlCap csi.GroupControllerServiceCapability_RPC_Type,
) *csi.GroupControllerServiceCapability {
	return &csi.GroupControllerServiceCapability{
		Type: &csi.GroupControllerServiceCapability_Rpc{
			Rpc: &csi.GroupControllerServiceCapability_RPC{
				Type: ctrlCap,
			},
		},
	}
}

// MiddlewareServerOptionConfig contains configuration parameters
// that are passed to the respective middleware interceptors that
// are instantiated when starting gRPC servers.
type MiddlewareServerOptionConfig struct {
	LogSlowOpInterval time.Duration
}

// NewMiddlewareServerOption creates a new grpc.ServerOption that configures a
// common format for log messages and other gRPC related handlers.
func NewMiddlewareServerOption(config MiddlewareServerOptionConfig) grpc.ServerOption {
	middleWare := []grpc.UnaryServerInterceptor{
		contextIDInjector,
		logGRPC,
	}

	if config.LogSlowOpInterval > 0 {
		middleWare = append(middleWare, func(
			ctx context.Context,
			req interface{},
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (interface{}, error) {
			return logSlowGRPC(
				config.LogSlowOpInterval, ctx, req, info, handler,
			)
		})
	}

	middleWare = append(middleWare, panicHandler)

	return grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(middleWare...))
}

// GetIDFromReplication returns the volumeID for Replication.
func GetIDFromReplication(req interface{}) string {
	getID := func(r interface {
		GetVolumeId() string
		GetReplicationSource() *replication.ReplicationSource
	},
	) string {
		reqID := ""
		src := r.GetReplicationSource()
		if src != nil && src.GetVolume() != nil {
			reqID = src.GetVolume().GetVolumeId()
		}
		if reqID == "" {
			if src != nil && src.GetVolumegroup() != nil {
				reqID = src.GetVolumegroup().GetVolumeGroupId()
			}
		}
		if reqID == "" {
			reqID = r.GetVolumeId() //nolint:nolintlint,staticcheck // req.VolumeId is deprecated
		}

		return reqID
	}

	switch r := req.(type) {
	case *replication.EnableVolumeReplicationRequest:
		return getID(r)
	case *replication.DisableVolumeReplicationRequest:
		return getID(r)
	case *replication.PromoteVolumeRequest:
		return getID(r)
	case *replication.DemoteVolumeRequest:
		return getID(r)
	case *replication.ResyncVolumeRequest:
		return getID(r)
	case *replication.GetVolumeReplicationInfoRequest:
		return getID(r)
	default:
		return ""
	}
}

func getReqID(req interface{}) string {
	// if req is nil empty string will be returned
	reqID := ""
	switch r := req.(type) {
	case *csi.CreateVolumeRequest:
		reqID = r.GetName()

	case *csi.DeleteVolumeRequest:
		reqID = r.GetVolumeId()

	case *csi.CreateSnapshotRequest:
		reqID = r.GetName()
	case *csi.DeleteSnapshotRequest:
		reqID = r.GetSnapshotId()

	case *csi.ControllerExpandVolumeRequest:
		reqID = r.GetVolumeId()

	case *csi.NodeStageVolumeRequest:
		reqID = r.GetVolumeId()
	case *csi.NodeUnstageVolumeRequest:
		reqID = r.GetVolumeId()

	case *csi.NodePublishVolumeRequest:
		reqID = r.GetVolumeId()
	case *csi.NodeUnpublishVolumeRequest:
		reqID = r.GetVolumeId()

	case *csi.NodeExpandVolumeRequest:
		reqID = r.GetVolumeId()

	case *csi.CreateVolumeGroupSnapshotRequest:
		reqID = r.GetName()
	case *csi.DeleteVolumeGroupSnapshotRequest:
		reqID = r.GetGroupSnapshotId()
	case *csi.GetVolumeGroupSnapshotRequest:
		reqID = r.GetGroupSnapshotId()

	// Replication
	case *replication.EnableVolumeReplicationRequest:
		reqID = GetIDFromReplication(r)
	case *replication.DisableVolumeReplicationRequest:
		reqID = GetIDFromReplication(r)
	case *replication.PromoteVolumeRequest:
		reqID = GetIDFromReplication(r)
	case *replication.DemoteVolumeRequest:
		reqID = GetIDFromReplication(r)
	case *replication.ResyncVolumeRequest:
		reqID = GetIDFromReplication(r)
	case *replication.GetVolumeReplicationInfoRequest:
		reqID = GetIDFromReplication(r)
	}

	return reqID
}

var id uint64

func contextIDInjector(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
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
	handler grpc.UnaryHandler,
) (interface{}, error) {
	log.ExtendedLog(ctx, "GRPC call: %s", info.FullMethod)
	log.TraceLog(ctx, "GRPC request: %s", protosanitizer.StripSecrets(req))

	resp, err := handler(ctx, req)
	if err != nil {
		klog.Errorf(log.Log(ctx, "GRPC error: %v"), err)
	} else {
		log.TraceLog(ctx, "GRPC response: %s", protosanitizer.StripSecrets(resp))
	}

	return resp, err
}

func logSlowGRPC(
	logInterval time.Duration,
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	handlerFinished := make(chan struct{})
	callStartTime := time.Now()

	// Ticks at a logInterval rate and logs a slow-call message until handler finishes.
	// This is called once the handler outlives its context, see below.
	doLogSlowGRPC := func() {
		ticker := time.NewTicker(logInterval)
		defer ticker.Stop()

		for {
			select {
			case t := <-ticker.C:
				timePassed := t.Sub(callStartTime).Truncate(time.Second)
				log.ExtendedLog(ctx,
					"Slow GRPC call %s (%s)", info.FullMethod, timePassed)
				log.TraceLog(ctx,
					"Slow GRPC request: %s", protosanitizer.StripSecrets(req))
			case <-handlerFinished:
				return
			}
		}
	}

	go func() {
		select {
		case <-ctx.Done():
			// The call (most likely) outlived its context. Start logging slow messages.
			doLogSlowGRPC()
		case <-handlerFinished:
			// The call finished, exit.
			return
		}
	}()

	resp, err := handler(ctx, req)
	close(handlerFinished)

	return resp, err
}

//nolint:nonamedreturns // named return used to send recovered panic error.
func panicHandler(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp interface{}, err error) {
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
func FilesystemNodeGetVolumeStats(
	ctx context.Context,
	mounter mount.Interface,
	targetPath string,
	includeInodes bool,
) (*csi.NodeGetVolumeStatsResponse, error) {
	isMnt, err := util.IsMountPoint(mounter, targetPath)
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

	res := &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: requirePositive(available),
				Total:     requirePositive(capacity),
				Used:      requirePositive(used),
				Unit:      csi.VolumeUsage_BYTES,
			},
		},
	}

	if includeInodes {
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

		res.Usage = append(res.Usage, &csi.VolumeUsage{
			Available: requirePositive(inodesFree),
			Total:     requirePositive(inodes),
			Used:      requirePositive(inodesUsed),
			Unit:      csi.VolumeUsage_INODES,
		})
	}

	// include marker for a healthy volume by default
	res.VolumeCondition = &csi.VolumeCondition{
		Abnormal: false,
		Message:  "volume is in a healthy condition",
	}

	return res, nil
}

// requirePositive returns the value for `x` when it is greater or equal to 0,
// or returns 0 in the acse `x` is negative.
//
// This is used for VolumeUsage entries in the NodeGetVolumeStatsResponse. The
// CSI spec does not allow negative values in the VolumeUsage objects.
func requirePositive(x int64) int64 {
	if x >= 0 {
		return x
	}

	return 0
}

// IsBlockMultiNode checks the volume capabilities for BlockMode and MultiNode.
func IsBlockMultiNode(caps []*csi.VolumeCapability) (bool, bool) {
	isMultiNode := false
	isBlock := false
	for _, capability := range caps {
		if capability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			isMultiNode = true
		}
		if capability.GetBlock() != nil {
			isBlock = true
		}
	}

	return isBlock, isMultiNode
}

// IsFileRWO checks if it is of type RWO and file mode, if it is return value
// will be set to true.
func IsFileRWO(caps []*csi.VolumeCapability) bool {
	// the return value has been set to true, if the volume is of file mode and if the capabilities are of RWO
	// kind, ie SINGLE NODE but flexible to have one or more writers. This is also used as a validation in caller
	// to preserve the backward compatibility we had with file mode RWO volumes.

	// to preserve backward compatibility we allow RWO filemode, ideally SINGLE_NODE_WRITER check is good enough,
	// however more granular level check could help us in future, so keeping it here as an additional measure.
	for _, cap := range caps {
		if cap.GetAccessMode() != nil {
			if cap.GetMount() != nil {
				switch cap.GetAccessMode().GetMode() { //nolint:exhaustive // only check what we want
				case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
					csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER:
					return true
				}
			}
		}
	}

	return false
}

// IsReaderOnly check and set return value true only when the access mode is `READER ONLY` regardless of file
// or block mode.
func IsReaderOnly(caps []*csi.VolumeCapability) bool {
	for _, cap := range caps {
		if cap.GetAccessMode() != nil {
			switch cap.GetAccessMode().GetMode() { //nolint:exhaustive // only check what we want
			case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
				csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY:
				return true
			}
		}
	}

	return false
}

// IsBlockMultiWriter validates the volume capability slice against the access modes and access type.
// if the capability is of multi write the first return value will be set to true and if the request
// is of type block, the second return value will be set to true.
func IsBlockMultiWriter(caps []*csi.VolumeCapability) (bool, bool) {
	// multiWriter has been set and returned after validating multi writer caps regardless of
	// single or multi node access mode. The caps check is agnostic to whether it is a filesystem or block
	// mode volume.
	var multiWriter bool

	// block has been set and returned if the passed in capability is of block volume mode.
	var block bool

	for _, cap := range caps {
		if cap.GetAccessMode() != nil {
			switch cap.GetAccessMode().GetMode() { //nolint:exhaustive // only check what we want
			case csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER:
				multiWriter = true
			}
		}
		if cap.GetBlock() != nil {
			block = true
		}
	}

	return multiWriter, block
}
