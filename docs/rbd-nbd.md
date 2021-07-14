# RBD NBD Mounter

- [RBD NBD Mounter](#rbd-nbd-mounter)
  - [Overview](#overview)
  - [Configuration](#configuration)
  - [Status](#status)
  - [Support Matrix](#support-matrix)
    - [CSI spec and Kubernetes version compatibility](#csi-spec-and-kubernetes-version-compatibility)

## Overview

The RBD CSI plugin will provision new RBD images and attach and mount those
to workloads. Currently, the default mounter is krbd, which uses the kernel
rbd driver to mount the rbd images onto the application node. Here on
at ceph-csi we will also have a userspace way of mounting the RBD images,
via RBD-NBD.

[Rbd-nbd](https://docs.ceph.com/en/latest/man/8/rbd-nbd/) is a client for
RADOS block device (rbd) images like the existing rbd kernel module. It
will map an rbd image to an NBD (Network Block Device) device, allowing
access to it as a regular local block device.

It’s worth to make a note that the rbd-nbd processes will run on the
client-side, which is inside the `csi-rbdplugin` node plugin.

## Configuration

To use the rbd-nbd mounter for RBD-backed PVs, set `mounter` to `rbd-nbd`
in the StorageClass.

## Status

Rbd-nbd support status: **Alpha**

## Support Matrix

| Features                                 | Feature Status | CSI Driver Version | Ceph Cluster Version | CSI Spec Version | Kubernetes Version |
| ---------------------------------------- | -------------- | ------------------ | -------------------- | ---------------- | ------------------ |
| Creating and deleting snapshot           | Alpha          | >= v3.4.0          | Pacific (>=16.0.0)   | >= v1.0.0        | >= v1.17.0         |
| Creating and deleting clones             | Alpha          | >= v3.4.0          | Pacific (>=16.0.0)   | >= v1.0.0        | >= v1.17.0         |
| Creating and deleting encrypted volumes  | Alpha          | >= v3.4.0          | Pacific (>=16.0.0)   | >= v1.0.0        | >= v1.14.0         |
| Expand volumes                           | Alpha          | >= v3.4.0          | Pacific (>=16.0.0)   | >= v1.1.0        | >= v1.15.0         |

`NOTE`: The `Alpha` status reflects possible non-backward compatible
changes in the future, and is thus not recommended for production use.

### CSI spec and Kubernetes version compatibility

Please refer to the [matrix](https://kubernetes-csi.github.io/docs/#kubernetes-releases)
in the Kubernetes documentation.
