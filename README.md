# Ceph CSI 1.0.0

This repo containes [Container Storage Interface(CSI)]
(<https://github.com/container-storage-interface/>) driver, provisioner,
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

## Contributing to this repo

Please follow [development-guide]
(<https://github.com/ceph/ceph-csi/tree/master/docs/development-guide.md>) and
[coding style guidelines](<https://github.com/ceph/ceph-csi/tree/master/docs/coding.md>)
if you are interested to contribute to this repo.

## Troubleshooting

Please submit an issue at: [Issues](https://github.com/ceph/ceph-csi/issues)

## Slack Channels

Join us at [Rook ceph-csi Channel](https://rook-io.slack.com/messages/CG3HUV94J/details/)
