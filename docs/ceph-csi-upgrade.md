# Ceph-csi Upgrade

- [Ceph-csi Upgrade](#ceph-csi-upgrade)
   - [Pre-upgrade considerations](#pre-upgrade-considerations)
      - [Snapshot-controller and snapshot crd](#snapshot-controller-and-snapshot-crd)
         - [Snapshot API version support matrix](#snapshot-api-version-support-matrix)
   - [Upgrading from previous releases](#upgrading-from-previous-releases)
   - [Upgrading from v3.8 to v3.9](#upgrading-from-v38-to-v39)
      - [Upgrading CephFS](#upgrading-cephfs)
         - [1. Upgrade CephFS Provisioner resources](#1-upgrade-cephfs-provisioner-resources)
            - [1.1 Update the CephFS Provisioner RBAC](#11-update-the-cephfs-provisioner-rbac)
            - [1.2 Update the CephFS Provisioner deployment](#12-update-the-cephfs-provisioner-deployment)
         - [2. Upgrade CephFS Nodeplugin resources](#2-upgrade-cephfs-nodeplugin-resources)
            - [2.1 Update the CephFS Nodeplugin RBAC](#21-update-the-cephfs-nodeplugin-rbac)
            - [2.2 Update the CephFS Nodeplugin daemonset](#22-update-the-cephfs-nodeplugin-daemonset)
            - [2.3 Manual deletion of CephFS Nodeplugin daemonset pods](#23-manual-deletion-of-cephfs-nodeplugin-daemonset-pods)
            - [2.4 Modifying MountOptions in Storageclass and PersistentVolumes](#24-modifying-mountoptions-in-storageclass-and-persistentvolumes)
         - [Delete removed CephFS PSP, Role and RoleBinding](#delete-removed-cephfs-psp-role-and-rolebinding)
      - [Upgrading RBD](#upgrading-rbd)
         - [3. Upgrade RBD Provisioner resources](#3-upgrade-rbd-provisioner-resources)
            - [3.1 Update the RBD Provisioner RBAC](#31-update-the-rbd-provisioner-rbac)
            - [3.2 Update the RBD Provisioner deployment](#32-update-the-rbd-provisioner-deployment)
         - [4. Upgrade RBD Nodeplugin resources](#4-upgrade-rbd-nodeplugin-resources)
            - [4.1 Update the RBD Nodeplugin RBAC](#41-update-the-rbd-nodeplugin-rbac)
            - [4.2 Update the RBD Nodeplugin daemonset](#42-update-the-rbd-nodeplugin-daemonset)
         - [Delete removed RBD PSP, Role and RoleBinding](#delete-removed-rbd-psp-role-and-rolebinding)
      - [Upgrading NFS](#upgrading-nfs)
         - [5. Upgrade NFS Provisioner resources](#5-upgrade-nfs-provisioner-resources)
            - [5.1 Update the NFS Provisioner RBAC](#51-update-the-nfs-provisioner-rbac)
            - [5.2 Update the NFS Provisioner deployment](#52-update-the-nfs-provisioner-deployment)
         - [6. Upgrade NFS Nodeplugin resources](#6-upgrade-nfs-nodeplugin-resources)
            - [6.1 Update the NFS Nodeplugin RBAC](#61-update-the-nfs-nodeplugin-rbac)
            - [6.2 Update the NFS Nodeplugin daemonset](#62-update-the-nfs-nodeplugin-daemonset)
      - [CSI Sidecar containers consideration](#csi-sidecar-containers-consideration)

## Pre-upgrade considerations

In some scenarios there is an issue in the CSI driver that will cause
application pods to be disconnected from their mounts when the CSI driver is
restarted. Since the upgrade would cause the CSI driver to restart if it is
updated, you need to be aware of whether this affects your applications. This
issue will happen when using the Ceph fuse client.

If you provision volumes for CephFS and have a kernel less than version 4.17,
the CSI driver will fall back to use the FUSE client.

If you are affected by this issue, you will need to proceed carefully during
the upgrade to restart your application pods. The recommended step is to modify
the update strategy of the CSI nodeplugin daemonsets to OnDelete so that you
can control when the CSI driver pods are restarted on each node.

To avoid this issue in future upgrades, we recommend that you do not use the
fuse client as of now.

This guide will walk you through the steps to upgrade the software in a cluster
from v3.8 to v3.9

### Snapshot-controller and snapshot crd

Its kubernetes distributor responsibility to install new snapshot
controller and snapshot CRD. more info can be found
[here](https://github.com/kubernetes-csi/external-snapshotter/tree/master#usage)

#### Snapshot API version support matrix

| Snapshot API version | Kubernetes Version   | Snapshot-Controller + CRDs Version | Sidecar Version |
| -------------------- | -------------------- | ---------------------------------- | --------------- |
| v1beta1              | v1.17 =< k8s < v1.20 | v2.x =< snapshot-controller < v4.x | sidecar >= v2.x |
| v1                   | k8s >= v1.20         | snapshot-controller >= v4.x        | sidecar >= v2.x |

**Note:** We recommend to use {sidecar, controller, crds} of same version

## Upgrading from previous releases

To upgrade from previous releases, refer to the following:

- [upgrade-from-v3.2-v3.3](https://github.com/ceph/ceph-csi/blob/v3.3.1/docs/ceph-csi-upgrade.md)
  to upgrade from cephcsi v3.2 to v3.3
- [upgrade-from-v3.3-v3.4](https://github.com/ceph/ceph-csi/blob/v3.4.0/docs/ceph-csi-upgrade.md)
  to upgrade from cephcsi v3.3 to v3.4
- [upgrade-from-v3.4-v3.5](https://github.com/ceph/ceph-csi/blob/v3.5.1/docs/ceph-csi-upgrade.md)
  to upgrade from cephcsi v3.4 to v3.5
- [upgrade-from-v3.5-v3.6](https://github.com/ceph/ceph-csi/blob/v3.6.1/docs/ceph-csi-upgrade.md)
  to upgrade from cephcsi v3.5 to v3.6
- [upgrade-from-v3.6-v3.7](https://github.com/ceph/ceph-csi/blob/v3.7.2/docs/ceph-csi-upgrade.md)
  to upgrade from cephcsi v3.6 to v3.7
- [upgrade-from-v3.7-v3.8](https://github.com/ceph/ceph-csi/blob/v3.8.0/docs/ceph-csi-upgrade.md)
  to upgrade from cephcsi v3.7 to v3.8

## Upgrading from v3.8 to v3.9

**Ceph-csi releases from devel are expressly unsupported.** It is strongly
recommended that you use [official
releases](https://github.com/ceph/ceph-csi/releases) of Ceph-csi. Unreleased
versions from the devel branch are subject to changes and incompatibilities
that will not be supported in the official releases. Builds from the devel
branch can have functionality changed and even removed at any time without
compatibility support and without prior notice.

**Also, we do not recommend any direct upgrades to 3.9 except from 3.8 to 3.9.**
For example, upgrading from 3.7 to 3.9 is not recommended.

git checkout v3.9.0 tag

```bash
git clone https://github.com/ceph/ceph-csi.git
cd ./ceph-csi
git checkout v3.9.0
```

```console
Warning: kubectl apply should be used on resource created by either kubectl create --save-config or kubectl apply
```

**Note:** While upgrading please Ignore above warning messages from kubectl output

### Upgrading CephFS

If existing cephfs storageclasses' `MountOptions` are set, please refer to
[modifying mount options](#24-modifying-mountoptions-in-storageclass-and-persistentvolumes)
section.
Upgrading cephfs csi includes upgrade of cephfs driver and as well as
kubernetes sidecar containers and also the permissions required for the
kubernetes sidecar containers, lets upgrade the things one by one

#### 1. Upgrade CephFS Provisioner resources

Upgrade provisioner resources include updating the provisioner RBAC and
Provisioner deployment

##### 1.1 Update the CephFS Provisioner RBAC

```bash
$ kubectl apply -f deploy/cephfs/kubernetes/csi-provisioner-rbac.yaml
serviceaccount/cephfs-csi-provisioner configured
clusterrole.rbac.authorization.k8s.io/cephfs-external-provisioner-runner configured
clusterrole.rbac.authorization.k8s.io/cephfs-external-provisioner-runner-rules configured
clusterrolebinding.rbac.authorization.k8s.io/cephfs-csi-provisioner-role configured
role.rbac.authorization.k8s.io/cephfs-external-provisioner-cfg configured
rolebinding.rbac.authorization.k8s.io/cephfs-csi-provisioner-role-cfg configured
```

##### 1.2 Update the CephFS Provisioner deployment

```bash
$ kubectl apply -f deploy/cephfs/kubernetes/csi-cephfsplugin-provisioner.yaml
service/csi-cephfsplugin-provisioner configured
deployment.apps/csi-cephfsplugin-provisioner configured
```

wait for the deployment to complete

```bash
$ kubectl get deployment
NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
csi-cephfsplugin-provisioner   3/3     1            3           104m
```

deployment UP-TO-DATE value must be same as READY

#### 2. Upgrade CephFS Nodeplugin resources

Upgrading nodeplugin resources include updating the nodeplugin RBAC and
nodeplugin daemonset

##### 2.1 Update the CephFS Nodeplugin RBAC

```bash
$ kubectl apply -f deploy/cephfs/kubernetes/csi-nodeplugin-rbac.yaml
serviceaccount/cephfs-csi-nodeplugin configured
```

If you determined in [Pre-upgrade considerations](#pre-upgrade-considerations)
that you were affected by the CSI driver restart issue that disconnects the
application pods from their mounts, continue with this section. Otherwise, you
can skip to step 2.2

```console
vi deploy/cephfs/kubernetes/csi-cephfsplugin.yaml
```

```yaml
kind: DaemonSet
apiVersion: apps/v1
metadata:
  name: csi-cephfsplugin
spec:
  selector:
    matchLabels:
      app: csi-cephfsplugin
  updateStrategy:
    type: OnDelete
  template:
    metadata:
      labels:
        app: csi-cephfsplugin
    spec:
      serviceAccountName: cephfs-csi-nodeplugin
```

in the above template we have added `updateStrategy` and its `type` to the
daemonset spec

##### 2.2 Update the CephFS Nodeplugin daemonset

```bash
$ kubectl apply -f deploy/cephfs/kubernetes/csi-cephfsplugin.yaml
daemonset.apps/csi-cephfsplugin configured
service/csi-metrics-cephfsplugin configured
```

##### 2.3 Manual deletion of CephFS Nodeplugin daemonset pods

If you determined in [Pre-upgrade considerations](#pre-upgrade-considerations)
that you were affected by the CSI driver restart issue that disconnects the
application pods from their mounts, continue with this section. Otherwise, you
can skip this section.

As we have set the updateStrategy to OnDelete the CSI driver pods will not be
updated until you delete them manually. This allows you to control when your
application pods will be affected by the CSI driver restart.

For each node:

- Drain your application pods from the node
- Delete the CSI driver pods on the node
   - The pods to delete will be named with a csi-cephfsplugin prefix and have a
     random suffix on each node. However, no need to delete the provisioner
     pods: csi-cephfsplugin-provisioner-* .
   - The pod deletion causes the pods to be restarted and updated automatically
     on the node.

##### 2.4 Modifying MountOptions in Storageclass and PersistentVolumes

CephCSI, starting from release v3.9.0, will pass the options specified in the
StorageClass's `MountOptions` during both `NodeStageVolume` (kernel cephfs or
ceph-fuse mount operation) and `NodePublishVolume` (bind mount) operations.
Therefore, only common options that is acceptable during both the above
described operations needs to be set in StorageClass's `MountOptions`.
If invalid mount options are set in StorageClass's `MountOptions`
such as `"debug"`, the mounting of cephFS PVCs will fail.

Follow the below steps to update the StorageClass's `MountOptions`:

- Take a backup of the StorageClass using
  `kubectl get sc <storageclass-name> -o yaml > sc.yaml`.
- Edit `sc.yaml` to remove the invalid mount options from `MountOptions` field.
- Delete the StorageClass using `kubectl delete sc <storageclass-name>`.
- Recreate the StorageClass using `kubectl create -f sc.yaml`.

Follow the below steps to update the PersistentVolume's `MountOptions`:

- Identify cephFS PersistentVolumes using
  `kubectl get pv | grep <storageclass-name>`.
- and remove invalid mount options from `MountOptions` field
  in the PersistentVolume's using `kubectl edit pv <pv-name>`.

#### Delete removed CephFS PSP, Role and RoleBinding

As PSP is deprecated in Kubernetes v1.21.0. Delete PSP related objects as PSP
support for CephFS is removed.

```console
kubectl delete psp cephfs-csi-provisioner-psp --ignore-not-found
kubectl delete role cephfs-csi-provisioner-psp --ignore-not-found
kubectl delete rolebinding cephfs-csi-provisioner-psp --ignore-not-found
kubectl delete psp cephfs-csi-nodeplugin-psp --ignore-not-found
kubectl delete role cephfs-csi-nodeplugin-psp --ignore-not-found
kubectl delete rolebinding cephfs-csi-nodeplugin-psp --ignore-not-found
```

we have successfully upgraded cephfs csi from v3.8 to v3.9

### Upgrading RBD

Upgrading rbd csi includes upgrade of rbd driver and as well as kubernetes
sidecar containers and also the permissions required for the kubernetes sidecar
containers, lets upgrade the things one by one

#### 3. Upgrade RBD Provisioner resources

Upgrading provisioner resources include updating the provisioner RBAC and
Provisioner deployment

##### 3.1 Update the RBD Provisioner RBAC

```bash
$ kubectl apply -f deploy/rbd/kubernetes/csi-provisioner-rbac.yaml
serviceaccount/rbd-csi-provisioner configured
clusterrole.rbac.authorization.k8s.io/rbd-external-provisioner-runner configured
clusterrole.rbac.authorization.k8s.io/rbd-external-provisioner-runner-rules configured
clusterrolebinding.rbac.authorization.k8s.io/rbd-csi-provisioner-role configured
role.rbac.authorization.k8s.io/rbd-external-provisioner-cfg configured
rolebinding.rbac.authorization.k8s.io/rbd-csi-provisioner-role-cfg configured
```

##### 3.2 Update the RBD Provisioner deployment

```bash
$ kubectl apply -f deploy/rbd/kubernetes/csi-rbdplugin-provisioner.yaml
service/csi-rbdplugin-provisioner configured
deployment.apps/csi-rbdplugin-provisioner configured
```

wait for the deployment to complete

```bash
$ kubectl get deployments
NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
csi-rbdplugin-provisioner      3/3     3            3           139m
```

deployment UP-TO-DATE value must be same as READY

#### 4. Upgrade RBD Nodeplugin resources

Upgrading nodeplugin resources include updating the nodeplugin RBAC and
nodeplugin daemonset

##### 4.1 Update the RBD Nodeplugin RBAC

```bash
$ kubectl apply -f deploy/rbd/kubernetes/csi-nodeplugin-rbac.yaml
serviceaccount/rbd-csi-nodeplugin configured
clusterrole.rbac.authorization.k8s.io/rbd-csi-nodeplugin configured
clusterrole.rbac.authorization.k8s.io/rbd-csi-nodeplugin-rules configured
clusterrolebinding.rbac.authorization.k8s.io/rbd-csi-nodeplugin configured
```

##### 4.2 Update the RBD Nodeplugin daemonset

```bash
$ kubectl apply -f deploy/rbd/kubernetes/csi-rbdplugin.yaml
daemonset.apps/csi-rbdplugin configured
service/csi-metrics-rbdplugin configured
```

#### Delete removed RBD PSP, Role and RoleBinding

As PSP is deprecated in Kubernetes v1.21.0. Delete PSP related objects as PSP
support for RBD is removed.

```console
kubectl delete psp rbd-csi-provisioner-psp --ignore-not-found
kubectl delete role rbd-csi-provisioner-psp --ignore-not-found
kubectl delete rolebinding rbd-csi-provisioner-psp --ignore-not-found
kubectl delete psp rbd-csi-nodeplugin-psp --ignore-not-found
kubectl delete role rbd-csi-nodeplugin-psp --ignore-not-found
kubectl delete rolebinding rbd-csi-nodeplugin-psp --ignore-not-found
kubectl delete psp rbd-csi-vault-token-review-psp --ignore-not-found
kubectl delete role rbd-csi-vault-token-review-psp --ignore-not-found
kubectl delete rolebinding rbd-csi-vault-token-review-psp --ignore-not-found
```

we have successfully upgraded RBD csi from v3.8 to v3.9

### Upgrading NFS

Upgrading nfs csi includes upgrade of nfs driver and as well as
kubernetes sidecar containers and also the permissions required for the
kubernetes sidecar containers, lets upgrade the things one by one

#### 5. Upgrade NFS Provisioner resources

Upgrade provisioner resources include updating the provisioner RBAC and
Provisioner deployment

##### 5.1 Update the NFS Provisioner RBAC

```bash
$ kubectl apply -f deploy/nfs/kubernetes/csi-provisioner-rbac.yaml
serviceaccount/nfs-csi-provisioner configured
clusterrole.rbac.authorization.k8s.io/nfs-external-provisioner-runner configured
clusterrolebinding.rbac.authorization.k8s.io/nfs-csi-provisioner-role configured
role.rbac.authorization.k8s.io/nfs-external-provisioner-cfg configured
rolebinding.rbac.authorization.k8s.io/nfs-csi-provisioner-role-cfg configured
```

##### 5.2 Update the NFS Provisioner deployment

```bash
$ kubectl apply -f deploy/nfs/kubernetes/csi-nfsplugin-provisioner.yaml
service/csi-nfsplugin-provisioner configured
deployment.apps/csi-nfsplugin-provisioner configured
```

wait for the deployment to complete

```bash
$ kubectl get deployment
NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
csi-nfsplugin-provisioner      5/5     1            5           104m
```

deployment UP-TO-DATE value must be same as READY

#### 6. Upgrade NFS Nodeplugin resources

Upgrading nodeplugin resources include updating the nodeplugin RBAC and
nodeplugin daemonset

##### 6.1 Update the NFS Nodeplugin RBAC

```bash
$ kubectl apply -f deploy/nfs/kubernetes/csi-nodeplugin-rbac.yaml
serviceaccount/nfs-csi-nodeplugin configured
```

##### 6.2 Update the NFS Nodeplugin daemonset

```bash
$ kubectl apply -f deploy/nfs/kubernetes/csi-nfsplugin.yaml
daemonset.apps/csi-nfsplugin configured
service/csi-metrics-nfsplugin configured
```

we have successfully upgraded nfs csi from v3.8 to v3.9

### CSI Sidecar containers consideration

With 3.2.0 version of ceph-csi we have updated the versions of CSI sidecar
containers. These versions are generally compatible with kubernetes
version>=1.17 but based on the kubernetes version you are using, you need to
update the templates with required sidecar versions.
You also might need to update or remove a few arguments based on the sidecar
versions you are using.
Refer
[sidecar-compatibility](https://kubernetes-csi.github.io/docs/sidecar-containers.html)
for more details.
