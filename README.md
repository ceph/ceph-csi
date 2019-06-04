# Ceph CSI

[![Go Report
Card](https://goreportcard.com/badge/github.com/ceph/ceph-csi)](https://goreportcard.com/report/github.com/ceph/ceph-csi)
[![Build
Status](https://travis-ci.org/ceph/ceph-csi.svg?branch=master)](https://travis-ci.org/ceph/ceph-csi)

This repo contains [Container Storage Interface(CSI)]
(<https://github.com/container-storage-interface/>) driver, provisioner,
and attacher for Ceph RBD and CephFS.

## Overview

Ceph CSI plugins implement an interface between CSI enabled Container Orchestrator
(CO) and Ceph cluster. It allows dynamically provisioning Ceph volumes and
attaching them to workloads.

Independent CSI plugins are provided to support RBD and CephFS backed volumes,

- For details about configuration and deployment of RBD and CephFS CSI plugins,
  see documentation in `docs/`.
- For example usage of RBD and CephFS CSI plugins, see examples in `examples/`.

## Project status

Status: **Alpha**

The **alpha** status reflects possible non-backward compatible changes in the
future, and is thus not recommended for production use. There is work in progress
that would change on-disk metadata for certain operations, possibly breaking
backward compatibility.

## Supported CO platforms

Ceph CSI drivers are currently developed and tested **exclusively** on Kubernetes
environments. There is work in progress to make this CO independent and thus
support other orchestration environments in the future.

For Kubernetes versions 1.11 and 1.12, please use [0.3 images and
deployments](https://github.com/ceph/ceph-csi/tree/csi-v0.3/deploy/).

For Kubernetes versions 1.13 and above, please use [1.0 images and
deployments](https://github.com/ceph/ceph-csi/tree/csi-v1.0/deploy/).

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

## Contributing to this repo

Please follow [development-guide]
(<https://github.com/ceph/ceph-csi/tree/master/docs/development-guide.md>) and
[coding style guidelines](<https://github.com/ceph/ceph-csi/tree/master/docs/coding.md>)
if you are interested to contribute to this repo.

## Troubleshooting

Please submit an issue at: [Issues](https://github.com/ceph/ceph-csi/issues)

## Slack Channels

Join us at [Rook ceph-csi Channel](https://rook-io.slack.com/messages/CG3HUV94J/details/)
