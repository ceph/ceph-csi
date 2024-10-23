# RBD NBD Mounter

- [RBD NBD Mounter](#rbd-nbd-mounter)
   - [Overview](#overview)
   - [Configuration](#configuration)
      - [Configuring logging path](#configuring-logging-path)
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

Please note that the minimum recommended kernel version to use rbd-nbd is
5.4 or higher.

### Configuring logging path

If you are using the default rbd nodePlugin DaemonSet and StorageClass
templates then `cephLogDir` will be `/var/log/ceph`, this directory will be
a host-path and the default log file path will be
`/var/log/ceph/rbd-nbd-<volID>.log`. rbd-nbd creates a log file per volume
under the `cephLogDir` path on NodeStage(map) and removed the same on
the respective NodeUnstage(unmap).

- There are different strategies to maintain the logs
   - `remove`: delete log file on unmap/detach (default behaviour)
   - `compress`: compress the log file to gzip on unmap/detach, in case there
    exists a `.gz` file from previous map/unmap of the same volume, then
    override the previous log with new log.
   - `preserve`: preserve the log file in text format

  You can tweak the log strategies through `cephLogStrategy` option from the
storageclass yaml

- In case if you need a customized log path, you should do below:

   - Edit the DaemonSet templates to change the ceph log directory host-path
      - If you are using helm charts, then you can use key `cephLogDirHostPath`

      ```
      helm install --set cephLogDirHostPath=/var/log/ceph-csi/my-dir
      ```

      - For standard templates edit [csi-rbdplugin.yaml](../deploy/rbd/kubernetes/csi-rbdplugin.yaml)
      to update `hostPath` for `ceph-logdir`.
      to update `pathPrefix` spec entries.
   - Update the StorageClass with the customized log directory path
      - Now update rbd StorageClass for `cephLogDir`, for example

      ```
      cephLogDir: "/var/log/prod-A-logs"
      ```

`NOTE`:

- On uninstall make sure to delete `cephLogDir` on host manually to freeup
  some space just in case if there are any uncleaned log files.
- In case if you do not need the rbd-nbd logging to persistent at all, then
  simply update the StorageClass for `cephLogDir` to use a non-persistent path.

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
