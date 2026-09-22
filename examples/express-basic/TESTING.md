# LogNorth E2E Testing

This document explains the two-part testing strategy for verifying LogNorth works end-to-end.

## Part 1: Complete System Flow Test (Go)

**Location:** `lognorth/tests/e2e_test.go` → `TestCompleteSystemFlow`

**Purpose:** Verifies the complete LogNorth pipeline from ingestion to storage.

**What it tests:**

1. **Ingestion** - Events posted to `/api/v1/events` are stored in `ingested_events` table
2. **Processing** - Background job moves events from `ingested_events` → `events` table
3. **Error Detection** - Automatic `is_error` flag based on `context.error` or `status >= 500`
4. **Storage** - Processed events queryable via `events.List()`
5. **Issues Grouping** - Errors aggregate into issues by error message

**Flow:**

```
HTTP POST → ingested_events (processed=0)
              ↓
         Event Processor Job
              ↓
         events table (processed=1)
              ↓
         Issues aggregation
```

**Run it:**

```bash
cd lognorth
go test -v -run TestCompleteSystemFlow ./tests/
```

**What gets verified:**

- ✅ Success event (200 status) stored as non-error
- ✅ Error event (500 status + context.error) stored as error
- ✅ Events start unprocessed (processed=0)
- ✅ After job runs, events are processed (processed=1)
- ✅ Events appear in events table
- ✅ Issues list shows exactly 1 issue (the error)

## Part 2: SDK Compatibility Test (Node.js)

**Location:** `examples/express-basic/test.js`

**Purpose:** Verifies the TypeScript SDK can communicate with LogNorth server.

**What it tests:**

1. **Regular logs** - `LogNorth.log()` sends events successfully
2. **Error logs** - `LogNorth.error()` sends errors with stack traces
3. **Batching** - Multiple events batched correctly
4. **Context preservation** - Nested objects, arrays, metadata sent intact

**Prerequisites:**

1. LogNorth server running (`make dev`)
2. API key created
3. SDK installed (`npm install`)

**Run it:**

```bash
cd examples/express-basic
npm install
LOGNORTH_API_KEY=lgn-your-key-here npm test
```

**What gets verified:**

- ✅ SDK can authenticate with Bearer token
- ✅ Regular logs batched and flushed
- ✅ Errors sent immediately (not batched)
- ✅ Rich context (nested objects, arrays) preserved
- ✅ Events accessible via API after ~5 seconds (processing delay)

## Example App: Production-Ready Demo

**Location:** `examples/express-basic/server.js`

This is a minimal Express app showing:

- Automatic HTTP request logging via middleware
- Manual event logging
- Error handling with LogNorth integration
- Proper shutdown with log flushing

**Routes:**

- `GET /` - Homepage
- `GET /success` - Logs custom event
- `GET /error` - Logs error (500 response)
- `GET /crash` - Unhandled error caught by error handler

**Usage:**

```bash
cd examples/express-basic
npm install
LOGNORTH_API_KEY=lgn-your-key npm start
```

## Publishing as Standalone Repo

The `examples/express-basic/` directory can be published as a separate GitHub repo:

```bash
lognorth-express-example/
├── server.js          # Express app
├── test.js            # SDK compatibility test
├── package.json       # Dependencies
├── README.md          # Setup instructions
├── TESTING.md         # This file
└── .gitignore         # Git ignore rules
```

Benefits:
- Users can clone and run immediately
- Shows real-world integration patterns
- Serves as compatibility smoke test
- Template for new projects

## Design Decisions

### Why Two Tests?

1. **Go test** - Fast, no external deps, verifies internal pipeline
2. **Node test** - Verifies SDK/server contract, catches serialization issues

### Why Minimal?

- Less brittle (no process spawning, no port conflicts)
- No timing issues (manual job triggering)
- Fast (in-memory SQLite)
- Simple to understand and maintain

### Why Not Combined?

Spawning both Go server + Node app in one test creates:
- Process coordination complexity
- Port conflicts
- Timing races (event processing is async)
- Harder to debug failures

Separate tests are clearer and more reliable.

## Running Both Tests

**Full verification:**

```bash
# Terminal 1: Run Go system test
cd lognorth
go test -v -run TestCompleteSystemFlow ./tests/

# Terminal 2: Start LogNorth
make dev

# Terminal 3: Run SDK compatibility test
cd examples/express-basic
npm install
LOGNORTH_API_KEY=lgn-$(uuidgen | tr -d '-' | cut -c1-40) npm test
```

Note: Get real API key from LogNorth dashboard for Terminal 3.
