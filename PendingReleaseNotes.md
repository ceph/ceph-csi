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
1. Added a `friendlyExportNames` StorageClass parameter for the NFS driver.
   When set to `"true"` and the external-provisioner runs with
   `--extra-create-metadata=true`, NFS-exports are named
   `<namespace>/<pvc-name>` instead of the generated volume ID. Off by
   default, and gated behind the parameter rather than the provisioner flag
   alone, since CephFS already reads that same metadata unconditionally for
   per-tenant KMS scoping.  Existing StorageClasses keep today's export
   names unless they opt in explicitly. Unlike the generated volume ID,
   `<namespace>/<pvc-name>` is not guaranteed unique over time (e.g. a PVC
   recreated under the same name before its old export was cleaned up);
   `CreateVolume` now fails with `AlreadyExists` rather than silently
   reusing another volume's export if the name is already claimed by a
   different subvolume.

## NOTE

- The RADOS lock that serializes fscrypt setup for encrypted CephFS volumes
  is now taken in the CephFS RADOS namespace instead of the default
  namespace of the metadata pool. This applies to every deployment with
  encrypted volumes. `cephFS.radosNamespace` defaults to `csi`, so the lock
  moves to this namespace even when the option was never directly
  configured. During a rolling nodeplugin upgrade the locks in the default
  namespace and `cephFS.radosNamespace` are taken, so that pods which have
  not been upgraded yet stay serialized against upgraded ones.
