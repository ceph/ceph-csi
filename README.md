# Ceph CSI

[![GitHub release](https://img.shields.io/github/release/ceph/ceph-csi/all.svg)](https://github.com/ceph/ceph-csi/releases)
[![Mergify Status](https://img.shields.io/endpoint.svg?url=https://api.mergify.com/v1/badges/ceph/ceph-csi&style=flat)](https://mergify.com)
[![Go Report
Card](https://goreportcard.com/badge/github.com/ceph/ceph-csi)](https://goreportcard.com/report/github.com/ceph/ceph-csi)
[![TODOs](https://badgen.net/https/api.tickgit.com/badgen/github.com/ceph/ceph-csi/devel)](https://www.tickgit.com/browse?repo=github.com/ceph/ceph-csi&branch=devel)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/5940/badge)](https://bestpractices.coreinfrastructure.org/projects/5940)

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

This repo contains the Ceph
[Container Storage Interface (CSI)](https://github.com/container-storage-interface/)
driver for RBD, CephFS and Kubernetes sidecar deployment YAMLs to support CSI
functionality:  provisioner, attacher, resizer, driver-registrar and snapshotter.

## Overview

Ceph CSI plugins implement an interface between a CSI-enabled Container Orchestrator
(CO) and Ceph clusters. They enable dynamically provisioning Ceph volumes and
attaching them to workloads.

Independent CSI plugins are provided to support RBD and CephFS backed volumes,

- For details about configuration and deployment of RBD plugin, please refer
  [rbd doc](https://github.com/ceph/ceph-csi/blob/devel/docs/deploy-rbd.md) and
  for CephFS plugin configuration and deployment please
  refer [cephFS doc](https://github.com/ceph/ceph-csi/blob/devel/docs/deploy-cephfs.md).
- For example usage of the RBD and CephFS CSI plugins, see examples in `examples/`.
- Stale resource cleanup, please refer [cleanup doc](docs/resource-cleanup.md).

NOTE:

- Ceph CSI **`Arm64`** support is experimental.

## Project status

Status: **GA**

## Known to work CO platforms

Ceph CSI drivers are currently developed and tested **exclusively** in Kubernetes
environments.

| Ceph CSI Version | Container Orchestrator Name | Version Tested|
| -----------------| --------------------------- | --------------|
| v3.9.0 | Kubernetes | v1.25, v1.26, v1.27|
| v3.8.0 | Kubernetes | v1.24, v1.25, v1.26, v1.27|

There is work in progress to make this CO-independent and thus
support other orchestration environments (Nomad, Mesos..etc).

NOTE:

The supported window of Ceph CSI versions is "N.(x-1)":
(N (Latest major release) . (x (Latest minor release) - 1)).

For example, if the Ceph CSI latest major version is `3.9.0` today, support is
provided for the versions above `3.8.0`. If users are running an unsupported
Ceph CSI version, they will be asked to upgrade when requesting support.

## Support Matrix

### Ceph-CSI features and available versions

Please refer [rbd nbd mounter](./docs/rbd-nbd.md#support-matrix)
for its support details.

| Plugin | Features                                                  | Feature Status | CSI Driver Version | CSI Spec Version | Ceph Cluster Version | Kubernetes Version |
| ------ | --------------------------------------------------------- | -------------- | ------------------ | ---------------- | -------------------- | ------------------ |
| RBD    | Dynamically provision, de-provision Block mode RWO volume | GA             | >= v1.0.0          | >= v1.0.0        | Octopus (>=15.0.0)  | >= v1.14.0         |
|        | Dynamically provision, de-provision Block mode RWX volume | GA             | >= v1.0.0          | >= v1.0.0        | Octopus (>=15.0.0)  | >= v1.14.0         |
|        | Dynamically provision, de-provision Block mode RWOP volume| Alpha          | >= v3.5.0          | >= v1.5.0        | Octopus (>=15.0.0)  | >= v1.22.0         |
|        | Dynamically provision, de-provision File mode RWO volume  | GA             | >= v1.0.0          | >= v1.0.0        | Octopus (>=15.0.0)  | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode RWOP volume | Alpha          | >= v3.5.0          | >= v1.5.0        | Octopus (>=15.0.0)  | >= v1.22.0         |
|        | Provision File Mode ROX volume from snapshot              | Alpha          | >= v3.0.0          | >= v1.0.0        | Octopus (>=v15.0.0) | >= v1.17.0         |
|        | Provision File Mode ROX volume from another volume        | Alpha          | >= v3.0.0          | >= v1.0.0        | Octopus (>=v15.0.0) | >= v1.16.0         |
|        | Provision Block Mode ROX volume from snapshot             | Alpha          | >= v3.0.0          | >= v1.0.0        | Octopus (>=v15.0.0) | >= v1.17.0         |
|        | Provision Block Mode ROX volume from another volume       | Alpha          | >= v3.0.0          | >= v1.0.0        | Octopus (>=v15.0.0) | >= v1.16.0         |
|        | Creating and deleting snapshot                            | GA             | >= v1.0.0          | >= v1.0.0        | Octopus (>=15.0.0)  | >= v1.17.0         |
|        | Provision volume from snapshot                            | GA             | >= v1.0.0          | >= v1.0.0        | Octopus (>=15.0.0)  | >= v1.17.0         |
|        | Provision volume from another volume                      | GA             | >= v1.0.0          | >= v1.0.0        | Octopus (>=15.0.0)  | >= v1.16.0         |
|        | Expand volume                                             | Beta           | >= v2.0.0          | >= v1.1.0        | Octopus (>=15.0.0)  | >= v1.15.0         |
|        | Volume/PV Metrics of File Mode Volume                     | GA             | >= v1.2.0          | >= v1.1.0        | Octopus (>=15.0.0)  | >= v1.15.0         |
|        | Volume/PV Metrics of Block Mode Volume                    | GA             | >= v1.2.0          | >= v1.1.0        | Octopus (>=15.0.0)  | >= v1.21.0         |
|        | Topology Aware Provisioning Support                       | Alpha          | >= v2.1.0          | >= v1.1.0        | Octopus (>=15.0.0)  | >= v1.14.0         |
| CephFS | Dynamically provision, de-provision File mode RWO volume  | GA             | >= v1.1.0          | >= v1.0.0        | Octopus (>=15.0.0)  | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode RWX volume  | GA             | >= v1.1.0          | >= v1.0.0        | Octopus (>=v15.0.0) | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode ROX volume  | Alpha          | >= v3.0.0          | >= v1.0.0        | Octopus (>=v15.0.0) | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode RWOP volume | Alpha          | >= v3.5.0          | >= v1.5.0        | Octopus (>=15.0.0)  | >= v1.22.0         |
|        | Creating and deleting snapshot                            | GA             | >= v3.1.0          | >= v1.0.0        | Octopus (>=v15.2.4)  | >= v1.17.0         |
|        | Provision volume from snapshot                            | GA             | >= v3.1.0          | >= v1.0.0        | Octopus (>=v15.2.4)  | >= v1.17.0         |
|        | Provision volume from another volume                      | GA             | >= v3.1.0          | >= v1.0.0        | Octopus (>=v15.2.4)  | >= v1.16.0         |
|        | Expand volume                                             | Beta           | >= v2.0.0          | >= v1.1.0        | Octopus (>=v15.0.0) | >= v1.15.0         |
|        | Volume/PV Metrics of File Mode Volume                     | GA             | >= v1.2.0          | >= v1.1.0        | Octopus (>=v15.0.0) | >= v1.15.0         |
| NFS    | Dynamically provision, de-provision File mode RWO volume  | Alpha          | >= v3.6.0          | >= v1.0.0        | Pacific (>=16.2.0)   | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode RWX volume  | Alpha          | >= v3.6.0          | >= v1.0.0        | Pacific (>=16.2.0)   | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode ROX volume  | Alpha          | >= v3.6.0          | >= v1.0.0        | Pacific (>=16.2.0)   | >= v1.14.0         |
|        | Dynamically provision, de-provision File mode RWOP volume | Alpha          | >= v3.6.0          | >= v1.5.0        | Pacific (>=16.2.0)   | >= v1.22.0         |
|        | Expand volume                                             | Alpha          | >= v3.7.0          | >= v1.1.0        | Pacific (>=16.2.0)   | >= v1.15.0         |
|        | Creating and deleting snapshot                            | Alpha          | >= v3.7.0          | >= v1.1.0        | Pacific (>=16.2.0)   | >= v1.17.0         |
|        | Provision volume from snapshot                            | Alpha          | >= v3.7.0          | >= v1.1.0        | Pacific (>=16.2.0)   | >= v1.17.0         |
|        | Provision volume from another volume                      | Alpha          | >= v3.7.0          | >= v1.1.0        | Pacific (>=16.2.0)   | >= v1.16.0         |

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
| v3.9.0 (Release)        | quay.io/cephcsi/cephcsi      | v3.9.0    |
| v3.8.0 (Release)        | quay.io/cephcsi/cephcsi      | v3.8.0    |

| Deprecated Ceph CSI Release/Branch | Container image name | Image Tag |
| ----------------------- | --------------------------------| --------- |
| v3.7.2 (Release)        | quay.io/cephcsi/cephcsi         | v3.7.2    |
| v3.7.1 (Release)        | quay.io/cephcsi/cephcsi         | v3.7.1    |
| v3.7.0 (Release)        | quay.io/cephcsi/cephcsi         | v3.7.0    |
| v3.6.1 (Release)        | quay.io/cephcsi/cephcsi         | v3.6.1    |
| v3.6.0 (Release)        | quay.io/cephcsi/cephcsi         | v3.6.0    |
| v3.5.1 (Release)        | quay.io/cephcsi/cephcsi         | v3.5.1    |
| v3.5.0 (Release)        | quay.io/cephcsi/cephcsi         | v3.5.0    |
| v3.4.0 (Release)        | quay.io/cephcsi/cephcsi         | v3.4.0    |
| v3.3.1 (Release)        | quay.io/cephcsi/cephcsi         | v3.3.1    |
| v3.3.0 (Release)        | quay.io/cephcsi/cephcsi         | v3.3.0    |
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

A regular dev standup takes place every [Tuesday at
12:00 PM UTC](https://meet.google.com/vit-qdhw-nyh) Convert to your local
timezone by executing command `date -d "12:00 UTC"` on terminal

Any changes to the meeting schedule will be added to the [agenda
doc](https://hackmd.io/6GL90WFGQL-L4DcIfIAKeQ).

Anyone who wants to discuss the direction of the project, design and
implementation reviews, or general questions with the broader community is
welcome and encouraged to join.

- Meeting link: <https://meet.google.com/vit-qdhw-nyh>
- [Current agenda](https://hackmd.io/6GL90WFGQL-L4DcIfIAKeQ)

## Contact

Please use the following to reach members of the community:

- Slack: Join our [Slack channel](https://ceph-storage.slack.com) to discuss
  anything related to this project. You can join the Slack by this [invite
  link](https://bit.ly/40FQu7u)
- Forums: [ceph-csi](https://groups.google.com/forum/#!forum/ceph-csi)
- Twitter: [@CephCsi](https://twitter.com/CephCsi)
