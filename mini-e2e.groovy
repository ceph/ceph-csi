def cico_retries = 16
def cico_retry_interval = 60
def ci_git_repo = 'https://github.com/ceph/ceph-csi'
def ci_git_branch = 'ci/centos'
def git_repo = 'https://github.com/ceph/ceph-csi'
def ref = "master"
def git_since = 'master'
def skip_e2e = 0
def doc_change = 0
def k8s_release = 'latest'
def ci_registry = 'registry-ceph-csi.apps.ocp.ci.centos.org'

def ssh(cmd) {
	sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} '${cmd}'"
}

node('cico-workspace') {
	stage('checkout ci repository') {
		git url: "${ci_git_repo}",
			branch: "${ci_git_branch}",
			changelog: false
	}

	// "github-api-token" is a secret text credential configured in Jenkins
	withCredentials([string(credentialsId: 'github-api-token', variable: 'GITHUB_API_TOKEN')]) {
		stage('skip ci/skip/e2e label') {
			if (params.ghprbPullId == null) {
				skip_e2e = 1
				return
			}

			skip_e2e = sh(
				script: "./scripts/get_github_labels.py --id=${ghprbPullId} --has-label=ci/skip/e2e",
				returnStatus: true)
		}

		stage("detect k8s-${k8s_version} patch release") {
			k8s_release = sh(
				script: "./scripts/get_patch_release.py --version=${k8s_version}",
				returnStdout: true).trim()
			echo "detected Kubernetes patch release: ${k8s_release}"
		}
	}

	// if skip_e2e returned 0, do not run full tests
	if (skip_e2e == 0) {
		currentBuild.result = 'SUCCESS'
		return
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
			if (params.ghprbPullId != null) {
				ref = "pull/${ghprbPullId}/merge"
			}
			sh 'scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no ./prepare.sh ./single-node-k8s.sh root@${CICO_NODE}:'
			ssh "./prepare.sh --workdir=/opt/build/go/src/github.com/ceph/ceph-csi --gitrepo=${git_repo} --ref=${ref}"
		}
		stage('pull base container images') {
			def base_image = ssh 'source /opt/build/go/src/github.com/ceph/ceph-csi/build.env && echo ${BASE_IMAGE}'

			withCredentials([usernamePassword(credentialsId: 'container-registry-auth', usernameVariable: 'CREDS_USER', passwordVariable: 'CREDS_PASSWD')]) {
				ssh "podman login --username=${CREDS_USER} --password='${CREDS_PASSWD}'"
			}

			// base_image is like ceph/ceph:v15
			ssh "podman pull ${ci_registry}/${base_image} && podman tag ${ci_registry}/${base_image} ${base_image}"
			// cephcsi:devel is used with 'make containerized-build'
			ssh "podman pull ${ci_registry}/cephcsi:devel"
		}
		stage('build artifacts') {
			// build container image
			ssh 'cd /opt/build/go/src/github.com/ceph/ceph-csi && make image-cephcsi GOARCH=amd64 CONTAINER_CMD=podman'
			// build e2e.test executable
			ssh "cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-build CONTAINER_CMD=podman TARGET=e2e.test ENV_CSI_IMAGE_NAME=${ci_registry}/cephcsi USE_PULLED_IMAGE=yes"
		}
		stage("deploy k8s-${k8s_version} and rook") {
			timeout(time: 30, unit: 'MINUTES') {
				ssh "./single-node-k8s.sh --k8s-version=${k8s_release}"
			}
		}
		stage('run e2e') {
			timeout(time: 120, unit: 'MINUTES') {
				ssh 'cd /opt/build/go/src/github.com/ceph/ceph-csi && make run-e2e'
			}
		}
	}

	finally {
		stage('return bare-metal machine') {
			sh 'cico node done ${CICO_SSID}'
		}
	}
}
