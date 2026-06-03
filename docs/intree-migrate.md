# In-tree storage plugin to CSI Driver Migration

This document covers the example usage of in-tree RBD storage plugin to CSI
migration feature which can be enabled in a Kubernetes cluster. At present, this
feature is only supported for RBD in-tree plugin. Once this feature is enabled,
the in-tree volume requests (`kubernetes.io/rbd`) will be redirected to a
corresponding CSI (`rbd.csi.ceph.com`) driver.

## RBD

- [Prerequisite](#prerequisite)
- [Volume operations after enabling CSI migration](#volume-operations-after-enabling-csi-migration)
   - [Create volume](#create-volume)
   - [Mount volume to a POD](#mount-volume-to-a-pod)
   - [Resize volume](#resize-volume)
   - [Unmount volume](#unmount-volume)
   - [Delete volume](#delete-volume)
- [References](#additional-references)

### Prerequisite

For in-tree RBD migration to CSI driver to be supported for your Kubernetes
cluster, the Kubernetes version running in your cluster should be >= v1.23. We
also need sidecar controllers (`csi-provisioner` and `csi-resizer`) which are
compatible with the Kubernetes version v1.23 to be available in your cluster.
You can enable the migration with a couple of feature gates in your Kubernetes
cluster. These feature gates are alpha in Kubernetes 1.23 release.

- `CSIMigrationRBD`: when enabled, it will redirect traffic from in-tree rbd
  plugin (`kubernetes.io/rbd`) to CSI driver (`rbd.csi.ceph.com`), default
  to `false` now.

- `IntreePluginRBDUnregister`: Disables the RBD in-tree driver

To enable feature gates, refer [feature gates](https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/)

As a Kubernetes cluster operator that administers storage, here are the
prerequisites that you must complete before you attempt migration to the RBD CSI
driver:

- You must install the Ceph CSI driver (`rbd.csi.ceph.com`), v3.5.0 or above,
  into your Kubernetes cluster.
- Configure `clusterID` field in the configmap as
  discussed [here](https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/intree-migrate.md#clusterid-field-in-the-migration-request)
- Configure migration secret as
  discussed [here](https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/intree-migrate.md#migration-secret-for-csi-operations)

In below examples, `fast-rbd` is in-tree storageclass with provisioner
referencing in-tree provisioner `Kubernetes.io/rbd`.

```console
$ kubectl describe sc fast-rbd |grep -i provisioner
Provisioner:           Kubernetes.io/rbd
```

### Volume operations after enabling CSI migration

This section covers the operations on volumes provisioned after enabling CSI
migration in a cluster.

#### Create Volume

``` console
$ kubectl create -f pvc.yaml
persistentvolumeclaim/testpvc created

$ kubectl get pvc,pv
NAME                               STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/testpvc      Bound    pvc-c4e7dca5-4be6-4168-8eb5-af6ade04261f   1Gi        RWO            fast-rbd       24s

NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM             STORAGECLASS   REASON   AGE
persistentvolume/pvc-c4e7dca5-4be6-4168-8eb5-af6ade04261f   1Gi        RWO            Delete           Bound    default/testpvc   fast-rbd                18s
```

#### Mount Volume to a POD

Create a pod with PVC and verify the mount inside the POD

```console
$ kubectl create -f pod.yaml
pod/task-pv-pod created

$ kubectl get pods
NAME                                         READY   STATUS    RESTARTS   AGE
task-pv-pod                                  1/1     Running   0          2m36s

$ kubectl get pvc
NAME         STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
testpvc      Bound    pvc-c4e7dca5-4be6-4168-8eb5-af6ade04261f   1Gi        RWO            fast-rbd       4m40s

$ kubectl exec -ti task-pv-pod -- df -kh |grep nginx
/dev/rbd0                                976M  2.6M  958M   1% /usr/share/nginx/html
```

#### Resize Volume

Resize PVC from 1Gi to 2Gi and verify the new size change in the POD

```console
$ kubectl patch pvc testpvc -p '{"spec":{"resources":{"requests":{"storage":"2Gi"}}}}}'
persistentvolumeclaim/testpvc patched

$ kubectl exec -ti task-pv-pod -- df -kh |grep nginx
/dev/rbd0                                2.0G  3.0M  2.0G   1% /usr/share/nginx/html
```

#### Unmount Volume

Delete POD and verify pod deleted successfully

```console
$ kubectl get pods
NAME                                         READY   STATUS    RESTARTS   AGE
task-pv-pod                                  1/1     Running   0          5m31s

$ kubectl delete pod task-pv-pod
pod "task-pv-pod" deleted

$ kubectl get pods
```

#### Delete volume

Delete PVC and verify PVC and PV objects are deleted

```console
$ kubectl delete pvc testpvc
persistentvolumeclaim "testpvc" deleted

$ kubectl get pvc
No resources found in default namespace.

$ kubectl get pv
No resources found
```

### Additional References

To know more about in-tree to CSI migration:

- [design doc](./design/proposals/intree-migrate.md)
- [Kubernetes 1.17 Feature: Kubernetes In-Tree to CSI Volume Migration Moves to Beta](https://Kubernetes.io/blog/2019/12/09/Kubernetes-1-17-feature-csi-migration-beta/)
