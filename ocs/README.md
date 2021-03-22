# Ceph-CSI Stream

Ceph-CSI Stream is the Red Hat downstream project that contains the pre-release
state of Ceph-CSI as used in the OpenShift Container Storage product.

## Git Repository

### Branches

This GitHub repository contains branches for different product versions.

### Pull Requests

Once
the product planning entered feature freeze, only backports with related
Bugzilla references will be allowed to get merged.

### Downstream-Only Changes

For working with the downstream tools, like OpenShift CI, there are a few
changes required that are not suitable for the upstream Ceph-CSI project.

1. `OWNERS` file: added with maintainers for reviewing and approving PRs
1. `ocs/` directory: additional files (like this `README.md`)
1. `ocs/Containerfile`: used to build the quay.io/ocs-dev/ceph-csi image

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

[ocp-release]: https://github.com/openshift/release/tree/master/ci-operator/config/openshift/ceph-csi
[bz-config]: https://github.com/openshift/release/blob/master/core-services/prow/02_config/_plugins.yaml
