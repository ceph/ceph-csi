# Ceph CSI 1.0.0

[Container Storage Interface
(CSI)](https://github.com/container-storage-interface/) driver, provisioner,
and attacher for Ceph RBD and CephFS.

## Overview

Ceph CSI plugins implement an interface between CSI enabled Container
Orchestrator (CO) and CEPH cluster.
It allows dynamically provisioning CEPH volumes and attaching them to
workloads.
Current implementation of Ceph CSI plugins was tested in Kubernetes
environment (requires Kubernetes 1.13+), but the code does not rely on
any Kubernetes specific calls (WIP to make it k8s agnostic) and
should be able to run with any CSI enabled CO.

For details about configuration and deployment of RBD and
CephFS CSI plugins, see documentation in `docs/`.

For example usage of RBD and CephFS CSI plugins, see examples in `examples/`.

## Support Matrix

Here is the matrix that describes the features supported by the different
versions of Ceph CSI driver

|              |        Features             | CSI driver<br>Version |Kubernetes<br>Version |
|--------------|-----------------------------|-----------------------|----------------------|
| **CephFS**   | Snapshot                    |         1.0           |   1.13 - 1.14        |
|              | Clone from VolumeSnapshot   |          -            |   1.13 - 1.14        |
|              | Clone from VolumeSource     |          -            |   1.13 - 1.14        |
|              | Resize                      |          -            |   1.14               |
|              |                             |                       |                      |
| **RBD**      | Snapshot                    |       0.3, 1.0        |   1.13 - 1.14        |
|              | Clone from VolumeSnapshot   |       0.3, 1.0        |   1.13 - 1.14        |
|              | Clone from VolumeSource     |          -            |   1.13 - 1.14        |
|              | Resize                      |          -            |   1.14               |

## Troubleshooting

Please submit an issue at: [Issues](https://github.com/ceph/ceph-csi/issues)

## Slack Channels

Join us at [Rook ceph-csi Channel](https://rook-io.slack.com/messages/CG3HUV94J/details/)
