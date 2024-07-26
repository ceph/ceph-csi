/*
Copyright 2024 The Ceph-CSI Authors.

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
	"context"
	"errors"

	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	ekr "github.com/csi-addons/spec/lib/go/encryptionkeyrotation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type EncryptionKeyRotationServer struct {
	*ekr.UnimplementedEncryptionKeyRotationControllerServer
	volLock *util.VolumeLocks
}

func NewEncryptionKeyRotationServer(volLock *util.VolumeLocks) *EncryptionKeyRotationServer {
	return &EncryptionKeyRotationServer{volLock: volLock}
}

func (ekrs *EncryptionKeyRotationServer) RegisterService(svc grpc.ServiceRegistrar) {
	ekr.RegisterEncryptionKeyRotationControllerServer(svc, ekrs)
}

func (ekrs *EncryptionKeyRotationServer) EncryptionKeyRotate(
	ctx context.Context,
	req *ekr.EncryptionKeyRotateRequest,
) (*ekr.EncryptionKeyRotateResponse, error) {
	// Get the volume ID from the request
	volID := req.GetVolumeId()
	if volID == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID in request")
	}

	if acquired := ekrs.volLock.TryAcquire(volID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volID)
	}
	defer ekrs.volLock.Release(volID)

	// Get the credentials required to authenticate
	// against a ceph cluster
	creds, err := util.NewUserCredentials(req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer creds.DeleteCredentials()

	rbdVol, err := rbd.GenVolFromVolID(ctx, volID, creds, req.GetSecrets())
	if err != nil {
		switch {
		case errors.Is(err, rbd.ErrImageNotFound):
			err = status.Errorf(codes.NotFound, "volume ID %s not found", volID)
		case errors.Is(err, util.ErrPoolNotFound):
			log.ErrorLog(ctx, "failed to get backend volume for %s: %v", volID, err)
			err = status.Errorf(codes.NotFound, err.Error())
		default:
			err = status.Errorf(codes.Internal, err.Error())
		}

		return nil, err
	}
	defer rbdVol.Destroy(ctx)

	err = rbdVol.RotateEncryptionKey(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal, "failed to rotate the key for volume with ID %q: %s", volID, err.Error())
	}

	// Success
	return &ekr.EncryptionKeyRotateResponse{}, nil
}
