# Ceph CSI driver Release Process

- [Ceph CSI driver Release Process](#Ceph-csi-driver-release-process)
  - [Introduction](#introduction)
  - [Versioning](#versioning)
  - [Tagging repositories](#tagging-repositories)
  - [Release process](#release-process)

## Introduction

This document provides details about Ceph CSI driver releases.

## Versioning

The Ceph CSI driver project uses
[semantic versioning](http://semver.org/)
for all releases.
Semantic versions are comprised of three
fields in the form:

```MAJOR.MINOR.PATCH```

For examples: `1.0.0`, `1.0.0-rc.2`.

Semantic versioning is used since the version
number is able to convey clear information about
how a new version relates to the previous version.
For example, semantic versioning can also provide
assurances to allow users to know when they must
upgrade compared with when they might want to upgrade:

- When `PATCH` increases, the new release contains important **security fixes**,
general bug fixes  and an upgrade is recommended.

 The patch field can contain extra details after the number.
 Dashes denote pre-release versions.`1.0.0-rc.2` in the example
 denotes the second release candidate for release `1.0.0`.

- When `MINOR` increases, the new release adds **new features**
and it must be backward compatible.

- When `MAJOR` increases, the new release adds **new features,
  bug fixes, or both** and which *changes the behavior from
  the previous release* (may be backward incompatible).

## Tagging repositories

The tag name must begin with "v" followed by the version number, conforming to
the [versioning](#versioning) requirements (e.g. a tag of `v1.0.0-rc2` for
version `1.0.0-rc2`). This tag format is used by the Travis CI infrastructure to
properly upload and tag releases to Quay and Docker Hub.

## Release process

The Release Owner must follow the following process, which is
designed to ensure clarity, quality, stability, and auditability
of each release:

- Raise an issue and assign to themselves.

[new issue in this repository](https://github.com/ceph/ceph-csi/issues/new)

  This issue is used to track the progress of the release with maximum visibility.

- Paste the release checklist into the issue.

  This is useful for tracking so that the stage of the release is visible
  to all interested parties. This checklist could be a list of issues/PRs
  tracked for a release. The issues/PRs will be labelled with release tag.
  For example,  a label called `1.0.0` ( for release version 1.0.0) can be
  attached to issues and PRs for better tracking release items.

- Once all steps are complete, close the issue.

