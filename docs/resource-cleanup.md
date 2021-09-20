# Stale Resource Cleanup

If the PVC is created with storage class which is having the `reclaimPolicy`
as `Retain` will not delete the PV object, backend omap metadata and backend image.
Manual deletion of PV will result in stale omap keys, values,
cephFS subvolume and rbd image.
It is required to cleanup metadata and image separately.

## Steps

### 1. Get PV name from PVC

a. get pv_name

  ```
  kubectl get pvc pvc_name -n namespace -owide
  ```

  ```bash
  $ kubectl get pvc mysql-pvc -owide -n prometheus
  NAME        STATUS   VOLUME
  mysql-pvc   Bound    pvc-bc537af8-67fc-4963-99c4-f40b3401686a

  CAPACITY   ACCESS MODES   STORAGECLASS   AGE   VOLUMEMODE
  20Gi       RWO            csi-rbd        14d   Filesystem
  ```

### 2. Get omap key/value

a. get omapkey (suffix of csi.volumes.default is value used for the CLI option
   [--instanceid](deploy-rbd.md#configuration) in the provisioner deployment.)

  ```
  rados listomapkeys csi.volumes.default -p pool_name | grep pv_name
  ```

  ```bash
  $ rados listomapkeys csi.volumes.default -p kube_csi | grep pvc-bc537af8-67fc-4963-99c4-f40b3401686a
  csi.volume.pvc-bc537af8-67fc-4963-99c4-f40b3401686a
  ```

b. get omapval

  ```
  rados getomapval csi.volumes.default omapkey -p pool_name
  ```

  ```bash
  $ rados getomapval csi.volumes.default csi.volume.pvc-bc537af8-67fc-4963-99c4-f40b3401686a -p kube_csi
  value (36 bytes) :
  00000000  64 64 32 34 37 33 64 30  2d 36 61 38 63 2d 31 31  |dd2473d0-6a8c-11|
  00000010  65 61 2d 39 31 31 33 2d  30 61 64 35 39 64 39 39  |ea-9113-0ad59d99|
  00000020  35 63 65 37                                       |5ce7|
  00000024
  ```

### 3. Delete the RBD image or CephFS subvolume

a. remove rbd image(csi-vol-omapval, the prefix csi-vol is value of [volumeNamePrefix](deploy-rbd.md#configuration))

  ```
  rbd remove rbd_image_name -p pool_name
  ```

  ```bash
  $ rbd remove csi-vol-dd2473d0-6a8c-11ea-9113-0ad59d995ce7 -p kube_csi
  Removing image: 100% complete...done.
  ```

b. remove cephFS subvolume(csi-vol-omapval)

  ```
  ceph fs subvolume rm volume_name subvolume_name group_name
  ```

  ```bash
  ceph fs subvolume rm  cephfs csi-vol-340daf84-5e8f-11ea-8560-6e87b41d7a6e csi
  ```

### 4. Delete omap object and omapkey

a. delete omap object

  ```
  rados rm csi.volume.omapval -p pool_name
  ```

  ```bash
  rados rm csi.volume.dd2473d0-6a8c-11ea-9113-0ad59d995ce7 -p kube_csi
  ```

b. delete omapkey

  ```
  rados rmomapkey csi.volumes.default csi.volume.omapkey -p pool_name
  ```

  ```bash
  rados rmomapkey csi.volumes.default csi.volume.pvc-bc537af8-67fc-4963-99c4-f40b3401686a -p kube_csi
  ```

### 5. Delete PV

a. delete pv

  ```
  kubectl delete pv pv_name -n namespace
  ```

  ```bash
  $ kubectl delete pv pvc-bc537af8-67fc-4963-99c4-f40b3401686a -n prometheus
  persistentvolume "pvc-bc537af8-67fc-4963-99c4-f40b3401686a" deleted
  ```
