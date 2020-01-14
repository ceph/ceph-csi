# Resize RBD PVCs

For filesystem resize to be supported for your kubernetes
cluster, the kubernetes version running in your cluster
should be >= v1.15 and for block volume resize support
the kubernetes version should be >=1.16. Also, `ExpandCSIVolumes`
feature gate has to be enabled for the volume resize
functionality to work.

- [Resize RBD PVCs](#resize-rbd-pvcs)
  - [Filesystem resize on RBD filesystem volume mode PVCs](#filesystem-resize-on-rbd-filesystem-volume-mode-pvcs)
  - [Resize Block PVC](#resize-block-pvc)

## Filesystem resize on RBD filesystem volume mode PVCs

pvc.yaml

```
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

```
[$]kubectl exec -it csi-rbd-demo-pod sh
# df -h /var/lib/www/html
Filesystem      Size  Used Avail Use% Mounted on
/dev/rbd0       976M  2.6M  958M   1% /var/lib/www/html
#

```

- Now resize the PVC by editing the PVC (pvc.spec.resource.requests.storage)

```bash
[$]kubectl edit pvc rbd-pvc
```

Check PVC status after editing the pvc storage

```console
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

Now you can see the pvc status as `FileSystemResizePending`, once the kubelet
calls the NodeExpandVolume to resize the PVC on node, the `status conditions`
and `status` will be updated

```bash
[$]kubectl get pvc
NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
rbd-pvc   Bound    pvc-efe688d6-a420-4041-900e-c5e19fd73ebf   10Gi       RWO            csi-rbd-sc   7m6s

```

- Now let us check the directory size inside the pod where PVC is mounted

```bash
[$]kubectl exec -it csi-rbd-demo-pod sh
# df -h /var/lib/www/html
Filesystem      Size  Used Avail Use% Mounted on
/dev/rbd0       9.9G  4.5M  9.8G   1% /var/lib/www/html
```

now you can see the size of `/var/lib/www/html` is updated from 976M to 9.9G

## Resize Block PVC

```bash
[$]kubectl get pvc raw-block-pvc -o yaml
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
[$]kubectl exec -it pod-with-raw-block-volume sh
sh-4.4# blockdev --getsize64 /dev/xvda
1073741824
```

rbd Block PVC is mounted at `/dev/xvda` of the pod, and the size is `1073741824`
bytes which is equal to `1Gib`

- Now resize the PVC

 To resize PVC, change `(pvc.spec.resource.requests.storage)` to the new size
 which should be greater than the current size.

```bash
[$]kubectl edit pvc raw-block-pvc

```

Check PVC status after editing the pvc storage

```console
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

Now you can see the pvc stats as FileSystemResizePending, once the kubelet calls
the NodeExpandVolume to resize the PVC on node, the status conditions will be updated
and status.capacity.storage will be updated.

```bash

[$]kubectl get pvc
NAME      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
raw-block-pvc   Bound    pvc-efe688d6-a420-4041-900e-c5e19fd73ebf   10Gi       RWO            csi-rbd-sc   7m6s

```

Device size in pod using this PVC

```bash

[$]kubectl exec -it pod-with-raw-block-volume sh
sh-4.4# blockdev --getsize64 /dev/xvda
10737418240

```

rbd Block PVC is mounted at `/dev/xvda` of the pod, and the size is
`10737418240` bytes which is equal to `10Gib`
