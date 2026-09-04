# v3.18 Pending Release Notes

## Breaking changes

- Journal metadata now uses sharded `csiDirectory` objects for new CSI name
  mappings. This is a forward-only upgrade for journal metadata: newer
  ceph-csi versions can read legacy `csiDirectory` entries, but older versions
  only read the legacy unsharded object and cannot see mappings written to
  shard oids after the upgrade. Downgrading after new entries have been written
  in the upgraded version can break idempotent lookups for those entries.

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
