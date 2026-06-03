/*
Copyright 2025 The Ceph-CSI Authors.

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

package nvmeoferrors

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TODO: next PR - look at nvmeof package and see if any errors can be moved here.
var (
	// ErrRbdQoSExists is returned when the user tries to set a QoS for namespace, and that namespace
	// already has a RBD QoS associated with it.
	ErrRbdQoSExists = errors.New("QoS limits already configured (RBD QoS exists)")
	// ErrMetadataNotFound is returned when NVMe-oF volume metadata is not found.
	ErrMetadataNotFound = errors.New("metadata not found")
	// ErrMetadataCorrupted is returned when NVMe-oF volume metadata(=rbd metadata) is corrupted or invalid.
	ErrMetadataCorrupted = errors.New("metadata corrupted or invalid")
)

// errorToGRPCCode maps custom errors to gRPC status codes.
var errorToGRPCCode = map[error]codes.Code{
	ErrMetadataNotFound:  codes.NotFound,
	ErrMetadataCorrupted: codes.Internal,
	ErrRbdQoSExists:      codes.InvalidArgument,
}

// ToGRPCError converts a custom error to a gRPC status error.
// If the error is not recognized, it returns codes.Internal.
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}

	// Check if it's one of our custom errors
	for customErr, code := range errorToGRPCCode {
		if errors.Is(err, customErr) {
			return status.Error(code, err.Error())
		}
	}

	// Unknown error - return Internal
	return status.Errorf(codes.Internal, "internal error: %v", err)
}
