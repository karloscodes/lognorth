#!/bin/bash
set -e

# LogNorth Release Script
# Usage: ./release.sh v1.0.0

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Usage: ./release.sh <version>"
    echo "Example: ./release.sh v0.1.0"
    exit 1
fi

# Validate version format
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Version must be in format v1.2.3"
    exit 1
fi

# Strip v prefix for Docker tag
DOCKER_VERSION="${VERSION#v}"
DOCKER_IMAGE="karloscodes/lognorth"

echo "=== LogNorth Release: $VERSION ==="

# Check for uncommitted changes
if [ -n "$(git status --porcelain)" ]; then
    echo "Error: Working directory has uncommitted changes"
    git status --short
    exit 1
fi

# Ensure we're on main
BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$BRANCH" != "main" ]; then
    echo "Warning: Not on main branch (currently on $BRANCH)"
    read -p "Continue anyway? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Run tests
echo ""
echo "=== Running Tests ==="
make test

# Cleanup runs no matter how this script exits — success, failure, or
# Ctrl-C. Belongs at the top so anything created below is covered.
CONTAINER_NAME="lognorth-release-test-$$"
cleanup() {
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
    docker rmi "$DOCKER_IMAGE:test" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

# Build Docker image (smoke test) — host platform only, just for the
# local health check. The pushed image (further down) is multi-arch.
echo ""
echo "=== Building Docker Image (smoke test, host arch) ==="
docker build --tag "$DOCKER_IMAGE:test" .

# Smoke test: start container and verify it becomes healthy
echo ""
echo "=== Smoke Testing Container ==="
TEST_PRIVATE_KEY=$(openssl rand -hex 32)
docker run -d --name "$CONTAINER_NAME" -p 3333:8080 \
  -e LOGNORTH_PRIVATE_KEY="$TEST_PRIVATE_KEY" \
  "$DOCKER_IMAGE:test"

# Wait for healthy (up to 30 seconds)
HEALTHY=false
for i in $(seq 1 15); do
    if curl -sf http://localhost:3333/_health > /dev/null 2>&1; then
        HEALTHY=true
        break
    fi
    sleep 2
done

if [ "$HEALTHY" = true ]; then
    echo "Container started and healthy!"
else
    echo "Error: Container failed to become healthy"
    docker logs "$CONTAINER_NAME" 2>&1 | tail -20
    exit 1
fi

# Push Docker image — multi-arch via buildx. Building only for the host
# arch (e.g. arm64 on Apple Silicon) silently ships a broken image to the
# amd64 servers in the fleet — which is exactly how v0.8.0 went out.
echo ""
echo "=== Pushing Multi-Arch Docker Image (linux/amd64, linux/arm64) ==="
BUILDER_NAME="lognorth-multiarch"
if ! docker buildx inspect "$BUILDER_NAME" >/dev/null 2>&1; then
    docker buildx create --name "$BUILDER_NAME" --driver docker-container --use >/dev/null
else
    docker buildx use "$BUILDER_NAME" >/dev/null
fi
docker buildx build \
    --platform linux/amd64,linux/arm64 \
    --tag "$DOCKER_IMAGE:$DOCKER_VERSION" \
    --tag "$DOCKER_IMAGE:latest" \
    --push \
    .

echo ""
echo "=== Creating Tag ==="
git tag -a "$VERSION" -m "Release $VERSION"
echo "Created tag: $VERSION"

echo ""
echo "=== Pushing Tag ==="
git push origin "$VERSION"
echo "Pushed tag to origin"

echo ""
echo "=== Release Complete ==="
echo ""
echo "Docker: $DOCKER_IMAGE:$DOCKER_VERSION"
echo "Docker: $DOCKER_IMAGE:latest"
echo ""
echo "GitHub Actions will create the release with binaries."
echo "Monitor at: https://github.com/karloscodes/lognorth/actions"
