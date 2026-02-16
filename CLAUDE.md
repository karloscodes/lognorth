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

**Terminal-inspired. Sharp edges. Black, white, red. Nothing soft.**

### Design Philosophy

This is a developer tool, not a consumer app. Hard edges = confidence. No rounded corners, no soft shadows, no friendly curves. Think terminal meets Swiss poster design.

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

| Don't | Do |
|-------|-----|
| `rounded-lg` | `rounded-sm` or nothing |
| `rounded-xl` | `rounded-sm` or nothing |
| `rounded-full` (on containers) | `rounded-sm` |
| `rounded-md` | `rounded-sm` or nothing |

Exception: `rounded-full` is OK for small status dots only.

### Navigation

No pill toggles. Use underline-based navigation:

```tsx
// Active nav link
className="font-semibold text-black border-b-2 border-black"

// Inactive nav link
className="font-medium text-black/60 hover:text-black"
```

Keep nav simple: one pattern, not three. No background pills, no toggle groups for primary nav.

### Typography

| Element | Class |
|---------|-------|
| Page titles | `text-2xl font-bold text-black` |
| Section headers | `text-lg font-bold text-black uppercase tracking-wide` |
| Important numbers | `text-xl font-bold` or larger - numbers should POP |
| Body text | `text-black font-medium` |
| Secondary text | `text-black/60` |
| Code/paths | `font-mono text-sm` |

**Rules:**
- Numbers that matter (error counts, occurrence counts) should be BIG and BOLD
- Use `font-mono` liberally - paths, methods, trace IDs, durations
- Uppercase + tracking for section labels (e.g., "ERROR", "ENDPOINT")

### Colors

- **Black** - Primary text, borders, buttons
- **White** - Backgrounds
- **Red** (`text-red-600`, `bg-red-600`) - Errors only. Make them impossible to miss.
- **Green dot** (`bg-green-500`) - Success/active status dots only

No other colors. No blue links. No yellow warnings. Binary: error or not.

### Buttons

```tsx
// Primary - sharp corners
className="bg-black text-white font-semibold px-4 py-2 hover:bg-black/80"

// Secondary - sharp corners
className="border border-black/20 font-semibold px-4 py-2 hover:bg-black/[0.02]"
```

### Forms

```tsx
// Input - sharp corners
className="border border-black/20 focus:ring-1 focus:ring-black focus:border-black"

// Label
className="text-sm font-semibold text-black uppercase tracking-wide"
```

### Cards & Containers

```tsx
// Card - sharp corners, visible border
className="border border-black/10 bg-white"

// NO shadows, NO rounded corners, NO subtle backgrounds
```

### Error States

Errors should be impossible to miss:

```tsx
// Error row/card
className="border-l-4 border-l-red-600 bg-red-50"

// Error count badge
className="bg-red-600 text-white font-bold px-2 py-1 text-lg"
```

## Strong Requirements

- **Phoenix Contexts:** Business logic in context packages
- **Background jobs:** Events processed asynchronously
- **No gray colors:** Only black, white, accent - use black/* opacity for neutrals
- **Test integrity:** Never skip tests
