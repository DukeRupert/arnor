package project

import (
	"bytes"
	"text/template"
)

var devWorkflowTmpl = template.Must(template.New("dev").Parse(`name: Deploy Dev

on:
  workflow_dispatch:
  push:
    branches: [dev]

env:
  IMAGE_NAME: {{ .DockerImage }}

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Login to DockerHub
        uses: docker/login-action@v3
        with:
          username: ${{ "{{" }} secrets.DOCKERHUB_USERNAME {{ "}}" }}
          password: ${{ "{{" }} secrets.DOCKERHUB_TOKEN {{ "}}" }}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ "{{" }} env.IMAGE_NAME {{ "}}" }}:dev-${{ "{{" }} github.sha {{ "}}" }}

      - name: Copy config files to VPS
        uses: appleboy/scp-action@v0.1.7
        with:
          host: ${{ "{{" }} secrets.VPS_HOST {{ "}}" }}
          username: ${{ "{{" }} secrets.DEV_VPS_USER {{ "}}" }}
          key: ${{ "{{" }} secrets.DEV_VPS_SSH_KEY {{ "}}" }}
          source: "docker-compose.yml,.env"
          target: ${{ "{{" }} secrets.DEV_VPS_DEPLOY_PATH {{ "}}" }}
          overwrite: true

      - name: Deploy to VPS
        uses: appleboy/ssh-action@v1
        with:
          host: ${{ "{{" }} secrets.VPS_HOST {{ "}}" }}
          username: ${{ "{{" }} secrets.DEV_VPS_USER {{ "}}" }}
          key: ${{ "{{" }} secrets.DEV_VPS_SSH_KEY {{ "}}" }}
          script: |
            cd ${{ "{{" }} secrets.DEV_VPS_DEPLOY_PATH {{ "}}" }}
            docker pull ${{ "{{" }} env.IMAGE_NAME {{ "}}" }}:dev-${{ "{{" }} github.sha {{ "}}" }}
            docker compose down || true
            export IMAGE_TAG=dev-${{ "{{" }} github.sha {{ "}}" }}
            docker compose up -d
`))

var prodWorkflowTmpl = template.Must(template.New("prod").Parse(`name: Deploy Prod

on:
  workflow_dispatch:
  push:
    tags: ["v*"]
    branches: [main]

env:
  IMAGE_NAME: {{ .DockerImage }}

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Get version from tag
        id: version
        run: echo "tag=${GITHUB_REF#refs/tags/}" >> "$GITHUB_OUTPUT"

      - name: Login to DockerHub
        uses: docker/login-action@v3
        with:
          username: ${{ "{{" }} secrets.DOCKERHUB_USERNAME {{ "}}" }}
          password: ${{ "{{" }} secrets.DOCKERHUB_TOKEN {{ "}}" }}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: |
            ${{ "{{" }} env.IMAGE_NAME {{ "}}" }}:${{ "{{" }} steps.version.outputs.tag {{ "}}" }}
            ${{ "{{" }} env.IMAGE_NAME {{ "}}" }}:latest

      - name: Copy config files to VPS
        uses: appleboy/scp-action@v0.1.7
        with:
          host: ${{ "{{" }} secrets.VPS_HOST {{ "}}" }}
          username: ${{ "{{" }} secrets.PROD_VPS_USER {{ "}}" }}
          key: ${{ "{{" }} secrets.PROD_VPS_SSH_KEY {{ "}}" }}
          source: "docker-compose.yml,.env"
          target: ${{ "{{" }} secrets.PROD_VPS_DEPLOY_PATH {{ "}}" }}
          overwrite: true

      - name: Deploy to VPS
        uses: appleboy/ssh-action@v1
        with:
          host: ${{ "{{" }} secrets.VPS_HOST {{ "}}" }}
          username: ${{ "{{" }} secrets.PROD_VPS_USER {{ "}}" }}
          key: ${{ "{{" }} secrets.PROD_VPS_SSH_KEY {{ "}}" }}
          script: |
            cd ${{ "{{" }} secrets.PROD_VPS_DEPLOY_PATH {{ "}}" }}
            docker pull ${{ "{{" }} env.IMAGE_NAME {{ "}}" }}:${{ "{{" }} steps.version.outputs.tag {{ "}}" }}
            docker compose down || true
            export IMAGE_TAG=${{ "{{" }} steps.version.outputs.tag {{ "}}" }}
            docker compose up -d
`))

type WorkflowData struct {
	DockerImage string
}

// GenerateDevWorkflow returns the dev deploy workflow YAML.
func GenerateDevWorkflow(dockerImage string) (string, error) {
	var buf bytes.Buffer
	if err := devWorkflowTmpl.Execute(&buf, WorkflowData{DockerImage: dockerImage}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// GenerateProdWorkflow returns the prod deploy workflow YAML.
func GenerateProdWorkflow(dockerImage string) (string, error) {
	var buf bytes.Buffer
	if err := prodWorkflowTmpl.Execute(&buf, WorkflowData{DockerImage: dockerImage}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
