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

**Color palette: Black, White, and ONE accent color. No gray.**

### No Gray, Ever

NEVER use `gray-*` Tailwind classes. Use `black/*` opacity for all neutral tones.

| Don't | Do |
|-------|-----|
| `text-gray-600` | `text-black/60` |
| `bg-gray-50` | `bg-black/[0.02]` |
| `border-gray-200` | `border-black/10` |
| `text-gray-400` | `text-black/40` |

### Bold, Confident Typography (Stripe-style)

Text must be readable and confident. Weak text looks uncertain.

| Element | Class | Example |
|---------|-------|---------|
| Page titles | `text-2xl font-bold text-black` | "Email (SMTP)" |
| Section headers | `text-xl font-bold text-black` | "Active Issues" |
| Card titles | `text-lg font-semibold text-black` | App names |
| Labels | `text-sm font-semibold text-black` | Form labels |
| Body text | `text-black` or `font-medium text-black` | Primary content |
| Secondary text | `text-black/80` | Helper text, timestamps |
| Placeholder | `placeholder:text-black/40` | Input hints |

**Rules:**
- Never use `text-black/70` or lighter for readable content - minimum is `text-black/80`
- Use `font-semibold` or `font-bold` liberally - confident beats subtle
- Buttons always `font-semibold`
- All nav links and interactive text should be `font-semibold`
- Cards use `border-2 border-black/10` not subtle backgrounds

### Accent Color

- `#00D678` (green) - Success states, active indicators (dots only, not text)
- Red (`text-red-600`, `bg-red-500`) - Errors, destructive actions
- Never use accent colors for body text - only for icons, dots, badges

### Buttons

```tsx
// Primary
className="bg-black text-white font-medium hover:bg-black/80"

// Secondary
className="border border-black/20 font-medium hover:bg-black/[0.02]"
```

### Forms

```tsx
// Input
className="border border-black/20 focus:ring-1 focus:ring-black"

// Label
className="text-sm font-semibold text-black"
```

## Strong Requirements

- **Phoenix Contexts:** Business logic in context packages
- **Background jobs:** Events processed asynchronously
- **No gray colors:** Only black, white, accent - use black/* opacity for neutrals
- **Test integrity:** Never skip tests
