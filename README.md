# Continuous Integration Jobs for the CentOS CI

- [dedicated Jenkins instance][ceph_csi_ci] for Ceph-CSI
- Jenkins is hosted on [OpenShift in the CentOS CI][app_ci_centos_org]
- scripts and Jenkins jobs are hosted in the Ceph-CSI repository (ci/centos
  branch)
- a Jenkins Pipeline is used to reserve bare metal system(s), and run jobs on
  those systems

## Repository/Branch Structure

This is the `ci/centos` branch, where all the scripts for the Jenkins jobs are
maintained. The tests that are executed by the jobs are part of the normal
projects branches.

As an example, the `containerized-tests` Jenkins job consists out of the
following files:

- `jobs/containerized-tests.yaml` is a [Jenkins Job Builder][jjb] configuration
  that describes the events when the job should get run and fetches the
  `.groovy` file from the git repository/branch
- `containerized-tests.groovy` is the [Jenkins Pipeline][pipeline] that
  contains the stages for the Jenkins Job itself. In order to work with [the
  bare-metal machines from the CentOS CI][centos_ci_hw], it executes the
  following stages:

  1. dynamically allocate a Jenkins Slave (`node('cico-workspace')`) with tools
     and configuration to request a bare-metal machine
  1. checkout the `centos/ci` branch of the repository, which contains scripts
     for provisioning and preparing the environment for running tests
  1. reserve a bare-metal machine with `cico` (configured on the Jenkins Slave)
  1. provision the reserved bare-metal machine with additional tools and
     dependencies to run the test (see `prepare.sh` below)
  1. run `make containerized-tests` and `make containerized-build` in parallel
  1. as final step, return the bare-metal machine to the CentOS CI for other
     users (it will be re-installed with a minimal CentOS environment again)

- `e2e.groovy` is the Jenkins Pipeline responsible for running End-to-End tests on
  a multi-node kubernetes cluster hosted on [Centos CI][centos_ci].
  It verifies complete e2e functionalities for a corresponding Pull request
  on [ceph-csi][git_repo] repository.
  It executes the following stages:

  1. dynamically allocate a Jenkins Slave (`node('cico-workspace')`) with tools
     and configuration to request a bare-metal machine.
  1. checkout the `centos/ci` branch of the repository, which contains scripts
     for provisioning and preparing the environment for running tests.
  1. reserve a bare-metal machine with `cico` (configured on the Jenkins Slave);
     retry if not immediately available.
  1. provision the reserved bare-metal machine with additional tools and
     dependencies to run the test (see `prepare.sh` below).
  1. set up a multi-node k8s cluster on the bare-metal machine for performing
     e2e tests (see `multi-node-k8s.sh` below).
  1. deploy rook on the multi node kubernetes cluster.
  1. run the e2e tests on the configured setup against the corresponding
     pull request on the [ceph-csi][git_repo]
     repository.
  1. as final step, return the bare-metal machine to the CentOS CI for other
     users (it will be re-installed with a minimal CentOS environment again).

- `prepare.sh` installs dependencies for the test, and checks out the git
  repository and branch (or Pull Request) that contains the commits to be
  tested (and the test itself)

- `multi-node-k8s.sh` installs the dependencies, and sets up a multi-node
  kubernetes cluster on the bare-metal machine.

## Deploying the Jenkins Jobs

The Jenkins Jobs are described in Jenkins Job Builder configuration files and
Jenkins Pipelines. These need to be imported in the Jenkins instance before
they can be run. Importing is done with the `jenkins-jobs` command, which runs
in a `jjb` container. To build the container, and provide the configuration for
Jenkins Job Builder, see the [documentation in the `deploy/`
directory](deploy/README.md).

[ceph_csi_ci]: https://jenkins-ceph-csi.apps.ocp.ci.centos.org
[app_ci_centos_org]: https://console-openshift-console.apps.ocp.ci.centos.org/k8s/cluster/projects/ceph-csi
[jjb]: https://jenkins-job-builder.readthedocs.io/en/latest/index.html
[pipeline]: https://docs.openstack.org/infra/jenkins-job-builder/project_pipeline.html
[centos_ci_hw]: https://wiki.centos.org/QaWiki/PubHardware
[centos_ci]: https://wiki.centos.org/QaWiki/CI
[git_repo]: https://github.com/ceph/ceph-csi
