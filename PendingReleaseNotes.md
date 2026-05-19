# v3.17 Pending Release Notes

## Breaking changes

- NFS CSIDriver object's `spec.attachRequired` is now set to `true`
  to accommodate Kubernetes ServiceAccount based volume access restriction
  feature. This is a breaking change for users who have already deployed the
  NFS driver and are upgrading to v3.17. Users will need to delete and recreate
  the CSIDriver object for NFS during upgradie to v3.17.
- NVME-OF Storageclass now needs publish secrets to accommodate the
  Kubernetes ServiceAccount based volume access restriction.
  This is a breaking change for users who have already deployed the NVME-OF driver
  and are upgrading to v3.17. Users will need to recreate the NVME-OF Storageclass
  with publish secrets during upgrade to v3.17.

## Features

- nfs: allow changing NFS-server through ControllerModifyVolume [PR](https://github.com/ceph/ceph-csi/pull/5829)
- util: add support for GKLM KMS over KMIP [PR](https://github.com/ceph/ceph-csi/pull/6048)
- rbd: add Kubernetes ServiceAccount based volume access restriction
- nvmeof: add Kubernetes ServiceAccount based volume access restriction
- cephfs: add Kubernetes ServiceAccount based volume access restriction
- nfs: add Kubernetes ServiceAccount based volume access restriction
- rbd-nbd: use VolumeAttributesClass feature implement rbd volume qos [PR](https://github.com/ceph/ceph-csi/pull/6160)

## NOTE

- The `--setmetadata` flag has been deprecated and has no effect. Metadata is
  now always set on RBD images and CephFS subvolumes. The flag will be removed
  in a future release.
- csi-common: the CSI driver process is now automatically restarted if any
  unary gRPC call is stuck for more than 10 minutes. ReclaimSpace calls are
  excluded from this limit. The kubelet will restart the container in-place.
  Use `--feature-gates=SlowGRPCRestart=false` to disable this behaviour.
