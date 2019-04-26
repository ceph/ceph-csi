# How to test RBD and CephFS plugins with Kubernetes 1.11

Both `rbd` and `cephfs` directories contain `plugin-deploy.sh` and `plugin-teardown.sh` helper scripts. You can use those to help you deploy/tear down RBACs, sidecar containers and the plugin in one go. By default, they look for the YAML manifests in `../../deploy/{rbd,cephfs}/kubernetes`. You can override this path by running `$ ./plugin-deploy.sh /path/to/my/manifests`.

Once the plugin is successfully deployed, you'll need to customize `storageclass.yaml` and `secret.yaml` manifests to reflect your Ceph cluster setup. Please consult the documentation for info about available parameters.

After configuring the secrets, monitors, etc. you can deploy a testing Pod mounting a RBD image / CephFS volume:

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

Before continuing, make sure you enabled the required [feature gate](https://kubernetes-csi.github.io/docs/snapshot-restore-feature.html#snapshot-apis) in your Kubernetes cluster.

In the `examples/rbd` directory you will find four files related to snapshots: [csi-snapshotter-rbac.yaml](./rbd/csi-snapshotter-rbac.yaml), [csi-snapshotter.yaml](./rbd/csi-snapshotter.yaml), [snapshotclass.yaml](./rbd/snapshotclass.yaml) and [snapshot.yaml](./rbd/snapshot.yaml).

Once you created your RBD volume, you'll need to customize at least `snapshotclass.yaml` and make sure the `monitors` and `pool` parameters match your Ceph cluster setup. If you followed the documentation to create the rbdplugin, you shouldn't have to edit any other file. If you didn't, make sure every parameters in `csi-snapshotter.yaml` reflect your configuration.

After configuring everything you needed, deploy the csi-snapshotter:

```bash
kubectl create -f csi-snapshotter-rbac.yaml
kubectl create -f csi-snapshotter.yaml
kubectl create -f snapshotclass.yaml
kubectl create -f snapshot.yaml
```

To verify if your volume snapshot has successfully been created, run the following:

```bash
$ kubectl get volumesnapshotclass
NAME                      AGE
csi-rbdplugin-snapclass   4s
$ kubectl get volumesnapshot
NAME               AGE
rbd-pvc-snapshot   6s
```

To be sure everything is OK you can run `rbd snap ls [your-pvc-name]` inside one of your Ceph pod.

