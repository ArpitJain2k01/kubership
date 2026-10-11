// Kubership CI: verify -> security scans -> build image -> scan image -> publish.
//
// Jenkins configuration this pipeline uses:
//   - REGISTRY and IMAGE_REPO: taken from Jenkins environment variables when the
//     pipeline can see them, otherwise from the defaults in `environment` below.
//     IMAGE_REPO must be lowercase, e.g. arpitjain2k01/kubership.
//   - A "Username with password" credential with ID `registry-creds`
//     holding a registry token with the minimum publish scope.
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
      image: golang:1.27.2@sha256:5bc7f572bbaa98885a3a1fd9c0aa76b59e3e14e8628bfc316bbfd0c701e4818c
      command: [cat]
      tty: true
    - name: gitleaks
      image: ghcr.io/gitleaks/gitleaks:v8.30.1
      command: [cat]
      tty: true
    - name: trivy
      image: aquasec/trivy:0.75.0
      command: [cat]
      tty: true
      env:
        - name: TRIVY_CACHE_DIR
          value: /tmp/trivy-cache
    - name: buildah
      image: quay.io/buildah/stable:v1.43.4-immutable
      command: [cat]
      tty: true
      env:
        - name: STORAGE_DRIVER
          value: vfs
        - name: BUILDAH_ISOLATION
          value: chroot
      volumeMounts:
        - name: buildah-storage
          mountPath: /var/lib/containers
      # Buildah makes mounts while unpacking layers and running RUN steps. The
      # nodes' default AppArmor/seccomp profiles deny that ("remount /: permission
      # denied"), so relax both for this one container. No extra capabilities and
      # not privileged. Only run trusted branches on this agent.
      securityContext:
        appArmorProfile:
          type: Unconfined
        seccompProfile:
          type: Unconfined
  volumes:
    # vfs copies whole layers, so give the builder bounded scratch space.
    - name: buildah-storage
      emptyDir:
        sizeLimit: 10Gi
'''
        }
    }

    // Global properties set in the Jenkins UI were not visible to this pod-agent
    // pipeline, so fall back to defaults. A value Jenkins does provide wins.
    environment {
        REGISTRY   = "${env.REGISTRY ?: 'ghcr.io'}"
        IMAGE_REPO = "${env.IMAGE_REPO ?: 'arpitjain2k01/kubership'}"
    }

    options {
        timeout(time: 30, unit: 'MINUTES')
        disableConcurrentBuilds()
        skipDefaultCheckout(true)
        timestamps()
        buildDiscarder(logRotator(numToKeepStr: '20'))
    }

    stages {
        stage('Checkout') {
            steps {
                script {
                    // Not named `scm`: that would shadow the built-in variable.
                    def vars = checkout scm
                    env.GIT_SHA = vars.GIT_COMMIT
                }
            }
        }

        stage('Preflight') {
            steps {
                script {
                    echo "REGISTRY=[${env.REGISTRY}] IMAGE_REPO=[${env.IMAGE_REPO}]"
                    if (!env.REGISTRY?.trim() || !env.IMAGE_REPO?.trim()) {
                        error 'REGISTRY and IMAGE_REPO must not be empty.'
                    }
                    if (env.IMAGE_REPO != env.IMAGE_REPO.toLowerCase()) {
                        error 'IMAGE_REPO must be lowercase.'
                    }
                    echo "Image: ${env.REGISTRY}/${env.IMAGE_REPO}:${env.GIT_SHA}"
                }
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

        stage('Security Scans') {
            failFast true
            parallel {
                stage('Secrets') {
                    steps {
                        container('gitleaks') {
                            // The workspace is owned by the Jenkins user, not the container's
                            // root user, so git needs it marked safe.
                            sh '''
                                git config --global --add safe.directory "$WORKSPACE"
                                gitleaks detect --source . --redact --exit-code 1 \
                                  --report-format json --report-path gitleaks-report.json
                            '''
                        }
                    }
                }
                stage('Go vulnerabilities') {
                    steps {
                        dir('app') {
                            sh '''
                                go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
                                govulncheck ./...
                            '''
                        }
                    }
                }
                stage('Filesystem') {
                    steps {
                        container('trivy') {
                            sh 'trivy fs --scanners vuln,secret,misconfig --exit-code 1 --severity HIGH,CRITICAL .'
                        }
                    }
                }
            }
        }

        stage('Compile') {
            steps {
                dir('app') {
                    // -buildvcs=false: the checkout is owned by the Jenkins user while the
                    // golang container runs as root, so git rejects the repo ("dubious
                    // ownership") and Go's VCS stamping fails with exit 128.
                    sh 'CGO_ENABLED=0 go build -buildvcs=false -o ../kubership .'
                    sh 'test -x ../kubership'
                }
            }
        }

        stage('Build Image') {
            steps {
                container('buildah') {
                    sh '''
                        set -eu
                        IMG="$REGISTRY/$IMAGE_REPO:$GIT_SHA"
                        buildah bud --format docker --build-arg VERSION="$GIT_SHA" -f Dockerfile -t "$IMG" .
                        buildah push "$IMG" docker-archive:image.tar
                    '''
                }
            }
        }

        // Scan the built image before it is pushed, so a vulnerable image never
        // reaches the registry.
        stage('Scan Image') {
            steps {
                container('trivy') {
                    sh 'trivy image --input image.tar --exit-code 1 --severity HIGH,CRITICAL'
                }
            }
        }

        stage('Publish') {
            when { branch 'main' }
            steps {
                container('buildah') {
                    withCredentials([usernamePassword(
                        credentialsId: 'registry-creds',
                        usernameVariable: 'REGISTRY_USER',
                        passwordVariable: 'REGISTRY_PASSWORD'
                    )]) {
                        // Single-quoted so the shell, not Groovy, reads the secret.
                        sh '''
                            set -eu
                            printf '%s' "$REGISTRY_PASSWORD" | buildah login \
                              --username "$REGISTRY_USER" --password-stdin "$REGISTRY"
                            buildah push --digestfile image-digest.txt "$REGISTRY/$IMAGE_REPO:$GIT_SHA"
                            echo "$REGISTRY/$IMAGE_REPO@$(cat image-digest.txt)" > image-ref.txt
                            buildah logout --all
                        '''
                    }
                }
                // The digest, not the mutable tag, identifies the artifact to promote.
                archiveArtifacts artifacts: 'image-digest.txt,image-ref.txt', fingerprint: true
            }
        }

        stage('Archive Evidence') {
            steps {
                archiveArtifacts(
                    artifacts: 'kubership,app/coverage.out,gitleaks-report.json',
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
