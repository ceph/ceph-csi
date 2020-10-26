def cico_retries = 16
def cico_retry_interval = 60
def ci_git_repo = 'https://github.com/ceph/ceph-csi'
def ci_git_branch = 'ci/centos'
def git_repo = 'https://github.com/ceph/ceph-csi'
def ref = "master"
def git_since = 'master'
def workdir = '/opt/build/go/src/github.com/ceph/ceph-csi'
def doc_change = 0

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
			sh 'scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no ./prepare.sh root@${CICO_NODE}:'
			// TODO: already checked out the PR on the node, scp the contents?
			sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} ./prepare.sh --workdir=${workdir} --gitrepo=${git_repo} --ref=${ref}"
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
