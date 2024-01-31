if (params.ghprbPullId != null) {
    GIT_BRANCH = "pull/${ghprbPullId}/merge"
}

node {
  stage('checkout ci repository') {
    checkout([$class: 'GitSCM', branches: [[name: 'FETCH_HEAD']],
      userRemoteConfigs: [[url: "${GIT_REPO}",
        refspec: "${GIT_BRANCH}"]]])
  }
  stage('validation') {
    sh "./deploy/jjb.sh --cmd validate --GIT_REF ${GIT_BRANCH} --GIT_REPO ${GIT_REPO}"
  }
}
