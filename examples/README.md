# How to test RBD and CephFS plugins with Kubernetes 1.14+

## Deploying Ceph-CSI services

Create [ceph-config](../deploy/ceph-conf.yaml) configmap using the following command.

```bash
kubectl apply -f ../deploy/ceph-conf.yaml
```

Both `rbd` and `cephfs` directories contain `plugin-deploy.sh` and
`plugin-teardown.sh` helper scripts.  You can use those to help you
deploy/teardown RBACs, sidecar containers and the plugin in one go.
By default, they look for the YAML manifests in
`../deploy/{rbd,cephfs}/kubernetes`.
You can override this path by running

```bash
./plugin-deploy.sh /path/to/my/manifests
```

## Creating CSI configuration

The CSI plugin requires configuration information regarding the Ceph cluster(s),
that would host the dynamically or statically provisioned volumes. This
is provided by adding a per-cluster identifier (referred to as clusterID), and
the required monitor details for the same, as in the provided [sample config
 map](../deploy/csi-config-map-sample.yaml).

Gather the following information from the Ceph cluster(s) of choice,

* Ceph monitor list
   * Typically in the output of `ceph mon dump`
   * Used to prepare a list of `monitors` in the CSI configuration file
* Ceph Cluster fsid
   * If choosing to use the Ceph cluster fsid as the unique value of clusterID,
      * Output of `ceph fsid`
   * Alternatively, choose a `<cluster-id>` value that is distinct per Ceph
    cluster in use by this kubernetes cluster

Update the [sample configmap](../deploy/csi-config-map-sample.yaml) with values
from a Ceph cluster and replace `<cluster-id>` with the chosen clusterID, to
create the manifest for the configmap which can be updated in the cluster
using the following command,

```bash
kubectl replace -f ../deploy/csi-config-map-sample.yaml
```

Storage class and snapshot class, using `<cluster-id>` as the value for the
option `clusterID`, can now be created on the cluster.

## Running CephCSI with pod networking

The current problem with Pod Networking, is when a CephFS/RBD/NFS volume is mounted
in a pod using Ceph CSI and then the CSI CephFS/RBD/NFS plugin is restarted or
terminated (e.g. by restarting or deleting its DaemonSet), all operations on
the volume become blocked, even after restarting the CSI pods.

The only workaround is to restart the node where the Ceph CSI plugin pod was
restarted. This can be mitigated by running the `rbd map`/`mount -t` commands
in a different network namespace which does not get deleted when the CSI
CephFS/RBD/NFS plugin is restarted or terminated.

If someone wants to run the CephCSI with the pod networking they can still do
by setting the `netNamespaceFilePath`. If this path is set CephCSI will execute
the `rbd map`/`mount -t` commands after entering the [network
namespace](https://man7.org/linux/man-pages/man7/network_namespaces.7.html)
specified by `netNamespaceFilePath` with the
[nsenter](https://man7.org/linux/man-pages/man1/nsenter.1.html) command.

`netNamespaceFilePath` should point to the network namespace of some
long-running process, typically it would be a symlink to
`/proc/<long running process id>/ns/net`.

The long-running process can also be another pod which is a Daemonset which
never restarts. This Pod should only be stopped and restarted when a node is
stopped so that volume operations do not become blocked. The new DaemonSet pod
can contain a single container, responsible for holding its pod network alive.
It is used as a passthrough by the CephCSI plugin pod which when mounting or
mapping will use the network namespace of this pod.

Once the pod is created get its PID and create a symlink to
`/proc/<PID>/ns/net` in the hostPath volume shared with the csi-plugin pod and
specify the path in the `netNamespaceFilePath` option.

*Note* This Pod should have `hostPID: true` in the Pod Spec.

## Deploying the storage class

Once the plugin is successfully deployed, you'll need to customize
`storageclass.yaml` and `secret.yaml` manifests to reflect your Ceph cluster
setup.
Please consult the documentation for info about available parameters.

After configuring the secrets, monitors, etc. you can deploy a
testing Pod mounting a RBD image / CephFS volume:

```bash
kubectl create -f secret.yaml
kubectl create -f storageclass.yaml
kubectl create -f pvc.yaml
kubectl create -f pod.yaml
```

Other helper scripts:

* `logs.sh` output of the plugin
* `exec-bash.sh` logs into the plugin's container and runs bash

### How to test RBD Snapshot feature

Before continuing, make sure you enabled the required
feature gate `VolumeSnapshotDataSource=true` in your Kubernetes cluster.

In the `examples/rbd` directory you will find two files related to snapshots:
[snapshotclass.yaml](./rbd/snapshotclass.yaml) and
[snapshot.yaml](./rbd/snapshot.yaml).

Once you created your RBD volume, you'll need to customize at least
`snapshotclass.yaml` and make sure the `clusterid` parameter matches
your Ceph cluster setup.
If you followed the documentation to create the rbdplugin, you shouldn't
have to edit any other file.

Note that it is recommended to create a volume snapshot or a PVC clone
only when the PVC is not in use.

After configuring everything you needed, deploy the snapshot class:

```bash
kubectl create -f snapshotclass.yaml
```

Verify that the snapshot class was created:

```console
$ kubectl get volumesnapshotclass
NAME                      AGE
csi-rbdplugin-snapclass   4s
```

Create a snapshot from the existing PVC:

```bash
kubectl create -f snapshot.yaml
```

To verify if your volume snapshot has successfully been created, run the following:

```console
$ kubectl get volumesnapshot
NAME               AGE
rbd-pvc-snapshot   6s
```

To check the status of the snapshot, run the following:

```bash
$ kubectl describe volumesnapshot rbd-pvc-snapshot
Name:         rbd-pvc-snapshot
Namespace:    default
Labels:       <none>
Annotations:  <none>
API Version:  snapshot.storage.k8s.io/v1alpha1
Kind:         VolumeSnapshot
Metadata:
  Creation Timestamp:  2019-02-06T08:52:34Z
  Finalizers:
    snapshot.storage.kubernetes.io/volumesnapshot-protection
  Generation:        5
  Resource Version:  84239
  Self Link:         /apis/snapshot.storage.k8s.io/v1alpha1/namespaces/default/volumesnapshots/rbd-pvc-snapshot
  UID:               8b9b5740-29ec-11e9-8e0f-b8ca3aad030b
Spec:
  Snapshot Class Name:    csi-rbdplugin-snapclass
  Snapshot Content Name:  snapcontent-8b9b5740-29ec-11e9-8e0f-b8ca3aad030b
  Source:
    API Group:  <nil>
    Kind:       PersistentVolumeClaim
    Name:       rbd-pvc
Status:
  Creation Time:  2019-02-06T08:52:34Z
  Ready To Use:   true
  Restore Size:   1Gi
Events:           <none>
```

To be sure everything is OK you can run `rbd snap ls [your-pvc-name]` inside
one of your Ceph pod.

To restore the snapshot to a new PVC, deploy
[pvc-restore.yaml](./rbd/pvc-restore.yaml) and a testing pod
[pod-restore.yaml](./rbd/pod-restore.yaml):

```bash
kubectl create -f pvc-restore.yaml
kubectl create -f pod-restore.yaml
```

### How to test RBD MULTI_NODE_MULTI_WRITER BLOCK feature

Requires feature-gates: `BlockVolume=true` `CSIBlockVolume=true`

*NOTE* The MULTI_NODE_MULTI_WRITER capability is only available for
Volumes that are of access_type `block`

*WARNING*  This feature is strictly for workloads that know how to deal
with concurrent access to the Volume (eg Active/Passive applications).
Using RWX modes on non clustered file systems with applications trying
to simultaneously access the Volume will likely result in data corruption!

Following are examples for issuing a request for a `Block`
`ReadWriteMany` Claim, and using the resultant Claim for a POD

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: block-pvc
spec:
  accessModes:
  - ReadWriteMany
  volumeMode: Block
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-rbd-sc
```

Create a POD that uses this PVC:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
spec:
  containers:
    - name: my-container
      image: docker.io/library/debian:latest
      command: ["/bin/bash", "-c"]
      args: [ "tail -f /dev/null" ]
      volumeDevices:
        - devicePath: /dev/rbdblock
          name: my-volume
      imagePullPolicy: IfNotPresent
  volumes:
    - name: my-volume
      persistentVolumeClaim:
        claimName: block-pvc

```

Now, we can create a second POD (ensure the POD is scheduled on a different
node; multiwriter single node works without this feature) that also uses this
PVC at the same time, again wait for the pod to enter running state, and verify
the block device is available.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: another-pod
spec:
  containers:
    - name: my-container
      image: docker.io/library/debian:latest
      command: ["/bin/bash", "-c"]
      args: [ "tail -f /dev/null" ]
      volumeDevices:
        - devicePath: /dev/rbdblock
          name: my-volume
      imagePullPolicy: IfNotPresent
  volumes:
    - name: my-volume
      persistentVolumeClaim:
        claimName: block-pvc
```

Wait for the PODs to enter Running state, check that our block device
is available in the container at `/dev/rdbblock` in both containers:

```bash
$ kubectl exec -it my-pod -- fdisk -l /dev/rbdblock
Disk /dev/rbdblock: 1 GiB, 1073741824 bytes, 2097152 sectors
Units: sectors of 1 * 512 = 512 bytes
Sector size (logical/physical): 512 bytes / 512 bytes
I/O size (minimum/optimal): 4194304 bytes / 4194304 bytes
```

```bash
$ kubectl exec -it another-pod -- fdisk -l /dev/rbdblock
Disk /dev/rbdblock: 1 GiB, 1073741824 bytes, 2097152 sectors
Units: sectors of 1 * 512 = 512 bytes
Sector size (logical/physical): 512 bytes / 512 bytes
I/O size (minimum/optimal): 4194304 bytes / 4194304 bytes
```

### How to create CephFS Snapshot and Restore

In the `examples/cephfs` directory you will find two files related to snapshots:
[snapshotclass.yaml](./cephfs/snapshotclass.yaml) and
[snapshot.yaml](./cephfs/snapshot.yaml).

Once you created your CephFS volume, you'll need to customize at least
`snapshotclass.yaml` and make sure the `clusterID` parameter matches
your Ceph cluster setup.

Note that it is recommended to create a volume snapshot or a PVC clone
only when the PVC is not in use.

After configuring everything you needed, create the snapshot class:

```bash
kubectl create -f ../examples/cephfs/snapshotclass.yaml
```

Verify that the snapshot class was created:

```console
$ kubectl get volumesnapshotclass
NAME                         DRIVER                DELETIONPOLICY   AGE
csi-cephfsplugin-snapclass   cephfs.csi.ceph.com   Delete           24m
```

Create a snapshot from the existing PVC:

```bash
kubectl create -f ../examples/cephfs/snapshot.yaml
```

To verify if your volume snapshot has successfully been created and to
get the details about snapshot, run the following:

```console
$ kubectl get volumesnapshot
NAME                  READYTOUSE   SOURCEPVC       SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS                SNAPSHOTCONTENT                                    CREATIONTIME   AGE
cephfs-pvc-snapshot   true         csi-cephfs-pvc                          1Gi           csi-cephfsplugin-snapclass   snapcontent-34476204-a14a-4d59-bfbc-2bbba695652c   3s             6s
```

To be sure everything is OK you can run
`ceph fs subvolume snapshot ls <vol_name> <sub_name> [<group_name>]`
inside one of your Ceph pod.

To restore the snapshot to a new PVC, deploy
[pvc-restore.yaml](./cephfs/pvc-restore.yaml) and a testing pod
[pod-restore.yaml](./cephfs/pod-restore.yaml):

```bash
kubectl create -f pvc-restore.yaml
kubectl create -f pod-restore.yaml
```

### Cleanup for CephFS Snapshot and Restore

Delete the testing pod and restored pvc.

```bash
kubectl delete pod <pod-restore name>
kubectl delete pvc <pvc-restore name>
```

Now, the snapshot is no longer in use, Delete the volume snapshot
and volume snapshot class.

```bash
kubectl delete volumesnapshot <snapshot name>
kubectl delete volumesnapshotclass <snapshotclass name>
```

### How to Clone CephFS Volumes

Create the clone from cephFS PVC:

```bash
kubectl create -f ../examples/cephfs/pvc-clone.yaml
kubectl create -f ../examples/cephfs/pod-clone.yaml
```

To verify if your clone has successfully been created, run the following:

```console
$ kubectl get pvc
NAME                 STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
csi-cephfs-pvc       Bound    pvc-1ea51547-a88b-4ab0-8b4a-812caeaf025d   1Gi        RWX            csi-cephfs-sc  20h
cephfs-pvc-clone     Bound    pvc-b575bc35-d521-4c41-b4f9-1d733cd28fdf   1Gi        RWX            csi-cephfs-sc  39s
```

### Cleanup

Delete the cloned pod and pvc:

```bash
kubectl delete pod <pod-clone name>
kubectl delete pvc <pvc-clone name>
```
