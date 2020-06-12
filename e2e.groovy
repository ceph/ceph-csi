def cico_retries = 16
def cico_retry_interval = 60
def ci_git_repo = 'https://github.com/Yuggupta27/ceph-csi'
def ci_git_branch = 'e2e-centosci'
def ref = "master"

node('cico-workspace') {
	stage('checkout ci repository') {
		git url: "${ci_git_repo}",
			branch: "${ci_git_branch}",
			changelog: false
	}

	stage('reserve bare-metal machine') {
		def firstAttempt = true
		retry(5) {
			if (!firstAttempt) {
				sleep(time: 5, unit: "MINUTES")
			}
			firstAttempt = false
			cico = sh(
				script: "cico node get -f value -c hostname -c comment --retry-count ${cico_retries} --retry-interval ${cico_retry_interval} --release=8",
				returnStdout: true
			).trim().tokenize(' ')
			env.CICO_NODE = "${cico[0]}.ci.centos.org"
			env.CICO_SSID = "${cico[1]}"
		}
	}
	try {
		stage('prepare bare-metal machine') {
			sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} uname -a"
			if (params.ghprbPullId != null) {
				ref = "pull/${ghprbPullId}/head"
			}
			sh 'scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no ./prepare.sh ./multi-node-k8s.sh root@${CICO_NODE}:'
			sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} ./prepare.sh --workdir=/opt/build/go/src/github.com/ceph/ceph-csi --gitrepo=${ci_git_repo} --ref=master"
		}

		stage('deploy kubernetes') {
			sh 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} ./multi-node-k8s.sh'
			sh 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} /opt/build/go/src/github.com/ceph/ceph-csi/scripts/install-snapshot.sh install'
			sleep(time: 2, unit: "MINUTES")
			sh 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} kubectl get nodes'
		}

		stage('deploy rook') {
			timeout(time: 25, unit: 'MINUTES') {
				//sh 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} virsh reboot k8s-vagrant-multi-node_node2; sleep 120'
				sh 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} /opt/build/go/src/github.com/ceph/ceph-csi/scripts/rook.sh deploy'
			}
		}

		stage('e2e') {
			sh 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} "cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-build TARGET=e2e.test"'
			// e2e.test uses relative paths to .yaml files, run from within the e2e/ directory
			sh 'ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} "cd /opt/build/go/src/github.com/ceph/ceph-csi/e2e && ../e2e.test -test.v -deploy-timeout=10 -test.timeout=30m"'
		}
	}

	finally {
		stage('return bare-metal machine') {
			sh 'cico node done ${CICO_SSID}'
		}
	}
}
