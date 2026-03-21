# Feature: OpenTelemetry Log Ingestion

## Goal
Let people using OpenTelemetry SDKs send logs to LogNorth without adding our SDK — reducing integration cost and vendor lock-in.

## Behavior

### Endpoint
- `POST /api/v1/otel/logs` accepts OTLP JSON log format (not protobuf)
- Authenticated with the same `Authorization: Bearer <api-key>` as the existing endpoint
- Same rate limiting as the existing ingest endpoint

### Translation
- Walks the nested OTLP structure: `resourceLogs[] > scopeLogs[] > logRecords[]`
- Each `logRecord` becomes one `IngestedEvent`:

| OTLP field | LogNorth field | Notes |
|------------|---------------|-------|
| `body.stringValue` | `message` | Required — skip record if missing |
| `timeUnixNano` | `timestamp` | Convert from nanoseconds to time.Time |
| `attributes` + `resource.attributes` | `context` | Flatten into key-value map |
| `severityNumber >= 17` (ERROR+) | `is_error = true` | OTel severity ERROR=17, FATAL=21 |
| `traceId` | `trace_id` | Hex-encoded string from OTel |
| Attribute `error.message` or `exception.message` | `context.error` | Standard OTel semantic conventions |
| Attribute `error.type` or `exception.type` | `context.error_class` | For issue grouping |

- Everything else is dropped
- Resource attributes (like `service.name`, `host.name`) are merged into `context` with their OTel key names
- If `context.error` or `context.error_class` exists → `is_error = true` (same existing logic applies on top)

### Error Detection
Combines OTel severity with existing LogNorth rules:
- `severityNumber >= 17` → `is_error = true`
- `context.error` exists → `is_error = true` (existing rule)
- `context.status >= 500` → `is_error = true` (existing rule)

### Response
- Returns `201` with `{"created": N}` (same format as existing endpoint)
- Returns `{"created": N, "errors": [...]}` for partial failures
- Returns `400` for unparseable body

## Acceptance
- [ ] `POST /api/v1/otel/logs` accepts OTLP JSON log exports
- [ ] API key authentication works the same as existing ingest
- [ ] Log records with `body.stringValue` map to `message`
- [ ] `timeUnixNano` correctly converts to timestamp
- [ ] OTel attributes + resource attributes flatten into `context`
- [ ] `severityNumber >= 17` sets `is_error = true`
- [ ] `traceId` maps to `trace_id`
- [ ] `exception.message` / `error.message` maps to `context.error`
- [ ] `exception.type` / `error.type` maps to `context.error_class`
- [ ] Events land in `ingested_events` table and get processed normally
- [ ] No new Go dependencies (parse OTLP JSON with `encoding/json`)
- [ ] Response format matches existing ingest endpoint

## Notes
- OTLP JSON only. No protobuf — avoids proto dependency and keeps it simple.
- No `/v1/otel/traces` or `/v1/otel/metrics` — LogNorth is not a tracing/metrics platform.
- OTel attributes use `KeyValue` with typed values (`stringValue`, `intValue`, `boolValue`, etc.) — need to unwrap these into plain Go values.
- OTLP JSON spec: https://opentelemetry.io/docs/specs/otlp/#otlphttp-request
