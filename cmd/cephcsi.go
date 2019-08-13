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
	"path/filepath"
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
	vtype      = flag.String("type", "", "driver type [rbd|cephfs]")
	endpoint   = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	driverName = flag.String("drivername", "", "name of the driver")
	nodeID     = flag.String("nodeid", "", "node id")
	instanceID = flag.String("instanceid", "", "Unique ID distinguishing this instance of Ceph CSI among other"+
		" instances, when sharing Ceph clusters across CSI instances for provisioning")
	metadataStorage = flag.String("metadatastorage", "", "metadata persistence method [node|k8s_configmap]")
	pluginPath      = flag.String("pluginpath", "/var/lib/kubelet/plugins/", "the location of cephcsi plugin")
	pidLimit        = flag.Int("pidlimit", 0, "the PID limit to configure through cgroups")

	// rbd related flags
	containerized = flag.Bool("containerized", true, "whether run as containerized")

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
	if vtype == nil || *vtype == "" {
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
	if driverName != nil && *driverName != "" {
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
	klog.Infof("Driver version: %s and Git version: %s", util.DriverVersion, util.GitCommit)
	var cp util.CachePersister

	driverType := getType()
	if driverType == "" {
		klog.Fatalln("driver type not specified")
	}

	dname := getDriverName()
	err := util.ValidateDriverName(dname)
	if err != nil {
		klog.Fatalln(err) // calls exit
	}
	csipluginPath := filepath.Join(*pluginPath, dname)
	if *metadataStorage != "" {
		cp, err = util.CreatePersistanceStorage(
			csipluginPath, *metadataStorage, *pluginPath)
		if err != nil {
			os.Exit(1)
		}
	}

	// the driver may need a higher PID limit for handling all concurrent requests
	if pidLimit != nil && *pidLimit != 0 {
		currentLimit, err := util.GetPIDLimit()
		if err != nil {
			klog.Errorf("Failed to get the PID limit, can not reconfigure: %v", err)
		} else {
			klog.Infof("Initial PID limit is set to %d", currentLimit)
			err = util.SetPIDLimit(*pidLimit)
			if err != nil {
				klog.Errorf("Failed to set new PID limit to %d: %v", *pidLimit, err)
			} else {
				s := ""
				if *pidLimit == -1 {
					s = " (max)"
				}
				klog.Infof("Reconfigured PID limit to %d%s", *pidLimit, s)
			}
		}
	}

	klog.Infof("Starting driver type: %v with name: %v", driverType, dname)
	switch driverType {
	case rbdType:
		driver := rbd.NewDriver()
		driver.Run(dname, *nodeID, *endpoint, *instanceID, *containerized, cp, driverType)

	case cephfsType:
		driver := cephfs.NewDriver()
		driver.Run(dname, *nodeID, *endpoint, *volumeMounter, *mountCacheDir, *instanceID, csipluginPath, cp, driverType)

	default:
		klog.Fatalln("invalid volume type", vtype) // calls exit
	}

	os.Exit(0)
}
