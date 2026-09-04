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

## NOTE

## Deprecations

- The `netNamespaceFilePath` configuration option is now deprecated and will be
  removed in a future release. Users should migrate to using host networking for
  CSI plugin pods instead. When this feature is detected, a deprecation warning
  will be logged at the WARNING level.
   - `hostPID` was changed from `true` to `false` in all static manifests
   - Static manifest users with `netNamespaceFilePath` must manually set
     `hostPID: true`
   - OpenShift users must also keep `allowHostPID: true` in their SCC (if they
     were using it)
