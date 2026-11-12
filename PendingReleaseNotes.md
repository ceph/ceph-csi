# v3.19 Pending Release Notes

## Breaking changes

## Features

## NOTE
<<<<<<< HEAD
=======

- The `--setmetadata` flag has been removed. The flag was deprecated in
  v3.17 and had no effect (metadata is always set on volumes).
- The `aws-metadata` and `aws-sts-metadata` KMS providers now use
  aws-sdk-go-v2, as aws-sdk-go (v1) has reached end-of-support. No
  configuration changes are required.
- RBD QoS configured through a `VolumeAttributesClass` is no longer inherited
  by clones or snapshot restores. Each volume receives only the QoS defined in
  its own `VolumeAttributesClass`. QoS is never changed based on the absence of
  keys. To drop a limit, set that key explicitly: `"max"` for cgroup v2 QoS
  (krbd) or `"0"` for traditional NBD QoS (rbd-nbd).
>>>>>>> 7965e2b (rbd: do not modify QoS based on absence of mutable params)
