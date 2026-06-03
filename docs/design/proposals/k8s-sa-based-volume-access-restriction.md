# Kubernetes ServiceAccount Based Volume Access Restriction

## Introduction

This proposal introduces an optional mechanism to restrict volume access based
on the Kubernetes ServiceAccount of the Pod mounting the volume. When
configured, only Pods running with one of the specified
ServiceAccounts are allowed to mount the volume. All
other mount attempts are rejected with a
`PermissionDenied` error.

The restriction is stored as metadata on the backend Ceph object (RBD image
metadata or CephFS subvolume metadata) and is enforced at mount time through
the CSI [`podInfoOnMount`][pod-info-on-mount] mechanism.
This design depends solely on Kubernetes providing the Pod's ServiceAccount
name in the volume context during `NodePublishVolume` calls via the standard
key `csi.storage.k8s.io/serviceAccount.name: {pod.Spec.ServiceAccountName}`.
No other validation is performed on the ServiceAccount name.

[pod-info-on-mount]:
<https://kubernetes-csi.github.io/docs/pod-info.html#pod-info-on-mount-with-csi-driver-object>

## Motivation

Ceph-CSI volumes are accessible to any Pod that has a valid PVC reference and
the necessary RBAC to use the StorageClass. In multi-tenant and data pipeline
environments, this is insufficient. There are scenarios where a volume should
be exclusively accessible to a specific workload identity even when other Pods
in the same namespace can reference the PVC.

### Use Case: Ceph VolSync Plugin Replication Destination PVC Protection

A primary motivator for this feature is the custom
[Ceph VolSync Plugin](https://github.com/RamenDR/ceph-volsync-plugin) that
performs incremental data replication across clusters. In a disaster recovery
or migration workflow:

1. A `ReplicationDestination` controller creates a PVC on the destination
   cluster to receive replicated data.
1. A replication worker Pod, running under a dedicated ServiceAccount (e.g.
   `volsync-worker-sa`), incrementally syncs data from the source cluster into
   this destination PVC.
1. The destination PVC must remain writable only by the replication worker
   until the replication is complete and a failover is triggered.

Without ServiceAccount based restriction, any Pod in the namespace with a
reference to the destination PVC could write to it, potentially corrupting the
replicated data or breaking the incremental sync state. By binding the
destination volume to the replication worker's ServiceAccount, the volume is
protected from unintended writes throughout the replication lifecycle. On
failover, the restriction is removed so the application workload can mount
the volume.

### Other Potential Use Cases

- **Sensitive data volumes**: Restrict access to volumes containing regulated
  data to only the ServiceAccount authorized to process them.
- **Custom usecases**: Similar usecases where a
  workload identity needs exclusive access to a volume
  for data integrity or security reasons.

## Dependency

- The [`podInfoOnMount`][pod-info-on-mount] field must
  be set to `true` in the CSIDriver specification.
  This causes Kubelet to inject Pod information
  (including the ServiceAccount name) into the volume
  context during `NodePublishVolume`. Without this,
  the restriction cannot be enforced. Since this
  parameter is a mutable field in the CSIDriver spec,
  it will be enabled by default going
  forward(cephcsi v3.17.0+).

## Design

### Metadata Keys

Each driver type uses a driver-specific metadata key to
store the allowed ServiceAccount name(s) as a
comma-separated list (e.g. `sa1,sa2,sa3`):

| Driver | Metadata Key | Storage |
|--------|-------------|---------|
| RBD | `.rbd.csi.ceph.com/serviceaccount` | RBD image metadata |
| CephFS | `.cephfs.csi.ceph.com/serviceaccount` | CephFS subvolume metadata |
| NVMe-oF | `.rbd.csi.ceph.com/serviceaccount` | RBD image metadata (via RBD backend) |
| NFS | `.cephfs.csi.ceph.com/serviceaccount` | CephFS subvolume metadata (via CephFS backend) |

One or more ServiceAccounts can be specified per volume,
separated by commas.

### CSI Flow

The restriction is enforced across two CSI RPCs:

1. **ControllerPublishVolume**: The controller reads the ServiceAccount
   metadata from the Ceph backend. If present, it is included in the publish
   context passed to the node.

1. **NodePublishVolume**: The node plugin splits the
   publish context value on commas and checks whether
   the Pod's ServiceAccount (provided by Kubelet via
   `csi.storage.k8s.io/serviceAccount.name` in the
   volume context) matches any entry. A mismatch
   results in a `PermissionDenied` error. If no
   restriction is set, or if `podInfoOnMount` is not
   enabled, the mount is allowed.

### Implementation

A shared validation function `ValidateServiceAccountRestriction` in
`internal/util/validate.go` is called at the beginning of `NodePublishVolume`
in all four drivers (RBD, CephFS, NFS, NVMe-oF), ensuring consistent
enforcement.

Each driver reads the restriction metadata in `ControllerPublishVolume` using
its backend:

- **RBD**: reads via `GetMetadata` in `internal/rbd/controllerserver.go`.
- **CephFS**: reads via `ListMetadata` in
  `internal/cephfs/controllerserver.go`.
- **NVMe-oF**: delegates to the RBD backend and propagates the publish context
  in `internal/nvmeof/controller/controllerserver.go`.
- **NFS**: delegates to the CephFS backend in
  `internal/nfs/controller/controllerserver.go`.

## Setting and Removing the Restriction

The restriction is managed through Ceph CLI commands. Refer to the
"Kubernetes ServiceAccount Based Volume Access" sections in
[RBD deploy.md](../../rbd/deploy.md) and
[CephFS deploy.md](../../cephfs/deploy.md) for usage
instructions and examples.

## Ceph VolSync Plugin Integration Example

1. The replication destination worker sets the
   ServiceAccount restriction on the backing Ceph
   object(RBD image or CephFS subvolume) to the
   replication worker's ServiceAccount
   (e.g.`volsync-worker-sa`) on first use.
1. Only the worker Pod mounts the destination PVC successfully because its
   ServiceAccount matches. Any other Pod attempting to mount the same PVC is
   rejected with `PermissionDenied` during NodePublish call, protecting data
   integrity during incremental sync.
1. On replication destination deletion, the controller spins up a cleanup job
   that removes the ServiceAccount restriction
   metadata, allowing the application workload to
   mount the volume.

## Limitations

- Enforced at CSI mount time only; does not prevent direct access to the
  underlying Ceph storage from outside Kubernetes.
- If `podInfoOnMount` is not enabled, the restriction is silently unenforced.
- Changing the restriction on an already-mounted volume does not affect
  existing mounts. The volume must be unmounted and remounted.
- Managed through Ceph CLI commands, not Kubernetes-native APIs.

## Future Enhancements

- Support restriction based on other attributes (e.g. name, namespace) in
  addition to ServiceAccount.
- Provide more flexible configuration key value options (e.g. receiving both
  expected key-value pairs in the volume context instead of a single
  ServiceAccount name).
- Support restriction for static volumes.
