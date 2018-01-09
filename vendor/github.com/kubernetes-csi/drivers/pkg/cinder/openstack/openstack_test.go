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

package openstack

import (
	"os"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/stretchr/testify/assert"
)

var fakeFileName = "cloud.conf"
var fakeUserName = "user"
var fakePassword = "pass"
var fakeAuthUrl = "https://169.254.169.254/identity/v3"
var fakeTenantID = "c869168a828847f39f7f06edd7305637"
var fakeDomainID = "2a73b8f597c04551a0fdc8e95544be8a"
var fakeRegion = "RegionOne"

// Test GetConfigFromFile
func TestGetConfigFromFile(t *testing.T) {
	// init file
	var fakeFileContent = `
[Global]
username=` + fakeUserName + `
password=` + fakePassword + `
auth-url=` + fakeAuthUrl + `
tenant-id=` + fakeTenantID + `
domain-id=` + fakeDomainID + `
region=` + fakeRegion + `
`

	f, err := os.Create(fakeFileName)
	if err != nil {
		t.Errorf("failed to create file: %v", err)
	}

	_, err = f.WriteString(fakeFileContent)
	f.Close()
	if err != nil {
		t.Errorf("failed to write file: %v", err)
	}
	defer os.Remove(fakeFileName)

	// Init assert
	assert := assert.New(t)

	expectedAuthOpts := gophercloud.AuthOptions{
		IdentityEndpoint: fakeAuthUrl,
		Username:         fakeUserName,
		Password:         fakePassword,
		TenantID:         fakeTenantID,
		DomainID:         fakeDomainID,
		AllowReauth:      true,
	}
	expectedEpOpts := gophercloud.EndpointOpts{
		Region: fakeRegion,
	}

	// Invoke GetConfigFromFile
	actualAuthOpts, actualEpOpts, err := GetConfigFromFile(fakeFileName)
	if err != nil {
		t.Errorf("failed to GetConfigFromFile: %v", err)
	}

	// Assert
	assert.Equal(expectedAuthOpts, actualAuthOpts)
	assert.Equal(expectedEpOpts, actualEpOpts)
}

// Test GetConfigFromEnv
func TestGetConfigFromEnv(t *testing.T) {
	// init env
	os.Setenv("OS_AUTH_URL", fakeAuthUrl)
	os.Setenv("OS_USERNAME", fakeUserName)
	os.Setenv("OS_PASSWORD", fakePassword)
	os.Setenv("OS_TENANT_ID", fakeTenantID)
	os.Setenv("OS_DOMAIN_ID", fakeDomainID)
	os.Setenv("OS_REGION_NAME", fakeRegion)

	// Init assert
	assert := assert.New(t)

	expectedAuthOpts := gophercloud.AuthOptions{
		IdentityEndpoint: fakeAuthUrl,
		Username:         fakeUserName,
		Password:         fakePassword,
		TenantID:         fakeTenantID,
		DomainID:         fakeDomainID,
	}
	expectedEpOpts := gophercloud.EndpointOpts{
		Region: fakeRegion,
	}

	// Invoke GetConfigFromEnv
	actualAuthOpts, actualEpOpts, err := GetConfigFromEnv()
	if err != nil {
		t.Errorf("failed to GetConfigFromEnv: %v", err)
	}

	// Assert
	assert.Equal(expectedAuthOpts, actualAuthOpts)
	assert.Equal(expectedEpOpts, actualEpOpts)
}
