pipeline {
    agent {
        kubernetes {
            cloud 'kubernetes'
            namespace 'jenkins-agents'
            defaultContainer 'golang'

            yaml '''
apiVersion: v1
kind: Pod
spec:
  restartPolicy: Never
  containers:
    - name: golang
      image: golang:1.24
      command:
        - cat
      tty: true
'''
        }
    }

    options {
        timeout(time: 15, unit: 'MINUTES')
        disableConcurrentBuilds()
        skipDefaultCheckout(true)
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Formatting') {
            steps {
                sh '''
                    test -z "$(gofmt -l app/*.go)" || {
                      echo "Go formatting errors found"
                      gofmt -l app/*.go
                      exit 1
                    }
                '''
            }
        }

        stage('Static Analysis') {
            steps {
                dir('app') {
                    sh 'go vet ./...'
                }
            }
        }

        stage('Unit Tests') {
            steps {
                dir('app') {
                    sh 'go test -v -race -coverprofile=coverage.out ./...'
                    sh 'go tool cover -func=coverage.out'
                }
            }
        }

        stage('Compile') {
            steps {
                dir('app') {
                    // -buildvcs=false: the checkout is owned by the Jenkins user while
                    // the golang container runs as root, so git refuses the repo
                    // ("dubious ownership") and Go's VCS stamping fails with exit 128.
                    sh 'CGO_ENABLED=0 go build -buildvcs=false -o ../kubership .'
                    sh 'test -x ../kubership'
                }
            }
        }

        stage('Archive Evidence') {
            steps {
                archiveArtifacts(
                    artifacts: 'kubership,app/coverage.out',
                    fingerprint: true
                )
            }
        }
    }

    post {
        success {
            echo 'CI passed.'
        }

        failure {
            echo 'CI failed. Review the stage logs.'
        }

        always {
            echo "Build result: ${currentBuild.currentResult}"
            cleanWs()
        }
    }
}
