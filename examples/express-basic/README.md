# LogNorth Express Example

Minimal Express.js application demonstrating LogNorth SDK integration.

## Features

- Automatic HTTP request logging via middleware
- Manual event logging
- Error tracking with stack traces
- SDK compatibility testing

## Setup

1. **Start LogNorth server:**

```bash
cd ../../lognorth
make dev
```

2. **Create an API key:**

Visit http://localhost:8080, create an account, and generate an API key.

3. **Install dependencies:**

```bash
npm install
```

4. **Configure environment:**

```bash
export LOGNORTH_URL=http://localhost:8080
export LOGNORTH_API_KEY=lb_your_key_here
```

## Running

**Start the example server:**

```bash
npm start
```

Visit the routes:
- http://localhost:3000/ - Homepage
- http://localhost:3000/success - Logs a success event
- http://localhost:3000/error - Logs an error (500 response)
- http://localhost:3000/crash - Simulates unhandled error

## Testing SDK Compatibility

**Run the compatibility test:**

```bash
LOGNORTH_API_KEY=lb_your_key_here npm test
```

This test verifies:
1. Regular logs are sent and batched
2. Errors are sent immediately
3. Batch events work correctly
4. Context data is preserved
5. Events are accessible via API

**Expected output:**

```
🧪 Starting SDK compatibility test...

📝 Test 1: Sending regular log...
✅ Regular log sent

📝 Test 2: Sending error...
✅ Error sent

📝 Test 3: Sending batch of events...
✅ Batch sent

📝 Test 4: Sending event with rich context...
✅ Context event sent

📝 Test 5: Verifying events via API...
✅ API accessible, found 123 total events

==================================================
Test Summary:
  ✅ Passed: 5
  ❌ Failed: 0
==================================================

🎉 All tests passed! SDK is compatible with LogNorth.
```

## What Gets Logged

**Automatic (via middleware):**
- HTTP method, path, status code
- Request duration
- User agent, IP address

**Manual:**
- Custom events with `LogNorth.log()`
- Errors with `LogNorth.error()`
- Arbitrary context data

## Public Repository

This example can be published as a standalone repository for users to:
- Learn LogNorth integration
- Test SDK compatibility
- Bootstrap new projects

## Code Structure

```
server.js - Express app with LogNorth middleware
test.js   - SDK compatibility test suite
```

## Next Steps

- Add more routes demonstrating different logging patterns
- Show trace_id usage for request correlation
- Demonstrate custom context enrichment
- Add TypeScript version
