/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"flag"
	"os"
	"path"

	"github.com/ceph/ceph-csi/pkg/rbd"
	"github.com/golang/glog"
)

func init() {
	flag.Set("logtostderr", "true")
}

var (
	endpoint   = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	driverName = flag.String("drivername", "rbdplugin", "name of the driver")
	nodeID     = flag.String("nodeid", "", "node id")
)

func main() {
	flag.Parse()

	if err := createPersistentStorage(path.Join(rbd.PluginFolder, "controller")); err != nil {
		glog.Errorf("failed to create persistent storage for controller %v", err)
		os.Exit(1)
	}
	if err := createPersistentStorage(path.Join(rbd.PluginFolder, "node")); err != nil {
		glog.Errorf("failed to create persistent storage for node %v", err)
		os.Exit(1)
	}

	handle()
	os.Exit(0)
}

func handle() {
	driver := rbd.GetRBDDriver()
	driver.Run(*driverName, *nodeID, *endpoint)
}

func createPersistentStorage(persistentStoragePath string) error {
	if _, err := os.Stat(persistentStoragePath); os.IsNotExist(err) {
		if err := os.MkdirAll(persistentStoragePath, os.FileMode(0755)); err != nil {
			return err
		}
	} else {
	}
	return nil
}
