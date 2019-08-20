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
	"path/filepath"
	"time"

	"github.com/ceph/ceph-csi/pkg/cephfs"
	"github.com/ceph/ceph-csi/pkg/liveness"
	"github.com/ceph/ceph-csi/pkg/rbd"
	"github.com/ceph/ceph-csi/pkg/util"
	"k8s.io/klog"
)

const (
	rbdType      = "rbd"
	cephfsType   = "cephfs"
	livenessType = "liveness"

	rbdDefaultName      = "rbd.csi.ceph.com"
	cephfsDefaultName   = "cephfs.csi.ceph.com"
	livenessDefaultName = "liveness.csi.ceph.com"
)

var (
	conf util.Config
)

func init() {

	// common flags
	flag.StringVar(&conf.Vtype, "type", "", "driver type [rbd|cephfs|liveness]")
	flag.StringVar(&conf.Endpoint, "endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	flag.StringVar(&conf.DriverName, "drivername", "", "name of the driver")
	flag.StringVar(&conf.NodeID, "nodeid", "", "node id")
	flag.StringVar(&conf.InstanceID, "instanceid", "", "Unique ID distinguishing this instance of Ceph CSI among other"+
		" instances, when sharing Ceph clusters across CSI instances for provisioning")
	flag.StringVar(&conf.MetadataStorage, "metadatastorage", "", "metadata persistence method [node|k8s_configmap]")
	flag.StringVar(&conf.PluginPath, "pluginpath", "/var/lib/kubelet/plugins/", "the location of cephcsi plugin")
	flag.IntVar(&conf.PidLimit, "pidlimit", 0, "the PID limit to configure through cgroups")
	flag.BoolVar(&conf.IsControllerServer, "controllerserver", false, "start cephcsi controller server")
	flag.BoolVar(&conf.IsNodeServer, "nodeserver", false, "start cephcsi node server")

	// rbd related flags
	flag.BoolVar(&conf.Containerized, "containerized", true, "whether run as containerized")

	// cephfs related flags
	flag.StringVar(&conf.VolumeMounter, "volumemounter", "", "default volume mounter (possible options are 'kernel', 'fuse')")
	flag.StringVar(&conf.MountCacheDir, "mountcachedir", "", "mount info cache save dir")

	// livenes related flags
	flag.IntVar(&conf.LivenessPort, "livenessport", 8080, "TCP port for liveness requests")
	flag.StringVar(&conf.LivenessPath, "livenesspath", "/metrics", "path of prometheus endpoint where metrics will be available")
	flag.DurationVar(&conf.PollTime, "polltime", time.Second*60, "time interval in seconds between each poll")
	flag.DurationVar(&conf.PoolTimeout, "timeout", time.Second*3, "probe timeout in seconds")
	klog.InitFlags(nil)
	if err := flag.Set("logtostderr", "true"); err != nil {
		klog.Exitf("failed to set logtostderr flag: %v", err)
	}
	flag.Parse()
}

func getDriverName() string {
	// was explicitly passed a driver name
	if conf.DriverName != "" {
		return conf.DriverName
	}
	// select driver name based on volume type
	switch conf.Vtype {
	case rbdType:
		return rbdDefaultName
	case cephfsType:
		return cephfsDefaultName
	case livenessType:
		return livenessDefaultName
	default:
		return ""
	}
}

func main() {
	klog.Infof("Driver version: %s and Git version: %s", util.DriverVersion, util.GitCommit)
	var cp util.CachePersister

	if conf.Vtype == "" {
		klog.Fatalln("driver type not specified")
	}

	dname := getDriverName()
	err := util.ValidateDriverName(dname)
	if err != nil {
		klog.Fatalln(err) // calls exit
	}
	csipluginPath := filepath.Join(conf.PluginPath, dname)
	if conf.MetadataStorage != "" {
		cp, err = util.CreatePersistanceStorage(
			csipluginPath, conf.MetadataStorage, conf.PluginPath)
		if err != nil {
			os.Exit(1)
		}
	}

	// the driver may need a higher PID limit for handling all concurrent requests
	if conf.PidLimit != 0 {
		currentLimit, err := util.GetPIDLimit()
		if err != nil {
			klog.Errorf("Failed to get the PID limit, can not reconfigure: %v", err)
		} else {
			klog.Infof("Initial PID limit is set to %d", currentLimit)
			err = util.SetPIDLimit(conf.PidLimit)
			if err != nil {
				klog.Errorf("Failed to set new PID limit to %d: %v", conf.PidLimit, err)
			} else {
				s := ""
				if conf.PidLimit == -1 {
					s = " (max)"
				}
				klog.Infof("Reconfigured PID limit to %d%s", conf.PidLimit, s)
			}
		}
	}

	klog.Infof("Starting driver type: %v with name: %v", conf.Vtype, dname)
	switch conf.Vtype {
	case rbdType:
		driver := rbd.NewDriver()
		driver.Run(&conf, cp)

	case cephfsType:
		driver := cephfs.NewDriver()
		driver.Run(&conf, cp)

	case livenessType:
		liveness.Run(&conf)

	default:
		klog.Fatalln("invalid volume type", conf.Vtype) // calls exit
	}

	os.Exit(0)
}
