/*
Copyright 2019 The Ceph-CSI Authors.

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
	"strings"

	"github.com/ceph/ceph-csi/pkg/cephfs"
	"github.com/ceph/ceph-csi/pkg/rbd"
	"github.com/ceph/ceph-csi/pkg/util"
	"k8s.io/klog"
)

const (
	rbdType    = "rbd"
	cephfsType = "cephfs"

	rbdDefaultName    = "rbd.csi.ceph.com"
	cephfsDefaultName = "cephfs.csi.ceph.com"
)

var (
	// common flags
	vtype           = flag.String("type", "", "driver type [rbd|cephfs]")
	endpoint        = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	driverName      = flag.String("drivername", "", "name of the driver")
	nodeID          = flag.String("nodeid", "", "node id")
	metadataStorage = flag.String("metadatastorage", "", "metadata persistence method [node|k8s_configmap]")

	// rbd related flags
	containerized = flag.Bool("containerized", true, "whether run as containerized")
	configRoot    = flag.String("configroot", "/etc/csi-config", "directory in which CSI specific Ceph"+
		" cluster configurations are present, OR the value \"k8s_objects\" if present as kubernetes secrets")

	// cephfs related flags
	volumeMounter = flag.String("volumemounter", "", "default volume mounter (possible options are 'kernel', 'fuse')")
	mountCacheDir = flag.String("mountcachedir", "", "mount info cache save dir")
)

func init() {
	klog.InitFlags(nil)
	if err := flag.Set("logtostderr", "true"); err != nil {
		klog.Exitf("failed to set logtostderr flag: %v", err)
	}
	flag.Parse()
}

func getType() string {
	if vtype == nil || len(*vtype) == 0 {
		a0 := path.Base(os.Args[0])
		if strings.Contains(a0, rbdType) {
			return rbdType
		}
		if strings.Contains(a0, cephfsType) {
			return cephfsType
		}
		return ""
	}
	return *vtype
}

func getDriverName() string {
	// was explicitly passed a driver name
	if driverName != nil && len(*driverName) != 0 {
		return *driverName
	}
	// select driver name based on volume type
	switch getType() {
	case rbdType:
		return rbdDefaultName
	case cephfsType:
		return cephfsDefaultName
	default:
		return ""
	}
}

func main() {
	driverType := getType()
	if len(driverType) == 0 {
		klog.Fatalln("driver type not specified")
	}

	dname := getDriverName()
	err := util.ValidateDriverName(dname)
	if err != nil {
		klog.Fatalln(err) // calls exit
	}

	switch driverType {
	case rbdType:
		rbd.PluginFolder = rbd.PluginFolder + dname
		cp, err := util.CreatePersistanceStorage(
			rbd.PluginFolder, *metadataStorage, dname)
		if err != nil {
			os.Exit(1)
		}
		driver := rbd.NewDriver()
		driver.Run(dname, *nodeID, *endpoint, *configRoot, *containerized, cp)

	case cephfsType:
		cephfs.PluginFolder = cephfs.PluginFolder + dname
		cp, err := util.CreatePersistanceStorage(
			cephfs.PluginFolder, *metadataStorage, dname)
		if err != nil {
			os.Exit(1)
		}
		driver := cephfs.NewDriver()
		driver.Run(dname, *nodeID, *endpoint, *volumeMounter, *mountCacheDir, cp)

	default:
		klog.Fatalln("invalid volume type", vtype) // calls exit
	}

	os.Exit(0)
}
