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
)

func TestGetPluginInfo(t *testing.T) {
	d := NewFakeDriver()

	ids := NewDefaultIdentityServer(d)

	req := csi.GetPluginInfoRequest{}
	resp, err := ids.GetPluginInfo(context.Background(), &req)
	assert.NoError(t, err)
	assert.Equal(t, resp.GetName(), fakeDriverName)
	assert.Equal(t, resp.GetVendorVersion(), vendorVersion)
}
