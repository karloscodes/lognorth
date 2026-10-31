# LogNorth reference

Field meanings and triage rules. Read this when the tool output needs interpreting; the steps are in SKILL.md.

## Data model

One event is one thing that happened: a request, a job, a log line.

| Field | Description |
|-------|-------------|
| `message` | What happened, human readable |
| `timestamp` | When |
| `duration_ms` | How long it took |
| `trace_id` | Correlation id. Everything from one request shares it |
| `context` | Arbitrary metadata: `method`, `path`, `status`, `error`, `environment`, plus whatever the app sent |
| `is_error` | Derived on ingest: `context.error` or `context.error_class` exists, or `context.status >= 500` |
| `error_class`, `error_message` | The exception, split out of the context |
| `error_file`, `error_line` | Where it was raised. Open this in the repo |

There are no severity levels. An event is an error or it is not.

Each LogNorth app is one environment (`shop-prod`, `shop-staging`). `context.environment` is a label on the event, not a way to group issues.

## Alerts

`list_alerts` returns what LogNorth is alerting on now.

| Kind | Fires when |
|------|------------|
| `error_rate` | The last 5 minutes, or the last hour, has a higher error rate than the same time of day on the last 7 days allows |
| `silence` | The last 5 minutes had far fewer requests than the same time of the week on the last 4 weeks |
| `down` (in `down`) | The app's URL stopped answering its uptime ping |

- `severity`: `warning` goes to a daily digest. `critical` sent an email.
- `since`: when LogNorth first saw it. The problem started up to 5 minutes earlier.
- `current` and `normal`: the rate in % for `error_rate`, requests per 5 minutes for `silence`.
- An alert clears after two clean checks in a row, about 10 minutes.

A new endpoint needs a day of history before error rate alerts, and a week before silence alerts. Until then `endpoint_timeline` returns `history_is_ready: false`.

## Timeline

`endpoint_timeline` returns one point per step: `requests`, `errors`, `error_rate`. Steps are 5 minutes up to 24 hours back and 1 hour beyond. A step with 0 requests is a real gap in traffic, not missing data. `normal` is for the current time of day.

## Endpoints

`list_endpoints` returns one row per endpoint over a window (`since`, default `1h`), most broken first: `path`, `total` requests, `errors`, `error_rate` in %, and `avg_duration` in ms. Use it to find the endpoint, then `endpoint_timeline` to find when it started.

## Uptime

`uptime_timeline` is the app's URL ping, checked every minute.

| Field | Description |
|-------|-------------|
| `status` | `up`, `failing` (a ping failed, not yet 3 in a row), `down`, `pending` (no ping yet), `off`, or `none` (no URL) |
| `latency_ms` | The last ping's response time |
| `uptime_24h` | % of the last day's pings that were up |
| `down_since` | When it went down, if it is down |
| `bars` | 96 bars of 15 minutes, oldest first: `checks`, `fails`, and `failures`, why they failed (`503`, `no answer within 5s`) |
| `last_failure` | The most recent failed ping: time, status, latency, error |

A 5xx, a timeout (5 seconds), or no connection is a failure. A 4xx means the app answered, so it counts as up. 3 failures in a row mark the app down. When failures line up with errors in `search_logs`, the app answered and failed. When the log goes quiet at the same time, nothing reached the app: it stopped, or the proxy or network in front of it failed.

## Issues

Issues group identical errors. The counts are the story.

| Field | What it tells you |
|-------|-------------------|
| `count` | Total ever |
| `count_24h` | Is it happening right now? |
| `count_7d` | Weekly volume: how big is this? |
| `trend` | "up" = getting worse, "down" = improving |
| `is_new` | First seen under 24h ago: likely a fresh regression |
| `first_seen` / `last_seen` | When it started, and whether it has stopped |
| `active` | Had occurrences in the last 24h |
| `hash` | Stable id. Pass it to `search_logs` as `issue` |

**Triage order**

1. `is_new`: new since the last deploy. Look here first.
2. `trend: "up"` with a high `count_24h`: actively getting worse.
3. High `count_7d`, low `count_24h`, `trend: "down"`: an old problem already fading. Say so instead of raising an alarm.

## Traces

`get_event` returns `relatedEvents`: every event sharing the `trace_id`, in order. That is the whole request: the HTTP entry point, the jobs it triggered, and where it broke.

## Presenting results

| View | Format |
|------|--------|
| Alerts | One line each: path, what is wrong, since when, normal level |
| Timeline | The start of the problem, and the level before and after it. Not every point |
| Issues | Table: error, `count_24h`, `count_7d`, trend, last seen |
| Events | Table: message, time, path, status, duration |
| One event | The context object, with the error and `error_file:error_line` called out |
| Trace | Chronological list, marking where it broke |

## Requirements

MCP needs LogNorth v0.16.0 or later. `list_alerts`, `endpoint_timeline`, and the `issue` and `until` arguments need the release after v0.16.2. `list_endpoints`, `uptime_timeline`, and the `after_id` argument need v0.20.0. The agent key is read-only, starts with `lgn-agent-`, and comes from Settings > Developer. It is separate from the app keys (`lgn-`) that SDKs use to send events, and it cannot ingest events or sign in to the web UI.
