# VolumeAttributesClass for Volume Modification

Kubernetes offers a method for modifying Volume parameters after they have been
created. This is done through VolumeAttributesClasses and is described in a
[blog
post](https://kubernetes.io/blog/2025/09/08/kubernetes-v1-34-volume-attributes-class).

## Prerequisites

- Kubernetes 1.34 is the first release where support for
  VolumeAttributesClasses (the `ControllerModifyVolume` CSI procedure) is GA.
  Older versions of Kubernetes may not work reliably.
- The Kubernetes CSI external-provisioner (`csi-provisioner` sidecar) release
  needs to be higher or equal to v6.1 (support for secrets).
- The Kubernetes CSI external-resizer (`csi-resizer` sidecar) release
  needs to be higher or equal to v2.1 (support for secrets).

## Secret references in the StorageClass

When setting a VolumeAttributesClass on a PersistentVolumeClaim, the
`ControllerModifyVolume` CSI procedure is called on the provisioner. This
procedure needs secrets (that contain the Ceph credentials) in order to
communicate with the Ceph cluster. Below are the keys that should be set in the
StorageClass' `parameters`:

- `csi.storage.k8s.io/controller-modify-secret-name`
- `csi.storage.k8s.io/controller-modify-secret-namespace`

In addition to the execution of `ControllerModifyVolume`, the Node-plugin needs
access to the Volume on the Ceph cluster to fetch the updated parameters.
Usually NFS and NVMe-oF do not need any Ceph credentials for the Node-plugin,
but for using VolumeAttributesClasses to modify Volumes this is a requirement.

Depending on the storage backend, Volumes are optionally _Staged_ before
getting _Published_. NVMe-oF uses the staging process, and needs credentials
there, hence these parameters should be set:

- `csi.storage.k8s.io/node-stage-secret-name`
- `csi.storage.k8s.io/node-stage-secret-namespace`

NFS does not use the staging process, and only needs the credentials during the
publishing process:

- `csi.storage.k8s.io/node-publish-secret-name`
- `csi.storage.k8s.io/node-publish-secret-namespace`

## Adding support to existing Volumes

PersistentVolumeClaims that have not been created with the right secret
references in the StorageClass will not be modifiable with a
VolumeAttributesClass without manual intervention.

In order to be able to modify parameters with a VolumeAttributesClass,
annotations should be added to the PersistentVolume that is _Bound_ to the
PersistentVolumeClaim. These annotations should refer to the namespace and
secret where the credentials are available (just like the namespace and secret
that would be referenced in the StorageClass).

- `volume.kubernetes.io/controller-modify-secret-name`
- `volume.kubernetes.io/controller-modify-secret-namespace`

Note that these annotations only make it possible for the provisioner to modify
parameters. This does not allow the Node-plugin accessing the Ceph cluster to
fetch updated parameters when the Volume is _staged_ or _published_. The
secrets for staging and publishing can not (easily) be updated after the fact,
these are part of the fixed parameters in the PersistentVolume.

## RBD Volume QoS with VolumeAttributesClass

Ceph-CSI supports two types of QoS for RBD volumes:

1. **Traditional QoS** (for rbd-nbd mounter only)
1. **Cgroup v2 QoS** (for krbd/kernel mounter, requires cgroup v2)

**Note**: You cannot mix traditional QoS parameters and cgroup v2 QoS parameters
in the same VolumeAttributesClass.

### Traditional QoS for rbd-nbd

Traditional QoS is applied at the RBD device level using rbd-nbd's built-in
QoS capabilities. This approach only works with the rbd-nbd mounter.

#### Create a VolumeAttributesClass for rbd-nbd QoS

- Define a VolumeAttributesClass

```console
---
apiVersion: storage.k8s.io/v1
kind: VolumeAttributesClass
metadata:
  name: silver
driverName: rbd.csi.ceph.com
parameters:
  baseIops: "1800"
  maxIops: "10000"
  baseBps: "104857600"
  maxBps: "188743680"
  iopsPerGiB: "12"
  bpsPerGiB: "209715"
  baseVolSizeBytes: "21474836480"
```

```console
kubectl create -f volumeattributesclass.yaml
```

- Verify VolumeAttributesClass has been created

```console
$ kubectl get vac
NAME     DRIVERNAME                   AGE
silver   rbd.csi.ceph.com             2s
```

### Create PVC with VolumeAttributesClass

- Define a PVC

```console
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rbd-pvc-vac
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 30Gi
  storageClassName: csi-rbd-sc
  volumeMode: Block
  volumeAttributesClassName: silver
```

```console
$ kubectl create -f pvc.yaml
persistentvolumeclaim/rbd-pvc-vac created
```

- Verify pvc by get

```console
$ kubectl get pvc
NAME          STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      VOLUMEATTRIBUTESCLASS   AGE
rbd-pvc-vac   Bound    pvc-8b2fcb47-233a-4bcf-bb94-f94e9aa1150a   30Gi       RWO            csi-rbd-sc        silver                  1m
```

### Modify PVC VolumeAttributesClass

- Create another VolumeAttributesClass

```console
---
apiVersion: storage.k8s.io/v1
kind: VolumeAttributesClass
metadata:
  name: gold
driverName: rbd.csi.ceph.com
parameters:
  baseIops: "1800"
  maxIops: "50000"
  baseBps: "104857600"
  maxBps: "367001600"
  iopsPerGiB: "50"
  bpsPerGiB: "524288"
  baseVolSizeBytes: "21474836480"
```

```console
$ kubectl create -f volumeattributesclass.yaml
volumeattributesclass.storage.k8s.io/silver created
```

- Verify VolumeAttributesClass by get

```console
$ kubectl get vac
NAME     DRIVERNAME                   AGE
gold     rbd.csi.ceph.com             1m
silver   rbd.csi.ceph.com             2m
```

- Modify PVC VolumeAttributesClassName

```console
$ kubectl patch pvc rbd-pvc-vac -p '{"spec":{"volumeAttributesClassName": "gold"}}'
persistentvolumeclaim/rbd-pvc-vac patched
```

- Verify modify pvc volumeAttributesClassName by get

```console
$ kubectl get pvc
NAME          STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      VOLUMEATTRIBUTESCLASS   AGE
rbd-pvc-vac   Bound    pvc-8b2fcb47-233a-4bcf-bb94-f94e9aa1150a   30Gi       RWO            csi-rbd-sc        gold                    2m
```

### Cgroup v2 QoS for krbd

Cgroup v2 QoS applies io.max limits at the container cgroup level, providing
QoS for volumes mapped using the krbd (kernel RBD) mounter. This approach works
on systems with cgroup v2 enabled.

#### Prerequisites for Cgroup v2 QoS

- Cgroup v2 must be enabled on all Kubernetes nodes
- `podInfoOnMount` must be enabled in the CSIDriver spec (enabled by default)
- NodePublishVolume secret must be configured in StorageClass parameters
  (`csi.storage.k8s.io/node-publish-secret-name`) or CSI ConfigMap
  (`rbd.nodePublishSecretRef`)

#### Create a VolumeAttributesClass for Cgroup v2 QoS

- Define a VolumeAttributesClass

```console
---
apiVersion: storage.k8s.io/v1
kind: VolumeAttributesClass
metadata:
  name: cgroup-qos
driverName: rbd.csi.ceph.com
parameters:
  # Cgroup v2 QoS parameters
  maxReadIops: "1000"         # 1000 read IOPS
  maxWriteIops: "2000"        # 2000 write IOPS
  maxReadBps: "104857600"     # 100 MiB/s
  maxWriteBps: "209715200"    # 200 MiB/s
```

```console
kubectl create -f volumeattributesclass-cgroup.yaml
```

- Verify VolumeAttributesClass has been created

```console
$ kubectl get vac
NAME          DRIVERNAME                   AGE
cgroup-qos    rbd.csi.ceph.com             2s
```

#### Configure NodePublishVolume Secret

##### Option 1: StorageClass Parameters (Recommended)

Add the following parameters to your StorageClass:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-rbd-sc
provisioner: rbd.csi.ceph.com
parameters:
  clusterID: <cluster-id>
  pool: <rbd-pool-name>
  # ... other parameters ...
  csi.storage.k8s.io/node-publish-secret-name: csi-rbd-secret
  csi.storage.k8s.io/node-publish-secret-namespace: default
```

##### Option 2: CSI ConfigMap Fallback

Alternatively, configure the default secret in the CSI ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ceph-csi-config
data:
  config.json: |-
    [
      {
        "clusterID": "<cluster-id>",
        "monitors": ["<mon1>", "<mon2>", "<mon3>"],
        "rbd": {
          "nodePublishSecretRef": {
            "name": "csi-rbd-secret",
            "namespace": "default"
          }
        }
      }
    ]
```

#### Create PVC with Cgroup v2 QoS

```console
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rbd-pvc-cgroup-qos
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: csi-rbd-sc
  volumeMode: Block
  volumeAttributesClassName: cgroup-qos
```

```console
$ kubectl create -f pvc.yaml
persistentvolumeclaim/rbd-pvc-cgroup-qos created
```

#### How Cgroup v2 QoS Works

1. QoS parameters are stored in RBD image metadata during
  `CreateVolume` or `ControllerModifyVolume`.
1. During `NodePublishVolume`, Ceph-CSI retrieves the QoS metadata
1. The device major:minor number is determined from the mapped RBD device
1. io.max limits are applied to the pod's cgroup
1. QoS limits can be dynamically modified by updating the VolumeAttributesClass
1. Pod need to be restart to get the new QoS limits applied.
