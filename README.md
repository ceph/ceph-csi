# Ceph CSI

[![Go Report
Card](https://goreportcard.com/badge/github.com/ceph/ceph-csi)](https://goreportcard.com/report/github.com/ceph/ceph-csi)
[![Build
Status](https://travis-ci.org/ceph/ceph-csi.svg?branch=master)](https://travis-ci.org/ceph/ceph-csi)

- [Ceph CSI](#Ceph-CSI)
  - [Overview](#Overview)
  - [Project status](#Project-status)
  - [Supported CO platforms](#Supported-CO-platforms)
  - [Support Matrix](#Support-Matrix)
    - [Ceph-CSI features and available versions](#Ceph-CSI-features-and-available-versions)
    - [CSI spec and Kubernetes version compatibility](#CSI-spec-and-Kubernetes-version-compatibility)
  - [Contributing to this repo](#Contributing-to-this-repo)
  - [Troubleshooting](#Troubleshooting)
  - [Contact](#Contact)

This repo contains Ceph
[Container Storage Interface (CSI)](https://github.com/container-storage-interface/)
driver for RBD, CephFS and kubernetes sidecar deployment yamls of provisioner,
attacher, node-driver-registrar and snapshotter for supporting CSI functionalities.

## Overview

Ceph CSI plugins implement an interface between CSI enabled Container Orchestrator
(CO) and Ceph cluster. It allows dynamically provisioning Ceph volumes and
attaching them to workloads.

Independent CSI plugins are provided to support RBD and CephFS backed volumes,

- For details about configuration and deployment of RBD plugin, please refer
  [rbd doc](https://github.com/ceph/ceph-csi/blob/master/docs/deploy-rbd.md) and
  for CephFS plugin configuration and deployment please
  refer [cephfs doc](https://github.com/ceph/ceph-csi/blob/master/docs/deploy-cephfs.md).
- For example usage of RBD and CephFS CSI plugins, see examples in `examples/`.

## Project status

Status: **GA**

## Supported CO platforms

Ceph CSI drivers are currently developed and tested **exclusively** on Kubernetes
environments. There is work in progress to make this CO independent and thus
support other orchestration environments in the future.

NOTE:

- **`csiv0.3`** is deprecated with release of **`csi v1.1.0`**

## Support Matrix

### Ceph-CSI features and available versions

| Plugin | Features                                                  | Feature Status | CSI Driver Version | CSI Spec Version | Ceph Cluster Version | Kubernetes Version |
|--------|-----------------------------------------------------------|----------------|--------------------|------------------|----------------------|--------------------|
| RBD    | Dynamically provision, de-provision Block mode RWO volume | GA             | >=v1.0.0           | >=v1.0.0         | >= Mimic             | >= v13.0.0         |
|        | Dynamically provision, de-provision Block mode RWX volume | GA             | >=v1.0.0           | >=v1.0.0         | >= Mimic             | >= v13.0.0         |
|        | Dynamically provision, de-provision File mode RWO volume  | GA             | >=v1.0.0           | >=v1.0.0         | >= Mimic             | >= v13.0.0         |
|        | Creating and deleting snapshot                            | Alpha          | >=v1.0.0           | >=v1.0.0         | >= Mimic             | >= v13.0.0         |
|        | Provision volume from snapshot                            | Alpha          | >=v1.0.0           | >=v1.0.0         | >= Mimic             | >= v13.0.0         |
|        | Provision volume from another volume                      | -              | -                  | -                | -                    | -                  |
|        | Resize volume                                             | -              | -                  | -                | -                    | -                  |
|        | Metrics Support                                           | -              | -                  | -                | -                    | -                  |
| CephFS | Dynamically provision, de-provision File mode RWO volume  | Alpha          | >=v1.1.0           | >=v1.0.0         | Nautilus             | >=v13.0.0          |
|        | Dynamically provision, de-provision File mode RWX volume  | Alpha          | >=v1.1.0           | >=v1.0.0         | Nautilus             | >=v13.0.0          |
|        | Creating and deleting snapshot                            | -              | -                  | -                | -                    | -                  |
|        | Provision volume from snapshot                            | -              | -                  | -                | -                    | -                  |
|        | Provision volume from another volume                      | -              | -                  | -                | -                    | -                  |
|        | Resize volume                                             | -              | -                  | -                | -                    | -                  |
|        | Metrics                                                   | -              | -                  | -                | -                    | -                  |
|        |                                                           |                |                    |                  |                      |                    |

`NOTE`: The `Alpha` status reflects possible non-backward
compatible changes in the future, and is thus not recommended
for production use.

### CSI spec and Kubernetes version compatibility

Please refer to the [matrix](https://kubernetes-csi.github.io/docs/#kubernetes-releases)
in the Kubernetes documentation.

## Contributing to this repo

Please follow [development-guide](<https://github.com/ceph/ceph-csi/tree/master/docs/development-guide.md>)
and [coding style guidelines](<https://github.com/ceph/ceph-csi/tree/master/docs/coding.md>)
if you are interested to contribute to this repo.

## Troubleshooting

Please submit an issue at: [Issues](https://github.com/ceph/ceph-csi/issues)

## Contact

Please use the following to reach members of the community:

- Slack: Join our [slack channel](https://cephcsi.slack.com) to discuss
  about anything related to this project. You can join the slack by
  this [invite link](https://bit.ly/32vcEfO )
- Forums: [ceph-csi](https://groups.google.com/forum/#!forum/ceph-csi)
- Twitter: [@CephCsi](https://twitter.com/CephCsi)
