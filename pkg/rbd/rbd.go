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

package rbd

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/golang/glog"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"

	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/util/nsenter"
	"k8s.io/utils/exec"
)

// PluginFolder defines the location of rbdplugin
const (
	PluginFolder      = "/var/lib/kubelet/plugins/csi-rbdplugin"
	rbdDefaultAdminId = "admin"
	rbdDefaultUserId  = rbdDefaultAdminId
)

type rbd struct {
	driver *csicommon.CSIDriver

	ids *identityServer
	ns  *nodeServer
	cs  *controllerServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
}

var (
	rbdDriver *rbd
	version   = "0.3.0"
)

var rbdVolumes map[string]*rbdVolume
var rbdSnapshots map[string]*rbdSnapshot

// Init checks for the persistent volume file and loads all found volumes
// into a memory structure
func init() {
	rbdVolumes = map[string]*rbdVolume{}
	rbdSnapshots = map[string]*rbdSnapshot{}
	if _, err := os.Stat(path.Join(PluginFolder, "controller")); os.IsNotExist(err) {
		glog.Infof("rbd: folder %s not found. Creating... \n", path.Join(PluginFolder, "controller"))
		if err := os.Mkdir(path.Join(PluginFolder, "controller"), 0755); err != nil {
			glog.Fatalf("Failed to create a controller's volumes folder with error: %v\n", err)
		}
	} else {
		// Since "controller" folder exists, it means the rbdplugin has already been running, it means
		// there might be some volumes left, they must be re-inserted into rbdVolumes map
		loadExVolumes()
	}
	if _, err := os.Stat(path.Join(PluginFolder, "controller-snap")); os.IsNotExist(err) {
		glog.Infof("rbd: folder %s not found. Creating... \n", path.Join(PluginFolder, "controller-snap"))
		if err := os.Mkdir(path.Join(PluginFolder, "controller-snap"), 0755); err != nil {
			glog.Fatalf("Failed to create a controller's snapshots folder with error: %v\n", err)
		}
	} else {
		// Since "controller-snap" folder exists, it means the rbdplugin has already been running, it means
		// there might be some snapshots left, they must be re-inserted into rbdSnapshots map
		loadExSnapshots()
	}
}

// loadExSnapshots check for any *.json files in the  PluginFolder/controller-snap folder
// and loads then into rbdSnapshots map
func loadExSnapshots() {
	rbdSnap := rbdSnapshot{}
	files, err := ioutil.ReadDir(path.Join(PluginFolder, "controller-snap"))
	if err != nil {
		glog.Infof("rbd: failed to read controller's snapshots folder: %s error:%v", path.Join(PluginFolder, "controller-snap"), err)
		return
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		fp, err := os.Open(path.Join(PluginFolder, "controller-snap", f.Name()))
		if err != nil {
			glog.Infof("rbd: open file: %s err %%v", f.Name(), err)
			continue
		}
		decoder := json.NewDecoder(fp)
		if err = decoder.Decode(&rbdSnap); err != nil {
			glog.Infof("rbd: decode file: %s err: %v", f.Name(), err)
			fp.Close()
			continue
		}
		rbdSnapshots[rbdSnap.SnapID] = &rbdSnap
	}
	glog.Infof("rbd: Loaded %d snapshots from %s", len(rbdSnapshots), path.Join(PluginFolder, "controller-snap"))
}

// loadExVolumes check for any *.json files in the  PluginFolder/controller folder
// and loads then into rbdVolumes map
func loadExVolumes() {
	rbdVol := rbdVolume{}
	files, err := ioutil.ReadDir(path.Join(PluginFolder, "controller"))
	if err != nil {
		glog.Infof("rbd: failed to read controller's volumes folder: %s error:%v", path.Join(PluginFolder, "controller"), err)
		return
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		fp, err := os.Open(path.Join(PluginFolder, "controller", f.Name()))
		if err != nil {
			glog.Infof("rbd: open file: %s err %%v", f.Name(), err)
			continue
		}
		decoder := json.NewDecoder(fp)
		if err = decoder.Decode(&rbdVol); err != nil {
			glog.Infof("rbd: decode file: %s err: %v", f.Name(), err)
			fp.Close()
			continue
		}
		rbdVolumes[rbdVol.VolID] = &rbdVol
	}
	glog.Infof("rbd: Loaded %d volumes from %s", len(rbdVolumes), path.Join(PluginFolder, "controller"))
}

func GetRBDDriver() *rbd {
	return &rbd{}
}

func NewIdentityServer(d *csicommon.CSIDriver) *identityServer {
	return &identityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

func NewControllerServer(d *csicommon.CSIDriver) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
	}
}

func NewNodeServer(d *csicommon.CSIDriver, containerized bool) (*nodeServer, error) {
	mounter := mount.New("")
	if containerized {
		ne, err := nsenter.NewNsenter(nsenter.DefaultHostRootFsPath, exec.New())
		if err != nil {
			return nil, err
		}
		mounter = mount.NewNsenterMounter("", ne)
	}
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
		mounter:           mounter,
	}, nil
}

func (rbd *rbd) Run(driverName, nodeID, endpoint string, containerized bool) {
	var err error
	glog.Infof("Driver: %v version: %v", driverName, version)

	// Initialize default library driver
	rbd.driver = csicommon.NewCSIDriver(driverName, version, nodeID)
	if rbd.driver == nil {
		glog.Fatalln("Failed to initialize CSI Driver.")
	}
	rbd.driver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	})
	rbd.driver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER})

	// Create GRPC servers
	rbd.ids = NewIdentityServer(rbd.driver)
	rbd.ns, err = NewNodeServer(rbd.driver, containerized)
	if err != nil {
		glog.Fatalln("failed to start node server, err %v", err)
	}
	rbd.cs = NewControllerServer(rbd.driver)
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(endpoint, rbd.ids, rbd.cs, rbd.ns)
	s.Wait()
}
