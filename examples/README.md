# How to test RBD and CephFS plugins with Kubernetes 1.13

Both `rbd` and `cephfs` directories contain `plugin-deploy.sh` and
`plugin-teardown.sh` helper scripts.  You can use those to help you
deploy/teardown RBACs, sidecar containers and the plugin in one go.
By default, they look for the YAML manifests in
`../../deploy/{rbd,cephfs}/kubernetes`.
You can override this path by running `$ ./plugin-deploy.sh /path/to/my/manifests`.

Once the plugin is successfuly deployed, you'll need to customize
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
[pod-restore.yaml](./rbd/pvc-restore.yaml):

```bash
kubectl create -f pvc-restore.yaml
kubectl create -f pod-restore.yaml
```
