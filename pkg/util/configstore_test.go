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
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

var basePath = "./test_artifacts"
var cs *ConfigStore

func cleanupTestData() {
	os.RemoveAll(basePath)
}

// nolint: gocyclo
func TestConfigStore(t *testing.T) {
	var err error
	var data string
	var content string
	var testDir string

	defer cleanupTestData()

	cs, err = NewConfigStore(basePath)
	if err != nil {
		t.Errorf("Fatal, failed to get a new config store")
	}

	err = os.MkdirAll(basePath, 0700)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should fail as fsid directory is missing
	_, err = cs.Mons("testfsid")
	if err == nil {
		t.Errorf("Failed: expected error due to missing parent directory")
	}

	testDir = basePath + "/" + "ceph-cluster-testfsid"
	err = os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should fail as mons file is missing
	_, err = cs.Mons("testfsid")
	if err == nil {
		t.Errorf("Failed: expected error due to missing mons file")
	}

	data = ""
	err = ioutil.WriteFile(testDir+"/"+csMonitors, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should fail as MONs is an empty string
	content, err = cs.Mons("testfsid")
	if err == nil {
		t.Errorf("Failed: want (%s), got (%s)", data, content)
	}

	data = "mon1,mon2,mon3"
	err = ioutil.WriteFile(testDir+"/"+csMonitors, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Fetching MONs should succeed
	content, err = cs.Mons("testfsid")
	if err != nil || content != data {
		t.Errorf("Failed: want (%s), got (%s), err (%s)", data, content, err)
	}

	data = "pool1,pool2"
	err = ioutil.WriteFile(testDir+"/"+csPools, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Fetching MONs should succeed
	listContent, err := cs.Pools("testfsid")
	if err != nil || strings.Join(listContent, ",") != data {
		t.Errorf("Failed: want (%s), got (%s), err (%s)", data, content, err)
	}

	data = "provuser"
	err = ioutil.WriteFile(testDir+"/"+csAdminID, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Fetching provuser should succeed
	content, err = cs.AdminID("testfsid")
	if err != nil || content != data {
		t.Errorf("Failed: want (%s), got (%s), err (%s)", data, content, err)
	}

	data = "pubuser"
	err = ioutil.WriteFile(testDir+"/"+csUserID, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Fetching pubuser should succeed
	content, err = cs.UserID("testfsid")
	if err != nil || content != data {
		t.Errorf("Failed: want (%s), got (%s), err (%s)", data, content, err)
	}

	data = "provkey"
	err = ioutil.WriteFile(testDir+"/"+csAdminKey, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Fetching provkey should succeed
	content, err = cs.CredentialForUser("testfsid", "provuser")
	if err != nil || content != data {
		t.Errorf("Failed: want (%s), got (%s), err (%s)", data, content, err)
	}

	data = "pubkey"
	err = ioutil.WriteFile(testDir+"/"+csUserKey, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Fetching pubkey should succeed
	content, err = cs.CredentialForUser("testfsid", "pubuser")
	if err != nil || content != data {
		t.Errorf("Failed: want (%s), got (%s), err (%s)", data, content, err)
	}

	// TEST: Fetching random user key should fail
	_, err = cs.CredentialForUser("testfsid", "random")
	if err == nil {
		t.Errorf("Failed: Expected to fail fetching random user key")
	}
}
