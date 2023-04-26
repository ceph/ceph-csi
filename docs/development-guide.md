# Development Guide

## New to Go

Ceph-csi is written in Go and if you are new to the language,
it is **highly** encouraged to:

* Take the [A Tour of Go](http://tour.golang.org/welcome/1) course.
* [Set up](https://golang.org/doc/code.html) Go development environment on your machine.
* Read [Effective Go](https://golang.org/doc/effective_go.html) for best practices.

## Development Workflow

### Workspace and repository setup

* [Download](https://golang.org/dl/) Go (>=1.17.x) and
   [install](https://golang.org/doc/install) it on your system.
* Setup the [GOPATH](http://www.g33knotes.org/2014/07/60-second-count-down-to-go.html)
   environment.
* `CGO_ENABLED` is enabled by default, if `CGO_ENABLED` is set to `0` we need
  to set it to `1` as we need to build with go-ceph bindings.
* `GO111MODULE` is enabled by default, if `GO111MODULE` is set to `off` we need
  to set it to `on` as cephcsi uses go modules for dependency.
* Ceph-CSI uses the native Ceph libraries through the [go-ceph
   package](https://github.com/ceph/go-ceph). It is required to install the
   Ceph C headers in order to compile Ceph-CSI. The packages are called
   `librados-devel` and `librbd-devel` on many Linux distributions. See the
   [go-ceph installation
   instructions](https://github.com/ceph/go-ceph#installation) for more
   details.
* Run

    ```console
    go get -d github.com/ceph/ceph-csi
    ```

   This will just download the source and not build it. The downloaded source
   will be at `$GOPATH/src/github.com/ceph/ceph-csi`
* Fork the [ceph-csi repo](https://github.com/ceph/ceph-csi) on Github.
* Add your fork as a git remote:

    ```console
    git remote add fork https://github.com/<your-github-username>/ceph-csi
    ```

* Set up a pre-commit hook to catch issues locally.

   ```console
   pip install pre-commit
   pre-commit install
   ```

   See the [pre-commit installation
   instructions](https://pre-commit.com/#installation) for more
   details.

   Pre-commit will be now be triggered next time we commit changes.
   This will catch some trivial style nitpicks (if any),
   which will then need resolving. Once the warnings are resolved,
   the user will be allowed to proceed with the commit.
> Editors: Our favorite editor is vim with the [vim-go](https://github.com/fatih/vim-go)
> plugin, but there are many others like [vscode](https://github.com/Microsoft/vscode-go)

### Building Ceph-CSI

To build ceph-csi locally run:

```console
make
```

To build ceph-csi in a container:

```console
make containerized-build
```

The built binary will be present under `_output/` directory.

### Running Ceph-CSI tests in a container

Once the changes to the sources compile, it is good practise to run the tests
that validate the style and other basics of the source code. Execute the unit
tests (in the `*_test.go` files) and check the formatting of YAML files,
MarkDown documents and shell scripts:

```console
make containerized-test
```

It is also possible to run only selected tests, these are the targets in the
`Makefile` in the root of the project. For example, run the different static
checks with:

```console
make containerized-test TARGET=static-check
```

In addition to running tests locally, each Pull Request that is created will
trigger Continuous Integration tests that include the `containerized-test`, but
also additional functionality tests that are defined under the `e2e/`
directory.

### Code contribution workflow

ceph-csi repository currently follows GitHub's
[Fork & Pull] (<https://help.github.com/articles/about-pull-requests/>) workflow
for code contributions.

Please read the [coding guidelines](coding.md) document before submitting a PR.

#### Certificate of Origin

By contributing to this project you agree to the Developer Certificate of
Origin (DCO). This document was created by the Linux Kernel community and is a
simple statement that you, as a contributor, have the legal right to make the
contribution. See the [DCO](DCO) file for details.

Contributors sign-off that they adhere to these requirements by adding a
Signed-off-by line to commit messages. For example:

```
subsystem: This is my commit message

More details on what this commit does

Signed-off-by: Random J Developer <random@developer.example.org>
```

If you have already made a commit and forgot to include the sign-off, you can
amend your last commit to add the sign-off with the following command, which
can then be force pushed.

```console
git commit --amend -s
```

We use a [DCO bot](https://github.com/apps/dco) to enforce the DCO on each pull
request and branch commits.

#### Commit Messages

We follow a rough convention for commit messages that is designed to answer two
questions: what changed and why? The subject line should feature the what and
the body of the commit should describe the why.

```
cephfs: update cephfs resize

use cephfs resize to resize subvolume

Signed-off-by: Random J Developer <random@developer.example.org>
```

The format can be described more formally as follows:

```
<component>: <subject of the change>
<BLANK LINE>
<paragraph(s) with reason/description>
<BLANK LINE>
<signed-off-by>
```

The `component` in the subject of the commit message can be one of the following:

* `cephfs`: bugs or enhancements related to CephFS
* `rbd`: bugs or enhancements related to RBD
* `doc`: documentation updates
* `util`: utilities shared between components use `cephfs` or `rbd` if the
   change is only relevant for one of the type of storage
* `journal`: any of the journaling functionalities
* `helm`: deployment changes for the Helm charts
* `deploy`: updates to Kubernetes templates for deploying components
* `build`: anything related to building Ceph-CSI, the executable or container
   images
* `ci`: changes related to the Continuous Integration, or testing
* `e2e`: end-to-end testing updates
* `cleanup`: general maintenance and cleanup changes
* `revert`: undo a commit that was merged by mistake, use of one of the other
   components is in most cases recommended

The first line is the subject and should be no longer than 70 characters, the
second line is always blank, and other lines should be wrapped at 80 characters.
This allows the message to be easier to read on GitHub as well as in various
git tools.

Here is a short guide on how to work on a new patch.  In this example, we will
work on a patch called *hellopatch*:

```console
git checkout devel
git pull
git checkout -b hellopatch
```

Do your work here and commit.

Run the test suite, which includes linting checks, static code check, and unit
tests:

```console
make test
```

Certain unit tests may require extended permissions or other external resources
that are not available by default. To run these tests as well, export the
environment variable `CEPH_CSI_RUN_ALL_TESTS=1` before running the tests.

You will need to provide unit tests and functional tests for your changes
wherever applicable.

Once you are ready to push, you will type the following:

```console
git push fork hellopatch
```

**Creating A Pull Request:**
When you are satisfied with your changes, you will then need to go to your repo
in GitHub.com and create a pull request for your branch. Automated tests will
be run against the pull request. Your pull request will be reviewed and merged.

If you are planning on making a large set of changes or a major architectural
change it is often desirable to first build a consensus in an issue discussion
and/or create an initial design doc PR. Once the design has been agreed upon
one or more PRs implementing the plan can be made.

Pull requests get labelled by maintainers of the repository. Labels help
reviewers with selecting the pull requests of interest based on components:

* component/build: Issues and PRs related to compiling Ceph-CSI
* component/cephfs: Issues related to CephFS
* component/deployment: Helm chart, kubernetes templates and configuration Issues/PRs
* component/docs: Issues and PRs related to documentation
* component/journal: This PR has a change in volume journal
* component/rbd: Issues related to RBD
* component/testing: Additional test cases or CI work
* component/util: Utility functions shared between CephFS and RBD

There are other labels as well, to indicate dependencies between projects:

* dependency/ceph: depends on core Ceph functionality
* dependency/go-ceph: depends on go-ceph functionality
* dependency/k8s: depends on Kubernetes features
* dependency/rook: depends on, or requires changes in Rook
* rebase: update the version of an external component

A few labels interact with automation around the pull requests:

* ready-to-merge: This PR is ready to be merged and it doesn't need second review
* DNM: DO NOT MERGE (Mergify will not merge this PR)
* ci/skip/e2e: skip running e2e CI jobs
* ci/skip/multi-arch-build: skip building container images for different architectures
* ok-to-test: PR is ready for e2e testing.

**Review Process:**
Once your PR has been submitted for review the following criteria will
need to be met before it will be merged:

* Each PR needs reviews accepting the change from at least two developers for merging.
* Each PR needs approval from
  [ceph-csi-contributors](https://github.com/orgs/ceph/teams/ceph-csi-contributors)
  and
  [ceph-csi-maintainers](https://github.com/orgs/ceph/teams/ceph-csi-maintainers).
* It is common to request reviews from those reviewers automatically suggested
  by GitHub.
* Each PR needs to have been open for at least 24 working hours to allow for
  community feedback.
* The 24 working hours counts hours occurring Mon-Fri in the local timezone
  of the submitter.
* ceph-csi-maintainers/ceph-csi-contributors can add `ok-to-test` label to the
  pull request when they think it is ready for e2e testing. This is done to avoid
  load on the CI.
* Each PR must be fully updated to devel and tests must have passed
* If the PR is having trivial changes or the reviewer is confident enough that
  PR doesn't need a second review, the reviewer can set `ready-to-merge` label
  on the PR. The bot will merge the PR if it's having one approval and the
  label `ready-to-merge`.

When the criteria are met, a project maintainer can merge your changes into
the project's devel branch.

### Backport a Fix to a Release Branch

The flow for getting a fix into a release branch is:

1. Open a PR to merge the changes to devel following the process outlined above.
1. Add the backport label to that PR such as `backport-to-release-vX.Y.Z`
1. After your PR is merged to devel, the mergify bot will automatically open a
   PR with your commits backported to the release branch
1. If there are any conflicts you will need to resolve them by pulling the
   branch, resolving the conflicts and force push back the branch
1. After the CI is green, the bot will automatically merge the backport PR.

### Retriggering the CI Jobs

The CI Jobs gets triggered automatically on these events, such as on
opening fresh PRs, rebase of PRs and force pushing changes to existing PRs.

Right now, we also have below commands to manually retrigger the CI jobs

1. To retrigger all the CI jobs, comment the PR with command: `/retest all`

   **Note**:

   This will rerun all the jobs including the jobs which are already passed

1. To retrigger a specific CI job, comment the PR with command: `/retest <job-name>`

   example:

   ```
   /retest ci/centos/containerized-tests
   ```

**Caution**: Please do not retrigger the CI jobs without an understanding of
             the root cause, because:

* We may miss some important corner cases which are true negatives,
  and hard to reproduce
* Retriggering jobs for known failures can unnecessarily put CI resources
  under pressure

Hence, it is recommended that you please go through the CI logs first, if you
are certain about the flaky test failure behavior, then comment on the PR
indicating the logs about a particular test that went flaky and use the
appropriate command to retrigger the job[s].
If you are uncertain about the CI failure, we prefer that you ping us on
[Slack channel #ceph-csi](https://ceph-storage.slack.com) with more details on
failures before retriggering the jobs, we will be happy to help.

### Retesting failed Jobs

The CI Jobs gets triggered automatically on these events, such as on opening
fresh PRs, rebase of PRs and force pushing changes to existing PRs.

In case of failed we already documented steps  to manually
[retrigger](#retriggering-the-ci-jobs) the CI jobs. Sometime the tests might be
flaky which required manually retriggering always. We have newly added a github
action which runs periodically to retest the failed PR's. Below are the criteria
for auto retesting the failed PR.

* Analyze the logs and make sure its a flaky test.
* Pull Request should have required approvals.
* `ci/retest/e2e` label should be set on the PR.
