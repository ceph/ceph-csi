/*
Copyright 2018 The Ceph-CSI Authors.

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
	csicommon "github.com/ceph/ceph-csi/pkg/csi-common"
	"github.com/ceph/ceph-csi/pkg/util"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/util/nsenter"
	"k8s.io/utils/exec"
)

/*
RADOS omaps usage:

This note details how we preserve idempotent nature of create requests and retain the relationship
between orchestrator (CO) generated Names and plugin generated names for images and snapshots

The implementation uses Ceph RADOS omaps to preserve the relationship between request name and
generated image (or snapshot) name. There are 4 types of omaps in use,
- A "csi.volumes.[csi-id]" (or "csi.volumes"+.+CsiInstanceID), we call this the csiVolsDirectory
  - stores keys named using the CO generated names for volume requests
  - keys are named "csi.volume."+[CO generated VolName]
  - Key value contains the RBD image uuid that is created or will be created, for the CO provided
  name

- A "csi.snaps.[csi-id]" (or "csi.snaps"+.+CsiInstanceID), we refer to this as the csiSnapsDirectory
  - stores keys named using the CO generated names for snapshot requests
  - keys are named "csi.snap."+[CO generated SnapName]
  - Key value contains the RBD snapshot uuid that is created or will be created, for the CO
  provided name

- A per image omap named "rbd.csi.volume."+[RBD image uuid], we refer to this as the rbdImageOMap
  - stores a single key named "csi.volname", that has the value of the CO generated VolName that
  this image refers to

- A per snapshot omap named "rbd.csi.snap."+[RBD snapshot uuid], we refer to this as the snapOMap
  - stores a key named "csi.snapname", that has the value of the CO generated SnapName that this
  snapshot refers to
  - also stores another key named "csi.source", that has the value of the image name that is the
  source of the snapshot

Creation of omaps:
When a volume create request is received (or a snapshot create, the snapshot is not detailed in this
	comment further as the process is similar),
- The csiVolsDirectory is consulted to find if there is already a key with the CO VolName, and if present,
it is used to read its references to reach the RBD image that backs this VolName, to check if the
RBD image can satisfy the requirements for the request
  - If during the process of checking the same, it is found that some linking information is stale
  or missing, the corresponding keys upto the key in the csiVolsDirectory is cleaned up, to start afresh
- If the key with the CO VolName is not found, or was cleaned up, the request is treated as a
new create request, and an rbdImageOMap is created first with a generated uuid, this ensures that we
do not use a uuid that is already in use
- Next, a key with the VolName is created in the csiVolsDirectory, and its value is updated to store the
generated uuid
- This is followed by updating the rbdImageOMap with the VolName in the rbdImageCSIVolNameKey
- Finally, the image is created (or promoted from a snapshot, if content source was provided) using
the uuid and a corresponding image name prefix (rbdImgNamePrefix or rbdSnapNamePrefix)

The entire operation is locked based on VolName hash, to ensure there is only ever a single entity
modifying the related omaps for a given VolName.

This ensures idempotent nature of creates, as the same CO generated VolName would attempt to use
the same RBD image name to serve the request, as the relations are saved in the respective omaps.

Deletion of omaps:
Delete requests would not contain the VolName, hence deletion uses the volume ID, which is encoded
with the image name in it, to find the image and the rbdImageOMap. The rbdImageOMap is read to get
the VolName that this image points to. This VolName can be further used to read and delete the key
from the csiVolsDirectory.

As we trace back and find the VolName, we also take a hash based lock on the VolName before
proceeding with deleting the image and the related omap entries, to ensure there is only ever a
single entity modifying the related omaps for a given VolName.
*/

const (
	// volIDVersion is the version number of volume ID encoding scheme
	volIDVersion      uint16 = 1
	rbdDefaultAdminID        = "admin"
	rbdDefaultUserID         = rbdDefaultAdminID

	// CSI volume-name keyname prefix, for key in csiVolsDirectory, suffix is the CSI passed volume name
	csiVolNameKeyPrefix = "csi.volume."
	// Per RBD image object map name prefix, suffix is the RBD image uuid
	rbdImageOMapPrefix = "csi.volume."
	// CSI volume-name key in per RBD image object map, containing CSI volume-name for which the
	// image was created
	rbdImageCSIVolNameKey = "csi.volname"
	// RBD image name prefix, suffix is a uuid generated per image
	rbdImgNamePrefix = "csi-vol-"

	//CSI snap-name keyname prefix, for key in csiSnapsDirectory, suffix is the CSI passed snapshot name
	csiSnapNameKeyPrefix = "csi.snap."
	// Per RBD snapshot object map name prefix, suffix is the RBD image uuid
	rbdSnapOMapPrefix = "csi.snap."
	// CSI snap-name key in per RBD snapshot object map, containing CSI snapshot-name for which the
	// snapshot was created
	rbdSnapCSISnapNameKey = "csi.snapname"
	// source image name key in per RBD snapshot object map, containing RBD source image name for
	// which the snapshot was created
	rbdSnapSourceImageKey = "csi.source"
	// RBD snapshot name prefix, suffix is a uuid generated per snapshot
	rbdSnapNamePrefix = "csi-snap-"
)

// PluginFolder defines the location of ceph plugin
var PluginFolder = "/var/lib/kubelet/plugins/"

// Driver contains the default identity,node and controller struct
type Driver struct {
	cd *csicommon.CSIDriver

	ids *IdentityServer
	ns  *NodeServer
	cs  *ControllerServer
}

var (
	version = "1.0.0"
	// confStore is the global config store
	confStore *util.ConfigStore
	// CsiInstanceID is the instance ID that is unique to an instance of CSI, used when sharing
	// ceph clusters across CSI instances, to differentiate omap names per CSI instance
	CsiInstanceID = "default"
	// csiVolsDirectory is the name of the CSI volumes object map that contains CSI volume-name
	// based keys
	csiVolsDirectory = "csi.volumes"
	// csiSnapsDirectory is the name of the CSI snapshots object map that contains CSI snapshot-name based keys
	csiSnapsDirectory = "csi.snaps"
)

// NewDriver returns new rbd driver
func NewDriver() *Driver {
	return &Driver{}
}

// NewIdentityServer initialize a identity server for rbd CSI driver
func NewIdentityServer(d *csicommon.CSIDriver) *IdentityServer {
	return &IdentityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

// NewControllerServer initialize a controller server for rbd CSI driver
func NewControllerServer(d *csicommon.CSIDriver) *ControllerServer {
	return &ControllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
	}
}

// NewNodeServer initialize a node server for rbd CSI driver.
func NewNodeServer(d *csicommon.CSIDriver, containerized bool) (*NodeServer, error) {
	mounter := mount.New("")
	if containerized {
		ne, err := nsenter.NewNsenter(nsenter.DefaultHostRootFsPath, exec.New())
		if err != nil {
			return nil, err
		}
		mounter = mount.NewNsenterMounter("", ne)
	}
	return &NodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
		mounter:           mounter,
	}, nil
}

// Run start a non-blocking grpc controller,node and identityserver for
// rbd CSI driver which can serve multiple parallel requests
func (r *Driver) Run(driverName, nodeID, endpoint, configRoot, instanceID string, containerized bool) {
	var err error

	klog.Infof("Driver: %v version: %v", driverName, version)

	// Initialize config store
	confStore, err = util.NewConfigStore(configRoot)
	if err != nil {
		klog.Fatalln("Failed to initialize config store")
	}

	// Create ceph.conf for use with CLI commands
	if err = util.WriteCephConfig(); err != nil {
		klog.Fatalf("failed to write ceph configuration file")
	}

	// Use passed in instance ID, if provided for omap suffix naming
	if instanceID != "" {
		CsiInstanceID = instanceID
	}
	csiVolsDirectory = csiVolsDirectory + "." + CsiInstanceID
	csiSnapsDirectory = csiSnapsDirectory + "." + CsiInstanceID

	// Initialize default library driver
	r.cd = csicommon.NewCSIDriver(driverName, version, nodeID)
	if r.cd == nil {
		klog.Fatalln("Failed to initialize CSI Driver.")
	}
	r.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
	})

	// We only support the multi-writer option when using block, but it's a supported capability for the plugin in general
	// In addition, we want to add the remaining modes like MULTI_NODE_READER_ONLY,
	// MULTI_NODE_SINGLE_WRITER etc, but need to do some verification of RO modes first
	// will work those as follow up features
	r.cd.AddVolumeCapabilityAccessModes(
		[]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})

	// Create GRPC servers
	r.ids = NewIdentityServer(r.cd)
	r.ns, err = NewNodeServer(r.cd, containerized)
	if err != nil {
		klog.Fatalf("failed to start node server, err %v\n", err)
	}

	r.cs = NewControllerServer(r.cd)

	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(endpoint, r.ids, r.cs, r.ns)
	s.Wait()
}
