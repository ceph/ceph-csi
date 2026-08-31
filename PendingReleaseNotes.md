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
1. CephFS: fscrypt file encryption now works with a KMIP KMS when
   `USE_CRYPTO_RPC` is set to `"false"`. The key material of the managed
   symmetric key is fetched with the KMIP `Get` operation and used as the
   fscrypt passphrase. RBD with `encryptionType: file` keeps rejecting a
   KMIP KMS until that combination has been tested.

## NOTE
