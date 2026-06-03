# Dynamically Expand Volume

- [Dynamically Expand Volume](#dynamically-expand-volume)
   - [Prerequisite](#prerequisite)
      - [Expand RBD PVCs](#expand-rbd-pvcs)
         - [Expand RBD Filesystem PVC](#expand-rbd-filesystem-pvc)
         - [Expand RBD Block PVC](#expand-rbd-block-pvc)
      - [Expand CephFS PVC](#expand-cephfs-pvc)
         - [Expand CephFS Filesystem PVC](#expand-cephfs-filesystem-pvc)

## Prerequisite

- For filesystem expansion to be supported for your kubernetes cluster, the
  kubernetes version running in your cluster should be >= v1.15 and for block
  volume expand support the kubernetes version should be >=1.16. Also,
  `ExpandCSIVolumes` feature gate has to be enabled for the volume expand
  functionality to work.

- The controlling StorageClass must have `allowVolumeExpansion` set to `true`.

### Expand RBD PVCs

#### Expand RBD Filesystem PVC

pvc.yaml

```yaml
apiVersion: v1
items:
- apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    annotations:
      pv.kubernetes.io/bind-completed: "yes"
      pv.kubernetes.io/bound-by-controller: "yes"
      volume.beta.kubernetes.io/storage-provisioner: rbd.csi.ceph.com
    creationTimestamp: "2019-12-19T05:44:45Z"
    finalizers:
    - kubernetes.io/pvc-protection
    name: rbd-pvc
    namespace: default
    resourceVersion: "3557"
    selfLink: /api/v1/namespaces/default/persistentvolumeclaims/rbd-pvc
    uid: efe688d6-a420-4041-900e-c5e19fd73ebf
  spec:
    accessModes:
    - ReadWriteOnce
    resources:
      requests:
        storage: 1Gi
    storageClassName: csi-rbd-sc
    volumeMode: Filesystem
    volumeName: pvc-efe688d6-a420-4041-900e-c5e19fd73ebf
  status:
    accessModes:
    - ReadWriteOnce
    capacity:
      storage: 1Gi
    phase: Bound
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```

- mounted Filesystem size in pod using this PVC

```bash
$ kubectl exec -it csi-rbd-demo-pod sh
sh-4.4# df -h /var/lib/www/html
Filesystem      Size  Used Avail Use% Mounted on
/dev/rbd0       976M  2.6M  958M   1% /var/lib/www/html
```

- Now expand the PVC by editing the PVC (pvc.spec.resource.requests.storage)

```bash
kubectl edit pvc rbd-pvc
```

Check PVC status after editing the pvc storage

```yaml
apiVersion: v1
items:
- apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    annotations:
      pv.kubernetes.io/bind-completed: "yes"
      pv.kubernetes.io/bound-by-controller: "yes"
      volume.beta.kubernetes.io/storage-provisioner: rbd.csi.ceph.com
    creationTimestamp: "2019-12-19T05:44:45Z"
    finalizers:
    - kubernetes.io/pvc-protection
    name: rbd-pvc
    namespace: default
    resourceVersion: "4773"
    selfLink: /api/v1/namespaces/default/persistentvolumeclaims/rbd-pvc
    uid: efe688d6-a420-4041-900e-c5e19fd73ebf
  spec:
    accessModes:
    - ReadWriteOnce
    resources:
      requests:
        storage: 10Gi
    storageClassName: csi-rbd-sc
    volumeMode: Filesystem
    volumeName: pvc-efe688d6-a420-4041-900e-c5e19fd73ebf
  status:
    accessModes:
    - ReadWriteOnce
    capacity:
      storage: 1Gi
    conditions:
    - lastProbeTime: null
      lastTransitionTime: "2019-12-19T05:49:39Z"
      message: Waiting for user to (re-)start a pod to finish file system resize of
        volume on node.
      status: "True"
      type: FileSystemResizePending
    phase: Bound
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```

Now you can see the PVC status as `FileSystemResizePending`, once the kubelet
calls the NodeExpandVolume to expand the PVC on node, the `status conditions`
and `status` will be updated

```bash
$ kubectl get pvc
NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
rbd-pvc   Bound    pvc-efe688d6-a420-4041-900e-c5e19fd73ebf   10Gi       RWO            csi-rbd-sc   7m6s
```

- Now let us check the directory size inside the pod where PVC is mounted

```bash
$ kubectl exec -it csi-rbd-demo-pod sh
sh-4.4# df -h /var/lib/www/html
Filesystem      Size  Used Avail Use% Mounted on
/dev/rbd0       9.9G  4.5M  9.8G   1% /var/lib/www/html
```

now you can see the size of `/var/lib/www/html` is updated from 976M to 9.9G

#### Expand RBD Block PVC

```bash
$ kubectl get pvc raw-block-pvc -o yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  annotations:
    pv.kubernetes.io/bind-completed: "yes"
    pv.kubernetes.io/bound-by-controller: "yes"
    volume.beta.kubernetes.io/storage-provisioner: rbd.csi.ceph.com
  creationTimestamp: "2019-12-19T05:56:02Z"
  finalizers:
  - kubernetes.io/pvc-protection
  name: raw-block-pvc
  namespace: default
  resourceVersion: "6370"
  selfLink: /api/v1/namespaces/default/persistentvolumeclaims/raw-block-pvc
  uid: 54885275-7ca9-4b89-8e7e-c99f375d1174
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-rbd-sc
  volumeMode: Block
  volumeName: pvc-54885275-7ca9-4b89-8e7e-c99f375d1174
status:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 1Gi
  phase: Bound
```

- Device size in pod using this PVC

```bash
$ kubectl exec -it pod-with-raw-block-volume sh
sh-4.4# blockdev --getsize64 /dev/xvda
1073741824
```

rbd Block PVC is mounted at `/dev/xvda` of the pod, and the size is `1073741824`
bytes which is equal to `1Gib`

- Now expand the PVC

 To expand PVC, change `(pvc.spec.resource.requests.storage)` to the new size
 which should be greater than the current size.

```bash
kubectl edit pvc raw-block-pvc
```

Check PVC status after editing the pvc storage

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  annotations:
    pv.kubernetes.io/bind-completed: "yes"
    pv.kubernetes.io/bound-by-controller: "yes"
    volume.beta.kubernetes.io/storage-provisioner: rbd.csi.ceph.com
  creationTimestamp: "2019-12-19T05:56:02Z"
  finalizers:
  - kubernetes.io/pvc-protection
  name: raw-block-pvc
  namespace: default
  resourceVersion: "7923"
  selfLink: /api/v1/namespaces/default/persistentvolumeclaims/raw-block-pvc
  uid: 54885275-7ca9-4b89-8e7e-c99f375d1174
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: csi-rbd-sc
  volumeMode: Block
  volumeName: pvc-54885275-7ca9-4b89-8e7e-c99f375d1174
status:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 1Gi
  conditions:
  - lastProbeTime: null
    lastTransitionTime: "2019-12-19T06:02:15Z"
    message: Waiting for user to (re-)start a pod to finish file system resize of
      volume on node.
    status: "True"
    type: FileSystemResizePending
  phase: Bound
```

Now you can see the PVC stats as `FileSystemResizePending`, once the kubelet calls
the NodeExpandVolume to expand the PVC on node, the status conditions will be updated
and `status.capacity.storage` will be updated.

```bash
$ kubectl get pvc
NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
raw-block-pvc   Bound    pvc-efe688d6-a420-4041-900e-c5e19fd73ebf   10Gi       RWO            csi-rbd-sc   7m6s
```

Device size in pod using this PVC

```bash
$ kubectl exec -it pod-with-raw-block-volume sh
sh-4.4# blockdev --getsize64 /dev/xvda
10737418240
```

rbd Block PVC is mounted at `/dev/xvda` of the pod, and the size is
`10737418240` bytes which is equal to `10Gib`

### Expand CephFS PVC

#### Expand CephFS Filesystem PVC

pvc.yaml

```yaml
apiVersion: v1
items:
- apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    annotations:
      pv.kubernetes.io/bind-completed: "yes"
      pv.kubernetes.io/bound-by-controller: "yes"
      volume.beta.kubernetes.io/storage-provisioner: cephfs.csi.ceph.com
    creationTimestamp: "2020-01-17T07:55:11Z"
    finalizers:
    - kubernetes.io/pvc-protection
    name: csi-cephfs-pvc
    namespace: default
    resourceVersion: "5955"
    selfLink: /api/v1/namespaces/default/persistentvolumeclaims/csi-cephfs-pvc
    uid: b84d07c9-ea67-40b4-96b9-4a79669b1ccc
  spec:
    accessModes:
    - ReadWriteMany
    resources:
      requests:
        storage: 5Gi
    storageClassName: csi-cephfs-sc
    volumeMode: Filesystem
    volumeName: pvc-b84d07c9-ea67-40b4-96b9-4a79669b1ccc
  status:
    accessModes:
    - ReadWriteMany
    capacity:
      storage: 5Gi
    phase: Bound
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```

- mounted Filesystem size in pod using this PVC

```bash
$ kubectl exec -it csi-cephfs-demo-pod sh
sh-4.4# df -h /var/lib/www
Filesystem                                                                     Size  Used Avail Use% Mounted on
10.108.149.216:6789:/volumes/csi/csi-vol-b0a1bc79-38fe-11ea-adb6-1a2797ee96de  5.0G     0  5.0G   0% /var/lib/www
```

- Now expand the PVC by editing the PVC (pvc.spec.resource.requests.storage)

```bash
kubectl edit pvc csi-cephfs-pvc
```

Check PVC status after editing the PVC storage

```yaml
apiVersion: v1
items:
- apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    annotations:
      pv.kubernetes.io/bind-completed: "yes"
      pv.kubernetes.io/bound-by-controller: "yes"
      volume.beta.kubernetes.io/storage-provisioner: cephfs.csi.ceph.com
    creationTimestamp: "2020-01-17T07:55:11Z"
    finalizers:
    - kubernetes.io/pvc-protection
    name: csi-cephfs-pvc
    namespace: default
    resourceVersion: "6902"
    selfLink: /api/v1/namespaces/default/persistentvolumeclaims/csi-cephfs-pvc
    uid: b84d07c9-ea67-40b4-96b9-4a79669b1ccc
  spec:
    accessModes:
    - ReadWriteMany
    resources:
      requests:
        storage: 10Gi
    storageClassName: csi-cephfs-sc
    volumeMode: Filesystem
    volumeName: pvc-b84d07c9-ea67-40b4-96b9-4a79669b1ccc
  status:
    accessModes:
    - ReadWriteMany
    capacity:
      storage: 10Gi
    phase: Bound
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
```

Now you can see the PVC status capacity storage is updated with request size

```bash
$ kubectl get pvc
NAME             STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS    AGE
csi-cephfs-pvc   Bound    pvc-b84d07c9-ea67-40b4-96b9-4a79669b1ccc   10Gi       RWX            csi-cephfs-sc   6m26s
```

- Now let us check the directory size inside the pod where PVC is mounted

```bash
$ kubectl exec -it csi-cephfs-demo-pod sh
sh-4.4#  df -h /var/lib/www
Filesystem                                                                     Size  Used Avail Use% Mounted on
10.108.149.216:6789:/volumes/csi/csi-vol-b0a1bc79-38fe-11ea-adb6-1a2797ee96de   10G     0   10G   0% /var/lib/www
```

now you can see the size of `/var/lib/www` is updated from 5G to 10G
