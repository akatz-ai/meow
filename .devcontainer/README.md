# Development Container

This project uses a devcontainer for consistent development environments.

## Quick Start

### Option 1: VS Code Dev Containers

1. Install the [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers)
2. Copy `.devcontainer/.env.example` to `.devcontainer/.env`
3. Set `BASE_IMAGE` to your dev container (or leave default for minimal setup)
4. Open command palette → "Dev Containers: Reopen in Container"

### Option 2: Docker Compose

```bash
cd .devcontainer
cp .env.example .env
# Edit .env with your BASE_IMAGE

docker compose up -d
docker compose exec dev zsh
```

## Base Image

This devcontainer uses a configurable base image via the `BASE_IMAGE` environment variable. This allows each developer to use their own base container with their preferred tools, while the project-specific configuration remains shared.

### Recommended Setup

1. Build your personal dev-container with your tools:
   ```bash
   git clone https://github.com/YOUR_USERNAME/dev-container
   cd dev-container
   docker build -t ghcr.io/YOUR_USERNAME/dev-container:latest .
   docker push ghcr.io/YOUR_USERNAME/dev-container:latest
   ```

2. Set your base image in `.devcontainer/.env`:
   ```
   BASE_IMAGE=ghcr.io/YOUR_USERNAME/dev-container:latest
   ```

### Without a Base Image

If you don't set `BASE_IMAGE`, the container falls back to `ubuntu:25.10`. You'll need to install tools manually, but the container will still work.

## Project-Specific Tools

This project requires:
- Go 1.21+

These should be included in your base image, or installed manually.
