# Reclaim Space

Reclaim Space is a [CSI-Addons](https://csi-addons.github.io/) feature that
enables the recovery of unused storage space from provisioned volumes. This
capability is essential for optimizing storage utilization, particularly for
filesystem volumes where deleted files may not immediately release space back
to the underlying storage system.

## Overview

Space reclamation in Ceph-CSI provides two complementary operations:

1. **NodeReclaimSpace**: Reclaims space on filesystem volumes by running
   `fstrim` on mounted volumes. This is the primary and most effective method
   for freeing unused space.

1. **ControllerReclaimSpace**: Attempts to sparsify RBD images at the storage
   layer. However, this operation has limited effectiveness in typical
   deployments.

The reclaim space feature is implemented through the CSI-Addons specification
and currently supports RBD (RADOS Block Device) storage with filesystem
volumes.

## How It Works

### NodeReclaimSpace (Filesystem Volumes)

NodeReclaimSpace operates on mounted filesystem volumes by:

1. **Identifying the Mount Point**: Locating the volume's staging path or
   volume path where the filesystem is mounted.

1. **Running fstrim**: Executing the `fstrim` command on the mounted
   filesystem, which:
   - Identifies unused blocks in the filesystem
   - Issues TRIM/DISCARD commands to the underlying block device
   - Allows the storage backend to reclaim the freed space

1. **Updating Storage**: The TRIM commands propagate through the storage stack
   (RBD → Ceph OSD), allowing the Ceph cluster to mark the corresponding
   objects as unused and reclaim the space.

**Example workflow:**

```
Application deletes files → Filesystem marks blocks as free →
NodeReclaimSpace runs fstrim → TRIM commands sent to RBD →
Ceph OSDs reclaim space → Storage capacity freed
```

### ControllerReclaimSpace (Block-Level Sparsification)

ControllerReclaimSpace attempts to sparsify RBD images at the storage layer:

1. **Volume Validation**: Verifies the volume exists and is accessible.

1. **Sparsify Operation**: Attempts to run the RBD sparsify operation, which
   scans the image for zero-filled regions and removes them from storage.

1. **In-Use Detection**: If the volume is currently in use (mounted), the
   operation is treated as a no-op and returns successfully without making
   changes.

**Important:** This operation has limited practical use because:

- Applications using block devices (like KubeVirt/QEMU) already discard freed
  blocks automatically
- Volumes are typically mounted and in use, preventing sparsification
- The operation only works on unmounted, detached volumes
- Filesystem-level space reclamation (NodeReclaimSpace) is more effective for
  active volumes

## CSI-Addons Operations

### NodeReclaimSpace

Reclaims space on a mounted filesystem volume by running `fstrim`. This is the
**recommended and most effective** method for space reclamation.

**Request Parameters:**

- `volume_id`: The ID of the volume to reclaim space from (required)
- `staging_target_path`: The staging path where the volume is mounted (optional)
- `volume_path`: The path where the volume is mounted in the application
  container (optional, used if staging_target_path is not provided)
- `volume_capability`: The volume capability (required for validation)

**Supported Configurations:**

- ✅ Single-node filesystem volumes (ReadWriteOnce)
- ❌ Multi-node filesystem volumes (ReadWriteMany) - not supported to prevent
  data corruption
- ❌ Block-mode volumes - not supported (no filesystem to trim)

**Example ReclaimSpaceJob:**

```yaml
apiVersion: csiaddons.openshift.io/v1alpha1
kind: ReclaimSpaceJob
metadata:
  name: reclaimspace-pvc-example
spec:
  target:
    persistentVolumeClaim: my-pvc
  backOffLimit: 10
  retryDeadlineSeconds: 900
  timeout: 600
```

**Example ReclaimSpaceCronJob:**

```yaml
apiVersion: csiaddons.openshift.io/v1alpha1
kind: ReclaimSpaceCronJob
metadata:
  name: reclaimspace-cronjob-example
spec:
  schedule: "@weekly"
  jobSpec:
    target:
      persistentVolumeClaim: my-pvc
    backOffLimit: 10
    retryDeadlineSeconds: 900
    timeout: 600
```

### ControllerReclaimSpace

Attempts to sparsify an RBD image at the storage layer. This operation is
**not expected to free space in most environments** because volumes are
typically in use.

**Request Parameters:**

- `volume_id`: The ID of the volume to sparsify (required)
- `secrets`: Ceph credentials for authentication (required)

**Behavior:**

- If the volume is in use (mounted), the operation returns successfully without
  making changes (treated as a no-op)
- If the volume is not in use, attempts to run RBD sparsify
- Only effective for volumes that are detached and unmounted

**Note:** For active volumes, use NodeReclaimSpace instead, as it is more
effective and works on mounted filesystems.

## Use Cases

### Regular Storage Maintenance

Periodic space reclamation helps maintain optimal storage utilization:

1. **Scheduled Cleanup**: Use ReclaimSpaceCronJob to automatically reclaim
   space on a regular schedule (e.g., weekly or monthly)

1. **Post-Deletion Cleanup**: Run space reclamation after bulk file deletions
   to immediately free storage capacity

1. **Capacity Management**: Prevent storage exhaustion by regularly reclaiming
   unused space from active volumes

### Application Lifecycle Management

Space reclamation is valuable during application operations:

1. **Log Rotation**: After log files are rotated and deleted, reclaim the
   freed space

1. **Cache Cleanup**: When applications clear caches or temporary files,
   recover the storage capacity

1. **Database Maintenance**: After database vacuum or cleanup operations,
   reclaim freed space

### Cost Optimization

For cloud or thin-provisioned storage:

1. **Reduce Storage Costs**: Free unused space to reduce storage consumption
   and associated costs

1. **Improve Thin Provisioning**: Allow thin-provisioned volumes to return
   space to the storage pool

1. **Optimize Capacity Planning**: Maintain accurate storage utilization
   metrics for capacity planning

## Configuration

### Enabling Space Reclamation

Space reclamation is enabled through the CSI-Addons endpoint configuration in
the Ceph-CSI driver deployment.

**Example deployment configuration:**

```yaml
args:
  - "--csi-addons-endpoint=unix:///tmp/csi-addons.sock"
```

### Volume Requirements

For NodeReclaimSpace to work effectively:

1. **Filesystem Support**: The filesystem must support TRIM/DISCARD operations
   (ext4, xfs, btrfs, etc.)

1. **Mount Options**: Ensure the filesystem is mounted with appropriate options
   (most modern filesystems enable TRIM by default)

1. **Volume Capability**: The volume must be a filesystem volume (not
   block-mode) with single-node access (ReadWriteOnce)

## Prerequisites

1. **CSI-Addons Controller**: The CSI-Addons controller must be deployed in
   the Kubernetes cluster. See the [CSI-Addons documentation](https://csi-addons.github.io/)
   for installation instructions.

1. **ReclaimSpace CRDs**: The ReclaimSpace Custom Resource Definitions must
   be installed:

   ```bash
   kubectl create -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.10.0/deploy/controller/crds.yaml
   ```

1. **RBAC Permissions**: Appropriate RBAC rules must be configured for the
   CSI-Addons operator to manage space reclamation jobs.

1. **Filesystem Support**: The filesystem on the volume must support
   TRIM/DISCARD operations (ext4, xfs, btrfs, etc.).

1. **Ceph Credentials**: Valid Ceph credentials with permissions to access
   and manage volumes (required for ControllerReclaimSpace).

## Limitations and Considerations

### NodeReclaimSpace Limitations

1. **Multi-Node Volumes Not Supported**: ReadWriteMany (RWX) volumes are not
   supported to prevent potential data corruption from concurrent trim
   operations.

1. **Block-Mode Not Supported**: Block-mode volumes cannot be trimmed as they
   have no filesystem layer.

1. **Filesystem Dependency**: The effectiveness of space reclamation depends
   on the filesystem's TRIM implementation and the underlying storage's
   support for DISCARD operations.

1. **Performance Impact**: Running `fstrim` on large filesystems can be
   I/O-intensive and may temporarily impact application performance.

### ControllerReclaimSpace Limitations

1. **Limited Effectiveness**: This operation rarely frees space because:
   - Applications using block devices (like KubeVirt/QEMU) already discard
     freed blocks automatically
   - Volumes are typically mounted and in use
   - Sparsify only works on detached, unmounted volumes
   - The operation is treated as a no-op when volumes are in use

1. **Use NodeReclaimSpace Instead**: For active volumes, NodeReclaimSpace is
   the recommended and more effective approach.

### General Considerations

1. **Volume Locking**: Space reclamation operations acquire a volume lock to
   prevent concurrent operations on the same volume.

1. **Operation Timeout**: Configure appropriate timeout values in
   ReclaimSpaceJob specifications to account for large volumes.

1. **Scheduling**: When using ReclaimSpaceCronJob, schedule operations during
   low-activity periods to minimize performance impact.

## Troubleshooting

### Checking Space Reclamation Status

To verify space reclamation job status:

```bash
# List ReclaimSpaceJobs
kubectl get reclaimspacejob -A

# Check job details
kubectl describe reclaimspacejob <job-name> -n <namespace>

# View job logs
kubectl logs -n <csi-addons-namespace> <csi-addons-controller-pod>
```

### Verifying Filesystem TRIM Support

To check if a filesystem supports TRIM:

```bash
# Check mount options
mount | grep <mount-point>

# Manually test fstrim (requires root access on the node)
fstrim -v <mount-point>
```

### Common Issues

1. **"multi-node space reclaim is not supported" error**: The volume has
   ReadWriteMany (RWX) access mode. Space reclamation is only supported for
   ReadWriteOnce (RWO) volumes to prevent data corruption.

   **Solution**: Ensure the PVC uses ReadWriteOnce access mode.

1. **"block-mode space reclaim is not supported" error**: The volume is
   configured in block mode without a filesystem.

   **Solution**: Use filesystem mode (volumeMode: Filesystem) for volumes that
   need space reclamation.

1. **"volume operation already exists" error**: Another operation is currently
   running on the same volume.

   **Solution**: Wait for the current operation to complete, or check for
   stuck operations that may need manual intervention.

1. **fstrim fails with "operation not supported"**: The filesystem or
   underlying storage does not support TRIM/DISCARD operations.

   **Solution**: Verify filesystem type and storage backend support for TRIM.
   Some older filesystems or storage systems may not support this feature.

1. **ControllerReclaimSpace doesn't free space**: This is expected behavior
   when volumes are in use (mounted).

   **Solution**: Use NodeReclaimSpace for active volumes, as it is designed to
   work on mounted filesystems and is more effective for space reclamation.

1. **Space reclamation job times out**: The operation is taking longer than
   the configured timeout.

   **Solution**: Increase the `timeout` value in the ReclaimSpaceJob
   specification, especially for large volumes.

### Manual Space Reclamation

If needed, you can manually reclaim space on a node:

```bash
# SSH to the node where the volume is mounted
# Find the mount point
mount | grep <volume-id>

# Run fstrim manually (requires root)
fstrim -v /path/to/mount/point
```

## Performance Considerations

### Impact on Applications

- **I/O Overhead**: `fstrim` operations can generate significant I/O activity,
  potentially impacting application performance during execution.

- **Scheduling**: Schedule space reclamation during maintenance windows or
  low-activity periods when possible.

- **Frequency**: Balance reclamation frequency against performance impact.
  Weekly or monthly schedules are often sufficient for most workloads.

### Optimization Tips

1. **Batch Operations**: If reclaiming space on multiple volumes, stagger the
   operations to avoid overwhelming the storage system.

1. **Monitor Performance**: Track application performance metrics during space
   reclamation to identify any adverse effects.

1. **Adjust Timeout**: Set appropriate timeout values based on volume size and
   storage performance characteristics.

## Best Practices

1. **Use NodeReclaimSpace for Active Volumes**: NodeReclaimSpace is the
   recommended method for reclaiming space on mounted, active volumes.
1. **Schedule Regular Reclamation**: Use ReclaimSpaceCronJob to automate
   periodic space reclamation, preventing storage exhaustion.
1. **Monitor Storage Utilization**: Track storage usage before and after
   reclamation to measure effectiveness and adjust schedules accordingly.
1. **Test Before Production**: Test space reclamation on non-production
   volumes first to understand performance impact and effectiveness.
1. **Document Schedules**: Maintain documentation of reclamation schedules and
   their rationale for operational clarity.
1. **Avoid Multi-Node Volumes**: Use ReadWriteOnce (RWO) access mode for
   volumes that require space reclamation, as ReadWriteMany (RWX) is not
   supported.

## See Also

- [CSI-Addons Documentation](https://csi-addons.github.io/)
- [CSI-Addons Specification](https://github.com/csi-addons/spec)
- [Linux fstrim Manual](https://man7.org/linux/man-pages/man8/fstrim.8.html)
- [Ceph RBD Sparsify Documentation](https://docs.ceph.com/en/latest/man/8/rbd/#commands)
- [Kubernetes CSI-Addons Operator](https://github.com/csi-addons/kubernetes-csi-addons)
