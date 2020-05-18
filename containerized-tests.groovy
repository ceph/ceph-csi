def cico_retries = 16
def cico_retry_interval = 60
def ci_git_repo = 'https://github.com/ceph/ceph-csi'
def ci_git_branch = 'ci/centos'
def ref = "master"

node('cico-workspace') {

	stage('checkout ci repository') {
		git url: "${ci_git_repo}",
			branch: "${ci_git_branch}",
			changelog: false
	}

	stage('reserve bare-metal machine') {
		def firstAttempt = true
		retry(30) {
			if (!firstAttempt) {
				sleep(time: 5, unit: "MINUTES")
			}
			firstAttempt = false
			cico = sh(
				script: "cico node get -f value -c hostname -c comment --retry-count ${cico_retries} --retry-interval ${cico_retry_interval}",
				returnStdout: true
			).trim().tokenize(' ')
			env.CICO_NODE = "${cico[0]}.ci.centos.org"
			env.CICO_SSID = "${cico[1]}"
		}
	}

	try {
		stage('prepare bare-metal machine') {
			if ("${ghprbPullId}".length() != 0) {
				ref = "pull/${ghprbPullId}/head"
			}
			sh 'scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no ./prepare.sh root@${CICO_NODE}:'
			sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} ./prepare.sh --workdir=/opt/build/go/src/github.com/ceph/ceph-csi --gitrepo=${ci_git_repo} --ref=${ref}"
		}
		stage('test & build') {
			parallel test: {
				node ('cico-workspace') {
					sh 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} "cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-test CONTAINER_CMD=podman"'
				}
			},
			build: { 
				node('cico-workspace') {
					sh 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} "cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-build CONTAINER_CMD=podman"'
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
