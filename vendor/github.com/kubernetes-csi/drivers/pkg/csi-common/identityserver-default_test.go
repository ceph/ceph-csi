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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetSupportedVersions(t *testing.T) {
	d := NewFakeDriver()

	ids := NewDefaultIdentityServer(d)

	req := csi.GetSupportedVersionsRequest{}

	// Test Get supported versions are valid.
	resp, err := ids.GetSupportedVersions(context.Background(), &req)
	assert.NoError(t, err)

	for _, fv := range fakeVersionsSupported {
		found := false
		for _, rv := range resp.GetSupportedVersions() {
			if fv.GetMajor() == rv.GetMajor() && fv.GetMinor() == rv.GetMinor() && fv.GetPatch() == rv.GetPatch() {
				found = true
			}
		}
		assert.True(t, found)
	}
}

func TestGetPluginInfo(t *testing.T) {
	d := NewFakeDriver()

	ids := NewDefaultIdentityServer(d)

	// Test invalid request
	req := csi.GetPluginInfoRequest{}
	resp, err := ids.GetPluginInfo(context.Background(), &req)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, s.Code(), codes.InvalidArgument)

	// Test valid request
	req.Version = &fakeVersion
	resp, err = ids.GetPluginInfo(context.Background(), &req)
	assert.NoError(t, err)
	assert.Equal(t, resp.GetName(), fakeDriverName)
}
