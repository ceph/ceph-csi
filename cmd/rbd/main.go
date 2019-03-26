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

	"github.com/ceph/ceph-csi/pkg/rbd"
	"github.com/ceph/ceph-csi/pkg/util"
	"k8s.io/klog"
)

var (
	endpoint      = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	driverName    = flag.String("drivername", "csi-rbdplugin", "name of the driver")
	nodeID        = flag.String("nodeid", "", "node id")
	containerized = flag.Bool("containerized", true, "whether run as containerized")
	configRoot    = flag.String("configroot", "/etc/csi-config", "directory in which CSI specific Ceph"+
		" cluster configurations are present, OR the value \"k8s_objects\" if present as kubernetes secrets")
	instanceID = flag.String("instanceid", "", "Unique ID distinguishing this instance of Ceph CSI among other"+
		" instances, when sharing Ceph clusters across CSI instances for provisioning")
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
	rbd.PluginFolder = rbd.PluginFolder + *driverName

	driver := rbd.NewDriver()
	driver.Run(*driverName, *nodeID, *endpoint, *configRoot, *instanceID, *containerized)

	os.Exit(0)
}
