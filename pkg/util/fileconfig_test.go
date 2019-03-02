/*
Copyright 2019 ceph-csi authors.

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

// nolint: gocyclo

package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

var testFsid = "dummy-fs-id"
var basePath = "./test_artifacts"

// nolint: gocyclo
func TestGetMons(t *testing.T) {
	var fc FileConfig
	var err error

	configFileDir := basePath + "/" + fNamePrefix + fNameSep + testFsid
	defer os.RemoveAll(basePath)

	fc.BasePath = basePath

	// TEST: Empty fsid should error out
	_, err = fc.GetMons("")
	if err == nil {
		t.Errorf("Call passed, expected to fail due to fsid missing!")
	}

	// TEST: Missing file should error out
	_, err = fc.GetMons(testFsid)
	if err == nil {
		t.Errorf("Call passed, expected to fail due to missing config file!")
	}

	// TEST: Empty file should error out
	err = os.MkdirAll(configFileDir, 0700)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	data := []byte{}
	err = ioutil.WriteFile(configFileDir+"/"+fNameCephConfig, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	_, err = fc.GetMons(testFsid)
	if err == nil {
		t.Errorf("Call passed, expected to fail due to missing config file!")
	}

	/* Tests with bad JSON content should get caught due to strongly typed JSON
	   struct in implementation and are not tested here */

	// TEST: Send JSON with incorrect fsid
	data = []byte(`
        {
            "version": 1,
            "cluster-config": {
                "cluster-fsid": "bad_fsid",
                "monitors": ["IP1:port1","IP2:port2"],
                "pools": ["pool1","pool2"]
            }
        }`)
	err = ioutil.WriteFile(configFileDir+"/"+fNameCephConfig, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	_, err = fc.GetMons(testFsid)
	if err == nil {
		t.Errorf("Expected to fail on bad fsid in JSON")
	}

	// TEST: Send JSON with empty mon list
	data = []byte(`
        {
            "version": 1,
            "cluster-config": {
                "cluster-fsid": "` + testFsid + `",
                "monitors": [],
                "pools": ["pool1","pool2"]
            }
        }`)
	err = ioutil.WriteFile(configFileDir+"/"+fNameCephConfig, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	_, err = fc.GetMons(testFsid)
	if err == nil {
		t.Errorf("Expected to fail in empty MON list in JSON")
	}

	// TEST: Check valid return from successful call
	data = []byte(`
        {
            "version": 1,
            "cluster-config": {
                "cluster-fsid": "` + testFsid + `",
                "monitors": ["IP1:port1","IP2:port2"],
                "pools": ["pool1","pool2"]
            }
        }`)
	err = ioutil.WriteFile(configFileDir+"/"+fNameCephConfig, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	output, err := fc.GetMons(testFsid)
	if err != nil {
		t.Errorf("Call failed %s", err)
	}
	if output != "IP1:port1,IP2:port2" {
		t.Errorf("Failed to generate correct output: expected %s, got %s",
			"IP1:port1,IP2:port2", output)
	}
}

func TestGetProvisionerSubjectID(t *testing.T) {
	var fc FileConfig
	var err error

	configFileDir := basePath + "/" + fNamePrefix + fNameSep + testFsid + fNameSep + fNameProvPrefix
	defer os.RemoveAll(basePath)

	fc.BasePath = basePath

	// TEST: Empty fsid should error out
	_, err = fc.GetProvisionerSubjectID("")
	if err == nil {
		t.Errorf("Call passed, expected to fail due to fsid missing!")
	}

	// TEST: Missing file should error out
	_, err = fc.GetProvisionerSubjectID(testFsid)
	if err == nil {
		t.Errorf("Call passed, expected to fail due to missing config file!")
	}

	// TEST: Empty file should error out
	err = os.MkdirAll(configFileDir, 0700)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	data := []byte{}
	err = ioutil.WriteFile(configFileDir+"/"+fNameProvSubject, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	_, err = fc.GetProvisionerSubjectID(testFsid)
	if err == nil {
		t.Errorf("Call passed, expected to fail due to missing config file!")
	}

	// TEST: Check valid return from successful call
	data = []byte("admin")
	err = ioutil.WriteFile(configFileDir+"/"+fNameProvSubject, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	output, err := fc.GetProvisionerSubjectID(testFsid)
	if err != nil || output != "admin" {
		t.Errorf("Failed to get valid subject ID: expected %s, got %s, err %s", "admin", output, err)
	}
}

func TestGetPublishSubjectID(t *testing.T) {
	var fc FileConfig
	var err error

	configFileDir := basePath + "/" + fNamePrefix + fNameSep + testFsid + fNameSep + fNamePubPrefix
	defer os.RemoveAll(basePath)

	fc.BasePath = basePath

	// TEST: Empty fsid should error out
	_, err = fc.GetPublishSubjectID("")
	if err == nil {
		t.Errorf("Call passed, expected to fail due to fsid missing!")
	}

	// TEST: Missing file should error out
	_, err = fc.GetPublishSubjectID(testFsid)
	if err == nil {
		t.Errorf("Call passed, expected to fail due to missing config file!")
	}

	// TEST: Empty file should error out
	err = os.MkdirAll(configFileDir, 0700)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	data := []byte{}
	err = ioutil.WriteFile(configFileDir+"/"+fNamePubSubject, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	_, err = fc.GetPublishSubjectID(testFsid)
	if err == nil {
		t.Errorf("Call passed, expected to fail due to missing config file!")
	}

	// TEST: Check valid return from successful call
	data = []byte("admin")
	err = ioutil.WriteFile(configFileDir+"/"+fNamePubSubject, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	output, err := fc.GetPublishSubjectID(testFsid)
	if err != nil || output != "admin" {
		t.Errorf("Failed to get valid subject ID: expected %s, got %s, err %s", "admin", output, err)
	}
}

// nolint: gocyclo
func TestGetCredentialForSubject(t *testing.T) {
	var fc FileConfig
	var err error

	configFileDir := basePath + "/" + fNamePrefix + fNameSep + testFsid + fNameSep + fNamePubPrefix
	defer os.RemoveAll(basePath)

	fc.BasePath = basePath

	// TEST: Empty fsid should error out
	_, err = fc.GetCredentialForSubject("", "subject")
	if err == nil {
		t.Errorf("Call passed, expected to fail due to fsid missing!")
	}

	// TEST: Missing file should error out
	_, err = fc.GetCredentialForSubject(testFsid, "")
	if err == nil {
		t.Errorf("Call passed, expected to fail due to missing config file!")
	}

	// TEST: Empty subject file should error out
	err = os.MkdirAll(configFileDir, 0700)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	data := []byte{}
	err = ioutil.WriteFile(configFileDir+"/"+fNamePubSubject, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	_, err = fc.GetCredentialForSubject(testFsid, "adminpub")
	if err == nil {
		t.Errorf("Call passed, expected to fail due to empty subject file!")
	}

	// TEST: Empty subject cred file should error out
	data = []byte("adminpub")
	err = ioutil.WriteFile(configFileDir+"/"+fNamePubSubject, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}
	data = []byte{}
	err = ioutil.WriteFile(configFileDir+"/"+fNamePubCred, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	_, err = fc.GetCredentialForSubject(testFsid, "adminpub")
	if err == nil {
		t.Errorf("Call passed, expected to fail due to missing cred content!")
	}

	// TEST: Success fetching pub creds
	data = []byte("testpwd")
	err = ioutil.WriteFile(configFileDir+"/"+fNamePubCred, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	output, err := fc.GetCredentialForSubject(testFsid, "adminpub")
	if err != nil || output != "testpwd" {
		t.Errorf("Failed to get valid Publish credentials: expected %s, got %s, err %s", "testpwd", output, err)
	}

	// TEST: Fetch missing prov creds
	configFileDir = basePath + "/" + fNamePrefix + fNameSep + testFsid + fNameSep + fNameProvPrefix
	err = os.MkdirAll(configFileDir, 0700)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	data = []byte("adminprov")
	err = ioutil.WriteFile(configFileDir+"/"+fNameProvSubject, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	fmt.Printf("Starting test")
	_, err = fc.GetCredentialForSubject(testFsid, "adminprov")
	if err == nil {
		t.Errorf("Call passed, expected to fail due to missing cred content!")
	}

	// TEST: Fetch prov creds successfully
	data = []byte("testpwd")
	err = ioutil.WriteFile(configFileDir+"/"+fNameProvCred, data, 0644)
	if err != nil {
		t.Errorf("Test utility error %s", err)
	}

	output, err = fc.GetCredentialForSubject(testFsid, "adminprov")
	if err != nil || output != "testpwd" {
		t.Errorf("Call passed, expected to fail due to missing cred content!")
	}
}
