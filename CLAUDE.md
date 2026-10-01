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

### Agent Access

Agents read the log through MCP only. `POST /mcp` speaks JSON-RPC over HTTP,
authenticated with the agent key from Settings > Developer, and exposes eight
read-only tools: `list_apps`, `list_alerts`, `list_endpoints`,
`endpoint_timeline`, `uptime_timeline`, `list_issues`, `search_logs`,
`get_event`. Tool calls need a valid license (`license.IsConfigured()`); without
one every tool answers `mcp.Unlicensed`. Nothing else checks the license.
`internal/mcp` is under the Fair Core License (`internal/mcp/LICENSE.md`); the
rest of the repo is MIT. Keep the license check and the tools inside that
directory.
There is no REST read API. The protocol lives in `internal/mcp`;
the handler is `internal/http/mcp.go`.

`north`, the open-source laptop CLI (`karloscodes/lognorth-cli`, MIT), reads
the same tools: `north tail` follows the log with `search_logs` `after_id`, and
`north top` reads `list_endpoints`, `list_alerts`, and `uptime_timeline`. Keep
those tools' output fields stable, or release north with the change. It shares
the agent rate limit, 60 calls a minute per IP. `cmd/manager` is the server
command, `lognorth`, and stays server-only.

Every alert email ends with `/lognorth:investigate <path or issue hash>`, a
command in the plugin repo (`lognorth-releases`). Keep the tool names in sync
with that plugin's skill and with the `lognorth` command.

### Alerting

Per-error alerts (new error, regression) are in `internal/events/alerting.go`.
Rate alerts (spike, drift, silence) are SPC, run by `jobs/anomaly_checker.go`
every 5 minutes on the last complete bucket of `endpoint_stats`:

- Baselines are computed from stored history, never learned or saved
  (`internal/events/spc_history.go`). Stats are kept 29 days, in UTC.
- Error rate: p-chart against the pooled rate at the same time of day over 7
  days, widened by measured dispersion. Drift: the same check on the last hour.
- Volume: median and MAD at the same time of week over 4 weeks.
- Warning (3σ) goes to the daily digest. Critical (5σ) sends one email per
  path per hour. An alert clears after 2 clean checks.

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

Dark only, and it reads like a terminal session. Same palette and font as
lognorth.com. Tokens live in `web/src/index.css`; use them through Tailwind,
never as hex values.

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
  alerts against its own history (`events.ListAlerts`), not by a fixed rate.
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
