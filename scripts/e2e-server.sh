#!/bin/bash
# Start LogNorth server for e2e testing

set -e

# Use a separate test database
export LOGNORTH_ENV=test
export LOGNORTH_STORAGE_PATH=./storage-e2e
export LOGNORTH_PORT=8080

# Clean up on exit
cleanup() {
    echo "Cleaning up e2e test environment..."
    rm -rf ./storage-e2e
}

# Handle script termination
trap cleanup EXIT

# Clean previous test data
rm -rf ./storage-e2e
mkdir -p ./storage-e2e

echo "Building LogNorth..."
cd web && npm run build && cd ..
go build -o tmp/lognorth-e2e cmd/lognorth/main.go

echo "Seeding test data..."
go run cmd/seed/main.go

echo "Starting LogNorth server on port 8080..."
exec ./tmp/lognorth-e2e
