# Provisioning and mounting CephFS snapshot-backed volumes

Snapshot-backed volumes allow CephFS subvolume snapshots to be exposed as
regular read-only PVCs. No data cloning is performed and provisioning such
volumes is done in constant time.

For more details please refer to [Snapshots as shallow read-only volumes](./design/proposals/cephfs-snapshot-shallow-ro-vol.md)
design document.

## Prerequisites

Prerequisites for this feature are the same as for creating PVCs with snapshot
volume source. See [Create snapshot and Clone Volume](./snap-clone.md) for more
information.

## Usage

### Provisioning a snapshot-backed volume from a volume snapshot

For provisioning new snapshot-backed volumes, following configuration must be
set for storage class(es) and their PVCs respectively:

* StorageClass:
   * Specify `backingSnapshot: "true"` parameter.
* PersistentVolumeClaim:
   * Set `storageClassName` to point to your storage class with backing
    snapshots enabled.
   * Define `spec.dataSource` for your desired source volume snapshot.
   * Set `spec.accessModes` to `ReadOnlyMany`. This is the only access mode that
    is supported by this feature.

### Mounting snapshots from pre-provisioned volumes

Steps for defining a PersistentVolume and PersistentVolumeClaim for
pre-provisioned CephFS subvolumes are identical to those described in
[Static PVC with ceph-csi](./static-pvc.md), except one additional parameter
must be specified: `backingSnapshotID`. CephFS-CSI driver will retrieve the
snapshot identified by the given ID from within the specified subvolume, and
expose it to workloads in read-only mode. Volume access mode must be set to
`ReadOnlyMany`.

Note that the snapshot retrieval is done by traversing `<rootPath>/.snap` and
searching for a directory that contains `backingSnapshotID` value in its name.
The specified snapshot ID does not necessarily need to be the complete directory
name inside `<rootPath>/.snap`, however it must be complete enough to uniquely
identify that directory.

Example:

```
$ ls .snap
_f279df14-6729-4342-b82f-166f45204233_1099511628283
_a364870e-6729-4342-b82f-166f45204233_1099635085072
```

`f279df14-6729-4342-b82f-166f45204233` would be considered a valid value for
`backingSnapshotID` volume parameter, whereas `6729-4342-b82f-166f45204233`
would not, as it would be ambiguous.

If the given snapshot ID is ambiguous, or no such snapshot is found, mounting
the PVC will fail with INVALID_ARGUMENT error code.
