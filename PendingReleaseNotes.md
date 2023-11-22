# v3.10 Pending Release Notes

## Breaking Changes

- Removed the deprecated grpc metrics flag's in [PR](https://github.com/ceph/ceph-csi/pull/4225)

- Support for pre-creation of cephFS subvolumegroup before creating subvolume
  is removed in [PR](https://github.com/ceph/ceph-csi/pull/4195)

## Features

- Support for configuring read affinity for individuals cluster within the ceph-csi-config
  ConfigMap in [PR](https://github.com/ceph/ceph-csi/pull/4165)

- Support for CephFS kernel and fuse mount options per cluster in [PR](https://github.com/ceph/ceph-csi/pull/4245)

## NOTE

- Support is limited to only the active Ceph releases. Support for EOLed Ceph
  releases are removed [PR](https://github.com/ceph/ceph-csi/pull/4262)
