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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DefaultIdentityServer struct {
	Driver *CSIDriver
}

//GetSupportedVersions(context.Context, *GetSupportedVersionsRequest) (*GetSupportedVersionsResponse, error)
//GetPluginInfo(context.Context, *GetPluginInfoRequest) (*GetPluginInfoResponse, error)
func (ids *DefaultIdentityServer) GetSupportedVersions(ctx context.Context, req *csi.GetSupportedVersionsRequest) (*csi.GetSupportedVersionsResponse, error) {
	glog.V(5).Infof("Using default GetSupportedVersions")
	return &csi.GetSupportedVersionsResponse{
		SupportedVersions: ids.Driver.supVers,
	}, nil
}

func (ids *DefaultIdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	glog.V(5).Infof("Using default GetPluginInnfo")

	if ids.Driver.name == "" {
		return nil, status.Error(codes.Unavailable, "Driver name not configured")
	}

	err := ids.Driver.CheckVersion(req.GetVersion())
	if err != nil {
		return nil, err
	}

	version := GetVersionString(ids.Driver.version)

	return &csi.GetPluginInfoResponse{
		Name:          ids.Driver.name,
		VendorVersion: version,
	}, nil
}
