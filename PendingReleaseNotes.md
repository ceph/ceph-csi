# v3.18 Pending Release Notes

## Breaking changes

## Deprecations

1. Helm chart deployments are no longer validated by the e2e test suite. Helm
   charts will be deprecated in v3.18 in favor of the
   [Ceph-CSI Operator](https://ceph.github.io/ceph-csi-operator) and to be
   removed in v3.19

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
