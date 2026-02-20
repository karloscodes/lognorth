#!/bin/sh
set -e

LOG_DIR="/app/logs"

# Ensure directories are writable by the app user
mkdir -p "$LOG_DIR"
chown -R lognorth:lognorth /app/storage /app/logs

# Log function
log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_DIR/entrypoint.log"
}

# Function to run migrations
run_migrations() {
  log "Running database migrations..."
  /app/lnctl migrate
  log "Migrations completed successfully"
}

# Handle different command modes
COMMAND=${1:-"server"}

case "$COMMAND" in
  migrate)
    log "Running migrations only..."
    run_migrations
    ;;

  server)
    log "Starting LogNorth server..."
    run_migrations
    chown -R lognorth:lognorth /app/storage /app/logs
    exec su-exec lognorth /app/lognorth-server
    ;;

  shell)
    log "Starting shell..."
    exec /bin/sh
    ;;

  status)
    log "Checking system status..."
    /app/lnctl status
    ;;

  *)
    log "Unknown command: $COMMAND"
    log "Valid commands: migrate, server (default), shell, status"
    exit 1
    ;;
esac
