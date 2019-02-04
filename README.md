# Ceph CSI

# Supported platforms

For Kubernetes 1.12 and below, please use [0.3 images and deployments](https://github.com/ceph/ceph-csi/tree/master/deploy/rbd/kubernetes).

For Kubernetes 1.13 and above, please use [1.0 images and deployments](https://github.com/ceph/ceph-csi/tree/csi-v1.0/deploy/rbd/kubernetes).

[Container Storage Interface (CSI)](https://github.com/container-storage-interface/) driver, provisioner, and attacher for Ceph RBD and CephFS.

## Overview

Ceph CSI plugins implement an interface between CSI enabled Container Orchestrator (CO) and CEPH cluster. It allows dynamically provisioning CEPH volumes and attaching them to workloads. Current implementation of Ceph CSI plugins was tested in Kubernetes environment (requires Kubernetes 1.11+), but the code does not rely on any Kubernetes specific calls (WIP to make it k8s agnostic) and should be able to run with any CSI enabled CO.

For details about configuration and deployment of RBD and CephFS CSI plugins, see documentation in `docs/`.

For example usage of RBD and CephFS CSI plugins, see examples in `examples/`.

## Troubleshooting

Please submit an issue at: [Issues](https://github.com/ceph/ceph-csi/issues)
