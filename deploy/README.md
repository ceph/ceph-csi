# Deploying Jenkins Jobs through OpenShift

This `deploy/` directory contains the configuration to prepare running Jenkins
Job Builder on OpenShift and update/add Jenkins Jobs in an environment hosted
in the same OpenShift project.

The used Jenkins environment is expected to be deployed already. This is done
by the CentOS CI team when a [request for CI resources](ci_request) is handled.
The deploying and configuration of Jenkins is therefor not part of this
document.

## Building the Jenkins Job Builder container image

OpenShift has a feature called ImageStreams. This can be used to build the
container image that contains the `jenkins-jobs` executable to test and
update/add jobs in a Jenkins environment.

All `.yaml` files in this directory need to be pushed into OpenShift, use `oc
create -f <file>` for that.

- the `Dockerfile` uses `pip` to install `jenkins-jobs`, the BuildConfig object
  in OpenShift can then be used to build the image
- `checkout-repo.sh` will be included in the container image, and checks out
  the `ci/centos` branch of the repository
- together with the `Makefile` (checked out with `checkout-repo.sh`), the
  Jenkins Jobs can be validated or deployed
- `jjb-buildconfig.yaml` creates the ImageStream and the BuildConfig objects.
  Once created with `oc create`, the OpenShift Console shows a `Build` button
  for the `jjb` image under the Builds/Builds menu
- `jjb-config.yaml` is the `/etc/jenkins_jobs/jenkins_jobs.ini` configuration
  files that contains username, password/token and URL to the Jenkins instance
  (**edit this file before pushing to OpenShift**)
- `jjb-validate.yaml` is the OpenShift Job that creates a Pod, runs the
  validation test and exits. The job needs to be deleted from OpenShift before
  it can be run again.
- `jjb-deploy.yaml` is the OpenShift Job that creates a Pod, runs
  `jenkins-jobs` to push the new jobs to the Jenkins environment. This pod uses
   the jjb-config ConfigMap to connect and login to the Jenkins instance. The
   job needs to be deleted from OpenShift before it can be run again.
- `jjb.sh` is a helper script that can be used to validate/deploy the Jenkins
  Jobs in the parent directory. It creates the validate or deploy job, waits
  until the job finishes, shows the log and exits with 0 on success. This
  script can be used in Jenkins Jobs to automate the validation and deployment of
  jobs.

[ci_request]: https://wiki.centos.org/QaWiki/CI/GettingStarted
