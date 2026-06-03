# Dynamic provisioning of NFS volumes

Ceph has [support for NFS-Ganesha to export directories][ceph_mgr_nfs] on
CephFS. This can be used to export CephFS based volumes over NFS.

## Node-Plugin for mounting NFS-exports

The Kubernetes CSI community provides and maintains a [NFS CSI][nfs_csi]
driver. This driver can be used as a Node-Plugin so that NFS CSI volumes can be
mounted. When a CSI volume has the `server` and `share`
[parameters][nfs_csi_params], the Node-Plugin will be able to mount the
NFS-export.

## Exporting CephFS based volumes over NFS

Ceph-CSI already creates CephFS volumes, that can be mounted over the native
CephFS protocol. A new provisioner in Ceph-CSI can create CephFS volumes, and
include the required [NFS CSI parameters][nfs_csi_params] so that the [NFS CSI
driver][nfs_csi] can mount the CephFS volume over NFS.

The provisioner that handles the CSI requests for NFS volume can call the [Ceph
Mgr commands to export/unexport][ceph_mgr_nfs] the CephFS volumes. The CephFS
volumes would be internally managed by the NFS provisioner, and only be exposed
as NFS CSI volumes towards the consumers.

### `CreateVolume` CSI operation

When the Ceph-CSI NFS provisioner is requested to create a NFS CSI volume, the
following steps need to be taken:

1. create a CephFS volume, use the CephFS `CreateVolume` call or other internal
   API
1. call the Ceph Mgr API to export the CephFS volume with NFS-Ganesha
1. return the NFS CSI volume, with `server` and `share` parameters (other
   parameters that are useful for CephFS volume management may be kept)

The 2nd step requires a NFS-cluster name for the Ceph Mgr call(s). The name of
the NFS-cluster as managed by Ceph should be provided in the parameters of the
`CreateVolume` operation. For Kubernetes that means the parameters is set as an
option in the `StorageClass`.

The `server` parameter of the volume is an other option that is managed by the
Ceph (or Rook) infrastructure. This parameter is also required to be provided
in the `CreateVolume` parameters.

Removing the NFS-export for the volume (or other operations) requires the name
of the NFS-cluster, as it is needed for the Ceph Mgr API. Like other parameters
of the CephFS volume, it will be needed to store the NFS-cluster name in the
OMAP journalling.

### `DeleteVolume` CSI operation

The `DeleteVolume` operation only receives the `volume_id` parameter, which
is to be used by the CSI Controller (provisioner) to locate the backing volume.
The `DeleteVolume` operation for the CephFS provisioner already knows how to
delete volumes by ID.

In order to remove the exported volume from the NFS-cluster, the operation
needs to fetch the name of the NFS-cluster from the journal where it was stored
during `CreateVolume`.

### Additional CSI operations

`CreateVolume` and `DeleteVolume` are the only required operations for the CSI
Controller (provisioner). Additional features as they are supported by CephFS
can forward the operations to the CephFS provisioner at a later time.

[ceph_mgr_nfs]: https://docs.ceph.com/en/latest/mgr/nfs/
[nfs_csi]: https://github.com/kubernetes-csi/csi-driver-nfs
[nfs_csi_params]: https://github.com/kubernetes-csi/csi-driver-nfs/blob/master/docs/driver-parameters.md
