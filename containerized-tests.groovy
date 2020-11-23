def cico_retries = 16
def cico_retry_interval = 60
def ci_git_repo = 'https://github.com/ceph/ceph-csi'
def ci_git_branch = 'ci/centos'
def git_repo = 'https://github.com/ceph/ceph-csi'
def ref = "master"
def git_since = 'master'
def workdir = '/opt/build/go/src/github.com/ceph/ceph-csi'
def doc_change = 0
// private, internal container image repository
def ci_registry = 'registry-ceph-csi.apps.ocp.ci.centos.org'
def cached_image = "ceph-csi"
def use_test_image = 'USE_PULLED_IMAGE=yes'
def use_build_image = 'USE_PULLED_IMAGE=yes'

def ssh(cmd) {
	sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} '${cmd}'"
}

def podman_login(registry, username, passwd) {
	ssh "podman login --authfile=~/.podman-auth.json --username=${username} --password='${passwd}' ${registry}"
	ssh 'cp container-registry.conf /etc/containers/registries.conf'
}

def podman_pull(registry, image) {
	ssh "podman pull --authfile=~/.podman-auth.json ${registry}/${image} && podman tag ${registry}/${image} ${image}"
}

node('cico-workspace') {

	stage('checkout ci repository') {
		git url: "${ci_git_repo}",
			branch: "${ci_git_branch}",
			changelog: false
	}

	stage('checkout PR') {
		if (params.ghprbPullId != null) {
			ref = "pull/${ghprbPullId}/merge"
		}
		if (params.ghprbTargetBranch != null) {
			git_since = "${ghprbTargetBranch}"
		}

		sh "git clone --depth=1 --branch='${git_since}' '${git_repo}' ~/build/ceph-csi"
		sh "cd ~/build/ceph-csi && git fetch origin ${ref} && git checkout -b ${ref} FETCH_HEAD"
	}

	stage('check doc-only change') {
		doc_change = sh(
			script: "cd ~/build/ceph-csi && \${OLDPWD}/scripts/skip-doc-change.sh origin/${git_since}",
			returnStatus: true)
	}
	// if doc_change (return value of skip-doc-change.sh is 1, do not run the other stages
	if (doc_change == 1) {
		currentBuild.result = 'SUCCESS'
		return
	}

	stage('reserve bare-metal machine') {
		def firstAttempt = true
		retry(30) {
			if (!firstAttempt) {
				sleep(time: 5, unit: "MINUTES")
			}
			firstAttempt = false
			cico = sh(
				script: "cico node get -f value -c hostname -c comment --release=8 --retry-count=${cico_retries} --retry-interval=${cico_retry_interval}",
				returnStdout: true
			).trim().tokenize(' ')
			env.CICO_NODE = "${cico[0]}.ci.centos.org"
			env.CICO_SSID = "${cico[1]}"
		}
	}

	try {
		stage('prepare bare-metal machine') {
			sh 'scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no ./prepare.sh container-registry.conf root@${CICO_NODE}:'
			// TODO: already checked out the PR on the node, scp the contents?
			ssh "./prepare.sh --workdir=${workdir} --gitrepo=${git_repo} --ref=${ref}"
		}

		// run two jobs in parallel, one for the test container, and
		// one for the devel container
		//
		// - check if the PR modifies the container image files
		// - pull the container image from the repository of no
		//   modifications are detected
		stage('pull container images') {
			def rebuild_test_container = sh(
				script: "cd ~/build/ceph-csi && \${OLDPWD}/scripts/container-needs-rebuild.sh test origin/${git_since}",
				returnStatus: true)

			def rebuild_devel_container = sh(
				script: "cd ~/build/ceph-csi && \${OLDPWD}/scripts/container-needs-rebuild.sh devel origin/${git_since}",
				returnStatus: true)

			withCredentials([usernamePassword(credentialsId: 'container-registry-auth', usernameVariable: 'CREDS_USER', passwordVariable: 'CREDS_PASSWD')]) {
				podman_login(ci_registry, "${CREDS_USER}", "${CREDS_PASSWD}")
			}

			parallel test: {
				node('cico-workspace') {
					if (rebuild_test_container == 10) {
						// container needs rebuild, don't pull
						use_test_image = 'USE_PULLED_IMAGE=no'
						return
					}

					podman_pull(ci_registry, "${cached_image}:test")
				}
			},
			devel: {
				node('cico-workspace') {
					if (rebuild_devel_container == 10) {
						// container needs rebuild, don't pull
						use_build_image = 'USE_PULLED_IMAGE=no'
						return
					}

					podman_pull(ci_registry, "${cached_image}:devel")
				}
			},
			ceph: {
				node('cico-workspace') {
					def base_image = sh(
						script: 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} \'source /opt/build/go/src/github.com/ceph/ceph-csi/build.env && echo ${BASE_IMAGE}\'',
						returnStdout: true
					).trim()

					// base_image is like ceph/ceph:v15
					podman_pull("docker.io", "${base_image}")
				}
			}
		}
		stage('test & build') {
			parallel test: {
				node ('cico-workspace') {
					ssh "cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-test CONTAINER_CMD=podman ENV_CSI_IMAGE_NAME=${cached_image} ${use_test_image}"
				}
			},
			verify: {
				node ('cico-workspace') {
					ssh "cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-test TARGET=mod-check CONTAINER_CMD=podman ENV_CSI_IMAGE_NAME=${cached_image} ${use_test_image}"
				}
			},
			build: {
				node('cico-workspace') {
					ssh "cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-build CONTAINER_CMD=podman ENV_CSI_IMAGE_NAME=${cached_image} ${use_build_image}"
				}
			}
		}
	}

	finally {
		stage('return bare-metal machine') {
			sh 'cico node done ${CICO_SSID}'
		}
	}
}
