# CI/CD Setup Guide

## Overview

LogNorth has two test jobs in CI:

1. **Go Tests** - Fast, always runs (even on forks)
2. **Full Test Suite** - Includes SDK compatibility tests (only runs on main repo)

## Quick Start

Just push to `main` or `develop`:

```bash
git add .
git commit -m "Add e2e tests"
git push
```

GitHub Actions will automatically run both test jobs.

## Test Jobs

### 1. Go Tests (Required)

**Runs:** Always, on all branches and PRs

**Tests:**
- Go unit tests (`internal/*/`)
- Integration tests (`tests/`)
- Complete system flow test
- Alerting tests

**Duration:** ~2-5 seconds

**Pass criteria:** All Go tests must pass

### 2. Full Test Suite (Optional)

**Runs:** Only on `karloscodes/lognorth` repo (not forks)

**Tests:**
- All Go tests (same as above)
- TypeScript SDK compatibility
- Go SDK compatibility

**Duration:** ~30-60 seconds

**Requires:** Access to SDK repos (private or public)

**Pass criteria:** All tests including SDK compatibility must pass

## Repository Structure in CI

The workflow checks out repos as siblings:

```
/home/runner/work/lognorth/lognorth/
├── lognorth/           # Main repo
├── lognorth-sdk-ts/    # TypeScript SDK (optional)
└── lognorth-sdk-go/    # Go SDK (optional)
```

This matches the local development structure.

## SDK Repository Access

### Public SDKs

If SDK repos are public, no setup needed. GitHub Actions will check them out automatically.

### Private SDKs

For private SDK repos, create a Personal Access Token (PAT):

1. Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Generate new token with `repo` scope
3. Add as repository secret: `Settings → Secrets → Actions → New repository secret`
4. Name it `GH_PAT`
5. Paste the token value

The workflow uses `secrets.GH_PAT || secrets.GITHUB_TOKEN` to try PAT first, fall back to default token.

## SDK Tests Are Optional

If SDK repos can't be checked out (private repos without PAT, or repos don't exist):

- ✅ Go tests still run and pass
- ⚠️  SDK tests are skipped with warnings:
  ```
  ⚠️  TypeScript SDK not found, skipping
  ⚠️  Go SDK not found, skipping
  ```
- ✅ CI passes overall (SDK tests are non-blocking)

## Testing Locally

Match the CI environment:

```bash
# Same structure as CI
/Users/you/Code/lognorth/
├── lognorth/
├── lognorth-sdk-ts/
└── lognorth-sdk-go/

# Run tests
cd lognorth
make test-all
```

## CI Workflow Files

- `.github/workflows/ci.yml` - Test workflow

## Debugging CI Failures

**Go tests fail:**
1. Run locally: `make test`
2. Check error in CI logs
3. Fix and push

**SDK tests fail:**
1. Check if SDKs were checked out (look for "Checkout" steps in logs)
2. If missing, verify PAT is configured (Settings → Secrets → `GH_PAT`)
3. Run locally: `make test-sdk`
4. Check script detects SDKs: `./scripts/test-sdk-compatibility.sh`

**Server fails to start in CI:**
1. Check `/tmp/lognorth-test-server.log` in CI output
2. Verify port 9876 is available
3. Check build succeeded

## Customization

### Change test branches

Edit `.github/workflows/ci.yml`:

```yaml
on:
  push:
    branches: [ main, develop, staging ]  # Add branches here
```

### Disable SDK tests

Remove or comment out the `full-tests` job in `.github/workflows/ci.yml`.

### Run only SDK tests

Change `make test-all` to `make test-sdk` in workflow.

## Status Badges

Add to README.md:

```markdown
![Tests](https://github.com/karloscodes/lognorth/actions/workflows/ci.yml/badge.svg)
```

## Next Steps

1. ✅ Push code to GitHub
2. ✅ Verify Go tests pass in Actions tab
3. ✅ (Optional) Configure `GH_PAT` secret for private SDK repos
4. ✅ Verify full test suite runs and passes
5. ✅ Add status badge to README
