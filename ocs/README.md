# Ceph-CSI Stream

Ceph-CSI Stream is the Red Hat downstream project that contains the pre-release
state of Ceph-CSI as used in the OpenShift Container Storage product.

## Git Repository

### Branches

This GitHub repository contains branches for different product versions.

## Backports

All changes in this repository are *backports* from the [upstream
project][upstream-ceph-csi]. There should be no functional changes (only
process/CI/building/..) in this repository compared to the upstream project.
Fixes for any of the release branches should first land in the master branch
before they may be backported to the release branch. A backport for the oldest
release should also be backported to all the newer releases in order to prevent
re-introducing a bug when a user updates.

### Sync `master` with upstream `ceph/ceph-csi:devel`

Syncing branches (including the `master` branch) from upstream should be done
with a Pull-Request. To create a PR that syncs the latest changes from
`ceph/ceph-csi:devel` into the `master branch`, [click here][sync-pr].

### Backporting changes from the `master` to `release-*` branches

Once a PR has been merged in the master branch that fixes an issue. A new PR
with the backport can be created. The easiest way is to use a command like

```
/cherry-pick release-4.8
```

The **openshift-cherrypick-robot** will automatically create a new PR for the
selected branch.

### Pull Requests

Once the product planning enters feature freeze, only backports with related
Bugzilla references will be allowed to get merged.

### Downstream-Only Changes

For working with the downstream tools, like OpenShift CI, there are a few
changes required that are not suitable for the upstream Ceph-CSI project.

1. `OWNERS` file: added with maintainers for reviewing and approving PRs
1. `ocs/` directory: additional files (like this `README.md`)
1. `ocs/Containerfile`: used to build the quay.io/ocs-dev/ceph-csi image
1. `.github/PULL_REQUEST_TEMPLATE/00_ocs-*`: guidance for creating PRs

## Continious Integration

OpenShift CI (Prow) is used for testing the changes that land in this GitHub
repository. The configuration of the jobs can be found in the [OpenShift
Release repository][ocp-release].

### Bugzilla Plugin

PRs that need a Bugzilla reference are handled by the Bugzilla Plugin which
runs as part of Prow. The configuration gates the requirement on BZs to be
linked, before the tests will pass and the PR can be merged. Once a branch is
added to the GitHub repository, [the configuration][bz-config] needs adaption
for the new branch as well.

[upstream-ceph-csi]: https://github.com/ceph/ceph-csi
[sync-pr]: https://github.com/openshift/ceph-csi/compare/master...ceph:devel
[ocp-release]: https://github.com/openshift/release/tree/master/ci-operator/config/openshift/ceph-csi
[bz-config]: https://github.com/openshift/release/blob/master/core-services/prow/02_config/_plugins.yaml
