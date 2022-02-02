# ceph-fuse: detection of corrupted mounts and their recovery

Mounts managed by ceph-fuse may get corrupted by e.g. the ceph-fuse process
exiting abruptly, or its parent Node Plugin container being terminated, taking
down its child processes with it.

This may manifest in concerned workloads like so:

```
# mount | grep fuse
ceph-fuse on /cephfs-share type fuse.ceph-fuse (rw,nosuid,nodev,relatime,user_id=0,group_id=0,allow_other)
# ls /cephfs-share
ls: /cephfs-share: Socket not connected
```

or,

```
# stat /home/kubelet/pods/ae344b80-3b07-4589-b1a1-ca75fa9debf2/volumes/kubernetes.io~csi/pvc-ec69de59-7823-4840-8eee-544f8261fef0/mount: transport endpoint is not connected
```

This feature allows CSI CephFS plugin to be able to detect if a ceph-fuse mount
is corrupted during the volume publishing phase, and will attempt to recover it
for the newly scheduled pod. Pods that already reside on a node whose
ceph-fuse mountpoints were broken may still need to be restarted, however.

## Detection

A mountpoint is deemed corrupted if `stat()`-ing it returns one of the
following errors:

* `ENOTCONN`
* `ESTALE`
* `EIO`
* `EACCES`
* `EHOSTDOWN`

## Recovery

Once a mountpoint corruption is detected, its recovery is performed by
remounting the volume associated with it.

Recovery is attempted only if `/csi/mountinfo` directory is made available to
CSI CephFS plugin (available by default in the Helm chart and Kubernetes
manifests).
