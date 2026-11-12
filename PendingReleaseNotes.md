# v3.19 Pending Release Notes

## Breaking changes

## Features

## NOTE

- RBD QoS configured through a `VolumeAttributesClass` is no longer inherited
  by clones or snapshot restores. Each volume receives only the QoS defined in
  its own `VolumeAttributesClass`. QoS is never changed based on the absence of
  keys. To drop a limit, set that key explicitly: `"max"` for cgroup v2 QoS
  (krbd) or `"0"` for traditional NBD QoS (rbd-nbd).
