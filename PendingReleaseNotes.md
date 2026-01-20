# v3.16 Pending Release Notes

## Breaking changes

## Features

- set nodeId:userId mapping in metadata [PR](https://github.com/ceph/ceph-csi/pull/5445)
   - refer design doc for more details - [here](https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/userID-mapping.md)
- cephfs-csi: fix mounting alternate filesystem when mounting by monitor lists [PR](https://github.com/ceph/ceph-csi/pull/5643)
- rbd: add block volume stats support [PR](https://github.com/ceph/ceph-csi/pull/5799)
- rbd: add `--rbdtrashmaxdelay` flag for configurable trash retention delay
   - When set to a positive duration, deleted RBD volumes are moved to Ceph trash and retained for the specified period
   - Users are responsible for purging RBD trash after the retention period using `rbd trash purge`

## NOTE
