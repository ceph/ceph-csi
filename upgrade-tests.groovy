def cico_retries = 16
def cico_retry_interval = 60
def duffy_pool = 'virt-ec2-t2-centos-8s-x86_64'
def ci_git_repo = 'https://github.com/ceph/ceph-csi'
def ci_git_branch = 'ci/centos'
def git_repo = 'https://github.com/ceph/ceph-csi'
def ref = "devel"
def git_since = 'devel'
def skip_e2e = 0
def doc_change = 0
def k8s_release = 'latest'
def ci_registry = 'registry-ceph-csi.apps.ocp.cloud.ci.centos.org'
def failure = null

def ssh(cmd) {
	sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} '${cmd}'"
}

def podman_login(registry, username, passwd) {
	ssh "podman login --authfile=~/.podman-auth.json --username='${username}' --password='${passwd}' ${registry}"
}

def create_duffy_config() {
	writeFile(
		file: '/home/jenkins/.config/duffy',
		text: """client:
			|  url: https://duffy.ci.centos.org/api/v1
			|  auth:
			|    name: ceph-csi
			|    key: ${env.CICO_API_KEY}
			|""".stripMargin()
	)
}

// podman_pull pulls image from the source (CI internal) registry, and tags it
// as unqualified image name and into the destination registry. This prevents
// pulling from the destination registry.
//
// Images need to be pre-pushed into the source registry, though.
def podman_pull(source, destination, image) {
	ssh "podman pull --authfile=~/.podman-auth.json ${source}/${image} && podman tag ${source}/${image} ${image} ${destination}/${image}"
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
		if (ref != git_since) {
			sh "cd ~/build/ceph-csi && git fetch origin ${ref} && git checkout -b ${ref} FETCH_HEAD"
		}
	}

	stage('check doc-only change') {
		doc_change = sh(
			script: "cd ~/build/ceph-csi && \${OLDPWD}/scripts/skip-doc-change.sh origin/${git_since}",
			returnStatus: true)
	}
	// if doc_change (return value of skip-doc-change.sh is 1, do not run the other stages
	if (doc_change == 1 && ref != git_since) {
		currentBuild.result = 'SUCCESS'
		return
	}

	stage('reserve bare-metal machine') {
		create_duffy_config()

		def firstAttempt = true
		retry(30) {
			if (!firstAttempt) {
				sleep(time: 5, unit: "MINUTES")
			}
			firstAttempt = false
			def cmd = sh(
				script: "duffy client request-session pool=${duffy_pool},quantity=1",
				returnStdout: true
			)
			def duffy = new groovy.json.JsonSlurper().parseText(cmd)
			env.CICO_NODE = "${duffy.session.nodes[0].hostname}"
			env.CICO_SSID = "${duffy.session.id}"
		}
	}

	try {
		stage('prepare bare-metal machine') {
			if (params.ghprbPullId != null) {
				ref = "pull/${ghprbPullId}/merge"
			}
			sh 'scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no ./prepare.sh ./single-node-k8s.sh ./podman2minikube.sh ./system-status.sh root@${CICO_NODE}:'
			ssh "./prepare.sh --workdir=/opt/build/go/src/github.com/ceph/ceph-csi --gitrepo=${git_repo} --ref=${ref}"
		}
		stage('pull base container images') {
			def base_image = sh(
				script: 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} \'source /opt/build/go/src/github.com/ceph/ceph-csi/build.env && echo ${BASE_IMAGE}\'',
				returnStdout: true
			).trim()
			def d_io_regex = ~"^docker.io/"

			withCredentials([usernamePassword(credentialsId: 'container-registry-auth', usernameVariable: 'CREDS_USER', passwordVariable: 'CREDS_PASSWD')]) {
				podman_login(ci_registry, '$CREDS_USER', '$CREDS_PASSWD')
			}

			// base_image is like ceph/ceph:v15 or docker.io/ceph/ceph:v15, strip "docker.io/"
			podman_pull(ci_registry, "docker.io", "${base_image}" - d_io_regex)
			// cephcsi:devel is used with 'make containerized-build'
			podman_pull(ci_registry, ci_registry, "ceph-csi:devel")
		}
		stage('build artifacts') {
			// build container image
			ssh 'cd /opt/build/go/src/github.com/ceph/ceph-csi && make image-cephcsi GOARCH=amd64 CONTAINER_CMD=podman'
			// build e2e.test executable
			ssh "cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-build CONTAINER_CMD=podman TARGET=e2e.test ENV_CSI_IMAGE_NAME=${ci_registry}/ceph-csi USE_PULLED_IMAGE=yes"
		}
		stage("deploy k8s-${k8s_version} and rook") {
			def rook_version = sh(
				script: 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} \'source /opt/build/go/src/github.com/ceph/ceph-csi/build.env && echo ${ROOK_VERSION}\'',
				returnStdout: true
			).trim()

			if (rook_version != '') {
				// single-node-k8s.sh pushes the image into minikube
				podman_pull(ci_registry, "docker.io", "rook/ceph:${rook_version}")
			}

			def rook_ceph_cluster_image = sh(
				script: 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} \'source /opt/build/go/src/github.com/ceph/ceph-csi/build.env && echo ${ROOK_CEPH_CLUSTER_IMAGE}\'',
				returnStdout: true
			).trim()
			def d_io_regex = ~"^docker.io/"

			if (rook_ceph_cluster_image != '') {
				// single-node-k8s.sh pushes the image into minikube
				podman_pull(ci_registry, "docker.io", rook_ceph_cluster_image - d_io_regex)
			}

			timeout(time: 30, unit: 'MINUTES') {
				ssh "./single-node-k8s.sh --k8s-version=${k8s_release}"
			}

			// vault:latest,1.8.5 and nginx:latest are used by the e2e tests
			podman_pull(ci_registry, "docker.io", "library/vault:latest")
			ssh "./podman2minikube.sh docker.io/library/vault:latest"
			podman_pull(ci_registry, "docker.io", "library/nginx:latest")
			ssh "./podman2minikube.sh docker.io/library/nginx:latest"
			podman_pull(ci_registry, "docker.io", "library/vault:1.8.5")
			ssh "./podman2minikube.sh docker.io/library/vault:1.8.5"
		}
		stage("run ${test_type} upgrade tests") {
			timeout(time: 120, unit: 'MINUTES') {
				upgrade_args = "--test-cephfs=false --test-rbd=true --upgrade-testing=true"
				if ("${test_type}" == "cephfs"){
					upgrade_args = "--test-cephfs=true --test-rbd=false --upgrade-testing=true"
				}
				ssh "cd /opt/build/go/src/github.com/ceph/ceph-csi && make run-e2e E2E_ARGS=\"--upgrade-version=${csi_upgrade_version} ${upgrade_args}\""
			}
		}
	}

	catch (err) {
		failure = err

		stage('log system status') {
			ssh './system-status.sh'
		}
	}

	finally {
		stage('return bare-metal machine') {
			sh 'duffy client retire-session ${CICO_SSID}'
		}

		if (failure) {
			throw failure
		}
	}
}
