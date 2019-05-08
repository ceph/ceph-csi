# Ceph CSI 1.0.0

[![Go Report
Card](https://goreportcard.com/badge/github.com/ceph/ceph-csi)](https://goreportcard.com/report/github.com/ceph/ceph-csi)
[![Build
Status](https://travis-ci.org/ceph/ceph-csi.svg?branch=master)](https://travis-ci.org/ceph/ceph-csi)

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

### Ceph-CSI features and available versions

|   Plugin |        Features                                           | CSI driver Version |
|----------|-----------------------------------------------------------|--------------------|
|   CephFS | Dynamically provision, de-provision File mode RWO volume  |      >=v0.3.0      |
|          | Dynamically provision, de-provision File mode RWX volume  |      >=v0.3.0      |
|          | Creating and deleting snapshot                            |          -         |
|          | Provision volume from snapshot                            |          -         |
|          | Provision volume from another volume                      |          -         |
|          | Resize volume                                             |          -         |
|          |                                                           |                    |
|   RBD    | Dynamically provision, de-provision Block mode RWO volume |      >=v0.3.0      |
|          | Dynamically provision, de-provision Block mode RWX volume |      >=v0.3.0      |
|          | Dynamically provision, de-provision File mode RWO volume  |        v1.0.0      |
|          | Creating and deleting snapshot                            |      >=v0.3.0      |
|          | Provision volume from snapshot                            |        v1.0.0      |
|          | Provision volume from another volume                      |          -         |
|          | Resize volume                                             |          -         |

### Ceph-CSI versions and CSI spec compatibility

| Ceph CSI driver Version | CSI spec version |
|-------------------------|------------------|
|         v0.3.0          |     v0.3         |
|         v1.0.0          |     v1.0.0       |

### CSI spec and Kubernetes version compatibility

Please refer to the [matrix](https://kubernetes-csi.github.io/docs/#kubernetes-releases)
in the Kubernetes documentation.

## Troubleshooting

Please submit an issue at: [Issues](https://github.com/ceph/ceph-csi/issues)

## Slack Channels

Join us at [Rook ceph-csi Channel](https://rook-io.slack.com/messages/CG3HUV94J/details/)
