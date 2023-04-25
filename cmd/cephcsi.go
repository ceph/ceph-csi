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
	"runtime"
	"time"

	"github.com/ceph/ceph-csi/internal/cephfs"
	"github.com/ceph/ceph-csi/internal/controller"
	"github.com/ceph/ceph-csi/internal/controller/persistentvolume"
	"github.com/ceph/ceph-csi/internal/liveness"
	nfsdriver "github.com/ceph/ceph-csi/internal/nfs/driver"
	rbddriver "github.com/ceph/ceph-csi/internal/rbd/driver"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"k8s.io/klog/v2"
)

const (
	rbdType        = "rbd"
	cephFSType     = "cephfs"
	nfsType        = "nfs"
	livenessType   = "liveness"
	controllerType = "controller"

	rbdDefaultName      = "rbd.csi.ceph.com"
	cephFSDefaultName   = "cephfs.csi.ceph.com"
	nfsDefaultName      = "nfs.csi.ceph.com"
	livenessDefaultName = "liveness.csi.ceph.com"

	pollTime     = 60 // seconds
	probeTimeout = 3  // seconds

	// use default namespace if namespace is not set.
	defaultNS = "default"

	defaultPluginPath  = "/var/lib/kubelet/plugins"
	defaultStagingPath = defaultPluginPath + "/kubernetes.io/csi/"
)

var conf util.Config

func init() {
	// common flags
	flag.StringVar(&conf.Vtype, "type", "", "driver type [rbd|cephfs|nfs|liveness|controller]")
	flag.StringVar(&conf.Endpoint, "endpoint", "unix:///tmp/csi.sock", "CSI endpoint")
	flag.StringVar(&conf.DriverName, "drivername", "", "name of the driver")
	flag.StringVar(&conf.DriverNamespace, "drivernamespace", defaultNS, "namespace in which driver is deployed")
	flag.StringVar(&conf.NodeID, "nodeid", "", "node id")
	flag.StringVar(&conf.PluginPath, "pluginpath", defaultPluginPath, "plugin path")
	flag.StringVar(&conf.StagingPath, "stagingpath", defaultStagingPath, "staging path")
	flag.StringVar(&conf.ClusterName, "clustername", "", "name of the cluster")
	flag.BoolVar(&conf.SetMetadata, "setmetadata", false, "set metadata on the volume")
	flag.StringVar(&conf.InstanceID, "instanceid", "", "Unique ID distinguishing this instance of Ceph CSI among other"+
		" instances, when sharing Ceph clusters across CSI instances for provisioning")
	flag.IntVar(&conf.PidLimit, "pidlimit", 0, "the PID limit to configure through cgroups")
	flag.BoolVar(&conf.IsControllerServer, "controllerserver", false, "start cephcsi controller server")
	flag.BoolVar(&conf.IsNodeServer, "nodeserver", false, "start cephcsi node server")
	flag.StringVar(
		&conf.DomainLabels,
		"domainlabels",
		"",
		"list of Kubernetes node labels, that determines the topology"+
			" domain the node belongs to, separated by ','")
	flag.BoolVar(&conf.EnableReadAffinity, "enable-read-affinity", false, "enable read affinity")
	flag.StringVar(
		&conf.CrushLocationLabels,
		"crush-location-labels",
		"",
		"list of Kubernetes node labels, that determines the"+
			" CRUSH location the node belongs to, separated by ','")

	// cephfs related flags
	flag.BoolVar(
		&conf.ForceKernelCephFS,
		"forcecephkernelclient",
		false,
		"enable Ceph Kernel clients on kernel < 4.17 which support quotas")
	flag.StringVar(
		&conf.KernelMountOptions,
		"kernelmountoptions",
		"",
		"Comma separated string of mount options accepted by cephfs kernel mounter")
	flag.StringVar(
		&conf.FuseMountOptions,
		"fusemountoptions",
		"",
		"Comma separated string of mount options accepted by ceph-fuse mounter")

	// liveness/grpc metrics related flags
	flag.IntVar(&conf.MetricsPort, "metricsport", 8080, "TCP port for liveness/grpc metrics requests")
	flag.StringVar(
		&conf.MetricsPath,
		"metricspath",
		"/metrics",
		"path of prometheus endpoint where metrics will be available")
	flag.DurationVar(&conf.PollTime, "polltime", time.Second*pollTime, "time interval in seconds between each poll")
	flag.DurationVar(&conf.PoolTimeout, "timeout", time.Second*probeTimeout, "probe timeout in seconds")

	flag.BoolVar(&conf.EnableGRPCMetrics, "enablegrpcmetrics", false, "[DEPRECATED] enable grpc metrics")
	flag.StringVar(
		&conf.HistogramOption,
		"histogramoption",
		"0.5,2,6",
		"[DEPRECATED] Histogram option for grpc metrics, should be comma separated value, "+
			"ex:= 0.5,2,6 where start=0.5 factor=2, count=6")

	flag.UintVar(
		&conf.RbdHardMaxCloneDepth,
		"rbdhardmaxclonedepth",
		8,
		"Hard limit for maximum number of nested volume clones that are taken before a flatten occurs")
	flag.UintVar(
		&conf.RbdSoftMaxCloneDepth,
		"rbdsoftmaxclonedepth",
		4,
		"Soft limit for maximum number of nested volume clones that are taken before a flatten occurs")
	flag.UintVar(
		&conf.MaxSnapshotsOnImage,
		"maxsnapshotsonimage",
		450,
		"Maximum number of snapshots allowed on rbd image without flattening")
	flag.UintVar(
		&conf.MinSnapshotsOnImage,
		"minsnapshotsonimage",
		250,
		"Minimum number of snapshots required on rbd image to start flattening")
	flag.BoolVar(&conf.SkipForceFlatten, "skipforceflatten", false,
		"skip image flattening if kernel support mapping of rbd images which has the deep-flatten feature")

	flag.BoolVar(&conf.Version, "version", false, "Print cephcsi version information")
	flag.BoolVar(&conf.EnableProfiling, "enableprofiling", false, "enable go profiling")

	// CSI-Addons configuration
	flag.StringVar(&conf.CSIAddonsEndpoint, "csi-addons-endpoint", "unix:///tmp/csi-addons.sock", "CSI-Addons endpoint")

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
	case cephFSType:
		return cephFSDefaultName
	case nfsType:
		return nfsDefaultName
	case livenessType:
		return livenessDefaultName
	default:
		return ""
	}
}

func printVersion() {
	fmt.Println("Cephcsi Version:", util.DriverVersion)
	fmt.Println("Git Commit:", util.GitCommit)
	fmt.Println("Go Version:", runtime.Version())
	fmt.Println("Compiler:", runtime.Compiler)
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if kv, err := util.GetKernelVersion(); err == nil {
		fmt.Println("Kernel:", kv)
	}
}

func main() {
	if conf.Version {
		printVersion()
		os.Exit(0)
	}
	log.DefaultLog("Driver version: %s and Git version: %s", util.DriverVersion, util.GitCommit)

	if conf.Vtype == "" {
		logAndExit("driver type not specified")
	}

	dname := getDriverName()
	err := util.ValidateDriverName(dname)
	if err != nil {
		logAndExit(err.Error())
	}

	setPIDLimit(&conf)

	if conf.EnableGRPCMetrics || conf.Vtype == livenessType {
		// validate metrics endpoint
		conf.MetricsIP = os.Getenv("POD_IP")

		if conf.MetricsIP == "" {
			klog.Warning("missing POD_IP env var defaulting to 0.0.0.0")
			conf.MetricsIP = "0.0.0.0"
		}
		err = util.ValidateURL(&conf)
		if err != nil {
			logAndExit(err.Error())
		}
	}

	if err = util.WriteCephConfig(); err != nil {
		log.FatalLogMsg("failed to write ceph configuration file (%v)", err)
	}

	log.DefaultLog("Starting driver type: %v with name: %v", conf.Vtype, dname)
	switch conf.Vtype {
	case rbdType:
		validateCloneDepthFlag(&conf)
		validateMaxSnaphostFlag(&conf)
		driver := rbddriver.NewDriver()
		driver.Run(&conf)

	case cephFSType:
		driver := cephfs.NewDriver()
		driver.Run(&conf)

	case nfsType:
		driver := nfsdriver.NewDriver()
		driver.Run(&conf)

	case livenessType:
		liveness.Run(&conf)

	case controllerType:
		cfg := controller.Config{
			DriverName:  dname,
			Namespace:   conf.DriverNamespace,
			ClusterName: conf.ClusterName,
			SetMetadata: conf.SetMetadata,
		}
		// initialize all controllers before starting.
		initControllers()
		err = controller.Start(cfg)
		if err != nil {
			logAndExit(err.Error())
		}
	}

	os.Exit(0)
}

func setPIDLimit(conf *util.Config) {
	// set pidLimit only for NodeServer
	// the driver may need a higher PID limit for handling all concurrent requests
	if conf.IsNodeServer && conf.PidLimit != 0 {
		currentLimit, pidErr := util.GetPIDLimit()
		if pidErr != nil {
			klog.Errorf("Failed to get the PID limit, can not reconfigure: %v", pidErr)
		} else {
			log.DefaultLog("Initial PID limit is set to %d", currentLimit)
			err := util.SetPIDLimit(conf.PidLimit)
			switch {
			case err != nil:
				klog.Errorf("Failed to set new PID limit to %d: %v", conf.PidLimit, err)
			case conf.PidLimit == -1:
				log.DefaultLog("Reconfigured PID limit to %d (max)", conf.PidLimit)
			default:
				log.DefaultLog("Reconfigured PID limit to %d", conf.PidLimit)
			}
		}
	}
}

// initControllers will initialize all the controllers.
func initControllers() {
	// Add list of controller here.
	persistentvolume.Init()
}

func validateCloneDepthFlag(conf *util.Config) {
	// keeping hardlimit to 14 as max to avoid max image depth
	if conf.RbdHardMaxCloneDepth == 0 || conf.RbdHardMaxCloneDepth > 14 {
		logAndExit("rbdhardmaxclonedepth flag value should be between 1 and 14")
	}

	if conf.RbdSoftMaxCloneDepth > conf.RbdHardMaxCloneDepth {
		logAndExit("rbdsoftmaxclonedepth flag value should not be greater than rbdhardmaxclonedepth")
	}
}

func validateMaxSnaphostFlag(conf *util.Config) {
	// maximum number of snapshots on an image are 510 [1] and 16 images in
	// a parent/child chain [2],keeping snapshot limit to 500 to avoid issues.
	// [1] https://github.com/torvalds/linux/blob/master/drivers/block/rbd.c#L98
	// [2] https://github.com/torvalds/linux/blob/master/drivers/block/rbd.c#L92
	if conf.MaxSnapshotsOnImage == 0 || conf.MaxSnapshotsOnImage > 500 {
		logAndExit("maxsnapshotsonimage flag value should be between 1 and 500")
	}

	if conf.MinSnapshotsOnImage > conf.MaxSnapshotsOnImage {
		logAndExit("minsnapshotsonimage flag value should be less than maxsnapshotsonimage")
	}
}

func logAndExit(msg string) {
	klog.Errorln(msg)
	os.Exit(1)
}
