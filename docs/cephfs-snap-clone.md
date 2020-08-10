# Create CephFS snapshot and Clone Volume

- [Create CephFS snapshot and Clone Volume](#create-cephfs-snapshot-and-clone-volume)
  - [Prerequisite](#prerequisite)
  - [Snapshot](#snapshot)
    - [Create snapshotclass](#create-snapshotclass)
    - [Create snapshot](#create-snapshot)
  - [Restore snapshot to a new PVC](#restore-snapshot-to-a-new-pvc)
  - [Clone PVC](#clone-pvc)

## Prerequisite

- For snapshot functionality to be supported for your Kubernetes cluster, the
  Kubernetes version running in your cluster should be >= v1.17. We also need the
  `snapshot controller` deployed in your Kubernetes cluster along with `csi-snapshotter`
  sidecar container.
  Refer [external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter/)
  for more information on these sidecar controllers. There should
  be a `volumesnapshotclass` object present in the cluster
  for snapshot request to be satisified.

## Snapshot

### Create snapshotclass

```console
kubectl create -f ../examples/cephfs/snapshotclass.yaml
```

### Create snapshot

The snapshot is created on/for an existing PVC. You should
have a PVC in bound state before creating snapshot from it.
please refer pvc creation [doc](https://github.com/ceph/ceph-csi/blob/master/docs/deploy-cephfs.md)
for more information on how to create a PVC.

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

## Restore snapshot to a new PVC

```console

$ kubectl create -f ../examples/cephfs/pvc-restore.yaml
$ kubectl get pvc
NAME                 STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
csi-cephfs-pvc       Bound    pvc-1ea51547-a88b-4ab0-8b4a-812caeaf025d   1Gi        RWX            csi-cephfs-sc  20h
cephfs-pvc-restore   Bound    pvc-95308c75-6c93-4928-a551-6b5137192209   1Gi        RWX            csi-cephfs-sc  11m
```

## Clone PVC

```console
$ kubectl create -f ../examples/cephfs/pvc-clone.yaml
$ kubectl get pvc
NAME                 STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
csi-cephfs-pvc       Bound    pvc-1ea51547-a88b-4ab0-8b4a-812caeaf025d   1Gi        RWX            csi-cephfs-sc  20h
cephfs-pvc-clone     Bound    pvc-b575bc35-d521-4c41-b4f9-1d733cd28fdf   1Gi        RWX            csi-cephfs-sc  39s
cephfs-pvc-restore   Bound    pvc-95308c75-6c93-4928-a551-6b5137192209   1Gi        RWX            csi-cephfs-sc  55m
```
