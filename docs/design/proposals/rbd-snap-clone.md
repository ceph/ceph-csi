# Steps and RBD CLI commands for RBD snapshot and clone operations

- [Steps and RBD CLI commands for RBD snapshot and clone operations](#steps-and-rbd-cli-commands-for-rbd-snapshot-and-clone-operations)
   - [Create a snapshot from PVC](#create-a-snapshot-from-pvc)
      - [steps to create a snapshot](#steps-to-create-a-snapshot)
      - [RBD CLI commands to create snapshot](#rbd-cli-commands-to-create-snapshot)
   - [Create PVC from a snapshot (datasource snapshot)](#create-pvc-from-a-snapshot-datasource-snapshot)
      - [steps to create a pvc from snapshot](#steps-to-create-a-pvc-from-snapshot)
      - [RBD CLI commands to create clone from snapshot](#rbd-cli-commands-to-create-clone-from-snapshot)
   - [Delete a snapshot](#delete-a-snapshot)
      - [steps to delete a snapshot](#steps-to-delete-a-snapshot)
      - [RBD CLI commands to delete a snapshot](#rbd-cli-commands-to-delete-a-snapshot)
   - [Delete a Volume (PVC)](#delete-a-volume-pvc)
      - [steps to delete a volume](#steps-to-delete-a-volume)
      - [RBD CLI commands to delete a volume](#rbd-cli-commands-to-delete-a-volume)
   - [Volume cloning (datasource pvc)](#volume-cloning-datasource-pvc)
      - [steps to create a Volume from Volume](#steps-to-create-a-volume-from-volume)
      - [RBD CLI commands to create a Volume from Volume](#rbd-cli-commands-to-create-a-volume-from-volume)

This document outlines the command used to create RBD snapshot, delete RBD
snapshot, Restore RBD snapshot and Create new RBD image from existing RBD image.

## Create a snapshot from PVC

Refer [snapshot](https://kubernetes.io/docs/concepts/storage/volume-snapshots/)
for more information related to Volume cloning in kubernetes.

### steps to create a snapshot

- Check if the parent image has more snapshots than the configured value, if
  it has more snapshot, add tasks to flatten all the temporary cloned images
  and return ResourceExhausted error message
- Create a temporary snapshot from the parent image
- Clone a new image from a temporary snapshot with options
  `--rbd-default-clone-format 2 --image-feature layering,deep-flatten`
- Delete temporary snapshot created
- Check the image chain depth, if the `softlimit` is reached add a task to flatten
  the cloned image and return success. If the depth is reached `hardlimit` add a
  task flatten the cloned image and return snapshot status ready as `false`

### RBD CLI commands to create snapshot

```
rbd snap ls <RBD image for src k8s volume> --all

// If the parent has more snapshots than the configured `maxsnapshotsonimage`
// add background tasks to flatten the temporary cloned images (temporary cloned
// image names will be same as snapshot names)
ceph rbd task add flatten <RBD image for temporary snap images>

rbd snap create <RBD image for src k8s volume>@<random snap name>
rbd clone --rbd-default-clone-format 2 --image-feature
    layering,deep-flatten <RBD image for src k8s volume>@<random snap>
    <RBD image for temporary snap image>
rbd snap rm <RBD image for src k8s volume>@<random snap name>
rbd snap create <RBD image for temporary snap image>@<random snap name>

// check the depth, if the depth is greater than configured hardlimit add a
// task to flatten the cloned image, return snapshot status ready as `false`,
// if the depth is greater than softlimit add a task to flatten the image
// and return success
ceph rbd task add flatten <RBD image for temporary snap image>
```

## Create PVC from a snapshot (datasource snapshot)

### steps to create a pvc from snapshot

- Check the depth(n) of the cloned image if `n>=(hardlimit -2)`, add task to
  flatten the image and return ABORT (to avoid image leak)
- Clone a new image from the snapshot with user-provided options

### RBD CLI commands to create clone from snapshot

```
// check the depth, if the depth is greater than configured (hardlimit)
// Add a task to value flatten the cloned image
ceph rbd task add flatten <RBD image for temporary snap image>

rbd clone --rbd-default-clone-format 2 --image-feature <k8s dst vol config>
    <RBD image for temporary snap image>@<random snap name>
    <RBD image for k8s dst vol>
// check the depth,if the depth is greater than configured hardlimit add a task
// to flatten the cloned image return ABORT error, if the depth is greater than
// softlimit add a task to flatten the image and return success
ceph rbd task add flatten <RBD image for k8s dst vol>
```

## Delete a snapshot

### steps to delete a snapshot

- Delete temporary snapshot on temporary cloned image
- Move the temporary cloned image to trash
- Add task to remove the image from trash

### RBD CLI commands to delete a snapshot

```
rbd snap create <RBD image for temporary snap image>@<random snap name>
rbd trash mv <RBD image for temporary snap image>
ceph rbd task trash remove <RBD image for temporary snap image ID>
```

## Delete a Volume (PVC)

With earlier implementation to delete the image we used to add a task to remove
the image. With new changes this cannot be done as the image may contains
snapshots or linking, so we will be following below steps to delete an
image(this will be applicable for both normal image and cloned image)

### steps to delete a volume

- Move the rbd image to trash
- Add task to remove the image from trash

### RBD CLI commands to delete a volume

```
1) rbd trash mv <image>
2) ceph rbd task trash remove <image>
```

## Volume cloning (datasource pvc)

Refer
[volume-cloning](https://kubernetes.io/docs/concepts/storage/volume-pvc-datasource/)
for more information related to Volume cloning in kubernetes.

### steps to create a Volume from Volume

- Check the depth(n) of the cloned image if `n>=((hard limit) -2)`, add task to
  flatten the image and return ABORT (to avoid image leak)
- Create snapshot of rbd image
- Clone the snapshot (temp clone)
- Delete the snapshot
- Snapshot the temp clone
- Clone the snapshot (final clone)
- Delete the snapshot

### RBD CLI commands to create a Volume from Volume

```
// check the image depth of the parent image if flatten required add a
// task to flatten image and return ABORT to avoid leak(same hardlimit and
// softlimit check will be done)
ceph rbd task add flatten <RBD image for src k8s volume>

rbd snap create <RBD image for src k8s volume>@<random snap name>
rbd clone --rbd-default-clone-format 2 --image-feature
    layering,deep-flatten <RBD image for src k8s volume>@<random snap>
    <RBD image for temporary snap image>
rbd snap rm <RBD image for src k8s volume>@<random snap name>
rbd snap create <RBD image for temporary snap image>@<random snap name>
rbd clone --rbd-default-clone-format 2 --image-feature <k8s dst vol config>
    <RBD image for temporary snap image>@<random snap name>
    <RBD image for k8s dst vol>
rbd snap rm <RBD image for src k8s volume>@<random snap name>
```
