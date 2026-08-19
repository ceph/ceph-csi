# v3.19 Pending Release Notes

## Breaking changes

## Features

1. NVMe-oF: moved the `ListListeners` query from `CreateVolume` to
   `ControllerPublishVolume`. Listeners are now fetched live at publish
   time and passed through the publish context to the node, so the list
   stays accurate across gateway scale-up/scale-down events
   (restart the test-pod is required).
   The node server reads listeners from the publish context instead of the volume
   context, failback to volume context for backward compatibility when node-server
   is updated, but provisioner is not.

## NOTE

- RBD QoS configured through a `VolumeAttributesClass` is no longer inherited
  by clones or snapshot restores. Each volume receives only the QoS defined in
  its own `VolumeAttributesClass`. QoS is never changed based on the absence of
  keys. To drop a limit, set that key explicitly: `"max"` for cgroup v2 QoS
  (krbd) or `"0"` for traditional NBD QoS (rbd-nbd).
