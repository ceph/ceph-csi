# Static PVC with ceph-csi

- [Static PVC with ceph-csi](#static-pvc-with-ceph-csi)
   - [RBD static PVC](#rbd-static-pvc)
      - [Create RBD image](#create-rbd-image)
      - [Create RBD static PV](#create-rbd-static-pv)
      - [RBD Volume Attributes in PV](#rbd-volume-attributes-in-pv)
      - [Create RBD static PVC](#create-rbd-static-pvc)
      - [Resize RBD image](#resize-rbd-image)
      - [Verify RBD static PVC](#verify-rbd-static-pvc)
   - [CephFS static PVC](#cephfs-static-pvc)
      - [Create CephFS subvolume](#create-cephfs-subvolume)
      - [Create CephFS static PV](#create-cephfs-static-pv)
      - [Node stage secret ref in CephFS PV](#node-stage-secret-ref-in-cephfs-pv)
      - [CephFS volume attributes in PV](#cephfs-volume-attributes-in-pv)
      - [Create CephFS static PVC](#create-cephfs-static-pvc)
      - [Verify CephFS static PVC](#verify-cephfs-static-pvc)

This document outlines how to create static PV and static PVC from
existing RBD image or CephFS volume.

> [!warning]
> static PVC can be created, deleted, mounted and unmounted but
currently ceph-csi doesn't support other operations like snapshot, clone,
resize, encryption etc for static PVC

## RBD static PVC

> [!warning]
> Features that require ceph-csi to manage the volume lifecycle are not
supported for static RBD PVC

RBD images created manually can be mounted and unmounted to an app, below step
shows how to create a RBD image, static PV, static PVC

### Create RBD image

> [!tip]
> If you already have a RBD image created and contains some data which you want
to access by the application pod you can skip this step.

Let's create a new RBD image in ceph cluster which we are going to use for
static PVC

```console
rbd create static-image --size=1024 --pool=replicapool
```

### Create RBD static PV

To create the RBD PV you need to know the `rbd image name`,`clusterID` and
`pool` name in which the RBD image is created

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: fs-static-pv
spec:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 1Gi
  csi:
    driver: rbd.csi.ceph.com
    fsType: ext4
    nodeStageSecretRef:
      # node stage secret name
      name: csi-rbd-secret
      # node stage secret namespace where above secret is created
      namespace: default
    volumeAttributes:
      # Required options from storageclass parameters need to be added in volumeAttributes
      "clusterID": "ba68226a-672f-4ba5-97bc-22840318b2ec"
      "pool": "replicapool"
      "staticVolume": "true"
      "imageFeatures": "layering"
      #mounter: rbd-nbd
    # volumeHandle should be same as rbd image name
    volumeHandle: static-image
  persistentVolumeReclaimPolicy: Retain
  # The volumeMode can be either `Filesystem` or `Block` if you are creating Filesystem PVC it should be `Filesystem`, if you are creating Block PV you need to change it to `Block`
  volumeMode: Filesystem
```

### RBD Volume Attributes in PV

Below table explains the list of volume attributes can be set when creating a
static RBD PV

|  Attributes   |                                                                     Description                                                                      | Required |
| :-----------: | :--------------------------------------------------------------------------------------------------------------------------------------------------: | :------: |
|   clusterID   | The clusterID is used by the CSI plugin to uniquely identify and use a Ceph cluster (this is the key in configmap created duing ceph-csi deployment) |   Yes    |
|     pool      |                                                     The pool name in which RBD image is created                                                      |   Yes    |
| staticVolume  |                                           Value must be set to `true` to mount and unmount static RBD PVC                                            |   Yes    |
| imageFeatures |       CSI RBD currently supports `layering, journaling, exclusive-lock` features. If `journaling` is enabled, must enable `exclusive-lock` too       |   Yes    |
|    mounter    |                      If set to `rbd-nbd`, use `rbd-nbd` on nodes that have `rbd-nbd` and `nbd` kernel modules to map RBD images                      |    No    |

> [!note]
> ceph-csi does not supports RBD image deletion for static PV.
`persistentVolumeReclaimPolicy` in PV spec must be set to `Retain` to avoid PV
delete attempt in csi-provisioner.

```bash
$ kubectl create -f fs-static-pv.yaml
persistentvolume/fs-static-pv created
```

### Create RBD static PVC

To create the RBD PVC you need to know the PV name which is created above

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: fs-static-pvc
  namespace: default
spec:
  storageClassName: ""
  accessModes:
  # ReadWriteMany is only supported for Block PVC
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  # The volumeMode can be either `Filesystem` or `Block` if you are creating Filesystem PVC it should be `Filesystem`, if you are creating Block PV you need to change it to `Block`
  volumeMode: Filesystem
  # volumeName should be same as PV name
  volumeName: fs-static-pv
```

```bash
$ kubectl create -f fs-static-pvc.yaml
persistentvolumeclaim/fs-static-pvc created
```

### Resize RBD image

Let us resize the RBD image in ceph cluster

```console
rbd resize static-image --size=2048 --pool=replicapool
```

Once the RBD image is resized in the ceph cluster, update the PV size and PVC
size to match the size of the RBD image.

Now scale down the application pod which is using `cephfs-static-pvc` and scale
up the application pod to resize the filesystem.

> [!note]
> If you have mounted same static PVC to multiple application pods, make
sure you will scale down all the application pods and make sure no application
pods using the static PVC is running on the node and scale up all the
application pods again(this will trigger `NodeStageVolumeRequest` which will
resize the filesystem for static volume).

### Verify RBD static PVC

We can verify the RBD static PVC by creating a Pod that mounts the PVC.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: rbd-static-pvc-test
  namespace: default
spec:
  containers:
    - name: busybox
      image: busybox:latest
      volumeMounts:
        - name: static-pvc
          mountPath: /data/pvc
      command: ["sleep", "3600"]
  volumes:
    - name: static-pvc
      persistentVolumeClaim:
        claimName: fs-static-pvc
```

```bash
$ kubectl create -f rbd-static-pvc-test.yaml
pod/rbd-static-pvc-test created
```

Verify that the PVC has been successfully mounted.

```bash
kubectl exec rbd-static-pvc-test -- df -h /data/pvc
```

Once you have completed the verification step, you can delete the test Pod for RBD.

```bash
$ kubectl delete pod rbd-static-pvc-test
pod "rbd-static-pvc-test" deleted
```

> [!note]
> deleting PV and PVC does not removed the backend RBD image, user need to
manually delete the RBD image if required

## CephFS static PVC

> [!warning]
> Features that require ceph-csi to manage the volume lifecycle are not
supported for static CephFS PVC

CephFS subvolume or volume created manually can be mounted and unmounted
to an app, below steps show how to create a CephFS subvolume or volume,
static PV and static PVC.

### Create CephFS subvolume

> [!tip]
> If you already have a CephFS subvolume or volume created and contains some data
which you want to access by the application pod, you can skip this step.

Let's create a new CephFS subvolume of size 1 GiB in ceph cluster which
we are going to use for static PVC, before that we need to create
the subvolumegroup. **myfs** is the filesystem name(volume name) inside
which subvolume should be created.

```console
ceph fs subvolumegroup create myfs testGroup
```

```console
ceph fs subvolume create myfs testSubVolume testGroup --size=1073741824
```

> [!note]
> volume here refers to the filesystem.

### Create CephFS static PV

To create the CephFS PV you need to know the `volume rootpath`, and `clusterID`,
here is the command to get the root path of subvolume in ceph cluster

```bash
$ ceph fs subvolume getpath myfs testSubVolume testGroup
/volumes/testGroup/testSubVolume
```

For volume, you can directly use the folder path relative to the
filesystem as the `rootpath`.

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: cephfs-static-pv
spec:
  accessModes:
  - ReadWriteMany
  capacity:
    storage: 1Gi
  csi:
    driver: cephfs.csi.ceph.com
    nodeStageSecretRef:
      # node stage secret name
      name: csi-cephfs-secret
      # node stage secret namespace where above secret is created
      namespace: default
    volumeAttributes:
      # optional file system to be mounted
      "fsName": "myfs"
      # Required options from storageclass parameters need to be added in volumeAttributes
      "clusterID": "ba68226a-672f-4ba5-97bc-22840318b2ec"
      "staticVolume": "true"
      "rootPath": /volumes/testGroup/testSubVolume
    # volumeHandle can be anything, need not to be same
    # as PV name or volume name. keeping same for brevity
    volumeHandle: cephfs-static-pv
  persistentVolumeReclaimPolicy: Retain
  volumeMode: Filesystem
```

### Node stage secret ref in CephFS PV

For static CephFS PV to work, userID and userKey needs to be specified in the
secret. Static PV will not work with adminID and adminKey.
Format for the secret should be same as detailed [here](https://github.com/ceph/ceph-csi/blob/3e656769b71a3c43d95f6875ed4934c82a8046e7/examples/cephfs/secret.yaml#L7-L10).

### CephFS volume attributes in PV

Below table explains the list of volume attributes can be set when creating a
static CephFS PV

|  Attributes  |                                                                     Description                                                                      | Required |
| :----------: | :--------------------------------------------------------------------------------------------------------------------------------------------------: | :------: |
|  clusterID   | The clusterID is used by the CSI plugin to uniquely identify and use a Ceph cluster (this is the key in configmap created duing ceph-csi deployment) |   Yes    |
|    fsName    |                                      CephFS filesystem name to be mounted. Not passing this option mounts the default file system.                                       |   No    |
| staticVolume |                                           Value must be set to `true` to mount and unmount static cephFS PVC                                         |   Yes    |
|   rootPath   |                     Actual path of the subvolume in ceph cluster which can be retrieved by issuing getpath command as described above, or folder path of the volume                    |   Yes    |

**Note** ceph-csi does not supports CephFS subvolume deletion for static PV.
`persistentVolumeReclaimPolicy` in PV spec must be set to `Retain` to avoid PV
delete attempt in csi-provisioner.

```bash
$ kubectl create -f cephfs-static-pv.yaml
persistentvolume/cephfs-static-pv created
```

### Create CephFS static PVC

To create the CephFS PVC you need to know the PV name which is created above

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: cephfs-static-pvc
  namespace: default
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  storageClassName: ""
  volumeMode: Filesystem
  # volumeName should be same as PV name
  volumeName: cephfs-static-pv
```

```bash
$ kubectl create -f cephfs-static-pvc.yaml
persistentvolumeclaim/cephfs-static-pvc created
```

### Verify CephFS static PVC

We can verify the CephFS static PVC by creating a Pod that mounts the PVC.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cephfs-static-pvc-test
  namespace: default
spec:
  containers:
    - name: busybox
      image: busybox:latest
      volumeMounts:
        - name: static-pvc
          mountPath: /data/pvc
      command: ["sleep", "3600"]
  volumes:
    - name: static-pvc
      persistentVolumeClaim:
        claimName: cephfs-static-pvc
        readOnly: false
```

```bash
$ kubectl create -f cephfs-static-pvc-test.yaml
pod/cephfs-static-pvc-test created
```

Verify that the PVC has been successfully mounted.

```bash
kubectl exec cephfs-static-pvc-test -- df -h /data/pvc
```

Once you have completed the verification step, you can delete the test Pod for CephFS.

```bash
$ kubectl delete pod cephfs-static-pvc-test
pod "cephfs-static-pvc-test" deleted
```

> [!note]
> deleting PV and PVC does not delete the backend CephFS subvolume or volume,
user needs to manually delete the CephFS subvolume or volume if required.
