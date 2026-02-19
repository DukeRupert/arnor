# GitHub Actions Deploy Workflow Guide

This document describes how to create GitHub Actions workflows that deploy a Dockerized application to a Hetzner VPS after building and pushing the image to DockerHub. The infrastructure is provisioned by `arnor project create`.

## Infrastructure Pre-conditions

Before the workflow runs, `arnor project create` has already set up:

- **DockerHub repository** — e.g. `dukerupert/myproject`
- **VPS deploy user** — e.g. `myproject-dev-deploy` (dev) or `myproject-deploy` (prod)
- **VPS deploy path** — e.g. `/opt/myproject-dev` (dev) or `/opt/myproject` (prod)
- **docker-compose.yml on the VPS** at the deploy path
- **Caddy reverse proxy config** — routes the domain to the app's port
- **DNS records** — A record + www CNAME pointing to the VPS

## Available GitHub Secrets

These secrets are set on the repository by arnor and available in workflows:

### Shared (same across environments)

| Secret | Description | Example |
|---|---|---|
| `VPS_HOST` | Server IP address | `128.140.1.50` |
| `DOCKERHUB_USERNAME` | DockerHub username | `dukerupert` |
| `DOCKERHUB_TOKEN` | DockerHub PAT or password | |

### Per-environment (prefixed `DEV_` or `PROD_`)

| Secret | Description | Example |
|---|---|---|
| `{PREFIX}_VPS_USER` | SSH deploy user on VPS | `myproject-dev-deploy` |
| `{PREFIX}_VPS_DEPLOY_PATH` | Absolute deploy directory | `/opt/myproject-dev` |
| `{PREFIX}_VPS_SSH_KEY` | PEM-encoded SSH private key for deploy user | |
| `{PREFIX}_PORT` | Application port on the host | `3001` |

## docker-compose.yml on the VPS

The compose file is pre-written to the deploy path with environment variable overrides:

```yaml
services:
  web:
    image: ${DOCKER_IMAGE:-dukerupert/myproject:latest}
    ports:
      - "${LISTEN_PORT:-3000}:80"
    restart: unless-stopped
```

- `DOCKER_IMAGE` — Override to deploy a specific tagged image (e.g. `dukerupert/myproject:dev-abc123`)
- `LISTEN_PORT` — Override to change the host port (defaults to the port chosen at project creation)

## Workflow Pattern

Every deploy workflow follows this sequence:

1. **Checkout** the repo
2. **Login** to DockerHub
3. **Build and push** the Docker image with an appropriate tag
4. **SSH into VPS** — login to DockerHub, pull the image, restart the container

### Dev Workflow

Triggers on push to `dev` branch and manual dispatch. Tags images with `dev-{commit_sha}`.

```yaml
name: Deploy Dev

on:
  workflow_dispatch:
  push:
    branches: [dev]

env:
  IMAGE_NAME: dukerupert/myproject

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Login to DockerHub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ env.IMAGE_NAME }}:dev-${{ github.sha }}

      - name: Deploy to VPS
        uses: appleboy/ssh-action@v1
        with:
          host: ${{ secrets.VPS_HOST }}
          username: ${{ secrets.DEV_VPS_USER }}
          key: ${{ secrets.DEV_VPS_SSH_KEY }}
          script: |
            echo "${{ secrets.DOCKERHUB_TOKEN }}" | docker login -u "${{ secrets.DOCKERHUB_USERNAME }}" --password-stdin
            cd ${{ secrets.DEV_VPS_DEPLOY_PATH }}
            export DOCKER_IMAGE=${{ env.IMAGE_NAME }}:dev-${{ github.sha }}
            docker compose pull
            docker compose down || true
            docker compose up -d
```

### Prod Workflow

Triggers on semver tags (`v*`) pushed to `main` and manual dispatch. Tags images with the version and `latest`.

```yaml
name: Deploy Prod

on:
  workflow_dispatch:
  push:
    tags: ["v*"]
    branches: [main]

env:
  IMAGE_NAME: dukerupert/myproject

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
          username: ${{ secrets.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: |
            ${{ env.IMAGE_NAME }}:${{ steps.version.outputs.tag }}
            ${{ env.IMAGE_NAME }}:latest

      - name: Deploy to VPS
        uses: appleboy/ssh-action@v1
        with:
          host: ${{ secrets.VPS_HOST }}
          username: ${{ secrets.PROD_VPS_USER }}
          key: ${{ secrets.PROD_VPS_SSH_KEY }}
          script: |
            echo "${{ secrets.DOCKERHUB_TOKEN }}" | docker login -u "${{ secrets.DOCKERHUB_USERNAME }}" --password-stdin
            cd ${{ secrets.PROD_VPS_DEPLOY_PATH }}
            export DOCKER_IMAGE=${{ env.IMAGE_NAME }}:${{ steps.version.outputs.tag }}
            docker compose pull
            docker compose down || true
            docker compose up -d
```

## Key Details

- **Image tagging**: Dev uses `dev-{sha}`, prod uses the git tag (e.g. `v1.2.3`) plus `latest`
- **`DOCKER_IMAGE` env var**: Must be exported before `docker compose up -d` so the compose file picks up the correct tag instead of the default
- **`docker compose pull`**: Always pull explicitly before `up` to ensure the newly pushed image is fetched
- **`docker compose down`**: Use `|| true` to tolerate the case where no container is running yet (first deploy)
- **No SCP step needed**: The docker-compose.yml already exists on the VPS from project creation. The workflow only needs to SSH in and run compose commands
- **Container listens on port 80 internally**: The compose file maps `LISTEN_PORT:80`. The app inside the container should serve on port 80
- **Caddy handles TLS**: Caddy is configured to reverse-proxy the domain to `localhost:{port}` with automatic HTTPS
