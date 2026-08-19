# v3.18 Pending Release Notes

## Breaking changes

## Features

1. Added `GetReplicationDestinationInfo` RPC to map source volume/volume
   group IDs to destination IDs across mirrored clusters. This enables DR
   orchestrators to discover the correct destination volume IDs when pools
   have different IDs across clusters. The RPC supports:
    - Volume replication: Maps source volume ID to destination volume ID
    - Volume group replication: Maps source group ID and all member volume
      IDs to their destination IDs
    - Pool name-based mapping via `replicationDestination` ConfigMap
      configuration
    - Backward compatibility with existing cluster-mapping.json via
      ClientProfileMapping integration

1. NVMe-oF: moved the `ListListeners` query from `CreateVolume` to
   `ControllerPublishVolume`. Listeners are now fetched live at publish
   time and passed through the publish context to the node, so the list
   stays accurate across gateway scale-up/scale-down events
   (restart the test-pod is required).
   The node server reads listeners from the publish context instead of the volume
   context, failback to volume context for backward compatibility when node-server
   is updated, but provisioner is not.

## NOTE
