/*
Copyright 2020 The Ceph-CSI Authors.

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

package kms

import (
	"errors"
	"os"
	"testing"

	loss "github.com/libopenstorage/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectAuthMountPath(t *testing.T) {
	t.Parallel()
	authMountPath, err := detectAuthMountPath(vaultDefaultAuthPath)
	if err != nil {
		t.Errorf("detectAuthMountPath() failed: %s", err)
	}
	if authMountPath != "kubernetes" {
		t.Errorf("authMountPath should be set to 'kubernetes', but is: %s", authMountPath)
	}

	authMountPath, err = detectAuthMountPath("kubernetes")
	if err != nil {
		t.Errorf("detectAuthMountPath() failed: %s", err)
	}
	if authMountPath != "kubernetes" {
		t.Errorf("authMountPath should be set to 'kubernetes', but is: %s", authMountPath)
	}
}

func TestCreateTempFile(t *testing.T) {
	t.Parallel()
	data := []byte("Hello World!")
	tmpfile, err := createTempFile("my-file", data)
	if err != nil {
		t.Errorf("createTempFile() failed: %s", err)
	}
	if tmpfile == "" {
		t.Errorf("createTempFile() returned an empty filename")
	}

	err = os.Remove(tmpfile)
	if err != nil {
		t.Errorf("failed to remove tmpfile (%s): %s", tmpfile, err)
	}
}

func TestSetConfigString(t *testing.T) {
	t.Parallel()
	const defaultValue = "default-value"
	options := make(map[string]interface{})

	// noSuchOption: no default value, option unavailable
	noSuchOption := ""
	err := setConfigString(&noSuchOption, options, "nonexistent")
	switch {
	case err == nil:
		t.Error("did not get an error when one was expected")
	case !errors.Is(err, errConfigOptionMissing):
		t.Errorf("expected errConfigOptionMissing, but got %T: %s", err, err)
	case noSuchOption != "":
		t.Error("value should not have been modified")
	}

	// noOptionDefault: default value, option unavailable
	noOptionDefault := defaultValue
	err = setConfigString(&noOptionDefault, options, "nonexistent")
	switch {
	case err == nil:
		t.Error("did not get an error when one was expected")
	case !errors.Is(err, errConfigOptionMissing):
		t.Errorf("expected errConfigOptionMissing, but got %T: %s", err, err)
	case noOptionDefault != defaultValue:
		t.Error("value should not have been modified")
	}

	// optionDefaultOverload: default value, option available
	optionDefaultOverload := defaultValue
	options["set-me"] = "non-default"
	err = setConfigString(&optionDefaultOverload, options, "set-me")
	switch {
	case err != nil:
		t.Errorf("unexpected error returned: %s", err)
	case optionDefaultOverload != "non-default":
		t.Error("optionDefaultOverload should have been updated")
	}
}

func TestDefaultVaultDestroyKeys(t *testing.T) {
	t.Parallel()

	vc := &vaultConnection{}
	config := make(map[string]interface{})
	config["vaultAddress"] = "https://vault.test.example.com"
	err := vc.initConnection(config)
	require.NoError(t, err)
	keyContext := vc.getDeleteKeyContext()
	destroySecret, ok := keyContext[loss.DestroySecret]
	assert.NotEqual(t, destroySecret, "")
	assert.True(t, ok)

	// setting vaultDestroyKeys to !true should remove the loss.DestroySecret entry
	config["vaultDestroyKeys"] = "false"
	err = vc.initConnection(config)
	require.NoError(t, err)
	keyContext = vc.getDeleteKeyContext()
	_, ok = keyContext[loss.DestroySecret]
	assert.False(t, ok)
}

func TestVaultKMSRegistered(t *testing.T) {
	t.Parallel()
	_, ok := kmsManager.providers[kmsTypeVault]
	assert.True(t, ok)
}
