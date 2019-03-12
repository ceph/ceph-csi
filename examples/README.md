# How to test RBD and CephFS plugins with Kubernetes 1.13

Both `rbd` and `cephfs` directories contain `plugin-deploy.sh` and
`plugin-teardown.sh` helper scripts.  You can use those to help you
deploy/teardown RBACs, sidecar containers and the plugin in one go.
By default, they look for the YAML manifests in
`../../deploy/{rbd,cephfs}/kubernetes`.
You can override this path by running `$ ./plugin-deploy.sh /path/to/my/manifests`.

Once the plugin is successfully deployed, you'll need to customize
`storageclass.yaml` and `secret.yaml` manifests to reflect your Ceph cluster
setup.
Please consult the documentation for info about available parameters.

**NOTE:** See section
[Cluster ID based configuration](#cluster-id-based-configuration) if using
the `clusterID` instead of `monitors` or `monValueFromSecret` option in the
storage class for RBD based provisioning before proceeding.

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

## How to test RBD Snapshot feature

Before continuing, make sure you enabled the required
feature gate `VolumeSnapshotDataSource=true` in your Kubernetes cluster.

In the `examples/rbd` directory you will find two files related to snapshots:
[snapshotclass.yaml](./rbd/snapshotclass.yaml) and
[snapshot.yaml](./rbd/snapshot.yaml).

Once you created your RBD volume, you'll need to customize at least
`snapshotclass.yaml` and make sure the `monitors` and `pool` parameters match
your Ceph cluster setup.
If you followed the documentation to create the rbdplugin, you shouldn't
have to edit any other file.

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

## How to test RBD MULTI_NODE_MULTI_WRITER BLOCK feature

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
  storageClassName: csi-rbd
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
      image: debian
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
      image: debian
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

## Cluster ID based configuration

Before creating a storage class that uses the option `clusterID` to refer to a
Ceph cluster, the following actions need to be completed.

Get the following information from the Ceph cluster,

* Admin ID and key, that has privileges to perform CRUD operations on the Ceph
  cluster and pools of choice
  * Key is typically the output of, `ceph auth get-key client.admin` where
    `admin` is the Admin ID
  * Used to substitute admin/user id and key values in the files below
* Ceph monitor list
  * Typically in the output of `ceph mon dump`
  * Used to prepare comma separated MON list where required in the files below
* Ceph Cluster fsid
  * If choosing to use the Ceph cluster fsid as the unique value of clusterID,
  * Output of `ceph fsid`
  * Used to substitute `<cluster-id>` references in the files below

Update the template `rbd/template-ceph-cluster-ID-secret.yaml` with values from
a Ceph cluster and replace `<cluster-id>` with the chosen clusterID to create
the following secret,

* `kubectl create -f rbd/template-ceph-cluster-ID-secret.yaml`

Storage class and snapshot class, using `<cluster-id>` as the value for the
option `clusterID`, can now be created on the cluster.

Remaining steps to test functionality remains the same as mentioned in the
sections above.
