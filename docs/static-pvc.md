# Static PVC with ceph-csi

- [Static PVC with ceph-csi](#static-pvc-with-ceph-csi)
  - [RBD static PVC](#rbd-static-pvc)
    - [Create RBD image](#create-rbd-image)
    - [Create RBD static PV](#create-rbd-static-pv)
    - [RBD Volume Attributes in PV](#rbd-volume-attributes-in-pv)
    - [Create RBD static PVC](#create-rbd-static-pvc)

This document outlines how to create static PV and static PVC from existing rbd image

**warning** static PVC can be created, deleted, mounted and unmounted but
currently ceph-csi doesn't support other operations like snapshot,clone,
resize, etc for static PVC

## RBD static PVC

RBD images created manually can be mounted and unmounted to an app, below step
shows how to create a rbd image, static PV, static PVC

### Create RBD image

If you already have a rbd image created and contains some data which you want
to access by the application pod you can skip this step.

Lets create a new rbd image in ceph cluster which we are going to use for
static PVC

```bash
[$]rbd create static-image --size=1024 --pool=replicapool
```

### Create RBD static PV

To create the rbd PV you need to know the `rbd image name`,`clusterID` and
`pool` name in which the rbd image is created

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

|  Attributes  |                                                                Description                                                                 | Required |
| :----------: | :----------------------------------------------------------------------------------------------------------------------------------------: | :------: |
|  clusterID   | The is used by the CSI plugin to uniquely identify and use a Ceph cluster (this is the key in configmap created duing ceph-csi deployment) |   Yes    |
|     pool     |                                                The pool name in which rbd image is created                                                 |   Yes    |
| staticVolume |                                      Value must be set to `true` to mount and unmount static rbd PVC                                       |   yes    |
|   mounter    |                 If set to `rbd-nbd`, use `rbd-nbd` on nodes that have `rbd-nbd` and `nbd` kernel modules to map rbd images                 |    No    |

**Note** ceph-csi does not supports rbd image deletion for static PV.
`persistentVolumeReclaimPolicy` in PV spec must be set to `Retain` to avoid PV
delete attempt in csi-provisioner.

```bash
[$] kubectl create -f fs-static-pv.yaml
persistentvolume/fs-static-pv created
```

### Create RBD static PVC

To create the rbd PVC you need to know the PV name which is created above

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: fs-static-pvc
  namespace: default
spec:
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
[$] kubectl create -f fs-static-pvc.yaml
persistentvolumeclaim/fs-static-pvc created
```

**Note** deleting PV and PVC doesnot deleted the backend rbd image, user need to
manually delete the rbd image if required
