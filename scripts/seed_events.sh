#!/bin/bash

# Seed script that sends 5000 realistic Rails/Sidekiq events to LogBuddy
# Usage: ./scripts/seed_events.sh <api_key>

set -e

API_KEY="${1:-}"
BASE_URL="${2:-http://localhost:8080}"
TOTAL_EVENTS=5000
BATCH_SIZE=100

if [ -z "$API_KEY" ]; then
  echo "Usage: $0 <api_key> [base_url]"
  echo "Example: $0 lb_abc123 http://localhost:8080"
  exit 1
fi

# Arrays for realistic data
CONTROLLERS=("UsersController" "OrdersController" "ProductsController" "SessionsController" "PaymentsController" "ApiController" "AdminController" "ReportsController")
ACTIONS=("index" "show" "create" "update" "destroy" "search" "export")
HTTP_METHODS=("GET" "GET" "GET" "POST" "PUT" "DELETE" "GET")  # Weighted towards GET
PATHS=("/users" "/users/123" "/orders" "/orders/456/items" "/products" "/products/search" "/api/v1/data" "/admin/dashboard" "/reports/daily")

SIDEKIQ_JOBS=("UserMailer" "OrderProcessor" "ReportGenerator" "DataExporter" "NotificationSender" "CacheWarmer" "CleanupJob" "SyncJob")
SIDEKIQ_QUEUES=("default" "mailers" "critical" "low")

ERROR_MESSAGES=(
  "ActiveRecord::RecordNotFound: Couldn't find User with id=999"
  "ActionController::RoutingError: No route matches [GET] /unknown"
  "Redis::CannotConnectError: Error connecting to Redis"
  "Timeout::Error: execution expired"
  "ActiveRecord::ConnectionTimeoutError: could not obtain a connection"
  "JSON::ParserError: unexpected token"
  "ArgumentError: invalid byte sequence in UTF-8"
  "NoMethodError: undefined method 'foo' for nil:NilClass"
)

# Generate random number in range
rand() {
  echo $((RANDOM % $1))
}

# Generate random trace ID
trace_id() {
  cat /dev/urandom | LC_ALL=C tr -dc 'a-f0-9' | head -c 32
}

# Generate ISO timestamp (last 24 hours)
timestamp() {
  local offset=$((RANDOM % 86400))
  if [[ "$OSTYPE" == "darwin"* ]]; then
    date -u -v-${offset}S +"%Y-%m-%dT%H:%M:%SZ"
  else
    date -u -d "-${offset} seconds" +"%Y-%m-%dT%H:%M:%SZ"
  fi
}

# Build a Rails HTTP request event
rails_request_event() {
  local is_error=$1
  local controller=${CONTROLLERS[$(rand ${#CONTROLLERS[@]})]}
  local action=${ACTIONS[$(rand ${#ACTIONS[@]})]}
  local method=${HTTP_METHODS[$(rand ${#HTTP_METHODS[@]})]}
  local path=${PATHS[$(rand ${#PATHS[@]})]}
  local duration=$((50 + RANDOM % 2000))
  local tid=$(trace_id)
  local ts=$(timestamp)

  local status=200
  local error_field=""

  if [ "$is_error" = "1" ]; then
    local error_type=$((RANDOM % 3))
    case $error_type in
      0) status=500; error_field=",\"error\":\"${ERROR_MESSAGES[$(rand ${#ERROR_MESSAGES[@]})]}\"" ;;
      1) status=502; error_field=",\"error\":\"Bad Gateway\"" ;;
      2) status=503; error_field=",\"error\":\"Service Unavailable\"" ;;
    esac
  else
    local status_type=$((RANDOM % 10))
    case $status_type in
      0) status=201 ;;
      1) status=204 ;;
      2) status=301 ;;
      3) status=302 ;;
      4) status=400 ;;
      5) status=401 ;;
      6) status=404 ;;
      *) status=200 ;;
    esac
  fi

  cat <<EOF
{
  "message": "${method} ${path} - ${status}",
  "timestamp": "${ts}",
  "duration_ms": ${duration},
  "trace_id": "${tid}",
  "context": {
    "controller": "${controller}",
    "action": "${action}",
    "method": "${method}",
    "path": "${path}",
    "status": ${status},
    "format": "html",
    "ip": "192.168.1.$((RANDOM % 255))"${error_field}
  }
}
EOF
}

# Build a Sidekiq job event
sidekiq_event() {
  local is_error=$1
  local job=${SIDEKIQ_JOBS[$(rand ${#SIDEKIQ_JOBS[@]})]}
  local queue=${SIDEKIQ_QUEUES[$(rand ${#SIDEKIQ_QUEUES[@]})]}
  local duration=$((100 + RANDOM % 5000))
  local jid=$(trace_id)
  local ts=$(timestamp)

  if [ "$is_error" = "1" ]; then
    local err=${ERROR_MESSAGES[$(rand ${#ERROR_MESSAGES[@]})]}
    cat <<EOF
{
  "message": "[Sidekiq] ${job} failed",
  "timestamp": "${ts}",
  "duration_ms": ${duration},
  "trace_id": "${jid}",
  "context": {
    "job": "${job}",
    "queue": "${queue}",
    "jid": "${jid}",
    "retry": $((RANDOM % 5)),
    "error": "${err}"
  }
}
EOF
  else
    cat <<EOF
{
  "message": "[Sidekiq] ${job} completed",
  "timestamp": "${ts}",
  "duration_ms": ${duration},
  "trace_id": "${jid}",
  "context": {
    "job": "${job}",
    "queue": "${queue}",
    "jid": "${jid}"
  }
}
EOF
  fi
}

# Build a traced request (multiple events with same trace_id)
traced_request() {
  local tid=$(trace_id)
  local base_ts=$(timestamp)
  local controller=${CONTROLLERS[$(rand ${#CONTROLLERS[@]})]}
  local action=${ACTIONS[$(rand ${#ACTIONS[@]})]}
  local method=${HTTP_METHODS[$(rand ${#HTTP_METHODS[@]})]}
  local path=${PATHS[$(rand ${#PATHS[@]})]}
  local is_error=$1

  local status=200
  local error_field=""
  if [ "$is_error" = "1" ]; then
    status=500
    error_field=",\"error\":\"${ERROR_MESSAGES[$(rand ${#ERROR_MESSAGES[@]})]}\""
  fi

  # Event 1: Request start
  cat <<EOF
{
  "message": "${method} ${path} started",
  "timestamp": "${base_ts}",
  "duration_ms": 0,
  "trace_id": "${tid}",
  "context": {
    "controller": "${controller}",
    "action": "${action}",
    "method": "${method}",
    "path": "${path}",
    "phase": "start"
  }
}
EOF
  echo ","

  # Event 2: DB query
  cat <<EOF
{
  "message": "SQL: SELECT * FROM users",
  "timestamp": "${base_ts}",
  "duration_ms": $((10 + RANDOM % 100)),
  "trace_id": "${tid}",
  "context": {
    "query": "SELECT * FROM users WHERE id = ?",
    "phase": "db"
  }
}
EOF
  echo ","

  # Event 3: Request complete
  cat <<EOF
{
  "message": "${method} ${path} - ${status}",
  "timestamp": "${base_ts}",
  "duration_ms": $((100 + RANDOM % 500)),
  "trace_id": "${tid}",
  "context": {
    "controller": "${controller}",
    "action": "${action}",
    "method": "${method}",
    "path": "${path}",
    "status": ${status}${error_field}
  }
}
EOF
}

# Build a batch of events
build_batch() {
  local count=$1
  echo '{"events":['

  local i=0
  while [ $i -lt $count ]; do
    local is_error=0
    if [ $((RANDOM % 10)) -eq 0 ]; then
      is_error=1
    fi

    # 30% chance of traced request (3 events), 70% single events
    if [ $((RANDOM % 10)) -lt 3 ] && [ $((i + 3)) -le $count ]; then
      traced_request $is_error
      i=$((i + 3))
    elif [ $((RANDOM % 10)) -lt 7 ]; then
      rails_request_event $is_error
      i=$((i + 1))
    else
      sidekiq_event $is_error
      i=$((i + 1))
    fi

    if [ $i -lt $count ]; then
      echo ","
    fi
  done

  echo ']}'
}

echo "Sending ${TOTAL_EVENTS} events to ${BASE_URL}..."
echo "API Key: ${API_KEY:0:10}..."
echo ""

sent=0
batches=$((TOTAL_EVENTS / BATCH_SIZE))

for ((b=0; b<batches; b++)); do
  payload=$(build_batch $BATCH_SIZE)

  response=$(curl -s -X POST "${BASE_URL}/api/v1/events" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/json" \
    -d "${payload}")

  created=$(echo "$response" | grep -o '"created":[0-9]*' | cut -d: -f2)
  sent=$((sent + created))

  progress=$((100 * sent / TOTAL_EVENTS))
  printf "\r[%-50s] %d%% (%d/%d events)" $(printf '#%.0s' $(seq 1 $((progress/2)))) $progress $sent $TOTAL_EVENTS
done

echo ""
echo "Done! Sent ${sent} events."
