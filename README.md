# Ceph CSI

[![GitHub release](https://img.shields.io/github/release/ceph/ceph-csi/all.svg)](https://github.com/ceph/ceph-csi/releases)
[![Go Report
Card](https://goreportcard.com/badge/github.com/ceph/ceph-csi)](https://goreportcard.com/report/github.com/ceph/ceph-csi)
[![TODOs](https://badgen.net/https/api.tickgit.com/badgen/github.com/ceph/ceph-csi/devel)](https://www.tickgit.com/browse?repo=github.com/ceph/ceph-csi&branch=devel)

- [Ceph CSI](#ceph-csi)
  - [Overview](#overview)
  - [Project status](#project-status)
  - [Known to work CO platforms](#known-to-work-co-platforms)
  - [Support Matrix](#support-matrix)
    - [Ceph-CSI features and available versions](#ceph-csi-features-and-available-versions)
    - [CSI spec and Kubernetes version compatibility](#csi-spec-and-kubernetes-version-compatibility)
  - [Ceph CSI Container images and release compatibility](#ceph-csi-container-images-and-release-compatibility)
  - [Contributing to this repo](#contributing-to-this-repo)
  - [Troubleshooting](#troubleshooting)
  - [Weekly Bug Triage call](#weekly-bug-triage-call)
  - [Dev standup](#dev-standup)
  - [Contact](#contact)

This repo contains Ceph
[Container Storage Interface (CSI)](https://github.com/container-storage-interface/)
driver for RBD, CephFS and kubernetes sidecar deployment yamls of provisioner,
attacher, resizer, driver-registrar and snapshotter for supporting CSI functionalities.

## Overview

Ceph CSI plugins implement an interface between CSI enabled Container Orchestrator
(CO) and Ceph cluster. It allows dynamically provisioning Ceph volumes and
attaching them to workloads.

Independent CSI plugins are provided to support RBD and CephFS backed volumes,

- For details about configuration and deployment of RBD plugin, please refer
  [rbd doc](https://github.com/ceph/ceph-csi/blob/devel/docs/deploy-rbd.md) and
  for CephFS plugin configuration and deployment please
  refer [cephFS doc](https://github.com/ceph/ceph-csi/blob/devel/docs/deploy-cephfs.md).
- For example usage of RBD and CephFS CSI plugins, see examples in `examples/`.
- Stale resource cleanup, please refer [cleanup doc](docs/resource-cleanup.md).

NOTE:

- Ceph CSI **`Arm64`** support is experimental.

## Project status

Status: **GA**

## Known to work CO platforms

Ceph CSI drivers are currently developed and tested **exclusively** on Kubernetes
environments.

| Ceph CSI Version | Container Orchestrator Name | Version Tested|
| -----------------| --------------------------- | --------------|
| v3.4.0 | Kubernetes | v1.20, v1.21, v1.22|
| v3.3.0 | Kubernetes | v1.20, v1.21, v1.22|

There is work in progress to make this CO independent and thus
support other orchestration environments (Nomad, Mesos..etc) in the future.

NOTE:

The supported window of Ceph CSI versions  is known as "N.(x-1)":
(N (Latest major release) . (x (Latest minor release) - 1)).

For example, if Ceph CSI latest major version is `3.4.0` today, support is
provided for the versions above `3.3.0`. If users are running an unsupported
Ceph CSI version, they will be asked to upgrade when requesting support for the
cluster.

## Support Matrix

### Ceph-CSI features and available versions

Please refer [rbd nbd mounter](./docs/rbd-nbd.md#support-matrix)
for its support details.

| Plugin | Features                                                  | Feature Status | CSI Driver Version | CSI Spec Version | Ceph Cluster Version | Kubernetes Version |
| ------ | --------------------------------------------------------- | -------------- | ------------------ | ---------------- | -------------------- | ------------------ |
| RBD    | Dynamically provision, de-provision Block mode RWO volume | GA             | >= v1.0.0          | >= v1.0.0        | Nautilus (>=14.0.0)  | >= v1.14.0         |
|        | Dynamically provision, de-provision Block mode RWX volume | GA             | >= v1.0.0          | >= v1.0.0        | Nautilus (>=14.0.0)  | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode RWO volume  | GA             | >= v1.0.0          | >= v1.0.0        | Nautilus (>=14.0.0)  | >= v1.14.0         |
|        | Provision File Mode ROX volume from snapshot              | Alpha          | >= v3.0.0          | >= v1.0.0        | Nautilus (>=v14.2.2) | >= v1.17.0         |
|        | Provision File Mode ROX volume from another volume        | Alpha          | >= v3.0.0          | >= v1.0.0        | Nautilus (>=v14.2.2) | >= v1.16.0         |
|        | Provision Block Mode ROX volume from snapshot             | Alpha          | >= v3.0.0          | >= v1.0.0        | Nautilus (>=v14.2.2) | >= v1.17.0         |
|        | Provision Block Mode ROX volume from another volume       | Alpha          | >= v3.0.0          | >= v1.0.0        | Nautilus (>=v14.2.2) | >= v1.16.0         |
|        | Creating and deleting snapshot                            | Beta           | >= v1.0.0          | >= v1.0.0        | Nautilus (>=14.0.0)  | >= v1.17.0         |
|        | Provision volume from snapshot                            | Beta           | >= v1.0.0          | >= v1.0.0        | Nautilus (>=14.0.0)  | >= v1.17.0         |
|        | Provision volume from another volume                      | Beta           | >= v1.0.0          | >= v1.0.0        | Nautilus (>=14.0.0)  | >= v1.16.0         |
|        | Expand volume                                             | Beta           | >= v2.0.0          | >= v1.1.0        | Nautilus (>=14.0.0)  | >= v1.15.0         |
|        | Volume/PV Metrics of File Mode Volume                     | Beta           | >= v1.2.0          | >= v1.1.0        | Nautilus (>=14.0.0)  | >= v1.15.0         |
|        | Volume/PV Metrics of Block Mode Volume                    | Beta           | >= v1.2.0          | >= v1.1.0        | Nautilus (>=14.0.0)  | >= v1.21.0         |
|        | Topology Aware Provisioning Support                       | Alpha          | >= v2.1.0          | >= v1.1.0        | Nautilus (>=14.0.0)  | >= v1.14.0         |
| CephFS | Dynamically provision, de-provision File mode RWO volume  | Beta           | >= v1.1.0          | >= v1.0.0        | Nautilus (>=14.2.2)  | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode RWX volume  | Beta           | >= v1.1.0          | >= v1.0.0        | Nautilus (>=v14.2.2) | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode ROX volume  | Alpha          | >= v3.0.0          | >= v1.0.0        | Nautilus (>=v14.2.2) | >= v1.14.0         |
|        | Creating and deleting snapshot                            | Beta           | >= v3.1.0          | >= v1.0.0        | Octopus (>=v15.2.3)  | >= v1.17.0         |
|        | Provision volume from snapshot                            | Beta           | >= v3.1.0          | >= v1.0.0        | Octopus (>=v15.2.3)  | >= v1.17.0         |
|        | Provision volume from another volume                      | Beta           | >= v3.1.0          | >= v1.0.0        | Octopus (>=v15.2.3)  | >= v1.16.0         |
|        | Expand volume                                             | Beta           | >= v2.0.0          | >= v1.1.0        | Nautilus (>=v14.2.2) | >= v1.15.0         |
|        | Volume/PV Metrics of File Mode Volume                     | Beta           | >= v1.2.0          | >= v1.1.0        | Nautilus (>=v14.2.2) | >= v1.15.0         |

`NOTE`: The `Alpha` status reflects possible non-backward
compatible changes in the future, and is thus not recommended
for production use.

### CSI spec and Kubernetes version compatibility

Please refer to the [matrix](https://kubernetes-csi.github.io/docs/#kubernetes-releases)
in the Kubernetes documentation.

## Ceph CSI Container images and release compatibility

| Ceph CSI Release/Branch | Container image name         | Image Tag |
| ----------------------- | ---------------------------- | --------- |
| devel (Branch)          | quay.io/cephcsi/cephcsi      | canary    |
| v3.4.0 (Release)        | quay.io/cephcsi/cephcsi      | v3.4.0    |
| v3.3.1 (Release)        | quay.io/cephcsi/cephcsi      | v3.3.1    |
| v3.3.0 (Release)        | quay.io/cephcsi/cephcsi      | v3.3.0    |

| Deprecated Ceph CSI Release/Branch | Container image name | Image Tag |
| ----------------------- | --------------------------------| --------- |
| v3.2.2 (Release)        | quay.io/cephcsi/cephcsi         | v3.2.2    |
| v3.2.1 (Release)        | quay.io/cephcsi/cephcsi         | v3.2.1    |
| v3.2.0 (Release)        | quay.io/cephcsi/cephcsi         | v3.2.0    |
| v3.1.2 (Release)        | quay.io/cephcsi/cephcsi         | v3.1.2    |
| v3.1.1 (Release)        | quay.io/cephcsi/cephcsi         | v3.1.1    |
| v3.1.0 (Release)        | quay.io/cephcsi/cephcsi         | v3.1.0    |
| v3.0.0 (Release)        | quay.io/cephcsi/cephcsi         | v3.0.0    |
| v2.1.2 (Release)        | quay.io/cephcsi/cephcsi         | v2.1.2    |
| v2.1.1 (Release)        | quay.io/cephcsi/cephcsi         | v2.1.1    |
| v2.1.0 (Release)        | quay.io/cephcsi/cephcsi         | v2.1.0    |
| v2.0.1 (Release)        | quay.io/cephcsi/cephcsi         | v2.0.1    |
| v2.0.0 (Release)        | quay.io/cephcsi/cephcsi         | v2.0.0    |
| v1.2.2 (Release)        | quay.io/cephcsi/cephcsi         | v1.2.2    |
| v1.2.1 (Release)        | quay.io/cephcsi/cephcsi         | v1.2.1    |
| v1.2.0 (Release)        | quay.io/cephcsi/cephcsi         | v1.2.0    |
| v1.1.0 (Release)        | quay.io/cephcsi/cephcsi         | v1.1.0    |
| v1.0.0 (Branch)         | quay.io/cephcsi/cephfsplugin    | v1.0.0    |
| v1.0.0 (Branch)         | quay.io/cephcsi/rbdplugin       | v1.0.0    |

## Contributing to this repo

Please follow [development-guide](<https://github.com/ceph/ceph-csi/tree/devel/docs/development-guide.md>)
and [coding style guidelines](<https://github.com/ceph/ceph-csi/tree/devel/docs/coding.md>)
if you are interested to contribute to this repo.

## Troubleshooting

Please submit an issue at: [Issues](https://github.com/ceph/ceph-csi/issues)

## Weekly Bug Triage call

We conduct weekly bug triage calls at our slack channel on Tuesdays.
More details are available [here](https://github.com/ceph/ceph-csi/issues/463)

## Dev standup

A regular dev standup takes place every [Monday,Tuesday and Thursday at
12:00 PM UTC](https://meet.google.com/nnn-txfp-cge). Convert to your local
timezone by executing command `date -d "12:00 UTC"` on terminal

Any changes to the meeting schedule will be added to the [agenda
doc](https://docs.google.com/document/d/1K1aerdMpraIh56-skdoEoVF9RZrO4NUcbHtjN-f3u1s).

Anyone who wants to discuss the direction of the project, design and
implementation reviews, or general questions with the broader community is
welcome and encouraged to join.

- Meeting link: <https://meet.google.com/nnn-txfp-cge>
- [Current agenda](https://docs.google.com/document/d/1K1aerdMpraIh56-skdoEoVF9RZrO4NUcbHtjN-f3u1s)

## Contact

Please use the following to reach members of the community:

- Slack: Join our [slack channel](https://cephcsi.slack.com) to discuss
  anything related to this project. You can join the slack by
  this [invite link](https://bit.ly/2MeS4KY )
- Forums: [ceph-csi](https://groups.google.com/forum/#!forum/ceph-csi)
- Twitter: [@CephCsi](https://twitter.com/CephCsi)
