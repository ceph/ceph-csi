# Development Guide

## New to Go

Ceph-csi is written in Go and if you are new to the language,
it is **highly** encouraged to:

* Take the [A Tour of Go](http://tour.golang.org/welcome/1) course.
* [Set up](https://golang.org/doc/code.html) Go development environment on your machine.
* Read [Effective Go](https://golang.org/doc/effective_go.html) for best practices.

## Development Workflow

### Workspace and repository setup

* [Download](https://golang.org/dl/) Go (>=1.11.x) and
   [install](https://golang.org/doc/install) it on your system.
* Setup the [GOPATH](http://www.g33knotes.org/2014/07/60-second-count-down-to-go.html)
   environment.
* Ceph-CSI uses the native Ceph libaries through the [go-ceph
   package](https://github.com/ceph/go-ceph). It is required to install the
   Ceph C headers in order to compile Ceph-CSI. The packages are called
   `libcephfs-devel`, `librados-devel` and `librbd-devel` on many Linux
   distributions. See the [go-ceph installaton
   instructions](https://github.com/ceph/go-ceph#installation) for more
   details.
* Run `$ go get -d github.com/ceph/ceph-csi`
   This will just download the source and not build it. The downloaded source
   will be at `$GOPATH/src/github.com/ceph/ceph-csi`
* Fork the [ceph-csi repo](https://github.com/ceph/ceph-csi) on Github.
* Add your fork as a git remote:
   `$ git remote add fork https://github.com/<your-github-username>/ceph-csi`

> Editors: Our favorite editor is vim with the [vim-go](https://github.com/fatih/vim-go)
> plugin, but there are many others like [vscode](https://github.com/Microsoft/vscode-go)

### Building Ceph-CSI

To build ceph-csi locally run:
`$ make`

To build ceph-csi in a container:
`$ make containerized-build`

The built binary will be present under `_output/` directory.

### Running Ceph-CSI tests in a container

Once the changes to the sources compile, it is good practise to run the tests
that validate the style and other basics of the source code. Execute the unit
tests (in the `*_test.go` files) and check the formatting of YAML files,
MarkDown documents and shell scripts:

`$ make containerized-test`

It is also possible to run only selected tests, these are the targets in the
`Makefile` in the root of the project. For example, run the different static
checks with:

`$ make containerized-test TARGET=static-check`

In addition to running tests locally, each Pull Request that is created will
trigger Continous Integration tests that include the `containerized-test`, but
also additional functionality tests that are defined under the `e2e/`
directory.

### Code contribution workflow

ceph-csi repository currently follows GitHub's
[Fork & Pull] (<https://help.github.com/articles/about-pull-requests/>) workflow
for code contributions.

Please read the [coding guidelines](coding.md) document before submitting a PR.

Here is a short guide on how to work on a new patch.  In this example, we will
work on a patch called *hellopatch*:

* `$ git checkout master`
* `$ git pull`
* `$ git checkout -b hellopatch`

Do your work here and commit.

Run the test suite, which includes linting checks, static code check, and unit
tests:

`$ make test`

Certain unit tests may require extended permissions or other external resources
that are not available by default. To run these tests as well, export the
environment variable `CEPH_CSI_RUN_ALL_TESTS=1` before running the tests.

You will need to provide unit tests and functional tests for your changes
wherever applicable.

Once you are ready to push, you will type the following:

`$ git push fork hellopatch`

**Creating A Pull Request:**
When you are satisfied with your changes, you will then need to go to your repo
in GitHub.com and create a pull request for your branch. Automated tests will
be run against the pull request. Your pull request will be reviewed and merged.

If you are planning on making a large set of changes or a major architectural
change it is often desirable to first build a consensus in an issue discussion
and/or create an initial design doc PR. Once the design has been agreed upon
one or more PRs implementing the plan can be made.

**Review Process:**
Once your PR has been submitted for review the following critieria will
need to be met before it will be merged:

* Each PR needs reviews accepting the change from at least two developers
* for merging
  * It is common to request reviews from those reviewers automatically suggested
  * by github
* Each PR needs to have been open for at least 24 working hours to allow for
* community feedback
  * The 24 working hours counts hours occuring Mon-Fri in the local timezone
  * of the submitter
* Each PR must be fully updated to master and tests must have passed

When the criteria are met, a project maintainer can merge your changes into
the project's master branch.

### Backport a Fix to a Release Branch

The flow for getting a fix into a release branch is:

1. Open a PR to merge the changes to master following the process outlined above.
1. Add the backport label to that PR such as `backport-to-release-vX.Y.Z`
1. After your PR is merged to master, the mergify bot will automatically open a
   PR with your commits backported to the release branch
1. If there are any conflicts you will need to resolve them by pulling the
   branch, resolving the conflicts and force push back the branch
1. After the CI is green, the bot will automatically merge the backport PR.
