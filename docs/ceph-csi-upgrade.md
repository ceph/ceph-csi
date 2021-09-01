# Ceph-csi Upgrade

- [Ceph-csi Upgrade](#ceph-csi-upgrade)
  - [Pre-upgrade considerations](#pre-upgrade-considerations)
    - [Snapshot-controller and snapshot crd](#snapshot-controller-and-snapshot-crd)
      - [Snapshot API version support matrix](#snapshot-api-version-support-matrix)
  - [Upgrading from v3.2 to v3.3](#upgrading-from-v32-to-v33)
  - [Upgrading from v3.3 to v3.4](#upgrading-from-v33-to-v34)
    - [Upgrading CephFS](#upgrading-cephfs)
      - [1. Upgrade CephFS Provisioner resources](#1-upgrade-cephfs-provisioner-resources)
        - [1.1 Update the CephFS Provisioner RBAC](#11-update-the-cephfs-provisioner-rbac)
        - [1.2 Update the CephFS Provisioner deployment](#12-update-the-cephfs-provisioner-deployment)
      - [2. Upgrade CephFS Nodeplugin resources](#2-upgrade-cephfs-nodeplugin-resources)
        - [2.1 Update the CephFS Nodeplugin RBAC](#21-update-the-cephfs-nodeplugin-rbac)
        - [2.2 Update the CephFS Nodeplugin daemonset](#22-update-the-cephfs-nodeplugin-daemonset)
        - [2.3 Manual deletion of CephFS Nodeplugin daemonset pods](#23-manual-deletion-of-cephfs-nodeplugin-daemonset-pods)
    - [Upgrading RBD](#upgrading-rbd)
      - [3. Upgrade RBD Provisioner resources](#3-upgrade-rbd-provisioner-resources)
        - [3.1 Update the RBD Provisioner RBAC](#31-update-the-rbd-provisioner-rbac)
        - [3.2 Update the RBD Provisioner deployment](#32-update-the-rbd-provisioner-deployment)
      - [4. Upgrade RBD Nodeplugin resources](#4-upgrade-rbd-nodeplugin-resources)
        - [4.1 Update the RBD Nodeplugin RBAC](#41-update-the-rbd-nodeplugin-rbac)
        - [4.2 Update the RBD Nodeplugin daemonset](#42-update-the-rbd-nodeplugin-daemonset)
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
from v3.3 to v3.4

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

## Upgrading from v3.2 to v3.3

Refer [upgrade-from-v3.2-v3.3](https://github.com/ceph/ceph-csi/blob/v3.3.1/docs/ceph-csi-upgrade.md)
to upgrade from cephcsi v3.2 to v3.3

## Upgrading from v3.3 to v3.4

**Ceph-csi releases from devel are expressly unsupported.** It is strongly
recommended that you use [official
releases](https://github.com/ceph/ceph-csi/releases) of Ceph-csi. Unreleased
versions from the devel branch are subject to changes and incompatibilities
that will not be supported in the official releases. Builds from the devel
branch can have functionality changed and even removed at any time without
compatibility support and without prior notice.

**Also, we do not recommend any direct upgrades to 3.4 except from 3.3 to 3.4.**
For example, upgrading from 3.2 to 3.4 is not recommended.

git checkout v3.4.0 tag

```bash
git clone https://github.com/ceph/ceph-csi.git
cd ./ceph-csi
git checkout v3.4.0
```

**Note:** While upgrading please Ignore warning messages from kubectl output

```console
Warning: kubectl apply should be used on resource created by either kubectl create --save-config or kubectl apply
```

### Upgrading CephFS

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
clusterrole.rbac.authorization.k8s.io/cephfs-csi-nodeplugin configured
clusterrole.rbac.authorization.k8s.io/cephfs-csi-nodeplugin-rules configured
clusterrolebinding.rbac.authorization.k8s.io/cephfs-csi-nodeplugin configured
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

we have successfully upgraded cephfs csi from v3.3 to v3.4

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

we have successfully upgraded RBD csi from v3.3 to v3.4

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
