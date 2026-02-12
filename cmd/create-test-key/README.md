# create-test-key - Test-Only API Key Generator

## ⚠️ SECURITY NOTICE

This utility is **ONLY for test environments**. It has multiple safeguards to prevent production misuse:

1. **Production check** - Refuses to run if `config.IsProduction()` returns true
2. **Environment check** - Requires `LOGNORTH_ENV=test` or `LOGNORTH_ENV=development`
3. **Clear naming** - Creates keys named "SDK Test" to identify them

## Usage

**Automated tests only:**
```bash
LOGNORTH_ENV=test LOGNORTH_DB_PATH=storage/test.db \
  go run cmd/create-test-key/main.go
```

**Output:** Prints API key to stdout (e.g., `lb_abc123...`)

## Protection Examples

**❌ Production (fails):**
```bash
$ LOGNORTH_ENV=production LOGNORTH_PRIVATE_KEY=... \
    go run cmd/create-test-key/main.go
ERROR: create-test-key cannot be run in production environment
exit status 1
```

**❌ Staging (fails):**
```bash
$ LOGNORTH_ENV=staging go run cmd/create-test-key/main.go
ERROR: create-test-key requires LOGNORTH_ENV=test or LOGNORTH_ENV=development (got: staging)
exit status 1
```

**✅ Test (works):**
```bash
$ LOGNORTH_ENV=test LOGNORTH_DB_PATH=storage/test.db \
    go run cmd/create-test-key/main.go
lb_2c7cee4718b71bff58704a8b2d8b787fa9a4b1aa...
```

## When Is This Used?

Automatically called by `scripts/test-sdk-compatibility.sh` to:
1. Create a temporary API key
2. Run SDK compatibility tests
3. Clean up test database after tests complete

## Manual Production Key Creation

For production, use the web dashboard:
1. Start LogNorth: `make dev`
2. Visit http://localhost:8080
3. Create account and generate API key
4. Copy key from dashboard

Or use the CLI (recommended for automation):
```bash
# TODO: Add production-safe CLI command
```

## Why This Exists

The test automation script needs to:
- Start a LogNorth server
- Create an API key to test with
- Run SDK tests against that server
- Clean up

Without this utility, we'd need to:
- Manually create keys before tests
- Hardcode test keys (security risk)
- Skip SDK compatibility tests in CI

## Security Checklist

- ✅ Refuses to run in production environment
- ✅ Requires explicit test/development environment variable
- ✅ Creates keys with "SDK Test" name (easy to identify)
- ✅ Only used in automated test scripts
- ✅ Documented as test-only utility
- ✅ No production equivalent (use dashboard instead)

## Alternative Approaches Considered

**1. Hardcoded test key**
- ❌ Security risk if committed
- ❌ Can't rotate without updating tests

**2. Mock API key validation**
- ❌ Doesn't test real authentication
- ❌ SDK tests wouldn't catch auth bugs

**3. Programmatic API via internal package**
- ✅ Could work, but requires more code
- ✅ This approach is simpler

**4. This approach (chosen)**
- ✅ Simple and secure
- ✅ Production-safe with multiple checks
- ✅ Easy to audit (single file)
- ✅ Fails loudly if misused
