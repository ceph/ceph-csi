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

package util_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ceph/ceph-csi/internal/util"
)

var basePath = "./test_artifacts"
var csiClusters = "csi-clusters.json"
var pathToConfig = basePath + "/" + csiClusters
var clusterID1 = "test1"
var clusterID2 = "test2"

func cleanupTestData() {
	os.RemoveAll(basePath)
}

// nolint: gocyclo
func TestCSIConfig(t *testing.T) {
	var err error
	var data string
	var content string

	defer cleanupTestData()

	err = os.MkdirAll(basePath, 0700)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should fail as clusterid file is missing
	_, err = util.Mons(pathToConfig, clusterID1)
	if err == nil {
		t.Errorf("Failed: expected error due to missing config")
	}

	data = ""
	err = ioutil.WriteFile(basePath+"/"+csiClusters, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should fail as file is empty
	content, err = util.Mons(pathToConfig, clusterID1)
	if err == nil {
		t.Errorf("Failed: want (%s), got (%s)", data, content)
	}

	data = "[{\"clusterIDBad\":\"" + clusterID2 + "\",\"monitors\":[\"mon1\",\"mon2\",\"mon3\"]}]"
	err = ioutil.WriteFile(basePath+"/"+csiClusters, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should fail as clusterID data is malformed
	content, err = util.Mons(pathToConfig, clusterID2)
	if err == nil {
		t.Errorf("Failed: want (%s), got (%s)", data, content)
	}

	data = "[{\"clusterID\":\"" + clusterID2 + "\",\"monitorsBad\":[\"mon1\",\"mon2\",\"mon3\"]}]"
	err = ioutil.WriteFile(basePath+"/"+csiClusters, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should fail as monitors key is incorrect/missing
	content, err = util.Mons(pathToConfig, clusterID2)
	if err == nil {
		t.Errorf("Failed: want (%s), got (%s)", data, content)
	}

	data = "[{\"clusterID\":\"" + clusterID2 + "\",\"monitors\":[\"mon1\",2,\"mon3\"]}]"
	err = ioutil.WriteFile(basePath+"/"+csiClusters, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should fail as monitor data is malformed
	content, err = util.Mons(pathToConfig, clusterID2)
	if err == nil {
		t.Errorf("Failed: want (%s), got (%s)", data, content)
	}

	data = "[{\"clusterID\":\"" + clusterID2 + "\",\"monitors\":[\"mon1\",\"mon2\",\"mon3\"]}]"
	err = ioutil.WriteFile(basePath+"/"+csiClusters, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should fail as clusterID is not present in config
	content, err = util.Mons(pathToConfig, clusterID1)
	if err == nil {
		t.Errorf("Failed: want (%s), got (%s)", data, content)
	}

	// TEST: Should pass as clusterID is present in config
	content, err = util.Mons(pathToConfig, clusterID2)
	if err != nil || content != "mon1,mon2,mon3" {
		t.Errorf("Failed: want (%s), got (%s) (%v)", "mon1,mon2,mon3", content, err)
	}

	data = "[{\"clusterID\":\"" + clusterID2 + "\",\"monitors\":[\"mon1\",\"mon2\",\"mon3\"]}," +
		"{\"clusterID\":\"" + clusterID1 + "\",\"monitors\":[\"mon4\",\"mon5\",\"mon6\"]}]"
	err = ioutil.WriteFile(basePath+"/"+csiClusters, []byte(data), 0644)
	if err != nil {
		t.Errorf("Test setup error %s", err)
	}

	// TEST: Should pass as clusterID is present in config
	content, err = util.Mons(pathToConfig, clusterID1)
	if err != nil || content != "mon4,mon5,mon6" {
		t.Errorf("Failed: want (%s), got (%s) (%v)", "mon4,mon5,mon6", content, err)
	}
}
