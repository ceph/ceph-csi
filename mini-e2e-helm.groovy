def cico_retries = 16
def cico_retry_interval = 60
def ci_git_repo = 'https://github.com/ceph/ceph-csi'
def ci_git_branch = 'ci/centos'
def git_repo = 'https://github.com/ceph/ceph-csi'
def ref = "master"
def git_since = "master"
def doc_change = 0
def namespace = 'cephcsi-e2e-' + UUID.randomUUID().toString().split('-')[-1]

// ssh executes a given command on the reserved bare-metal machine
// NOTE: do not pass " symbols on the command line, use ' only.
def ssh(cmd) {
	sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} \"${cmd}\""
}

node('cico-workspace') {
	stage('checkout ci repository') {
		git url: "${ci_git_repo}",
			branch: "${ci_git_branch}",
			changelog: false
	}

	stage('checkout PR') {
		if (params.ghprbPullId != null) {
			ref = "pull/${ghprbPullId}/head"
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
		stage("deploy k8s v${k8s_version} and rook") {
			timeout(time: 30, unit: 'MINUTES') {
				ssh "./single-node-k8s.sh --k8s-version=v${k8s_version}"
			}
		}
		stage('deploy ceph-csi through helm') {
			timeout(time: 30, unit: 'MINUTES') {
				ssh 'cd /opt/build/go/src/github.com/ceph/ceph-csi && ./scripts/install-helm.sh up'
				ssh "kubectl create namespace '${namespace}'"
				ssh "cd /opt/build/go/src/github.com/ceph/ceph-csi && ./scripts/install-helm.sh install-cephcsi '${namespace}'"
			}
		}
		stage('run e2e') {
			timeout(time: 120, unit: 'MINUTES') {
				ssh "cd /opt/build/go/src/github.com/ceph/ceph-csi && make run-e2e NAMESPACE='${namespace}' E2E_ARGS='--deploy-cephfs=false --deploy-rbd=false'"
			}
		}
		stage('cleanup ceph-csi deployment') {
			timeout(time: 30, unit: 'MINUTES') {
				ssh "cd /opt/build/go/src/github.com/ceph/ceph-csi && ./scripts/install-helm.sh cleanup-cephcsi '${namespace}'"
				ssh 'cd /opt/build/go/src/github.com/ceph/ceph-csi && ./scripts/install-helm.sh clean'
				ssh "kubectl delete namespace '${namespace}'"			}
		}
	}

	finally {
		stage('return bare-metal machine') {
			sh 'cico node done ${CICO_SSID}'
		}
	}
}
