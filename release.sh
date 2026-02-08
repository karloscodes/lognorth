#!/bin/bash
set -e

# Fusionaly Pro Release Script
# Usage: ./release.sh v1.0.0

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Usage: ./release.sh <version>"
    echo "Example: ./release.sh v1.0.1"
    exit 1
fi

# Validate version format
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Version must be in format v1.2.3"
    exit 1
fi

echo "=== Fusionaly Pro Release: $VERSION ==="

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
make test-all

# Build to verify compilation
echo ""
echo "=== Building ==="
make build

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
echo "GitHub Actions will now build and publish the release."
echo "Monitor at: https://github.com/karloscodes/fusionaly-pro/actions"
echo ""
echo "To test GoReleaser locally (current platform only):"
echo "  goreleaser build --snapshot --clean --single-target"
