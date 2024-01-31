node {
  stage('build-images') {
    parallel jjb: {
      sh "oc start-build --follow jjb"
    },
    mirror_images: {
      sh 'oc start-build --follow mirror-images'
    }
  }
  stage('checkout ci repository') {
    git url: "${GIT_REPO}", branch: "${GIT_BRANCH}", changelog: false
  }
  stage('deployment') {
    sh "./deploy/jjb.sh --cmd deploy --GIT_REF ${GIT_BRANCH} --GIT_REPO ${GIT_REPO}"
  }
}
