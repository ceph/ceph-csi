# NMVe-oF TCP for Ceph-CSI

Extending Ceph-CSI to provide volumes over NVMe-oF with Ceph RBD as backend.

## :beginner: Introduction

Users of the container platform (like Kubernetes) should be able to create
volumes that can be used over NVMe-oF TCP. This makes it possible for Windows
container hosts to consume storage from a Ceph cluster (RBD). It also allows
external hosts to use a standard protocol (not Ceph RBD) to attach
block-devices.

The below diagram shows how the general architecture of the involved components
will look like:

```
           .--------------------.
           | Container Platform |
           |     Kubernetes     |
           '--------------------'
              ^             ^
              |             |
              v             v
.-------------------.    .------------------.
| Controller-plugin |    |    Node-plugin   |
|   Provisioner     |    | Attacher/Mounter |
'-------------------'    '------------------'
   ^          ^             ^
   |          |             |
   |          v             v
   |        .-----------------.
   |        | NVMe-oF Gateway |
   |        |   ceph-nvmeof   |
   |        '-----------------'
   |          ^
   |          |
   v          v
 .--------------.
 | Ceph cluster |
 '--------------'
```

:small_blue_diamond:Container Platform: calls CSI procedures to administrate
the storage.

:small_blue_diamond:Controller-plugin: Ceph-CSI instance handling the CSI
Controller procedures.

:small_blue_diamond:Node-plugin: Ceph-CSI instance handling the CSI Node
procedures.

:small_blue_diamond:NVMe-oF Gateway: one or more instances of `ceph-nvmeof`.

:small_blue_diamond:Ceph cluster: Storage backend with RBD.

## :wrench: Requirements

The Minimum Viable Product should provide the most important functionalities
that users expect. There is a need to be prepared for extending the
functionality in the future.

### CSI Controller-plugin / Provisioner

The Controller-plugin should implement the following CSI procedures:

- `CreateVolume` / `DeleteVolume`
  1. Create RBD-image on the Ceph cluster (see Considerations)
  1. Create the NVMe-subsystem (see Considerations)
  1. Expose the RBD-image through the NVMe-oF Gateway

- `ControllerPublishVolume` / `ControllerUnpublishVolume`
  1. Allow access to the subsystem from the host where the Node-plugin will
     attach the volume

There are several parameters that can be configurable when a user creates a
volume. Some of these parameters are set by the Container Platform
administrator, others may be configurable by users.

Administrator settable parameters, this extends the parameters that can be set
for RBD volumes (set in a StorageClass):

- _all parameters for the Ceph-CSI RBD driver_
- NVMe-oF Gateway (or gateway group?)
- NVMe subsystem (or possibly detected based on Kubernetes Namespace?)

User controlled parameters (set in a PersistentVolumeClaim):

- size of the volume
- filesystem or raw-block access

### CSI Node-plugin / Attacher & Mounter

The Node-plugin should implement the following CSI procedures:

- `NodeStageVolume` / `NodeUnstageVolume`
  1. attach the NVMe-oF namespace from the NVMe-oF Gateway
  1. (Filesystem-mode only) mount the filesystem of the block-device

- `NodePublishVolume` / `NodeUnpublishVolume`
  1. bind mount the block-device or filesystem in the target location

There are likely additional requirements for attaching the NVMe-oF namespace,
including loading of kernel modules and access to nvme-tools in the
container-image.

## :incoming_envelope: Communication between components

### CSI Controller-plugin <-> Ceph Cluster

Certain operations need to be executed directly on the Ceph Cluster. These
operations will need to use go-ceph and the RADOS authentication (credentials
from the StorageClass/Volume).

### CSI Controller-plugin <-> NVMe-oF Gateway

The NVMe-oF Gateway uses a gRPC management protocol. Go can use this protocol
directly to manage the NVMe subsystems and namespaces.

### CSI Node-plugin <-> NVMe-oF Gateway

For attaching an NVMe namespace the host, the NVMe-oF TCP protocol will be
used.

## :feet: Implementation

A practical start for implementing would be have the NVMe-oF Controller-plugin
wrap CSI procedures. The Controller-plugin can call Ceph-CSI RBD
Controller-plugin procedures, and add extra calls to the NVMe-oF Gateway to
configure the RBD-image in a subsystem.

A similar wrapping of CSI procedures is already done by the Ceph-CSI NFS
Controller-plugin. The NFS Controller-plugin calls the CephFS Controller-plugin
as an internal library and executes additional `ceph nfs ...` commands.

The Ceph-CSI executable has different commandline flags for configuring the
functionality of an instance. The `-type` flag will need an extra `nvmeof`
option, this can then be set in a new Deployment and DaemonSet for deploying
the Controller-plugin and Node-plugin Pods.

### Controller-plugin

This code example is based on the [Ceph-CSI NFS
Controller-plugin](https://github.com/ceph/ceph-csi/blob/devel/internal/nfs/controller/controllerserver.go).
In this example Ceph-CSI RBD Controller-plugin is used to create the RBD-image,
after which the RBD-image is configured in the NVMe-oF Gateway.

```go
type Server struct {
    csi.UnimplementedControllerServer

    // backendServer handles the RBD requests
    backendServer *rbd.ControllerServer
}

func NewControllerServer(d *csicommon.CSIDriver) *Server {
    return &Server{
        backendServer: rbd.NewControllerServer(d),
    }
}


func (cs *Server) CreateVolume(
    ctx context.Context,
    req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {
    res, err := cs.backendServer.CreateVolume(ctx, req)
    if err != nil {
        return nil, err
    }

    backend := res.GetVolume()

    // use the backend.GetVolumeContext() to get RBD-image details
    // add the RBD-image to a subsystem in the gateway (over gRPC)

    // once the volume has been configured in the gateway,
    // add extra parameters for attaching to the VolumeContext
    backend.VolumeContext["nvmeof-gateway-address"] = ".."
    backend.VolumeContext["nvmeof-gateway-port"] = ".."
    backend.VolumeContext["nvmeof-subsystem"] = ".."
    backend.VolumeContext["nvmeof-namespace"] = ".."

    return &csi.CreateVolumeResponse{Volume: backend}, nil
}
```

### Node-plugin

The Node-plugin has the task of the NVMe-oF TCP initiator. It needs to make
sure all the runtime dependencies are available.

See also [Ceph
Docs](https://docs.ceph.com/en/latest/rbd/nvmeof-initiator-linux) for the Linux
NVMe-oF initiator.

1. The Node-plugin can load kernel modules, it already does that for CephFS and
   RBD. The nvme tools would need to be part of the container-image, either the
   Ceph base layer, or installed in the final Ceph-CSI layer.

1. When using `nvme connect-all`, some reference counting needed; the number of
   volumes in a subsystem that are active on the current worker node. Only when
   the number of active volumes is back to 0, the subsystem should be
   disconnected. Tracking this reference count can be done in a file on the host
   (needs a way to get wiped on a reboot of the host?).

   By inspecting the reference counter during `NodeStageVolume`, unnecessary
   `nvme connect-all` calls can be prevented.

> [!NOTE]  
> A possible alternative for the reference counter would be to list attached
> devices from the same gateway (group) and subsystem. This can prevent
> difficulties of counting stale volumes after a reboot.

## :question: Considerations / Open Topics

> [!WARNING]  
> Details that are under discussion and/or investigation are listed here.

- Which component should create RBD-images?

  1. the `ceph-nvmeof` gateway with `ns add --rbd-create-image`
  1. Ceph-CSI RBD so that all Ceph-CSI procedures work 'normally'

  The preference goes to Ceph-CSI RBD creating the RBD-images. It will make it
  easier to do other CSI procedures for resizing, snapshotting and cloning.

- Are credentials required for attaching a NVMe namespace on a host?

  1. Can these credentials be stored in the VolumeContext of a
     PersistentVolume? (Probably they should not.)
  1. Are the credentials unique per NVMe namespace?
  1. Can these credentials be obtained from the NVMe Gateway?

  Credentials are optional, some access permissions are in place through the
  ControllerPublishVolume CSI procedure where the worker node that attaches the
  NVMe device will be allowed access (IP-address). Other nodes do not have
  access. The IP address is removed from the access-list again during
  ControllerUnpublishVolume.

- What is the difference for using a single NVMe-oF Gateway vs a Gateway Group?

  1. Is there a single address for a group that can be used (like a
     Kubernetes Service, redirecting to one of the gateways)?
  1. Are gRPC API calls for configuring subsystems/namespaces different?
  1. A Gateway or Gateway Group should probably be configured by the admin in
     the StorageClass (different StorageClasses for different Gateway
     Groups).

  Gateway groups are recommended to use, it provides high-availability. This
  means that the Node-plugin should use `nvme connect-all` and only call `nvme
  disconnect` when the last volume is detached from the host in
  NodeUnstageVolume.

- What component should create the NVMe-subsystem(s)?

  There are two options for creating a subsystem:

  1. Should be created in advance while deploying the NVMe-oF Gateway (would be
     empty). The subsystem should be included in the parameters of the
     StorageClass. All NVMe-namespaces would be added to a the single subsystem.

  1. The Controller-plugin creates the namespace during CreateVolume.  This
     would allow additional logic for placing namespaces in different
     subsystems (based on Kubernetes namespaces aka tenants).
