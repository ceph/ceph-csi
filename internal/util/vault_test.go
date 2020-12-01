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

package util

import (
	"errors"
	"os"
	"testing"
)

func TestDetectAuthMountPath(t *testing.T) {
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
