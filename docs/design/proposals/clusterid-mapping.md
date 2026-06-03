# Design to handle clusterID and poolID for DR

During disaster recovery/migration of a cluster, as part of the failover, the
kubernetes artifacts like deployment, PVC, PV, etc. will be restored to a new
cluster by the admin. Even if the kubernetes objects are restored the
corresponding RBD/CephFS subvolume cannot be retrieved during CSI operations as
the clusterID and poolID are not the same in both clusters. Let's see the
problem in more detail below.

`0001-0009-rook-ceph-0000000000000002-b0285c97-a0ce-11eb-8c66-0242ac110002`

The above is the sample volumeID sent back in response to the CreateVolume
operation and added as a volumeHandle in the PV spec. CO (Kubernetes) uses above
as the identifier for other operations on the volume/PVC.

The VolumeID is encoded as,

```text
0001 -->                              [csi_id_version=1:4byte] + [-:1byte]
0009 -->                              [length of clusterID=1:4byte] + [-:1byte]
rook-ceph -->                         [clusterID:36bytes (MAX)] + [-:1byte]
0000000000000002 -->                  [poolID:16bytes] + [-:1byte]
b0285c97-a0ce-11eb-8c66-0242ac110002 --> [ObjectUUID:36bytes]
Total of constant field lengths, including '-' field separators would hence be,
4+1+4+1+1+16+1+36 = 64
```

When mirroring is enabled volume which is `csi-vol-ObjectUUID` is mirrored to
the other cluster.

> `csi-vol` is const name and over has the option to override it in
> storageclass.

During the Disaster Recovery (failover operation) the PVC and PV will be
recreated on the other cluster. When Ceph-CSI receives the request for
operations like (NodeStage, ExpandVolume, DeleteVolume, etc.) the volumeID is
sent in the request which will help to identify the volume.

```yaml=
apiVersion: v1
kind: ConfigMap
data:
  config.json: |-
    [
      {
       "clusterID": "rook-ceph",
       "rbd": {
          "radosNamespace": "<rados-namespace>",
       },
       "monitors": [
         "192.168.39.82:6789"
       ],
       "cephFS": {
         "subvolumeGroup": "<subvolumegroup for cephfs volumes>"
       }
      },
      {
       "clusterID": "fs-id",
       "rbd": {
          "radosNamespace": "<rados-namespace>",
       },
       "monitors": [
         "192.168.39.83:6789"
       ],
       "cephFS": {
         "subvolumeGroup": "<subvolumegroup for cephfs volumes>"
       }
      }
    ]
metadata:
  name: ceph-csi-config
```

During CSI/Replication operations, Ceph-CSI will decode the volumeID and gets
the monitor configuration from the configmap and by the poolID will get the pool
Name and retrieves the OMAP data stored in the rados OMAP and finally check the
volume is present in the pool.

## Problems with volumeID Replication

* The clusterID can be different
   * as the clusterID is the namespace where rook is deployed, the Rook might
    be deployed in the different namespace on a secondary cluster
   * In standalone Ceph-CSI the clusterID is fsID and fsID is unique per
    cluster

* The poolID can be different
   * PoolID which is encoded in the volumeID won't remain the same across
    clusters

To solve this problem we need to have a new mapping between clusterID's and the
poolID's.

Example configmap Need to be created before failover to `site2-storage` from
`site1-storage` and `site3-storage`.

```yaml=
apiVersion: v1
kind: ConfigMap
data:
  cluster-mapping.json: |-
  [{
    "clusterIDMapping": {
    "site1-storage" (clusterID on site1): "site2-storage" (clusterID on site2)
   },
    "RBDPoolIDMapping": [{
    "1" (poolID on site1): "2" (poolID on site2),
    "11": "12"
   }],
    "CephFSFscIDMapping": [{
    "13" (FscID on site1): "34" (FscID on site2),
    "3": "4"
   }]
  }, {
   "clusterIDMapping": {
   "site3-storage"  (clusterID on site3): "site2-storage" (clusterID on site2)
   },
   "RBDPoolIDMapping": [{
   "5" (poolID on site3): "2" (poolID on site2),
   "16": "12"
   }],
   "CephFSFscIDMapping": [{
   "3"(FscID on site3): "34" (FscID on site2),
   "4": "4"
   }]
 }]
metadata:
  name: ceph-csi-config
```

**Note:-** the configmap will be mounted as a volume to the CSI (provisioner and
node plugin) pods.

The above configmap will get created as it is or updated (if new Pools are
created on the existing cluster) with new entries when the admin choose to
failover/failback the cluster.

Whenever Ceph-CSI receives a CSI/Replication request it will first decode the
volumeHandle and try to get the required OMAP details. If it is not able to
retrieve the poolID or clusterID details from the decoded volumeHandle, Ceph-CSI
will check for the clusterID and PoolID mapping.

If the old volumeID
`0001-00013-site1-storage-0000000000000001-b0285c97-a0ce-11eb-8c66-0242ac110002`
contains the `site1-storage` as the clusterID, now Ceph-CSI will look for the
corresponding clusterID `site2-storage` from the above configmap. If the
clusterID mapping is found now Ceph-CSI will look for the poolID mapping ie
mapping between `1` and `2`.

Example:- pool with the same name exists on both the clusters with different IDs
Replicapool with ID `1` on site1 and Replicapool with ID `2` on site2.

After getting the required mapping Ceph-CSI has the required information to get
more details from the rados OMAP. If we have multiple clusterID mapping it will
loop through all the mapping and checks the corresponding pool to get the OMAP
data. If the clusterID mapping does not exist Ceph-CSI will return a `Not Found`
error message to the caller.

After failover to the cluster `site2-storage`, the admin might have created new
PVCs on the primary cluster `site2-storage`. Later after recovering the
cluster `site1-storage`, the admin might choose to failback from
`site2-storage` to `site1-storage`. Now admin needs to copy all the newly
created kubernetes artifacts to the failback cluster. For clusterID mapping, the
admin needs to copy the above-created configmap `ceph-clusterid-mapping` to the
failback cluster. When Ceph-CSI receives a CSI/Replication request for the
volumes created on the `site2-storage` it will decode the volumeID and retrieves
the clusterID ie `site2-storage`. In the above configmap
`ceph-clusterid-mapping` the `site2-storage` is the value and `site1-storage`
is the key in the `clusterIDMapping` entry.

Ceph-CSI will check both `key` and `value` to check the clusterID mapping. If it
is found in `key` it will consider `value` as the corresponding mapping, if it
is found in `value` place it will treat `key` as the corresponding mapping and
retrieves all the poolID details of the cluster.

This mapping on the remote cluster is only required when we are doing a failover
operation from the primary cluster to a remote cluster. The existing volumes
that are created on the remote cluster does not require any mapping as the
volumeHandle already contains the required information about the local cluster (
clusterID, poolID etc).
