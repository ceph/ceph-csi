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

- The RADOS lock that serializes fscrypt setup for encrypted CephFS volumes
  is now taken in the CephFS RADOS namespace instead of the default
  namespace of the metadata pool. This applies to every deployment with
  encrypted volumes. `cephFS.radosNamespace` defaults to `csi`, so the lock
  moves to this namespace even when the option was never directly
  configured.
