pipeline {
    agent any

    environment {
        REGISTRY_URL = 'REGISTRY_URL_PLACEHOLDER' // e.g. docker.io/username or private-registry
        IMAGE_NAME = 'gateway-api'
        DOCKER_CREDENTIALS_ID = 'docker-credentials-id' // ID of credentials in Jenkins
        GIT_CREDENTIALS_ID = 'git-credentials-id' // ID for git push
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Build Docker Image') {
            steps {
                script {
                    dockerImage = docker.build("${REGISTRY_URL}/${IMAGE_NAME}:${env.BUILD_NUMBER}")
                }
            }
        }

        stage('Push Docker Image') {
            steps {
                script {
                    docker.withRegistry("https://${REGISTRY_URL}", "${DOCKER_CREDENTIALS_ID}") {
                        dockerImage.push()
                        dockerImage.push("latest")
                    }
                }
            }
        }

        stage('Update Manifest') {
            steps {
                script {
                    // Update the deployment image tag
                    sh "sed -i 's|image: .*${IMAGE_NAME}.*|image: ${REGISTRY_URL}/${IMAGE_NAME}:${env.BUILD_NUMBER}|' k8s/deployment.yaml"
                    
                    // Commit and push changes
                    withCredentials([usernamePassword(credentialsId: "${GIT_CREDENTIALS_ID}", passwordVariable: 'GIT_PASSWORD', usernameVariable: 'GIT_USERNAME')]) {
                        sh """
                            git config user.email "jenkins@example.com"
                            git config user.name "Jenkins CI"
                            git add k8s/deployment.yaml
                            git commit -m "Update image to ${env.BUILD_NUMBER}"
                            // git push https://${GIT_USERNAME}:${GIT_PASSWORD}@github.com/hotkhwan/gateway-api.git HEAD:main
                            // Uncomment the push command above after verifying credentials
                        """
                    }
                }
            }
        }
    }
}
