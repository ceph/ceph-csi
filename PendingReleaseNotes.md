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
1. Added a `friendlyExportNames` StorageClass parameter for the NFS driver.
   When set to `"true"` and the external-provisioner runs with
   `--extra-create-metadata=true`, NFS-exports are named
   `<namespace>/<pvc-name>` instead of the generated volume ID. Off by
   default, and gated behind the parameter rather than the provisioner flag
   alone, since CephFS already reads that same metadata unconditionally for
   per-tenant KMS scoping.  Existing StorageClasses keep today's export
   names unless they opt in explicitly.

## NOTE
