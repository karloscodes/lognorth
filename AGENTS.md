# Developer Agent Guidelines

This is the **canonical source** for all AI coding assistants (Claude, Copilot, Codex) working on LogNorth.

---

## Core Philosophy

- **Simple over clever:** readable, debuggable, maintainable code
- **Pragmatic, not perfect:** clarity over architectural purity
- **Sustainability:** one person should be able to maintain this for years
- **Convention over configuration:** minimize setup, maximize productivity
- **No fluff:** avoid unnecessary dependencies, abstractions, or buzzwords

> **When in doubt, choose the simplest path that works — and that you can maintain alone.**

---

## Architecture: Phoenix Contexts Pattern

LogNorth follows **Phoenix Contexts** architecture for clean separation of concerns:

```
internal/
├── http/                 # HTTP Transport Layer
│   ├── home.go          # Thin handlers (HTTP concerns only)
│   ├── setup.go
│   ├── requests.go
│   ├── issues.go
│   ├── ingest.go
│   ├── live.go          # SSE live events
│   └── render.go        # Inertia rendering helper
│
├── events/              # Context: Events domain (ingestion, listing, issues)
├── apikeys/             # Context: API keys domain
├── config/              # App configuration
└── app.go               # Application setup and routes
```

### Context Rules

1. **Domain-focused:** Each context owns its business logic
2. **Top-level functions:** Export functions, not service objects
3. **Accept `*gorm.DB`:** All functions take DB as parameter
4. **No HTTP dependencies:** Contexts never import `internal/http`
5. **Clean boundaries:** Contexts can call other contexts, but minimize coupling

### Handler Pattern

```go
// ✅ GOOD: Thin handler delegating to context
func HomePageAction(ctx *cartridge.Context) error {
    db := ctx.DB()

    recentEvents, total, _ := events.List(db, events.ListFilters{Limit: 10})
    issues, _ := events.ListIssues(db, 0, 5) // 0 = all apps
    stats := events.GetStats(db, 0)

    return RenderPage(ctx.Ctx, "Home", Props{
        "recentEvents": recentEvents,
        "stats":        stats,
    })
}

// ❌ BAD: Business logic in handler
func HomePageAction(ctx *cartridge.Context) error {
    db := ctx.DB()
    var errorCount int64
    db.Model(&events.Event{}).Where("level = ?", "error").Count(&errorCount)
    // ... complex calculations in handler ...
}
```

---

## Development Workflow

### Commands

```bash
make dev          # Run Go API + React dev server with hot reload
make test         # Run Go tests
make seed         # Generate sample data
make build        # Produce production binary + bundled frontend
```

### Low-Risk Changes

```bash
# 1. Make changes
vim internal/events/event.go

# 2. Run tests
make test

# 3. Check the UI
# Browser: http://localhost:8080
```

---

## Project Snapshot

- **Stack**: Go 1.23+ (Fiber, GORM, SQLite, Cartridge) + React 19/TypeScript/Tailwind
- **Architecture**: Phoenix Contexts pattern
- **Domain**: Self-hosted request debugger
- **Database**: SQLite with WAL mode
- **Binary**: `cmd/lognorth` (server)

### Repo Landmarks

- **`internal/http/`**: HTTP handlers
- **`internal/app.go`**: Application setup and route registration
- **`internal/events/`**: Events context (ingestion, listing, issues, stats)
- **`internal/apikeys/`**: API keys context
- **`web/`**: React SPA (shadcn/ui, Tailwind, Inertia.js)
- **`cmd/lognorth/`**: Main binary
- **`cmd/seed/`**: Sample data generator

---

## UI Architecture

**Inertia.js for all pages**

All pages use Inertia.js patterns for server-side rendering with React.

### Architecture Overview

1. **ALL PAGES**: Inertia.js + React + Tailwind (shadcn/ui)
   - Every page is a React component receiving props via Inertia
   - Server uses `RenderPage()` to pass data
   - React components receive props directly

2. **DATA FLOW**:
   - Server prepares ALL data needed for render
   - Pass data as Inertia props
   - NO client-side data fetching on page load (except SSE for live)

3. **EXCEPTION: API endpoints** for:
   - Event ingestion (`POST /api/v1/events`)
   - SSE live stream (`GET /api/v1/events/live`)
   - API key creation (JSON response with full key)

### Server Handler Pattern

```go
// Page render
func RequestsPageAction(ctx *cartridge.Context) error {
    db := ctx.DB()
    events, total, _ := events.List(db, filters)

    return RenderPage(ctx.Ctx, "Requests", Props{
        "events":     events,
        "pagination": pagination,
    })
}
```

---

## Styling Rules

**Color palette: Black, White, and ONE accent color. No gray.**

NEVER use `gray-*` Tailwind classes. Use `black/*` opacity for all neutral tones.

| Don't use | Use instead |
|-----------|-------------|
| `text-gray-900` | `text-black` |
| `text-gray-600` | `text-black/60` |
| `text-gray-500` | `text-black/50` |
| `text-gray-400` | `text-black/40` |
| `bg-gray-50` | `bg-black/[0.02]` |
| `bg-gray-100` | `bg-black/5` |
| `border-gray-200` | `border-black/10` |
| `hover:bg-gray-50` | `hover:bg-black/[0.02]` |

Semantic colors (red for errors, amber for warnings) are allowed for meaning.

---

## Strong Requirements

- **Phoenix Contexts:** Business logic in context packages, not handlers
- **No gray colors:** Only black, white, accent - use `black/*` opacity for neutrals
- **Test integrity:** Never skip tests or mark as pending
- **No arbitrary docs:** Don't create .md files unless explicitly requested

---

## Testing

- **Always** run `make test` after changes
- **A failure is a failure:** Never cheat on tests - fix the actual issue

---

Keep AGENTS.md synchronized when workflows change.
