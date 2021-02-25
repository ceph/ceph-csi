# Create snapshot and Clone Volume

- [Prerequisite](#prerequisite)
- [Create CephFS Snapshot and Clone Volume](#create-cephfs-snapshot-and-clone-volume)
  - [Create CephFS SnapshotClass](#create-cephfs-snapshotclass)
  - [Create CephFS Snapshot](#create-cephfs-snapshot)
  - [Restore CephFS Snapshot to a new PVC](#restore-cephfs-snapshot-to-a-new-pvc)
  - [Clone CephFS PVC](#clone-cephfs-pvc)
- [Create RBD Snapshot and Clone Volume](#create-rbd-snapshot-and-clone-volume)
  - [Create RBD SnapshotClass](#create-rbd-snapshotclass)
  - [Create RBD Snapshot](#create-rbd-snapshot)
  - [Restore RBD Snapshot to a new PVC](#restore-rbd-snapshot-to-a-new-pvc)
  - [Clone RBD PVC](#clone-rbd-pvc)

## Prerequisite

- For snapshot functionality to be supported for your Kubernetes cluster, the
  Kubernetes version running in your cluster should be >= v1.17. We also need the
  `snapshot controller` deployed in your Kubernetes cluster along with `csi-snapshotter`
  sidecar container.
  Refer [external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter/)
  for more information on these sidecar controllers. There should
  be a `volumesnapshotclass` object present in the cluster
  for snapshot request to be satisfied.

  - To install snapshot controller and CRD

    ```console
    ./scripts/install-snapshot.sh install
    ```

    To install from specific external-snapshotter version, you can leverage
    `SNAPSHOT_VERSION` variable, for example:

    ```console
    SNAPSHOT_VERSION="v3.0.1" ./scripts/install-snapshot.sh install
    ```

  - In the future, you can choose to cleanup by running

    ```console
    ./scripts/install-snapshot.sh cleanup
    ```

**NOTE: At present, there is a limit of 400 snapshots per cephFS filesystem.
Also PVC cannot be deleted if it's having snapshots. Make sure all the snapshots
on the PVC are deleted before you delete the PVC.**

## Create CephFS Snapshot and Clone Volume

### Create SnapshotClass

```console
kubectl create -f ../examples/cephfs/snapshotclass.yaml
```

### Create Snapshot

The snapshot is created on/for an existing PVC. You should
have a PVC in bound state before creating snapshot from it.
It is recommended to create a volume snapshot or a PVC clone
only when the PVC is not in use.
Please refer pvc creation [doc](https://github.com/ceph/ceph-csi/blob/devel/docs/deploy-cephfs.md)
for more information on how to create a PVC.

- Verify if PVC is in Bound state

```console
$ kubectl get pvc
NAME              STATUS        VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
csi-cephfs-pvc    Bound         pvc-1ea51547-a88b-4ab0-8b4a-812caeaf025d   1Gi        RWX            csi-cephfs-sc  20h
```

- Create snapshot of the bound PVC

```console
$ kubectl create -f ../examples/cephfs/snapshot.yaml
volumesnapshot.snapshot.storage.k8s.io/cephfs-pvc-snapshot created
```

- Get details about the snapshot

```console
$ kubectl get volumesnapshot
NAME                  READYTOUSE   SOURCEPVC       SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS                SNAPSHOTCONTENT                                    CREATIONTIME   AGE
cephfs-pvc-snapshot   true         csi-cephfs-pvc                          1Gi           csi-cephfsplugin-snapclass   snapcontent-34476204-a14a-4d59-bfbc-2bbba695652c   3s             6s
```

- Get details about the volumesnapshotcontent

```console
$ kubectl get volumesnapshotcontent
NAME                                               READYTOUSE   RESTORESIZE   DELETIONPOLICY   DRIVER                VOLUMESNAPSHOTCLASS          VOLUMESNAPSHOT        AGE
snapcontent-34476204-a14a-4d59-bfbc-2bbba695652c   true         1073741824    Delete           cephfs.csi.ceph.com   csi-cephfsplugin-snapclass   cephfs-pvc-snapshot   64s
```

### Restore Snapshot to a new PVC

```console
kubectl create -f ../examples/cephfs/pvc-restore.yaml
```

```console
$ kubectl get pvc
NAME                 STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
csi-cephfs-pvc       Bound    pvc-1ea51547-a88b-4ab0-8b4a-812caeaf025d   1Gi        RWX            csi-cephfs-sc  20h
cephfs-pvc-restore   Bound    pvc-95308c75-6c93-4928-a551-6b5137192209   1Gi        RWX            csi-cephfs-sc  11m
```

### Clone PVC

```console
kubectl create -f ../examples/cephfs/pvc-clone.yaml
```

```console
$ kubectl get pvc
NAME                 STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
csi-cephfs-pvc       Bound    pvc-1ea51547-a88b-4ab0-8b4a-812caeaf025d   1Gi        RWX            csi-cephfs-sc  20h
cephfs-pvc-clone     Bound    pvc-b575bc35-d521-4c41-b4f9-1d733cd28fdf   1Gi        RWX            csi-cephfs-sc  39s
cephfs-pvc-restore   Bound    pvc-95308c75-6c93-4928-a551-6b5137192209   1Gi        RWX            csi-cephfs-sc  55m
```

## Create RBD Snapshot and Clone Volume

In the `examples/rbd` directory you will find two files related to snapshots:
[snapshotclass.yaml](../examples/rbd/snapshotclass.yaml)
and [snapshot.yaml](../examples/rbd/snapshot.yaml)

Once you created RBD PVC, you'll need to customize `snapshotclass.yaml` and
make sure the `clusterid` parameter matches `clusterid` mentioned in the
storageclass from which the PVC got created.
If you followed the documentation to create the rbdplugin, you shouldn't
have to edit any other file.

After configuring everything you needed, deploy the snapshotclass:

### Create RBD-SnapshotClass

```bash
kubectl create -f snapshotclass.yaml
```

### Verify that the SnapshotClass was created

```console
$ kubectl get volumesnapshotclass
NAME                      AGE
csi-rbdplugin-snapclass   4s
```

### Create rbd-Snapshot

```bash
kubectl create -f snapshot.yaml
```

### Verify if your Volume Snapshot has successfully been created

```console
$ kubectl get volumesnapshot
NAME               AGE
rbd-pvc-snapshot   6s
```

### Check the status of the Snapshot

```console
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

### Restore the Snapshot

To restore the snapshot to a new PVC, create
[pvc-restore.yaml](../examples/rbd/pvc-restore.yaml)
and a testing pod [pod-restore.yaml](../examples/rbd/pod-restore.yaml)

```bash
kubectl create -f pvc-restore.yaml
kubectl create -f pod-restore.yaml
```

### Clone the PVC

```console
$ kubectl create -f ../examples/rbd/pvc-clone.yaml
$ kubectl get pvc
NAME              STATUS    VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
rbd-pvc           Bound     pvc-c2ffdc98-3abe-4b07-838c-35a2a8067771   1Gi        RWO            rook-ceph-block   41m
rbd-pvc-clone     Bound     pvc-b575bc35-d521-4c41-b4f9-1d733cd28fdf   1Gi        RWO            rook-ceph-block   45m
rbd-pvc-restore   Bound     pvc-95308c75-6c93-4928-a551-6b5137192209   1Gi        RWO            rook-ceph-block   45m
```