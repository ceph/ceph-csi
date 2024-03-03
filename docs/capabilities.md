# Capabilities of a user required for ceph-csi in a Ceph cluster

Ceph uses the term _capabilities_ to describe authorizations for an
authenticated user
to exercise the functionality of the monitors, OSDs and metadata servers.
Capabilities can also restrict access to data within a pool or pool namespace.
A Ceph administrative user sets a user's capabilities when creating or
updating a user. In secret we have user id and user key and in order to
perform certain actions, the user needs to have some specific capabilities.
Hence, those capabilities are documented below.

## RBD

We have provisioner, controller expand and node stage secrets in storageclass.
For RBD the user needs to have the below Ceph capabilities:

```
mgr "profile rbd pool=csi"
osd "profile rbd pool=csi"
mon "profile rbd"
```

## CephFS

Similarly in CephFS, we have provisioner, controller expand and node stage
secrets in storageclass, the user needs to have the below mentioned ceph
capabilities:

```
mgr "allow rw"
osd "allow rw tag cephfs metadata=cephfs, allow rw tag cephfs data=cephfs"
mds "allow r fsname=cephfs path=/volumes, allow rws fsname=cephfs path=/volumes/csi"
mon "allow r fsname=cephfs"
```

To get more insights on capabilities of CephFS you can refer
[this document](https://ceph.readthedocs.io/en/latest/cephfs/client-auth/)

## Command to a create user with required capabilities

`USER`, `POOL` and `FS_NAME` with `SUB_VOL` variables below is subject to
change, please adjust them to your needs.

### create user for RBD

The command for provisioner and node stage secret for rbd will be same as
they have similar capability requirements.

```bash
USER=csi-rbd
POOL=csi
ceph auth get-or-create client.$USER \
  mgr "profile rbd pool=$POOL" \
  osd "profile rbd pool=$POOL"
  mon "profile rbd"
```

### create user for CephFS

```bash
USER=csi-cephfs
FS_NAME=cephfs
SUB_VOL=csi
ceph auth get-or-create client.$USER \
  mgr "allow rw" \
  osd "allow rw tag cephfs metadata=$FS_NAME, allow rw tag cephfs data=$FS_NAME" \
  mds "allow r fsname=$FS_NAME path=/volumes, allow rws fsname=$FS_NAME path=/volumes/$SUB_VOL" \
  mon "allow r fsname=$FS_NAME"
```
