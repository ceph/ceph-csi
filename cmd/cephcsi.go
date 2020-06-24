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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ceph/ceph-csi/internal/cephfs"
	"github.com/ceph/ceph-csi/internal/liveness"
	"github.com/ceph/ceph-csi/internal/rbd"
	"github.com/ceph/ceph-csi/internal/util"

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
	flag.StringVar(&conf.DomainLabels, "domainlabels", "", "list of kubernetes node labels, that determines the topology"+
		" domain the node belongs to, separated by ','")

	// cephfs related flags
	// marking this as deprecated, remove it in next major release
	flag.StringVar(&conf.MountCacheDir, "mountcachedir", "", "mount info cache save dir")
	flag.BoolVar(&conf.ForceKernelCephFS, "forcecephkernelclient", false, "enable Ceph Kernel clients on kernel < 4.17 which support quotas")

	// liveness/grpc metrics related flags
	flag.IntVar(&conf.MetricsPort, "metricsport", 8080, "TCP port for liveness/grpc metrics requests")
	flag.StringVar(&conf.MetricsPath, "metricspath", "/metrics", "path of prometheus endpoint where metrics will be available")
	flag.DurationVar(&conf.PollTime, "polltime", time.Second*60, "time interval in seconds between each poll")
	flag.DurationVar(&conf.PoolTimeout, "timeout", time.Second*3, "probe timeout in seconds")

	flag.BoolVar(&conf.EnableGRPCMetrics, "enablegrpcmetrics", false, "[DEPRECATED] enable grpc metrics")
	flag.StringVar(&conf.HistogramOption, "histogramoption", "0.5,2,6",
		"[DEPRECATED] Histogram option for grpc metrics, should be comma separated value, ex:= 0.5,2,6 where start=0.5 factor=2, count=6")

	flag.UintVar(&conf.RbdHardMaxCloneDepth, "rbdhardmaxclonedepth", 8, "Hard limit for maximum number of nested volume clones that are taken before a flatten occurs")
	flag.UintVar(&conf.RbdSoftMaxCloneDepth, "rbdsoftmaxclonedepth", 4, "Soft limit for maximum number of nested volume clones that are taken before a flatten occurs")
	flag.BoolVar(&conf.Version, "version", false, "Print cephcsi version information")

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
	if conf.Version {
		fmt.Println("Cephcsi Version:", util.DriverVersion)
		fmt.Println("Git Commit:", util.GitCommit)
		fmt.Println("Go Version:", runtime.Version())
		fmt.Println("Compiler:", runtime.Compiler)
		fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		if kv, err := util.GetKernelVersion(); err == nil {
			fmt.Println("Kernel:", kv)
		}
		os.Exit(0)
	}

	klog.V(1).Infof("Driver version: %s and Git version: %s", util.DriverVersion, util.GitCommit)
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
		currentLimit, pidErr := util.GetPIDLimit()
		if pidErr != nil {
			klog.Errorf("Failed to get the PID limit, can not reconfigure: %v", pidErr)
		} else {
			klog.V(1).Infof("Initial PID limit is set to %d", currentLimit)
			err = util.SetPIDLimit(conf.PidLimit)
			if err != nil {
				klog.Errorf("Failed to set new PID limit to %d: %v", conf.PidLimit, err)
			} else {
				s := ""
				if conf.PidLimit == -1 {
					s = " (max)"
				}
				klog.V(1).Infof("Reconfigured PID limit to %d%s", conf.PidLimit, s)
			}
		}
	}

	if conf.EnableGRPCMetrics || conf.Vtype == livenessType {
		// validate metrics endpoint
		conf.MetricsIP = os.Getenv("POD_IP")

		if conf.MetricsIP == "" {
			klog.Warning("missing POD_IP env var defaulting to 0.0.0.0")
			conf.MetricsIP = "0.0.0.0"
		}
		err = util.ValidateURL(&conf)
		if err != nil {
			klog.Fatalln(err)
		}
	}

	klog.V(1).Infof("Starting driver type: %v with name: %v", conf.Vtype, dname)
	switch conf.Vtype {
	case rbdType:
		validateCloneDepthFlag(&conf)
		driver := rbd.NewDriver()
		driver.Run(&conf, cp)

	case cephfsType:
		if conf.MountCacheDir != "" {
			klog.Warning("mountcachedir option is deprecated")
		}
		driver := cephfs.NewDriver()
		driver.Run(&conf, cp)

	case livenessType:
		liveness.Run(&conf)

	default:
		klog.Fatalln("invalid volume type", conf.Vtype) // calls exit
	}

	os.Exit(0)
}

func validateCloneDepthFlag(conf *util.Config) {
	// keeping hardlimit to 14 as max to avoid max image depth
	if conf.RbdHardMaxCloneDepth == 0 || conf.RbdHardMaxCloneDepth > 14 {
		klog.Fatalln("rbdhardmaxclonedepth flag value should be between 1 and 14")
	}

	if conf.RbdSoftMaxCloneDepth > conf.RbdHardMaxCloneDepth {
		klog.Fatalln("rbdsoftmaxclonedepth flag value should not be greater than rbdhardmaxclonedepth")
	}
}
