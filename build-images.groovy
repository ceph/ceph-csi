def GIT_REPO = 'http://github.com/ceph/ceph-csi'
def GIT_BRANCH = 'devel'
node {
  stage('checkout repository') {
    git url: "${GIT_REPO}", branch: "${GIT_BRANCH}", changelog: false
  }
  stage('build images') {
    def base_image = sh(script: 'source ${WORKSPACE}/build.env && echo ${BASE_IMAGE}',
                        returnStdout: true).trim()
    parallel canary: {
      sh "oc start-build --follow --build-arg=BASE_IMAGE='${base_image}' --build-arg=GO_ARCH=amd64 ceph-csi-canary"
    },
    test: {
      sh 'oc start-build --follow --build-arg=GOARCH=amd64 ceph-csi-test'
    },
    devel: {
      sh "oc start-build --follow --build-arg=BASE_IMAGE='${base_image}' --build-arg=GOARCH=amd64 ceph-csi-devel"
    }
  }
}
