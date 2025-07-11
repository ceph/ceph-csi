# UserID Mapping

Ceph-CSI connects to the Ceph cluster with the credentials of a Ceph user account.
The account is used to communicate with the MONs, OSDs and other services to
attach RBD-images and mount CephFS volumes. Currently, there is no mechanism to track
and expose which Ceph user account was used to map a volume on a node. Going forward,
Ceph user account and node mapping will be referred to as nodeID:userID mapping for
simplicity.

## Problem

Without a mechanism to track nodeID:userID mappings per volume, it becomes
challenging to:

* Monitor which userIDs are actively being used across the cluster
* Make informed decisions about key rotation, allowing safe cleanup of old keys

## Important Note

> [!WARNING]
> This feature is only effective after upgrading to a Ceph-CSI version that
> supports this functionality and rebooting all nodes in the cluster.
> Without rebooting all nodes, for volumes that do not have this metadata,
> it could mean either:
> * The volume is not currently mounted
> * The volume was mounted with an older Ceph-CSI version that doesn't support
> this feature

## Proposed Solution

The solution involves tracking userID information by storing it in the metadata
of RBD images and CephFS subvolumes using the key:
`.[rbd|cephfs].csi.ceph.com/userID/<NodeId>:<userID>`

## Implementation Details

### NodeStageVolume()

* Set the metadata on the volume:

```
.[rbd|cephfs].csi.ceph.com/userID/<NodeId>:<userID>
```

### ControllerUnpublishVolume()

* Remove the metadata from the volume:

```
.[rbd|cephfs].csi.ceph.com/userID/<NodeId>:<userID>
```

> Note: Refer to this [section](./non-graceful-node-shutdown.md#workaround-for-older-pvs)
for more details on secrets requirement for this operation.

## Future Usage

The stored userID mapping information enables several valuable use cases:

* **Metrics Generation**: Track and report on userID usage patterns across the
  cluster

* **Alert Management**: Generate alerts when old userIDs are detected being used
  by specific volumes on specific nodes

* **Key Rotation**: Automate key cleanup by identifying when old userIDs are no
  longer in use on any volumes, allowing safe deletion of associated old keyrings

These capabilities can be implemented by listing all RBD images or CephFS
subvolumes and examining their `.[rbd|cephfs].csi.ceph.com/userID/<NodeId>:<userID>`
metadata.
Depending on the scope of the associated Ceph user account, the listing
of RBD images and CephFS subvolumes may be either confined to one, many or all
RADOS-Namespaces and subvolume groups respectively. Similar situation may apply
if the use case spans multiple RBD pools and Ceph Filesystems.

## Alternatives

The following alternatives were considered but were not chosen for reasons
listed below:

* Extracting Ceph user account from `/sys/bus/rbd/devices/<dev id>/config_info` file:
   * This approach is not feasible as each node has to be checked individually
     for the presence of the file and its content, which is quite tedious.
* Extracting Ceph user account from MDS session ls:
   * The Ceph user is not visible in `session ls` yet. Refer to
     [tracker](https://tracker.ceph.com/issues/71937).
   * Even after the fix, the information present may only give an incomplete view
     of what's in use.
* Using single Ceph user account per node:
   * This approach is not feasible since Kubernetes CSI does not differentiate credentials
     per node.
