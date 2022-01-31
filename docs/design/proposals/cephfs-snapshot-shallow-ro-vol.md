# Snapshots as shallow read-only volumes

CSI spec doesn't have a notion of "mounting a snapshot". Instead, the idiomatic
way of accessing snapshot contents is first to create a volume populated with
snapshot contents and then mount that volume to workloads.

CephFS exposes snapshots as special, read-only directories of a subvolume
located in `<subvolume>/.snap`. cephfs-csi can already provision writable
volumes with snapshots as their data source, where snapshot contents are cloned
to the newly created volume. However, cloning a snapshot to volume is a very
expensive operation in CephFS as the data needs to be fully copied. When the
need is to only read snapshot contents, snapshot cloning is extremely
inefficient and wasteful.

This proposal describes a way for cephfs-csi to expose CephFS snapshots as
shallow, read-only volumes, without needing to clone the underlying snapshot
data.

## Use-cases

What's the point of such read-only volumes?

* **Restore snapshots selectively:** users may want to traverse snapshots,
  restoring data to a writable volume more selectively instead of restoring the
  whole snapshot.
* **Volume backup:** users can't backup a live volume, they first need to
  snapshot it. Once a snapshot is taken, it still can't be backed-up, as backup
  tools usually work with volumes (that are exposed as file-systems)
  and not snapshots (which might have backend-specific format). What this means
  is that in order to create a snapshot backup, users have to clone snapshot
  data twice:

    1. first time, when restoring the snapshot into a temporary volume from
       where the data will be read,
    1. and second time, when transferring that volume into some backup/archive
       storage (e.g. object store).

  The temporary backed-up volume will most likely be thrown away after the
  backup transfer is finished. That's a lot of wasted work for what we
  originally wanted to do! Having the ability to create volumes from snapshots
  cheaply would be a big improvement for this use case.

## Alternatives

* _Snapshots are stored in `<subvolume>/.snap`. Users could simply visit this
  directory by themselves._

  `.snap` is CephFS-specific detail of how snapshots are exposed. Users / tools
  may not be aware of this special directory, or it may not fit their workflow.
  At the moment, the idiomatic way of accessing snapshot contents in CSI drivers
  is by creating a new volume and populating it with snapshot.

## Design

Key points:

* Volume source is a snapshot, volume access mode is `*_READER_ONLY`.
* No actual new subvolumes are created in CephFS.
* The resulting volume is a reference to the source subvolume snapshot. This
  reference would be stored in `Volume.volume_context` map. In order to
  reference a snapshot, we need subvolume name and snapshot name.
* Mounting such volume means mounting the respective CephFS subvolume and
  exposing the snapshot to workloads.
* Let's call a *shallow read-only volume with a subvolume snapshot as its data
  source* just a *shallow volume* from here on out for brevity.

### Controller operations

Care must be taken when handling life-times of relevant storage resources. When
a shallow volume is created, what would happen if:

* _Parent subvolume of the snapshot is removed while the shallow volume still
  exists?_

  This shouldn't be a problem already. The parent volume has either
  `snapshot-retention` subvolume feature in which case its snapshots remain
  available, or if it doesn't have that feature, it will fail to be deleted
  because it still has snapshots associated to it.
* _Source snapshot from which the shallow volume originates is removed while
  that shallow volume still exists?_

  We need to make sure this doesn't happen and some book-keeping is necessary.
  Ideally we could employ some kind of reference counting.

#### Reference counting for shallow volumes

As mentioned above, this is to protect shallow volumes, should their source
snapshot be requested for deletion.

When creating a volume snapshot, a reference tracker (RT), represented by a
RADOS object, would be created for that snapshot. It would store information
required to track the references for the backing subvolume snapshot. Upon a
`CreateSnapshot` call, the reference tracker (RT) would be initialized with a
single reference record, where the CSI snapshot itself is the first reference to
the backing snapshot. Each subsequent shallow volume creation would add a new
reference record to the RT object. Each shallow volume deletion would remove
that reference from the RT object. Calling `DeleteSnapshot` would remove the
reference record that was previously added in `CreateSnapshot`.

The subvolume snapshot would be removed from the Ceph cluster only once the RT
object holds no references. Note that this behavior would permit calling
`DeleteSnapshot` even if it is still referenced by shallow volumes.

* `DeleteSnapshot`:
* RT holds no references or the RT object doesn't exist:
  delete the backing snapshot too.
* RT holds at least one reference: keep the backing snapshot.
* `DeleteVolume`:
* RT holds no references: delete the backing snapshot too.
* RT holds at least one reference: keep the backing snapshot.

To enable creating shallow volumes from snapshots that were provisioned by older
versions of cephfs-csi (i.e. before this feature is introduced),
`CreateVolume` for shallow volumes would also create an RT object in case it's
missing. It would be initialized to two: the source snapshot and the newly
created shallow volume.

##### Concurrent access to RT objects

RADOS API provides access to compound atomic read and write operations. These
will be used to implement reference tracking functionality, protecting
modifications of reference records.

#### `CreateVolume`

A read-only volume with snapshot source would be created under these conditions:

1. `CreateVolumeRequest.volume_content_source` is a snapshot,
1. `CreateVolumeRequest.volume_capabilities[*].access_mode` is any of read-only
   volume access modes.
1. Possibly other volume parameters in `CreateVolumeRequest.parameters`
   specific to shallow volumes.

`CreateVolumeResponse.Volume.volume_context` would then contain necessary
information to identify the source subvolume / snapshot.

Things to look out for:

* _What's the volume size?_

  It doesn't consume any space on the filesystem. `Volume.capacity_bytes` is
  allowed to contain zero. We could use that.
* _What should be the requested size when creating the volume (specified e.g. in
  PVC)?_

  This one is tricky. CSI spec allows for
  `CreateVolumeRequest.capacity_range.{required_bytes,limit_bytes}` to be zero.
  On the other hand,
  `PersistentVolumeClaim.spec.resources.requests.storage` must be bigger than
  zero. cephfs-csi doesn't care about the requested size (the volume will be
  read-only, so it has no usable capacity) and would always set it to zero. This
  shouldn't case any problems for the time being, but still is something we
  should keep in mind.

`CreateVolume` and behavior when using volume as volume source (PVC-PVC clone):

| New volume     | Source volume  | Behavior                                                                          |
|----------------|----------------|-----------------------------------------------------------------------------------|
| shallow volume | shallow volume | Create a new reference to the parent snapshot of the source shallow volume.       |
| regular volume | shallow volume | Equivalent for a request to create a regular volume with snapshot as its source.  |
| shallow volume | regular volume | Such request doesn't make sense and `CreateVolume` should return an error.        |

### `DeleteVolume`

Volume deletion is trivial.

### `CreateSnapshot`

Snapshotting read-only volumes doesn't make sense in general, and should be
rejected.

### `ControllerExpandVolume`

Same thing as above. Expanding read-only volumes doesn't make sense in general,
and should be rejected.

## Node operations

Two cases need to be considered:

* (a) Volume/snapshot provisioning is handled by cephfs-csi
* (b) Volume/snapshot provisioning is handled externally (e.g. pre-provisioned
  manually, or by OpenStack Manila, ...)

### `NodeStageVolume`, `NodeUnstageVolume`

Here we're mounting the source subvolume onto the node. Subsequent volume
publish calls then use bind mounts to expose the snapshot directory located in
`.snap/<SNAPSHOT DIRECTORY NAME>`. Unfortunately, we cannot mount snapshots
directly because they are not visible during mount time. We need to mount the
whole subvolume first, and only then perform the binds to target paths.

#### For case (a)

Subvolume paths are normally retrieved by
`ceph fs subvolume info/getpath <VOLUME NAME> <SUBVOLUME NAME> <SUBVOLUMEGROUP NAME>`
, which outputs a path like so:

```
/volumes/<VOLUME NAME>/<SUBVOLUME NAME>/<UUID>
```

Snapshots are then accessible in:

* `/volumes/<VOLUME NAME>/<SUBVOLUME NAME>/.snap` and
* `/volumes/<VOLUME NAME>/<SUBVOLUME NAME>/<UUID>/.snap`.

`/volumes/<VOLUME NAME>/<SUBVOLUME NAME>/<UUID>` may be deleted if the source
subvolume is deleted, but thanks to the `snapshot-retention` feature, snapshots
in `/volumes/<VOLUME NAME>/<SUBVOLUME NAME>/.snap` will remain to be available.

The CephFS mount should therefore have its root set to the parent of what
`fs subvolume getpath` returns, i.e. `/volumes/<VOLUME NAME>/<SUBVOLUME NAME>`.
That way we will have snapshots available regardless of whether the subvolume
itself still exists or not.

#### For case (b)

For cases where subvolumes are managed externally and not by cephfs-csi, we must
assume that the cephx user we're given can access only
`/volumes/<VOLUME NAME>/<SUBVOLUME NAME>/<UUID>` so users won't be able to
benefit from snapshot retention. Users will need to be careful not to delete the
parent subvolumes and snapshots while they are associated by these shallow RO
volumes.

### `NodePublishVolume`, `NodeUnpublishVolume`

Node publish is trivial. We bind staging path to target path as a read-only
mount.

### `NodeGetVolumeStats`

`NodeGetVolumeStatsResponse.usage[*].available` should be always zero.

## Volume parameters, volume context

This section provides a discussion around determining what volume parameters and
volume context parameters will be used to convey necessary information to the
cephfs-csi driver in order to support shallow volumes.

Volume parameters `CreateVolumeRequest.parameters`:

* Should be "shallow" the default mode for all `CreateVolume` calls that have
  (a) snapshot as data source and (b) read-only volume access mode? If not, a
  new volume parameter should be introduced: e.g `isShallow: <bool>`. On the
  other hand, does it even makes sense for users to want to create full copies
  of snapshots and still have them read-only?

Volume context `Volume.volume_context`:

* Here we definitely need `isShallow` or similar. Without it we wouldn't be able
  to distinguish between a regular volume that just happens to have a read-only
  access mode, and a volume that references a snapshot.
* Currently cephfs-csi recognizes `subvolumePath` for dynamically provisioned
  volumes and `rootPath` for pre-previsioned volumes. As mentioned in
  [`NodeStageVolume`, `NodeUnstageVolume` section](#NodeStageVolume-NodeUnstageVolume)
  , snapshots cannot be mounted directly. How do we pass in path to the parent
  subvolume?
* a) Path to the snapshot is passed in via `subvolumePath` / `rootPath`, e.g.
  `/volumes/<VOLUME NAME>/<SUBVOLUME NAME>/<UUID>/.snap/<SNAPSHOT NAME>`. From
  that we can derive path to the subvolume: it's the parent of `.snap`
  directory.
* b) Similar to a), path to the snapshot is passed in via `subvolumePath` /
  `rootPath`, but instead of trying to derive the right path we introduce
  another volume context parameter containing path to the parent subvolume
  explicitly.
* c) `subvolumePath` / `rootPath` contains path to the parent subvolume and we
  introduce another volume context parameter containing name of the snapshot.
  Path to the snapshot is then formed by appending
  `/.snap/<SNAPSHOT NAME>` to the subvolume path.
