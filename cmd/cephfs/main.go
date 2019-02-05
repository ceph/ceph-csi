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

	"github.com/ceph/ceph-csi/pkg/cephfs"
	"github.com/ceph/ceph-csi/pkg/util"
	"k8s.io/klog"
)

func init() {
	if err := flag.Set("logtostderr", "true"); err != nil {
		glog.Errorf("failed to set logtostderr flag: %v", err)
		os.Exit(1)
	}
}

var (
	endpoint        = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	driverName      = flag.String("drivername", "csi-cephfsplugin", "name of the driver")
	nodeID          = flag.String("nodeid", "", "node id")
	volumeMounter   = flag.String("volumemounter", "", "default volume mounter (possible options are 'kernel', 'fuse')")
	metadataStorage = flag.String("metadatastorage", "", "metadata persistence method [node|k8s_configmap]")
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	if err := createPersistentStorage(path.Join(cephfs.PluginFolder, "controller")); err != nil {
		klog.Errorf("failed to create persistent storage for controller: %v", err)
		os.Exit(1)
	}

	if err := createPersistentStorage(path.Join(cephfs.PluginFolder, "node")); err != nil {
		klog.Errorf("failed to create persistent storage for node: %v", err)
		os.Exit(1)
	}

	cp, err := util.NewCachePersister(*metadataStorage, *driverName)
	if err != nil {
		klog.Errorf("failed to define cache persistence method: %v", err)
		os.Exit(1)
	}

	driver := cephfs.NewDriver()
	driver.Run(*driverName, *nodeID, *endpoint, *volumeMounter, cp)

	os.Exit(0)
}

func createPersistentStorage(persistentStoragePath string) error {
	return os.MkdirAll(persistentStoragePath, os.FileMode(0755))
}
