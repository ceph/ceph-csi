# Continuous Integration Jobs for the CentOS CI

- [dedicated Jenkins instance][ceph_csi_ci] for Ceph-CSI
- Jenkins is hosted on [OpenShift in the CentOS CI][app_ci_centos_org]
- Scripts and Jenkins jobs are hosted in the Ceph-CSI repository (ci/centos
  branch)
- A Jenkins Pipeline is used to reserve bare metal system(s), and run jobs on
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
  contains the stages for the Jenkins Job itself. In order to work with the
  bare-metal machines from the CentOS CI, it executes the following stages:

  1. Dynamically allocate a Jenkins Slave (`node('cico-workspace')`) with tools
     and configuration to request a bare-metal machine
  1. Checkout the `centos/ci` branch of the repository, which contains scripts
     for provisioning and preparing the environment for running tests
  1. Reserve a bare-metal machine with `duffy` (configured on the Jenkins
     Slave)
  1. Provision the reserved bare-metal machine with additional tools and
     dependencies to run the test (see `prepare.sh` below)
  1. Run `make containerized-tests` and `make containerized-build` in parallel
  1. As final step, return the bare-metal machine to the CentOS CI for other
     users (it will be re-installed with a minimal CentOS environment again)

- `prepare.sh` installs dependencies for the test, and checks out the git
  repository and branch (or Pull Request) that contains the commits to be
  tested (and the test itself)

## Deploying the Jenkins Jobs

The Jenkins Jobs are described in Jenkins Job Builder configuration files and
Jenkins Pipelines. These need to be imported in the Jenkins instance before
they can be run. Importing is done with the `jenkins-jobs` command, which runs
in a `jjb` container. To build the container, and provide the configuration for
Jenkins Job Builder, see the [documentation in the `deploy/`
directory](deploy/README.md).

[ceph_csi_ci]: https://jenkins-ceph-csi.apps.ocp.cloud.ci.centos.org
[app_ci_centos_org]: https://console-openshift-console.apps.ocp.cloud.ci.centos.org/k8s/cluster/projects/ceph-csi
[jjb]: https://jenkins-job-builder.readthedocs.io/en/latest/index.html
[pipeline]: https://jenkins-job-builder.readthedocs.io/en/latest/project_pipeline.html
