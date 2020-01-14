# Ceph-csi Upgrade

- [Ceph-csi Upgrade](#ceph-csi-upgrade)
  - [Pre-upgrade considerations](#pre-upgrade-considerations)
  - [Upgrading from v1.2.x to v2.0.0](#upgrading-from-v12x-to-v200)
    - [Upgrading CephFS](#upgrading-cephfs)
      - [1. Upgrade CephFS Provisioner resources](#1-upgrade-cephfs-provisioner-resources)
        - [1.1 Update the CephFS Provisioner RBAC](#11-update-the-cephfs-provisioner-rbac)
        - [1.2 Update the CephFS Provisioner deployment/statefulset](#12-update-the-cephfs-provisioner-deploymentstatefulset)
      - [2. Upgrade CephFS Nodeplugin resources](#2-upgrade-cephfs-nodeplugin-resources)
        - [2.1 Update the CephFS Nodeplugin RBAC](#21-update-the-cephfs-nodeplugin-rbac)
        - [2.2 Update the CephFS Nodeplugin daemonset](#22-update-the-cephfs-nodeplugin-daemonset)
        - [2.3 Manual deletion of CephFS Nodeplugin daemonset pods](#23-manual-deletion-of-cephfs-nodeplugin-daemonset-pods)
    - [Upgrading RBD](#upgrading-rbd)
      - [3. Upgrade RBD Provisioner resources](#3-upgrade-rbd-provisioner-resources)
        - [3.1 Update the RBD Provisioner RBAC](#31-update-the-rbd-provisioner-rbac)
        - [3.2 Update the RBD Provisioner deployment/statefulset](#32-update-the-rbd-provisioner-deploymentstatefulset)
      - [4. Upgrade RBD Nodeplugin resources](#4-upgrade-rbd-nodeplugin-resources)
        - [4.1 Update the RBD Nodeplugin RBAC](#41-update-the-rbd-nodeplugin-rbac)
        - [4.2 Update the RBD Nodeplugin daemonset](#42-update-the-rbd-nodeplugin-daemonset)
        - [4.3 Manual deletion of RBD Nodeplugin daemonset pods](#43-manual-deletion-of-rbd-nodeplugin-daemonset-pods)

## Pre-upgrade considerations

In some scenarios there is an issue in the CSI driver that will cause
application pods to be disconnected from their mounts when the CSI driver is
restarted. Since the upgrade would cause the CSI driver to restart if it is
updated, you need to be aware of whether this affects your applications. This
issue will happen when using the Ceph fuse client or rbd-nbd:

CephFS: If you are provision volumes for CephFS and have a kernel less than
version 4.17, The CSI driver will fall back to use the FUSE client.

RBD: If you have set the mounter: rbd-nbd option in the RBD storage class, the
NBD mounter will have this issue.

If you are affected by this issue, you will need to proceed carefully during
the upgrade to restart your application pods. The recommended step is to modify
the update strategy of the CSI nodeplugin daemonsets to OnDelete so that you
can control when the CSI driver pods are restarted on each node.

To avoid this issue in future upgrades, we recommend that you do not use the
fuse client or rbd-nbd as of now.

This guide will walk you through the steps to upgrade the software in a cluster
from v1.2.x to v2.0.0

## Upgrading from v1.2.x to v2.0.0

**Ceph-csi releases from master are expressly unsupported.** It is strongly
recommended that you use [official
releases](https://github.com/ceph/ceph-csi/releases) of Ceph-csi. Unreleased
versions from the master branch are subject to changes and incompatibilities
that will not be supported in the official releases. Builds from the master
branch can have functionality changed and even removed at any time without
compatibility support and without prior notice.

git checkout release v2.0.0 branch

```bash
[$] git clone https://github.com/ceph/ceph-csi.git
[$] git check v2.0.0
[$] cd ./ceph-csi
```

**Note:** While upgrading please Ignore warning messages from kubectl output

```console
Warning: kubectl apply should be used on resource created by either kubectl create --save-config or kubectl apply
```

### Upgrading CephFS

Upgrading cephfs csi includes upgrade of cephfs driver and as well as
kubernetes sidecar containers and also the permissions required for the
kubernetes sidecar containers, lets upgrade the things one by one

**Note** If you are using ceph-csi with kubernetes v1.13 use templates from
v1.13 directory

#### 1. Upgrade CephFS Provisioner resources

Upgrade provisioner resources include updating the provisioner RBAC and
Provisioner deployment

##### 1.1 Update the CephFS Provisioner RBAC

```bash
[$] kubectl apply -f deploy/cephfs/kubernetes/v1.14+/csi-provisioner-rbac.yaml
serviceaccount/cephfs-csi-provisioner configured
clusterrole.rbac.authorization.k8s.io/cephfs-external-provisioner-runner configured
clusterrole.rbac.authorization.k8s.io/cephfs-external-provisioner-runner-rules configured
clusterrolebinding.rbac.authorization.k8s.io/cephfs-csi-provisioner-role configured
role.rbac.authorization.k8s.io/cephfs-external-provisioner-cfg configured
rolebinding.rbac.authorization.k8s.io/cephfs-csi-provisioner-role-cfg configured
```

##### 1.2 Update the CephFS Provisioner deployment/statefulset

```bash
[$]kubectl apply -f deploy/cephfs/kubernetes/v1.14+/csi-cephfsplugin-provisioner.yaml
service/csi-cephfsplugin-provisioner configured
deployment.apps/csi-cephfsplugin-provisioner configured
```

wait for the deployment to complete

```bash
[$]kubectl get deployment
NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
csi-cephfsplugin-provisioner   3/3     1            3           104m
```

deployment UP-TO-DATE value must be same as READY

#### 2. Upgrade CephFS Nodeplugin resources

Upgrading nodeplugin resources include updating the nodeplugin RBAC and
nodeplugin daemonset

##### 2.1 Update the CephFS Nodeplugin RBAC

```bash
[$]kubectl apply -f deploy/cephfs/kubernetes/v1.14+/csi-nodeplugin-rbac.yaml
serviceaccount/cephfs-csi-nodeplugin configured
clusterrole.rbac.authorization.k8s.io/cephfs-csi-nodeplugin configured
clusterrole.rbac.authorization.k8s.io/cephfs-csi-nodeplugin-rules configured
clusterrolebinding.rbac.authorization.k8s.io/cephfs-csi-nodeplugin configured
```

If you determined in [Pre-upgrade considerations](#pre-upgrade-considerations)
that you were affected by the CSI driver restart issue that disconnects the
application pods from their mounts, continue with this section.  Otherwise, you
can skip to step 2.2

```console
vi deploy/cephfs/kubernetes/v1.14+/csi-cephfsplugin.yaml
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
      serviceAccount: cephfs-csi-nodeplugin
```

in the above template we have added `updateStrategy` and its `type` to the
daemonset spec

##### 2.2 Update the CephFS Nodeplugin daemonset

```bash
[$]kubectl apply -f deploy/cephfs/kubernetes/v1.14+/csi-cephfsplugin.yaml
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

we have successfully upgrade cephfs csi from v1.2.2 to v2.0.0

### Upgrading RBD

Upgrading rbd csi includes upgrade of rbd driver and as well as kubernetes
sidecar containers and also the permissions required for the kubernetes sidecar
containers, lets upgrade the things one by one

**Note:** If you are using ceph-csi with kubernetes v1.13 use templates from
v1.13 directory

#### 3. Upgrade RBD Provisioner resources

Upgrading provisioner resources include updating the provisioner RBAC and
Provisioner deployment

##### 3.1 Update the RBD Provisioner RBAC

```bash
[$]kubectl apply -f deploy/rbd/kubernetes/v1.14+/csi-provisioner-rbac.yaml
serviceaccount/rbd-csi-provisioner configured
clusterrole.rbac.authorization.k8s.io/rbd-external-provisioner-runner configured
clusterrole.rbac.authorization.k8s.io/rbd-external-provisioner-runner-rules configured
clusterrolebinding.rbac.authorization.k8s.io/rbd-csi-provisioner-role configured
role.rbac.authorization.k8s.io/rbd-external-provisioner-cfg configured
rolebinding.rbac.authorization.k8s.io/rbd-csi-provisioner-role-cfg configured
```

##### 3.2 Update the RBD Provisioner deployment/statefulset

```bash
[$]kubectl apply -f deploy/rbd/kubernetes/v1.14+/csi-rbdplugin-provisioner.yaml
service/csi-rbdplugin-provisioner configured
deployment.apps/csi-rbdplugin-provisioner configured
```

wait for the deployment to complete

```bash
[$]kubectl get deployments
NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
csi-rbdplugin-provisioner      3/3     3            3           139m
```

deployment UP-TO-DATE value must be same as READY

#### 4. Upgrade RBD Nodeplugin resources

Upgrading nodeplugin resources include updating the nodeplugin RBAC and
nodeplugin daemonset

##### 4.1 Update the RBD Nodeplugin RBAC

```bash
[$]kubectl apply -f deploy/rbd/kubernetes/v1.14+/csi-nodeplugin-rbac.yaml
serviceaccount/rbd-csi-nodeplugin configured
clusterrole.rbac.authorization.k8s.io/rbd-csi-nodeplugin configured
clusterrole.rbac.authorization.k8s.io/rbd-csi-nodeplugin-rules configured
clusterrolebinding.rbac.authorization.k8s.io/rbd-csi-nodeplugin configured
```

If you determined in [Pre-upgrade considerations](#pre-upgrade-considerations)
that you were affected by the CSI driver restart issue that disconnects the
application pods from their mounts, continue with this section. Otherwise, you
can skip to step 4.2

```console
vi deploy/rbd/kubernetes/v1.14+/csi-rbdplugin.yaml
```

```yaml
kind: DaemonSet
apiVersion: apps/v1
metadata:
  name: csi-rbdplugin
spec:
  selector:
    matchLabels:
      app: csi-rbdplugin
  updateStrategy:
    type: OnDelete
  template:
    metadata:
      labels:
        app: csi-rbdplugin
    spec:
      serviceAccount: rbd-csi-nodeplugin
```

in the above template we have added `updateStrategy` and its `type` to the
daemonset spec

##### 4.2 Update the RBD Nodeplugin daemonset

```bash
[$]kubectl apply -f deploy/rbd/kubernetes/v1.14+/csi-rbdplugin.yaml
daemonset.apps/csi-rbdplugin configured
service/csi-metrics-rbdplugin configured
```

##### 4.3 Manual deletion of RBD Nodeplugin daemonset pods

If you determined in [Pre-upgrade considerations](#pre-upgrade-considerations)
that you were affected by the CSI driver restart issue that disconnects the
application pods from their mounts, continue with this section.
Otherwise, you can skip this section.

As we have set the updateStrategy to OnDelete the CSI driver pods will not be
updated until you delete them manually. This allows you to control when your
application pods will be affected by the CSI driver restart.

For each node:

- Drain your application pods from the node
- Delete the CSI driver pods on the node
  - The pods to delete will be named with a csi-rbdplugin prefix and have a
    random suffix on each node. However, no need to delete the provisioner pods
    csi-rbdplugin-provisioner-* .
  - The pod deletion causes the pods to be restarted and updated automatically
    on the node.

we have successfully upgrade RBD csi from v1.2.2 to v2.0.0
