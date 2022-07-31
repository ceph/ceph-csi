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
For the provisioner and controller expand stage secrets in storageclass, the
user needs to have the below Ceph capabilities.

```
"mon", "profile rbd",
"mgr", "allow rw",
"osd", "profile rbd"
```

And for the node stage secret in storageclass, the user needs to have the
below mentioned ceph capabilities.

```
"mon", "profile rbd",
"osd", "profile rbd",
"mgr", "allow rw"
```

## CephFS

Similarly in CephFS, for the provisioner and controller expand stage secret in
storageclass, the user needs to have the below mentioned ceph capabilities.

```
"mon", "allow r",
"mgr", "allow rw",
"osd", "allow rw tag cephfs metadata=*"
```

And for node stage secret in storageclass, the user needs to have
the below mentioned ceph capabilities.

```
"mon", "allow r",
"mgr", "allow rw",
"osd", "allow rw tag cephfs *=*",
"mds", "allow rw"
```

To get more insights on capabilities of CephFS you can refer
[this document](https://ceph.readthedocs.io/en/latest/cephfs/client-auth/)

## Command to a create user with required capabilities

`kubernetes` in the below commands represents an user which is subjected
to change as per your requirement.

### create user for RBD

The command for provisioner and node stage secret for rbd will be same as
they have similar capability requirements.

```bash
ceph auth get-or-create client.kubernetes \
mon 'profile rbd' \
osd 'profile rbd' \
mgr 'allow rw'
```

### create user for CephFS

```bash
ceph auth get-or-create client.kubernetes \
mon 'allow r' \
osd 'allow rw tag cephfs metadata=*' \
mgr 'allow rw'
```

```bash
ceph auth get-or-create client.kubernetes \
mon 'allow r' \
osd 'allow rw tag cephfs *=*' \
mgr 'allow rw' \
mds 'allow rw'
```
