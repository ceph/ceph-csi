def cico_retries = 16
def cico_retry_interval = 60
def duffy_pool = 'virt-ec2-t2-centos-8s-x86_64'
def ci_git_repo = 'https://github.com/ceph/ceph-csi'
def ci_git_branch = 'ci/centos'
def ref = "devel"
def git_since = 'origin/devel'

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

node('cico-workspace') {
	stage('checkout ci repository') {
		git url: "${ci_git_repo}",
			branch: "${ci_git_branch}",
			changelog: false
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
			sh 'scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no ./prepare.sh root@${CICO_NODE}:'
			sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} ./prepare.sh --workdir=/opt/build/go/src/github.com/ceph/ceph-csi --gitrepo=${ci_git_repo} --ref=${ref} --history"
		}

		stage('run commitlint') {
			if (params.ghprbTargetBranch != null) {
				git_since = "origin/${ghprbTargetBranch}"
			}
			sh "ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@${CICO_NODE} 'cd /opt/build/go/src/github.com/ceph/ceph-csi && make containerized-test CONTAINER_CMD=podman TARGET=commitlint GIT_SINCE=${git_since}'"
		}
	}

	finally {
		stage('return bare-metal machine') {
			sh 'duffy client retire-session ${CICO_SSID}'
		}
	}
}
