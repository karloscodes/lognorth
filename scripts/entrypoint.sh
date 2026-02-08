#!/bin/sh
set -e

echo "Starting LogBuddy..."

# Run the server (it handles migrations internally)
exec /app/logbuddy-server
