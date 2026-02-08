# LogNorth Event Model

## Terminology

| Term | What it means |
|------|---------------|
| **Event** | A single log entry (message + context) |
| **Error** | An event where `is_error=true` |
| **Issue** | A unique error type, grouped by `context.error` |
| **Occurrence** | Each time an issue happens |
| **Alert** | Email notification (new error, regression, or spike) |

## Schema

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `message` | **Yes** | string | What happened (human readable) |
| `context` | No | object | All metadata (arbitrary key-value) |
| `timestamp` | No | ISO 8601 | When it happened (defaults to now) |
| `duration_ms` | No | int | How long it took |
| `trace_id` | No | string | Correlation ID for grouping related events |
| `is_error` | Auto | bool | Derived on ingest, not sent by client |

## Error Detection

`is_error` is computed on ingest:

```
is_error = true IF:
  - context.error exists (non-empty string), OR
  - context.status >= 500
```

No "levels" (info/warn/error). Binary: error or not.

**Rationale:** 400s are user errors, not bugs. Only 500s and exceptions need attention.

## Examples

### HTTP Request (success)

```json
{
  "message": "POST /api/users → 201",
  "duration_ms": 150,
  "trace_id": "req-abc-123",
  "context": {
    "method": "POST",
    "path": "/api/users",
    "status": 201,
    "user": "user@example.com"
  }
}
```

### HTTP Request (error)

```json
{
  "message": "POST /api/checkout → 500",
  "duration_ms": 50,
  "context": {
    "method": "POST",
    "path": "/api/checkout",
    "status": 500,
    "error": "Database connection timeout"
  }
}
```

### Background Job (success)

```json
{
  "message": "SendWeeklyEmails completed",
  "duration_ms": 45000,
  "trace_id": "job-xyz-789",
  "context": {
    "job": "SendWeeklyEmails",
    "emails_sent": 1523
  }
}
```

### Background Job (failed)

```json
{
  "message": "ProcessPayments failed",
  "duration_ms": 5000,
  "context": {
    "job": "ProcessPayments",
    "error": "Redis connection refused"
  }
}
```

### Generic Log

```json
{
  "message": "User signed up",
  "context": {
    "user_id": 123,
    "plan": "pro"
  }
}
```

## Issues Grouping

Events with `is_error=true` are grouped by `context.error` into issues.

Same error message (per app) = same issue.

## Alerts

Three alert types:

| Alert | Trigger | Signal |
|-------|---------|--------|
| **New error** | First occurrence ever | Something broke |
| **Regression** | Error quiet 24h+, then returns | Fix failed |
| **Spike** | Endpoint error rate jumps >N% | Crisis happening |

Configure in Settings → Notifications.
