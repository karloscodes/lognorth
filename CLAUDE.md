# CLAUDE.md

**See [AGENTS.md](./AGENTS.md) for complete guidelines.**

## Quick Reference

```bash
make dev          # Run Go API + React dev server
make test         # Run Go tests
make seed         # Generate sample data
make build        # Build production binary
```

## Architecture Overview

LogNorth follows the **Phoenix Contexts** pattern with **background job processing**:

```
internal/
├── http/                 # HTTP Transport Layer (thin handlers)
├── events/              # Context: Events domain
│   ├── ingested.go      # IngestedEvent model (raw events)
│   ├── event.go         # Event model (processed events)
│   └── issues.go        # Issues aggregation
├── apikeys/             # Context: API keys
├── alerts/              # Context: when to alert, and the channels that tell
├── mcp/                 # Context: MCP protocol + read-only agent tools
├── jobs/                # Context: Background jobs (Cartridge JobDispatcher)
│   ├── event_processor.go
│   └── cleanup.go
├── config/              # Configuration
└── app.go               # Application setup + routes
```

### Event Processing Flow

```
SDK → POST /api/v1/events → IngestedEvent table (fast write)
                                    ↓
                          Background Job (periodic)
                                    ↓
                          Event table (processed)
                                    ↓
                          Aggregates (issues, stats)
```

Counts are written as events move, never counted on a page load:
`endpoint_stats` holds 5-minute buckets per endpoint, and `app_hours` holds each
app's events and errors per UTC hour (`events.HourBatch`, in the processor's
transaction). `endpoint_durations` holds each endpoint's requests per UTC hour
and duration bin, each bin 10% wider than the one before
(`events.DurationBatch`), so the Endpoints page and `list_endpoints` read p95
and p99 within 10%. It is WITHOUT ROWID, keyed in the order the percentile
query groups, so the sum needs no sort. Traffic from before an install
counted bins has no percentiles. The home page, the log histogram, and the digest read
`app_hours`. Retention deletes whole hours, so the counts always match the
events. An event written outside the processor, as the demo seed writes
them, needs a recount (`events.RecountAppHours`).

An issue is one error of an app: its class, its message, its file and line.
The hash reads the message with its ids, numbers, UUIDs, emails, and hex
addresses replaced (`events.normalizeMessage`), so "Couldn't find User with
'id'=1001" and "'id'=1002" are one issue and one alert; the stored message
keeps them.

An endpoint is a route, not a URL (`events.EndpointKey`, `events/endpoint.go`):
the route the SDK sends when it covers the path segment by segment (older
Express SDKs send it relative to its router, so `/:id` under `/api/users` does
not count; a Go pattern loses its method and host, and `{id}` reads `:id`),
else the path. Either way its ids are replaced by `:id` (all digits, a UUID,
or hex of 16 characters or more), so a router mounted on `/orgs/:orgId` keys
as `/orgs/:id/...`. So `/users/123` and `/users/124` share one history and
one baseline. The pages, the MCP tools, and north show the key as the path.
`endpoint_timeline` finds the endpoint of a URL (`events.EndpointOf`), and a
log search for a key with a parameter (`/users/:id`, as the Endpoints page and
north send it) reads as the endpoint. Issues keep the paths as sent.

`issues` holds one row per issue: its error, first and last seen, count, and
first 10 paths (`events.IssueBatch`, in the same transaction). The issue list
pages it and counts only the last 14 days of errors of the issues on the page
(24h, 7d, trend, sparkline). An issue's count and first seen are all-time:
retention deletes an issue only when its last event goes, and the issue page
reads the same record. The owner's state of an issue (seen, muted, the
regression clock) stays in `issues_seen`: it is not derived from events.

### Agent Access

Agents read the log through MCP only. `POST /mcp` speaks JSON-RPC over HTTP,
authenticated with the agent key from Settings > Developer, and exposes eight
read-only tools: `list_apps`, `list_alerts`, `list_endpoints`,
`endpoint_timeline`, `uptime_timeline`, `list_issues`, `search_logs`,
`get_event`. There is no REST read API. The protocol lives in `internal/mcp`;
the handler is `internal/http/mcp.go`.

`north`, the open-source laptop CLI (`karloscodes/lognorth-cli`, MIT), reads
the same tools: `north tail` follows the log with `search_logs` `after_id`, and
`north top` reads `list_endpoints`, `list_alerts`, and `uptime_timeline`. Keep
those tools' output fields stable, or release north with the change. It shares
the agent rate limit, 60 calls a minute per IP.

Every alert email ends with `/lognorth:investigate <path or issue hash>`, a
command of the agent plugin in `plugin/`. Keep the tool names in sync with that
plugin's skill and with the `lognorth` command.

### Install

New installs run on Chasen (chasenhq.com). `install.sh` is the script behind
`curl -fsSL lognorth.com/install | bash`: it installs chasen-server and deploys
`karloscodes/lognorth:latest` with nightly updates. Each release attaches it in
this repo and in `lognorth-releases`, and lognorth.com/install redirects to the
newest one in `lognorth-releases`. On Chasen the image needs no settings: it reads `PRIVATE_KEY` and
`BASE_URL`, which Chasen sets. `e2e/chasen.sh`, `e2e/installer.sh`, and
`e2e/move-to-chasen.sh` test the three paths; `.github/workflows/chasen.yml`
runs them on fresh GitHub runners, because they install chasen-server on the
machine. A release runs them on the tag's commit before it publishes.

The older installer (`cmd/manager`, the `lognorth` command, built on matcha)
is gone since 22 October 2026 (user: remove matcha), and releases attach no
binaries. Its servers keep the `lognorth` command they have (v0.28.1 is the
last), and its nightly update still pulls each new image. Never attach a
`lognorth-linux-*` binary to a release again: servers of the older installer
update themselves from the newest lognorth-releases release, and a binary
without `update` would end their nightly update. `e2e/move-to-chasen.sh` sets
up the old server from v0.28.1.

### lognorth-releases

`karloscodes/lognorth-releases` is phased out slowly, with no end date (user, 31
October 2026). Until then each thing is in both places, and a change to one must
reach the other:

- **Releases:** each release is published here and there, with `install.sh`.
- **Image:** `ghcr.io/karloscodes/lognorth` and
  `ghcr.io/karloscodes/lognorth-releases` get the same tags.
- **Agent plugin:** the source is `plugin/`. `.github/workflows/plugin.yml`
  copies it to that repo's root, where agents install it. Never edit it there.
- **Stats:** lognorth.com counts from both: the releases of both repos, each tag
  once, and the pulls of both packages.

Never delete that repo: it holds the plugin that agents installed and the
binaries up to v0.28.1. A failed self-update of the older installer is only a
warning, so stopping its releases does not stop those servers' image updates.

### Alerting

Alerting is the product's moat. Everything that decides or sends an alert is
in `internal/alerts`, a context of its own. The detectors never send email:
each records an `Alert` (`alerts/alert.go`), and `alerts.Deliver` (the
`AlertSender` job, every 15 seconds) sends them. Time is a parameter
everywhere, so the scenario tests replay days of traffic in seconds.

**Who hears what, and when** (`Deliver`), per app:

- An app's alerts go out together once the oldest waited 30 seconds, so a
  burst (a deploy that breaks 20 things) lands in one email.
- An app gets at most one email per 10 minutes. The first "down" of an
  incident does not wait for it.
- In quiet hours nothing goes out. When they end, the night's alerts go out
  in one email per app ("while you were off"). Quiet hours, the digest hour,
  and the times in an email are in the owner's zone (`settings.TimeZone`, saved
  from the browser; UTC without one).
- Without a channel, alerts are dropped, so setting one up later sends
  nothing old. An alert that cannot go out for a day is dropped.
- One alert keeps the subject it always had ("shop — new: TypeError: boom").
  Several are counted ("shop — down, 15 endpoints failing, 1 new error") and
  listed, at most 10.

**The detectors:**

- New errors and regressions (`alerts/errors.go`, `CheckError`): an issue is
  new, or back after a quiet day. A muted issue stays quiet. The hash ignores
  ids in the message (`events/issue.go`).
- Endpoints (`alerts/endpoints.go`, every 5 minutes, on the bucket that ended
  at least a minute ago). Baselines come from stored history, never learned
  or saved (`alerts/spc_history.go`); stats are kept 29 days, in UTC, keyed on
  the route.
  - Spike: the bucket against the same time of day over 7 days, a p-chart
    widened by measured dispersion. The normal rate drops past outages, so a
    repeat still emails.
  - Drift: the last hour, without its spike buckets, against the same hour on
    the last 7 days. It holds only while the last 15 minutes are still above
    normal, so it catches a slow rise and never echoes a spike that is over.
  - Small endpoints, too small for the spike check: critical when the last 15
    minutes have 5 errors and half the requests failed, the bucket just
    checked fails as much, and the endpoint normally fails under 10%.
  - Silence: median and MAD at the same time of week over 4 weeks.
  - Warning (3σ) goes to the daily digest. Critical (5σ) also needs a real
    change, at least double the normal rate and a point above it, and then
    records one alert per path per hour, only while its last check was bad. An
    alert clears after 2 clean checks. While the app is down, an endpoint's
    alert waits: the down alert names the incident and the errors that surged.
- Uptime (`alerts/uptime.go`, `ApplyPing`): down after 3 failed pings, timed
  from the first; recovered after 5 good pings in a row, so a flapping app
  sends one down.
- Leaks (`alerts/leaks.go`): once per app and kind of secret.

The scenario tests (`alerts/scenario_*_test.go`, the `world` in
`world_test.go`) replay four weeks of history and the incident through the
real detectors and sender, and count the emails a webhook gets. A change to
when LogNorth alerts starts with a scenario.

Secrets (`internal/events/secrets.go`, the alert in `alerts/leaks.go`): `moveEvents` calls
`events.RedactSecrets` before it stores an event, so no stored event, email,
or MCP answer holds one. A value under a secret name (password,
authorization, cookie, api_key, *token) is redacted silently. A value in a
known format (Stripe live key, AWS, GitHub, Slack, Anthropic, OpenAI, private
key) is redacted and alerts once per app and kind (`secrets_seen`). A leak
seen before a channel exists tries again on the next one.

### Status Pages

One public page per product (`status_pages`, Settings > Status pages). An app
is on one page at most (`status_page_id` on `api_keys`, the Apps page `[edit]`
panel). A page never shows the ping URL. It answers at `/status/:slug`, and at
its own domain when it has one:

- `StatusDomainMiddleware` runs before every route. On a page's domain, `/` is
  the page, static files load, and every other path redirects to `/`, so the
  dashboard and the login never answer there. The page's domain decides, never
  BASE_URL: Chasen sets BASE_URL to the domain added last, which can be a
  page's. A page can't take the host the owner saves it from, or the saved
  base URL's host.
- The proxy must send the domain to the container: `chasen domains add`. The
  session cookie is host-only, so notes are posted on the LogNorth domain.

`internal/status` builds a page from `uptime.HistoryFor`:

- Pings are kept 30 days. The cleanup job runs `uptime.RollUpAll` first, which
  stores each finished UTC day once in `uptime_days` (pings, fails, minutes
  down), kept for good. The bars read stored days past the pings, and
  `History.AllTime` adds them up since the first ping.
- Incidents are `uptime.Outages`, not a table, so they go back 30 days. A
  note (`status_comments`) points at its outage by app and first failed ping.
- Visitors get a page cached for 1 minute (`internal/http/status.go`). The
  signed-in owner gets a fresh one and the note form.

### Correlations

Show a link between two things only when the data makes it strong. Weak
correlations stay out of the UI, the emails, and the MCP tools.

- **Same event:** the SDKs stamp `user` (all requests) and `release` (errors
  only) on the event, and `user_agent` on failed requests.
  `events.GetIssueReach` counts the users and names the releases of an issue.
- **Outage and issue:** `uptime.Outages` is a run of 3 or more failed pings
  (`uptime.FailuresToDown`, the same as the down alert). An issue surged in an
  outage when it has 3 or more errors from 5 minutes before to the end, at 5
  times its own rate over the day before (`events.SurgedDuring`). An issue that
  errs all day never qualifies. `uptime.RecentOutages` joins the two for the
  issue page, the down email, and `uptime_timeline`.
- **Same request:** issue A is a cause of issue B when A failed first in the
  same trace in at least 80% of B's failed requests (5 or more, 7 days), and A
  is at least 5 times more common there than in all requests (the lift keeps
  out an error that happens everywhere). `events.ChainsOf` feeds the issue
  page, `events.CausesByIssue` the issue list and `list_issues` (`after`).
- **Same user:** `events.StepsBefore` lists the requests (a method and a path)
  of an error's user in the 30 minutes before it, for the issue page, the log
  detail, and `get_event` (`stepsBefore`).
- **Deploys:** the SDKs log `Release <version> started` at boot. The processor
  keeps each release's first time in `releases` (`events.NoteRelease`), and
  the logs chart, the uptime strip, and the issue heatmap mark it
  (`components/release-marks.tsx`). No other source sets a deploy time.

### Event Model

See [docs/EVENT_MODEL.md](./docs/EVENT_MODEL.md) for full specification.

**Schema:**
| Field | Required | Description |
|-------|----------|-------------|
| `message` | Yes | What happened (human readable) |
| `timestamp` | No | When it happened (defaults to now) |
| `duration_ms` | No | How long it took |
| `trace_id` | No | Correlation ID for grouping related events |
| `context` | No | All metadata (arbitrary key-value JSON) |
| `is_error` | Auto | Derived on ingest, not sent by client |

**Error Detection:** `is_error` is computed on ingest:
- `context.error` exists → is_error = true
- `context.status >= 500` → is_error = true

No levels (info/warn/error). Binary: error or not.

## Handler Pattern

```go
// ✅ GOOD: Thin handler
func EventsPageAction(ctx *cartridge.Context) error {
    eventList, total, _ := events.List(ctx.DB(), filters)
    stats := events.GetStats(ctx.DB(), apiKeyID)
    return RenderPage(ctx.Ctx, "Events", Props{"events": eventList, "stats": stats})
}

// ❌ BAD: Business logic in handler
func EventsPageAction(ctx *cartridge.Context) error {
    db := ctx.DB()
    var count int64
    db.Model(&Event{}).Count(&count) // Direct DB access
}
```

## Styling Rules

**Black, white, and red. Sharp edges. Confident typography. Human voice.**

### Design Philosophy

Small company energy. Not enterprise software. This is a tool made by developers for developers. Opinionated, confident, no hedging. Think 37signals meets Swiss poster design.

### Voice & Copy

Write like you talk. No jargon. No corporate speak.

| Don't | Do |
|-------|-----|
| "No log entries found" | "Quiet so far" |
| "0 issues detected" | "No issues. Enjoy your day." |
| "X occurrences" | "X times" |
| "Streaming data" | "Watching for requests" |
| "Affected Endpoints" | "Where it happened" |
| "entries" | "requests" |

**Tagline:** "Logging you own."

**Empty states should have personality:**
- "Quiet so far" (no data yet)
- "Nothing here" (filtered to empty)
- "Enjoy your day." (no issues)
- "Nice work." (all clear)

### Terminal Theme

It reads like a terminal session. North, the default, has the same palette and
font as lognorth.com. Tokens live in `web/src/index.css`; use them through
Tailwind, never as hex values. Other themes (Catppuccin, Gruvbox, Nord,
Dracula, Amber) redefine the base tokens under `[data-theme]`; pick one
with `:theme` or Settings > Appearance (`web/src/lib/theme.ts`). The accent
stays green, because it means live and up. Amber is the one exception: a
one-color CRT whose accent is the bright phosphor. It also sets `--font` to
VT323 (bundled, OFL) and adds scanlines.

| Token (Tailwind) | Use |
|---|---|
| `bg-background` | page (#0c0a09) |
| `bg-raise` | selected row, hover, status bar (#1c1917) |
| `text-foreground` | headings, primary values |
| `text-text` | body copy |
| `text-subtle` | secondary text |
| `text-dim` | tertiary text; the floor for readable text (~5.3:1) |
| `border-border` / `border-line` | strong line (panes) / hairline (rows) |
| `text-error` / `bg-error` | errors only (#e11d48) |
| `text-accent` / `bg-accent` | lime (#a3e635): wordmark brackets, active tab brackets, live dots, key hints in navigate mode |

- **No gray, no hex:** never `gray-*` or `text-[#...]`. Use the tokens above.
- **Red means error.** Never decorative. An endpoint turns red only when it
  alerts against its own history (`alerts.ListAlerts`), not by a fixed rate.
- **No rounded corners.** `--radius` is 0. Radios and checkboxes are squares.
- **Table columns in px, not `ch`.** Header rows are 11px and data rows 13px,
  and `ch` would size the same column differently in each.
- **Font:** IBM Plex Mono only, bundled in `web/src/fonts/` (OFL) and loaded by
  `@font-face` in `index.css`. Vite hashes the files into `/assets`, so a
  self-hosted install makes no third-party font requests. Max weight is 700.
  Ligatures are off so `->` and `!=` stay literal.

### Components

- **Pane:** `.pane` with a `.pane-title` (and optional `.pane-right`) set into
  the top border: `<div className="pane"><span className="pane-title">stack trace</span>...`
- **Section label:** `.rule-label` renders `active  last 7 days ─────`.
- **Key chip:** `.key` with a `<kbd>`: `<button className="key"><kbd>m</kbd> mute</button>`.
- **Toggles:** the active option wears lime brackets: `[errors]`, `[7d]`.
- **Page header:** `PageHeader` in `lib/shared.tsx` (26px bold title, subtle subtitle).
- **Tabs:** `1 [issues] 2 logs 3 endpoints`; the active tab gets lime brackets.
- **Stack traces:** `components/stack-trace.tsx` shows app frames and folds
  library frames (`lib/stack.ts` decides which is which).

### Keyboard (herdr style)

- **Prefix:** `ctrl+b` opens navigate mode (the layout owns it). Tab numbers
  light up and hint badges appear on every `data-key` element.
  Then `1-3` tabs, `h/l` move, `enter` go, `0` home, `a` apps, `s` settings,
  `w` app picker, `?` keys, `esc` cancel.
- **Normal mode:** keys go to what is on screen. Pages add their own with
  `pageKey(e)` from `lib/keys.ts`, which stays quiet while typing or while
  navigate mode is open. Issues: `j/k enter m e E`. Issue and log pages: `esc`.
  Logs: `/`.
- **Status bar** (`layout.tsx`, desktop only): mode, screen, and the keys that
  work right now. Add a screen's keys to `SCREEN_KEYS`.
- **Command line:** `:` opens a vim-style line in the status bar
  (`components/command-line.tsx`). Commands that work anywhere live in
  `COMMANDS` in `lib/commands.ts`; a page adds its own while it is on screen
  with `usePageCommand('mute', fn)`. A command returns nothing or a status
  line, and a line that starts with `E` shows red. Tab completes, ↑ ↓ recall.

### Page Subtitles

Consistent pattern for counts:

```tsx
<span className="font-mono tabular-nums">{count}</span> request{count !== 1 ? 's' : ''}
```

Examples:
- "10 unique errors"
- "5,000 requests"
- "9 endpoints · 3,553 requests · 379 errors"

## Strong Requirements

- **Phoenix Contexts:** Business logic in context packages
- **Background jobs:** Events processed asynchronously
- **Theme tokens only:** no `gray-*`, no hex colors. See Terminal Theme above
- **Test integrity:** Never skip tests
