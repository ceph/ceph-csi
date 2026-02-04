pipeline {
  agent any

  parameters {
    string(
      name: 'CEPH_VERSION',
      defaultValue: 'main',
      description: 'Ceph version/branch to test against'
    )
  }

  environment {
    CEPH_VERSION = "${params.CEPH_VERSION}"
  }

  stages {
    stage('Prepare environment') {
      steps {
        sh './prepare.sh'
      }
    }

    stage('Run Mini E2E (Ceph main)') {
      steps {
        sh '''
          echo "Running mini E2E against Ceph ${CEPH_VERSION}"
          export CEPH_VERSION=${CEPH_VERSION}
          make mini-e2e
        '''
      }
    }
  }

  post {
    always {
      archiveArtifacts artifacts: '**/logs/**', allowEmptyArchive: true
    }
    failure {
      echo 'Mini E2E failed'
    }
  }
}
