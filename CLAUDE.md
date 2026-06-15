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

### No Gray, Ever

NEVER use `gray-*` Tailwind classes. Use `black/*` opacity for all neutral tones.

| Don't | Do |
|-------|-----|
| `text-gray-600` | `text-black/60` |
| `bg-gray-50` | `bg-black/[0.02]` |
| `border-gray-200` | `border-black/10` |
| `text-gray-400` | `text-black/40` |

### No Rounded Corners

NEVER use `rounded-*` classes larger than `rounded-sm`. Hard edges only.

Exception: `rounded-full` is OK for small status dots only.

### Typography

| Element | Class |
|---------|-------|
| Logo | `text-2xl font-black tracking-tight` |
| Hero numbers | `text-6xl font-black` - the answer to "do I have a problem?" |
| Page titles | `text-2xl font-bold text-black` |
| Section headers | `text-sm font-bold text-black/50 uppercase tracking-wide` |
| Important counts | `text-xl font-black font-mono tabular-nums` |
| Body text | `text-black` or `font-medium text-black` |
| Secondary text | `text-black/50` or `text-black/60` |
| Code/paths | `font-mono text-sm` |

**Rules:**
- Numbers that matter use `font-mono tabular-nums` for alignment
- Hero status (Home page) is massive - 6xl font-black
- Count badges (38×, 124×) are font-black, not subtle
- Use `font-semibold` liberally on nav, buttons, labels

### Colors

- **Black** - Primary text, borders, buttons
- **White** - Backgrounds
- **Red** (`text-red-600`, `border-red-500`) - Errors. Make them impossible to miss.
- **Green** (`#00D678`) - Success states, active indicators (dots, borders)

No other colors. No blue links. No yellow warnings. Binary: error or not.

### Navigation

Underline-based for primary nav:

```tsx
// Active
className="font-semibold text-black border-b-2 border-black"

// Inactive
className="font-semibold text-black/50 hover:text-black"
```

Toggle groups are OK for filters (All/Success/Errors, Today/7d/30d/All).

### Buttons

```tsx
// Primary
className="bg-black text-white font-semibold px-4 py-2 hover:bg-black/80"

// Secondary
className="border border-black/20 font-semibold px-4 py-2 hover:border-black/40"
```

### Cards & Containers

```tsx
// Default card
className="border border-black/10 bg-white hover:border-black/30 transition-all"

// Card with errors
className="border-2 border-red-500/30 bg-red-50 hover:border-red-500"

// Active/healthy card
className="border-2 border-[#00D678]/40 border-l-4 border-l-[#00D678]"
```

Cards should have visible hover states - borders get bolder.

### Error States

Errors demand attention:

```tsx
// Error row - red left border
className="border-l-2 border-l-red-500"

// Error count - bold and black (or red if unseen)
className="font-mono text-xl font-black tabular-nums text-black"

// Unseen error count
className="font-mono text-xl font-black tabular-nums text-red-600"
```

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
- **No gray colors:** Only black, white, accent - use black/* opacity for neutrals
- **Test integrity:** Never skip tests
