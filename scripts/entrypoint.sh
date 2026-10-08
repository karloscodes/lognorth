#!/bin/sh
set -e

LOG_DIR="${LOGNORTH_LOGS_DIR:-/app/logs}"

mkdir -p "$LOG_DIR"

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

    # Demo mode: reset and seed on every start.
    #
    # Wiping the DB is destructive, so we require an explicit second opt-in
    # (LOGNORTH_DEMO_RESET_OK=true) on top of LOGNORTH_DEMO=true. Without it,
    # demo containers boot against whatever data is already on disk — which
    # is what you want if LOGNORTH_DEMO ever gets set on a non-demo container
    # by accident (e.g. copy-pasted env config). One env var typo should not
    # be able to wipe a customer database.
    if [ "$LOGNORTH_DEMO" = "true" ]; then
      if [ "$LOGNORTH_DEMO_RESET_OK" = "true" ]; then
        log "Demo mode: resetting database..."
        rm -f /app/storage/*.db /app/storage/*.db-*
        # Seed runs in the background so /_health answers within the
        # kamal-proxy timeout. The demo page will look empty for ~60s
        # after a restart — acceptable tradeoff vs. failed deploys.
        (/app/seed-demo && log "Demo data seeded.") &
      else
        log "Demo mode active but LOGNORTH_DEMO_RESET_OK is not 'true' — skipping DB reset/seed. Existing data on disk will be served as-is."
      fi
    fi

    exec /app/lognorth-server
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
