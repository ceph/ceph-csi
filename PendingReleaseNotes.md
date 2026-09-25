# v3.19 Pending Release Notes

## Breaking changes

1. RBD and NVMe-oF: ext4 filesystems are now mounted with `errors=remount-ro`
   by default, instead of the ext4 kernel default of `errors=continue`. When
   ext4 detects an error in the filesystem, for example a directory block that
   fails its checksum, the filesystem is remounted read-only so that the
   workload does not keep writing to a damaged filesystem. The default
   is used the next time a volume is staged on a node, there is no need to
   update the `mountOptions` of the StorageClass or of existing PVs. Workloads
   that need the previous behavior can set `errors=continue` in the
   `mountOptions` of the StorageClass. Existing PVs need the option added to
   their `spec.mountOptions` as well.

## Features

1. Added an optional `TLS_MIN_VERSION` setting to the `kmip` KMS provider,
   which sets the minimum TLS version the provider may use. That minimum was
   previously hardcoded to TLS 1.2, and TLS 1.3 was used whenever the KMIP
   server offered it, with no way to require it. The setting defaults to
   `"1.2"` and keeps that behavior. Setting it to `"1.3"` requires TLS 1.3, so
   that a deployment held to a TLS 1.3 baseline fails to connect to a KMIP
   server that only offers TLS 1.2 instead of silently continuing over
   TLS 1.2. The negotiated TLS version and cipher suite are now logged for
   each KMIP connection.

1. NVMe-oF: moved the `ListListeners` query from `CreateVolume` to
   `ControllerPublishVolume`. Listeners are now fetched live at publish
   time and passed through the publish context to the node, so the list
   stays accurate across gateway scale-up/scale-down events
   (restart the test-pod is required).
   The node server reads listeners from the publish context instead of the volume
   context, failback to volume context for backward compatibility when node-server
   is updated, but provisioner is not.

## NOTE
