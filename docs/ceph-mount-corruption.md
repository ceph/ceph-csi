# Ceph mount corruption detection and recover

## ceph-fuse: detection of corrupted mounts and their recovery

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

### ceph-fuse corruption detection

A mountpoint is deemed corrupted if `stat()`-ing it returns one of the
following errors:

* `ENOTCONN`
* `ESTALE`
* `EIO`
* `EACCES`
* `EHOSTDOWN`

### ceph-fuse recovery

Once a mountpoint corruption is detected, its recovery is performed by
remounting the volume associated with it.

Recovery is attempted only if `/csi/mountinfo` directory is made available to
CSI CephFS plugin (available by default in the Helm chart and Kubernetes
manifests).

## kernel client: detection of corrupted mounts and their recovery

Mounts managed by ceph-kernel may get corrupted e.g. if your network
connection is disrupted for a long enough time, the client will be forcibly
disconnected from the system. More details can be found
[here](https://docs.ceph.com/en/quincy/cephfs/troubleshooting/#disconnected-remounted-fs)

The above case may manifest in concerned workloads like so:

```
# mount | grep ceph
10.102.104.172:6789:/volumes/csi/csi-vol-7fed1ce7-97cf-43ef-9b84-2a49ab992515/d61be75e-74ae-428c-a5d1-48f79d1d3c8c on /var/lib/kubelet/plugins/kubernetes.io/csi/cephfs.csi.ceph.com/bc0146ec2b5d9a9db62e698abbe0adcae19c0e01f5cf15d3d593ed33c7bc1a8d/globalmount type ceph (rw,relatime,name=csi-cephfs-node,secret=<hidden>,fsid=00000000-0000-0000-0000-000000000000,acl,mds_namespace=myfs,_netdev)
10.102.104.172:6789:/volumes/csi/csi-vol-7fed1ce7-97cf-43ef-9b84-2a49ab992515/d61be75e-74ae-428c-a5d1-48f79d1d3c8c on /var/lib/kubelet/pods/8087df68-9756-4f38-86ef-6c81e1075607/volumes/kubernetes.io~csi/pvc-15e63d0a-77de-4886-8d0f-516f9fecbeb4/mount type ceph (rw,relatime,name=csi-cephfs-node,secret=<hidden>,fsid=00000000-0000-0000-0000-000000000000,acl,mds_namespace=myfs,_netdev)# ls /cephfs-share

sh-4.4# ls /var/lib/kubelet/plugins/kubernetes.io/csi/cephfs.csi.ceph.com/bc0146ec2b5d9a9db62e698abbe0adcae19c0e01f5cf15d3d593ed33c7bc1a8d/globalmount
ls: cannot access '/var/lib/kubelet/plugins/kubernetes.io/csi/cephfs.csi.ceph.com/bc0146ec2b5d9a9db62e698abbe0adcae19c0e01f5cf15d3d593ed33c7bc1a8d/globalmount': Permission denied
```

### kernel client corruption detection

A mountpoint is deemed corrupted if `stat()`-ing it returns one of the
following errors:

* `ENOTCONN`
* `ESTALE`
* `EIO`
* `EACCES`
* `EHOSTDOWN`

More details about the error codes can be found [here](https://www.gnu.org/software/libc/manual/html_node/Error-Codes.html)

For such mounts, The CephCSI nodeplugin returns volume_condition as
abnormal for `NodeGetVolumeStats` RPC call.

### kernel client recovery

Once a mountpoint corruption is detected,
Below are the two methods to recover from it.

* Reboot the node where the abnormal volume behavior is observed.
* Scale down all the applications using the CephFS PVC
  on the node where abnormal mounts are present.
  Once all the applications are deleted, scale up the application
  to remount the CephFS PVC to application pods.
