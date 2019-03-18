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

	"github.com/ceph/ceph-csi/pkg/cephfs"
	"github.com/ceph/ceph-csi/pkg/util"
	"k8s.io/klog"
)

var (
	endpoint        = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	driverName      = flag.String("drivername", "cephfs.csi.ceph.com", "name of the driver")
	nodeID          = flag.String("nodeid", "", "node id")
	volumeMounter   = flag.String("volumemounter", "", "default volume mounter (possible options are 'kernel', 'fuse')")
	metadataStorage = flag.String("metadatastorage", "", "metadata persistence method [node|k8s_configmap]")
)

func init() {
	klog.InitFlags(nil)
	if err := flag.Set("logtostderr", "true"); err != nil {
		klog.Exitf("failed to set logtostderr flag: %v", err)
	}
	flag.Parse()
}

func main() {

	err := util.ValidateDriverName(*driverName)
	if err != nil {
		klog.Fatalln(err)
	}
	//update plugin name
	cephfs.PluginFolder = cephfs.PluginFolder + *driverName

	cp, err := util.CreatePersistanceStorage(cephfs.PluginFolder, *metadataStorage, *driverName)
	if err != nil {
		os.Exit(1)
	}

	driver := cephfs.NewDriver()
	driver.Run(*driverName, *nodeID, *endpoint, *volumeMounter, cp)

	os.Exit(0)
}
