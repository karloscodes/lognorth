---
name: lognorth-integrate
description: Use when setting up LogNorth in a project — installs the SDK, adds the middleware, and confirms events arrive. For a language without an SDK (Python, PHP, Elixir, Java, .NET, and others), writes a small client that follows the LogNorth protocol. Trigger when the user says "add lognorth", "set up logging", or "integrate lognorth".
---

# LogNorth Integration

Add LogNorth to the user's project, then prove it works by reading the events back. The job is done when you see the event, not when the code compiles.

## 1. Detect the stack

| File | Stack |
|------|-------|
| `go.mod` | Go |
| `package.json` | Node.js / Bun |
| `Gemfile` | Rails |
| Anything else | No SDK: write a client, see "Any other language" below |

If the project already uses OpenTelemetry (`@opentelemetry` in package.json, otel imports), use the OpenTelemetry setup below instead of an SDK.

## 2. Get the URL and one key per environment

- **URL**, for example `https://logs.yoursite.com`.
- **An app key per environment.** In LogNorth, each environment is its own app: `shop-prod`, `shop-staging`. Each app has its own key (`lgn-`, from the app's page). Do not share one key across environments.

Read them from the environment: `LOGNORTH_URL` and `LOGNORTH_API_KEY`. Never write a key into source code. The agent key (`lgn-agent-`) is for reading and cannot send events.

The SDKs send events in every environment except `development` and `test`. Staging and preview report too. Do not add a production-only check.

## 3. Install and configure

### Go

```bash
go get github.com/karloscodes/lognorth-sdk-go
```

```go
lognorth.Configure(lognorth.Options{
    URL:         os.Getenv("LOGNORTH_URL"),
    APIKey:      os.Getenv("LOGNORTH_API_KEY"),
    Environment: os.Getenv("APP_ENV"), // "development" or "test" turns sending off
})
lognorth.IgnorePaths("/healthz", "/metrics")

http.ListenAndServe(":8080", lognorth.Middleware(mux))
```

If the project logs with slog: `slog.SetDefault(slog.New(lognorth.NewHandler()))`.

### Node.js / Bun

```bash
npm install lognorth
```

```typescript
import LogNorth from 'lognorth'
import { middleware } from 'lognorth/express' // or /hono

LogNorth.config(process.env.LOGNORTH_URL!, process.env.LOGNORTH_API_KEY!) // environment defaults to NODE_ENV
app.use(middleware({ ignorePaths: ['/healthz', '/metrics'] }))
```

Next.js route handlers: `export const GET = withLogger()(handler)` from `lognorth/next`. Pino: see the SDK README.

### Rails

```ruby
# Gemfile
gem "lognorth"
```

Put the URL and key in credentials (`bin/rails credentials:edit`):

```yaml
lognorth:
  url: https://logs.yoursite.com
  api_key: lgn-...
```

The Railtie reads them and adds the middleware and the error subscriber. No initializer. It ignores `/up` by default. Only add what differs, for example `config.lognorth.ignored_paths = %w[/up /healthz]`.

### OpenTelemetry (any language)

Set the standard exporter variables. LogNorth accepts OTLP over HTTP/protobuf, the OTel default:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=https://logs.yoursite.com/otel
OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer lgn-..."
```

### Any other language

No SDK and no OpenTelemetry: write a small client in the project's language. The protocol is at https://lognorth.com/docs/integrations/protocol/. Read it if you can; the rules below are the part that must not be skipped.

**Send.** `POST {LOGNORTH_URL}/api/v1/events/batch` with `Authorization: Bearer {LOGNORTH_API_KEY}`, `Content-Type: application/json`, and `{"events": [...]}`. Batches of at most 500 events and 1 MB of JSON. Each event: `message` (required), `timestamp` (RFC 3339 UTC, set when it happens), `duration_ms`, `trace_id`, `context`.

**Time and order.** Stamp `timestamp` the moment the event happens (UTC, milliseconds) and keep it on every retry. Keep the logged order: first in, first out, a failed batch back at the front, one request at a time. LogNorth shows events by when they happened, so a late batch lands at its own time.

**Capture.** A middleware sends one event per request after the response: message `"METHOD /path → status"`, and `method`, `path` (no query string), `status` (a number), and `environment` in `context`. Unhandled errors add `error`, `error_class`, `error_file`, `error_line`, and `stack_trace`. Jobs send one event when they end. Send nothing in `development` and `test`.

**User and release.** Add `user` (an ID, never an email) to a request's events when someone is signed in, and `user_agent` to failed requests. Read the release once at startup (an option, or `LOGNORTH_RELEASE`, `GIT_SHA`, `KAMAL_VERSION`, or the host's commit variable), put it as `release` on error events, and log one event `Release <version> started` with `context.release` at startup.

**Buffer.** Never send on the request path and never raise into the app.
- Trim each event to 64 KB before it is buffered: `message` 1,000 characters, `stack_trace` 16 KB (keep the top), other context strings 8 KB; set `context.truncated = true`.
- Limit the buffer to 10,000 events or 10 MB of JSON, whichever comes first. At a limit, drop the oldest event that is not an error; drop an error only when nothing else is left. Count drops.
- Send at 10 events, at once for an error, at once past half of either limit, or 5 seconds after the first event. One request at a time. Flush on SIGTERM, SIGINT, and exit within a few seconds.

**Retry. Keep the events; drop only what can never be accepted.**

| Answer | Do |
|---|---|
| `2xx` | Done. Reset the backoff. If anything was dropped, send one event `LogNorth client dropped N events` with `context.dropped` and `context.dropped_errors`. |
| `503`, `429` | Put the batch back at the front. Wait the seconds in `Retry-After` (LogNorth always sends it), then send again. Leave the backoff as it is: it is for answers that do not say when. |
| Other `5xx`, `408`, network error, timeout | Put the batch back at the front. Back off 1s, 2s, 4s up to 60s, with 20% jitter. |
| `401`, `403`, `404` | Put the batch back at the front. One stderr line. Retry every minute, then up to every 5 minutes. |
| `413`, `400`, other `4xx` | Split the batch in half and resend each half. Drop and count only a single event that is still refused. |

Use a 5-second connect timeout and a 10-second request timeout. Keep accepting events while waiting. A retry takes a full batch from the front again, not only the events of the failed one.

Put the client in one small module the project owns, with no new dependency when the standard library can do HTTP and JSON. Test the parts that decide, against a local HTTP server: a 503 and a 429 with `Retry-After` are retried after the wait and do not grow the backoff, a 500 and a dropped connection are retried with backoff, a 401 keeps the events, a 413 splits the batch, the buffer limits drop non-errors first, and the order survives retries.

### Name the user

The issue page counts the users an issue hit, and shows what a user did before an error. Name the signed-in user where the app knows it, after authentication. Use an ID, never an email:

```go
lognorth.SetUser(r.Context(), strconv.Itoa(user.ID)) // inside lognorth.Middleware
```

```typescript
LogNorth.setUser(user.id) // inside a request the middleware wraps
```

```ruby
before_action { LogNorth.user = current_user&.id } # only when the app has no Current.user
```

The release needs no code when the deploy sets `KAMAL_VERSION`, `GIT_SHA`, or a host's commit variable. Otherwise pass it: `Release` in Go's `Options`, `release` in `LogNorth.config`, `release:` in Rails. Each release then shows on LogNorth's charts as a dashed line. These need SDK 0.2.0 or later.

## 4. Log what the middleware cannot see

Only where it earns its place: a payment, a signup, a background job.

```go
lognorth.Log("User signed up", map[string]any{"user_id": 123})
lognorth.Error("Checkout failed", err, map[string]any{"order_id": 42})
```

```typescript
LogNorth.log('User signed up', { user_id: 123 })
LogNorth.error('Checkout failed', err, { order_id: 42 })
```

```ruby
LogNorth.log("User signed up", user_id: 123)
LogNorth.error("Checkout failed", e, order_id: 42)
```

`Log` is batched (10 events or 5 seconds). `Error` is sent at once. The SDKs flush on SIGINT and SIGTERM.

## 5. Verify

1. Run the app with a non-development environment (for example `APP_ENV=staging`), because development does not send.
2. Make a request that reaches the middleware.
3. If the `lognorth` MCP tools are connected, call `search_logs` with `since: 5m` and the request's path. Seeing the event is the proof.
4. No MCP tools: ask the user to look at the app in LogNorth. Events appear within seconds.

Nothing arrived? Check, in order: the environment name, the URL and key variables, and whether the request path is in the ignore list.

Full options are in each SDK's README: [Go](https://github.com/karloscodes/lognorth-sdk-go), [Node](https://github.com/karloscodes/lognorth-sdk-ts), [Rails](https://github.com/karloscodes/lognorth-sdk-rails). For any other language, the [protocol](https://lognorth.com/docs/integrations/protocol/).
