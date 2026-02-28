---
name: lognorth
description: Use when debugging production errors, checking logs, or user mentions "lognorth". Queries LogNorth error tracking API.
---

# LogNorth Agent API

Query your LogNorth instance to see errors, logs, and traces.

**Your role:** The user is debugging. They might ask "What's breaking in production?" or "Find events for trace abc-123". Query the API and present results clearly — tables for lists, details for single events, summaries for issues.

## Environment Variables

Requires:
- `LOGNORTH_URL` - Your LogNorth instance (e.g., `https://logs.yoursite.com`)
- `LOGNORTH_API_KEY` - API key from Apps page

## Data Model

| Concept | What it means |
|---------|---------------|
| **Event** | A single log entry (message + context metadata) |
| **Error** | An event where `is_error=true` (has `context.error` or status >= 500) |
| **Issue** | Unique error type, grouped by error message. Has occurrence count. |
| **Trace** | Events sharing a `trace_id` — typically one request's full journey |

### Event Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Unique identifier |
| `message` | string | What happened (human readable) |
| `timestamp` | ISO 8601 | When it happened |
| `duration_ms` | int | How long it took (optional) |
| `trace_id` | string | Correlation ID for related events (optional) |
| `is_error` | bool | True if error detected |
| `context` | object | All metadata — method, path, status, error, user, etc. |

### Issue Fields

| Field | Type | Description |
|-------|------|-------------|
| `hash` | string | Unique identifier for this error type |
| `error_message` | string | The error text |
| `count` | int | Total occurrences |
| `first_seen` | timestamp | When first occurred |
| `last_seen` | timestamp | Most recent occurrence |

## API Endpoints

### List Events

```bash
curl -s "$LOGNORTH_URL/api/v1/agent/events" \
  -H "Authorization: Bearer $LOGNORTH_API_KEY" | jq
```

**Filters:**

| Parameter | Example | Description |
|-----------|---------|-------------|
| `is_error` | `true` or `false` | Only errors or only successes |
| `search` | `connection+refused` | Text search in message/context |
| `limit` | `50` | Max results (default 50) |
| `offset` | `100` | Pagination offset |
| `start_time` | `2024-01-01T00:00:00Z` | Filter by time range |
| `end_time` | `2024-01-02T00:00:00Z` | Filter by time range |

**Examples:**

Recent errors:
```bash
curl -s "$LOGNORTH_URL/api/v1/agent/events?is_error=true&limit=10" \
  -H "Authorization: Bearer $LOGNORTH_API_KEY" | jq '.events[] | {id, message, timestamp}'
```

Search by trace ID:
```bash
curl -s "$LOGNORTH_URL/api/v1/agent/events?search=trace-abc-123" \
  -H "Authorization: Bearer $LOGNORTH_API_KEY" | jq '.events'
```

Events from last hour:
```bash
curl -s "$LOGNORTH_URL/api/v1/agent/events?start_time=$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ)" \
  -H "Authorization: Bearer $LOGNORTH_API_KEY" | jq '.events'
```

### Get Event Detail

```bash
curl -s "$LOGNORTH_URL/api/v1/agent/events/123" \
  -H "Authorization: Bearer $LOGNORTH_API_KEY" | jq
```

Returns:
- `event` — Full event with all context
- `eventContext` — Parsed context object
- `relatedEvents` — Other events with same trace_id

### List Issues

Grouped errors with occurrence counts:

```bash
curl -s "$LOGNORTH_URL/api/v1/agent/issues" \
  -H "Authorization: Bearer $LOGNORTH_API_KEY" | jq '.issues[] | {hash, error_message, count, last_seen}'
```

## Debugging Workflow

When user asks about production errors:

1. **Get the lay of the land** — List recent errors to see what's happening
   ```bash
   curl -s "$LOGNORTH_URL/api/v1/agent/events?is_error=true&limit=10" ...
   ```

2. **Find related events** — If error has a trace_id, search for it to see the full request
   ```bash
   curl -s "$LOGNORTH_URL/api/v1/agent/events?search=<trace_id>" ...
   ```

3. **Get full context** — Fetch event detail for complete metadata
   ```bash
   curl -s "$LOGNORTH_URL/api/v1/agent/events/<id>" ...
   ```

4. **Check issue frequency** — Is this a one-off or recurring pattern?
   ```bash
   curl -s "$LOGNORTH_URL/api/v1/agent/issues" ...
   ```

## Presenting Results

- **Errors list**: Table with message, timestamp, key context (path, status)
- **Single event**: Show full context object, highlight the error
- **Trace view**: Chronological list showing request flow
- **Issues**: Table with error message, count, first/last seen

Be concise. Surface the important details. Link related events by trace_id.
