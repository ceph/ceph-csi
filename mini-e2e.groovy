def cico_retries = 16
def cico_retry_interval = 60
// temporary git repository for testing purpose
def ci_git_repo = 'https://github.com/nixpanic/ceph-csi'
def ci_git_branch = 'mini-e2e'
def git_repo = 'https://github.com/ceph/ceph-csi'
def ref = "master"

def ssh(cmd) {
	sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} '${cmd}'"
}

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
				script: "cico node get -f value -c hostname -c comment --release=8 --retry-count=${cico_retries} --retry-interval=${cico_retry_interval}",
				returnStdout: true
			).trim().tokenize(' ')
			env.CICO_NODE = "${cico[0]}.ci.centos.org"
			env.CICO_SSID = "${cico[1]}"
		}
	}

	try {
		stage('prepare bare-metal machine') {
			if (params.ghprbPullId != null) {
				ref = "pull/${ghprbPullId}/head"
			}
			sh 'scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no ./prepare.sh ./single-node-k8s.sh root@${CICO_NODE}:'
			ssh "./prepare.sh --workdir=/opt/build/go/src/github.com/ceph/ceph-csi --gitrepo=${git_repo} --ref=${ref}"
		}
		stage('build artifacts') {
			// build container image
			ssh 'cd /opt/build/go/src/github.com/ceph/ceph-csi && make image-cephcsi GOARCH=amd64 CONTAINER_CMD=podman'
			// build e2e.test executable
			ssh 'cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-build CONTAINER_CMD=podman TARGET=e2e.test'
		}
		stage('deploy k8s v1.18.3 and rook') {
			timeout(time: 30, unit: 'MINUTES') {
				ssh './single-node-k8s.sh --k8s-version=v1.18.3'
			}
		}
		stage('run e2e') {
			timeout(time: 60, unit: 'MINUTES') {
				ssh 'cd /opt/build/go/src/github.com/ceph/ceph-csi && make run-e2e'
			}
		}
	}

	catch(exc) {
		stage('debug time!') {
			ssh 'sleep 8h'
		}
	}

	finally {
		stage('return bare-metal machine') {
			sh 'cico node done ${CICO_SSID}'
		}
	}
}
