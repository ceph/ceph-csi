# Stale Resource Cleanup

If the PVC is created with storage class which is having the `reclaimPolicy`
as `Retain` will not delete the PV object, backend omap metadata and backend image.
Manual deletion of PV will result in stale omap keys, values,
cephFS subvolume and rbd image.
It is required to cleanup metadata and image separately.

New CSI journal request-name mappings are stored in sharded `csiDirectory`
objects. For volume journals this means new entries are written to
`csi.volumes.<instance_id>.<shard>` where `<shard>` is in the range `0..10`.
During upgrades, older mappings may still exist in the legacy unsharded
`csi.volumes.<instance_id>` object. Manual cleanup should check the shard oids
first and also clean the legacy oid when applicable.

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

a. get omapkey (suffix of `csi.volumes.<instance_id>` is value used for the CLI
   option [--instanceid](rbd/deploy.md#configuration) in the provisioner
   deployment). New entries are stored in one of the 11 shard oids
   `csi.volumes.<instance_id>.0` ... `csi.volumes.<instance_id>.10`, while
   upgraded clusters may still have older entries in the legacy unsharded
   `csi.volumes.<instance_id>` oid.

  ```
  pv_name="pvc-bc537af8-67fc-4963-99c4-f40b3401686a"
  instance_id="default"
  key="csi.volume.${pv_name}"
  csi_directory_oid=""

  for shard in $(seq 0 10); do
    oid="csi.volumes.${instance_id}.${shard}"
    if rados listomapkeys "${oid}" -p pool_name 2>/dev/null | grep -Fx "${key}" >/dev/null; then
      csi_directory_oid="${oid}"
      break
    fi
  done

  if [ -z "${csi_directory_oid}" ]; then
    legacy_oid="csi.volumes.${instance_id}"
    if rados listomapkeys "${legacy_oid}" -p pool_name 2>/dev/null | grep -Fx "${key}" >/dev/null; then
      csi_directory_oid="${legacy_oid}"
    fi
  fi

  echo "${csi_directory_oid}"
  ```

  ```bash
  $ echo "${csi_directory_oid}"
  csi.volumes.default.3
  ```

b. get omapval

  ```
  rados getomapval csi_directory_oid omapkey -p pool_name
  ```

  ```bash
  $ rados getomapval "${csi_directory_oid}" "${key}" -p kube_csi
  value (36 bytes) :
  00000000  64 64 32 34 37 33 64 30  2d 36 61 38 63 2d 31 31  |dd2473d0-6a8c-11|
  00000010  65 61 2d 39 31 31 33 2d  30 61 64 35 39 64 39 39  |ea-9113-0ad59d99|
  00000020  35 63 65 37                                       |5ce7|
  00000024
  ```

### 3. Delete the RBD image or CephFS subvolume

a. remove rbd image(csi-vol-omapval, the prefix csi-vol is value of [volumeNamePrefix](rbd/deploy.md#configuration))

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

b. delete omapkey from the found oid. If the key was found in a shard oid, also
   try to delete it from the legacy unsharded oid to cleanup upgraded clusters.

  ```bash
  rados rmomapkey "${csi_directory_oid}" "${key}" -p pool_name

  legacy_oid="csi.volumes.${instance_id}"
  if [ "${csi_directory_oid}" != "${legacy_oid}" ]; then
    rados rmomapkey "${legacy_oid}" "${key}" -p pool_name || true
  fi
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
