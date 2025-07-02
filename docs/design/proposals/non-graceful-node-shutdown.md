# Non graceful node shutdown

In Kubernetes, when a node becomes dysfunctional or is intentionally drained,
taints `node.kubernetes.io/out-of-service` may be applied to mark the node
as unavailable _refer [k8s doc](https://kubernetes.io/docs/concepts/cluster-administration/node-shutdown/#non-graceful-node-shutdown)_.
This results in the forceful deletion of pods scheduled on the node and the
cleanup of associated VolumeAttachment objects. However, this forceful cleanup
sequence bypasses key CSI lifecycle calls most notably, the
`NodeUnpublishVolume` and `NodeUnstageVolume` entry points are not invoked
prior to `ControllerUnpublishVolume` call.

## Problem

When `ControllerUnpublishVolume` is called without the node first having
cleaned up the volume (via `NodeUnpublishVolume` and `NodeUnstageVolume`),
the CSI driver has no opportunity to revoke the node's access to the volume.
The node may still hold active mounts, open file handles, or client sessions.
This can lead to data corruption as applications may still be running on the
broken node with active client sessions, even though the node is marked as out
of service.

## Important Note

> ⚠️ **WARNING**: When a node becomes out of service, its mounts and device
> mappings will persist until the node undergoes a complete power lifecycle
> (includes shutdown and startup). To prevent data inconsistency or corruption,
> administrators **MUST NOT** remove the `node.kubernetes.io/out-of-service`
> taint until the node has successfully completed a full power cycle. Removing
> the taint prematurely may leave stale device states, active client sessions,
> or lingering mounts, which can lead to serious data integrity issues.

## Proposed solution

To ensure safe volume reuse and prevent stale client access during node
disruptions, the proposed solution is to track the client address during
the `NodeStageVolume()` operation and store it in the image or subvolume
metadata under the key: `.[rbd|cephfs].csi.ceph.com/clientAddress/<NodeId>`.

> Note: This metadata should not be copied when creating clones, snapshots,
> or mirror images of the volume.

This stored address can then be used:

* In `ControllerUnpublishVolume()` to blocklist or evict the client if the node
  has the `out-of-service` taints, ensuring it no longer has access to the
  volume.

* In `ControllerPublishVolume()` to remove the client from the blocklist,
  allowing the volume to be accessed again when the node recovers.

## Implementation Details

### ControllerPublishVolume()

* Check if the metadata key `.[rbd|cephfs].csi.ceph.com/clientAddress/<NodeId>`
  exists (`NodeId` from the request)

* If it exists:
   * Remove the client from the blocklist:

    ```
    ceph osd blocklist rm <clientAddress>
    ```

   * Remove the metadata key after removing from blocklist

    ```
    # RBD
    rbd image-meta remove <pool>/<namespace>/<image> .rbd.csi.ceph.com/clientAddress/<NodeId>

    # CephFS
    ceph fs subvolume metadata rm <vol_name> <sub_name> .cephfs.csi.ceph.com/clientAddress/<NodeId> [<group_name>]
    ```

### NodeStageVolume()

* **Applicable to**: RBD and CephFS

* Before volume staging:
   * Retrieve the client address:

   * Always set it in the image/subvolume metadata:
    (`NodeId` from the plugin container argument `--nodeid`)

    ```
    .[rbd|cephfs].csi.ceph.com/clientAddress/<NodeId>: <clientAddress>
    ```

   * Continue with Volume staging

### ControllerUnpublishVolume()

* Retrieve the Node object using the provided `NodeId` from the request.

* If the node has the `out-of-service` taint:
   * Check for the metadata key:

    ```
    .[rbd|cephfs].csi.ceph.com/clientAddress/<NodeId>
    ```

   * If present, proceed to revoke client access:
      * **For RBD**:

      ```
      # Add client to blocklist with extended duration to prevent stale client access.
      # The blocklist must persist until we can confirm the node has gone through
      # a complete power cycle, as premature expiration could lead to data corruption
      ceph osd blocklist add <clientAddress> 157784760
      ```

      * **For CephFS**:
         * List active clients and match against `clientAddress` to get the `clientId`.
            * Evict the client and blocklist the address:

        ```
        ceph tell mds.* client evict id=<clientId>
        # Add client to blocklist with extended duration to prevent stale client access.
        # The blocklist must persist until we can confirm the node has gone through
        # a complete power cycle, as premature expiration could lead to data corruption
        ceph osd blocklist add <clientAddress> 157784760
        ```

* If the node doesn't have the `out-of-service` taint:
   * Remove the `.[rbd|cephfs].csi.ceph.com/clientAddress/<NodeId>` metadata
      key

    ```
    # RBD
    rbd image-meta remove <pool>/<namespace>/<image> .rbd.csi.ceph.com/clientAddress/<NodeId>

    # CephFS
    ceph fs subvolume metadata rm <vol_name> <sub_name> .cephfs.csi.ceph.com/clientAddress/<NodeId> [<group_name>]
    ```

### Workaround for older PVs

**Problem**: `ControllerPublishVolume()`/`ControllerUnpublishVolume()` requires
controller-publish-secret. The secret needed to access the Ceph cluster may be
missing for older PVs if the following parameters were not specified in their
corresponding StorageClass at the time of provisioning:

```
csi.storage.k8s.io/controller-publish-secret-name
csi.storage.k8s.io/controller-publish-secret-namespace
```

**Solution 1**: Fallback to default secrets if available in csi-config-map
ConfigMap.

```yaml
[
  {
    "clusterID": "my-cluster",
    "rbd": {
      ...
      "controllerPublishSecretRef": {
        "secretName": "publish-secret-name",
        "secretNamespace": "publish-secret-namespace"
      },
      ...
    },
    "cephfs": {
      ...
      "controllerPublishSecretRef": {
        "secretName": "publish-secret-name",
        "secretNamespace": "publish-secret-namespace"
      },
      ...
    }
  }
]
```
