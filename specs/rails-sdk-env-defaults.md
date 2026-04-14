# Feature: Rails SDK — Sane Env Defaults

## Goal
Stop spamming LogNorth with test/development noise, and stop printing initializer chatter to stdout in every Rails console / test run.

## Behavior

### Default enabled state
- Production: `enabled = true`
- All other envs (development, test, staging, custom): `enabled = false`

Equivalent to `config.lognorth.enabled = Rails.env.production?`.

Users can override explicitly:
```ruby
config.lognorth.enabled = !Rails.env.test?  # turn it on for dev too
```

### Initializer logging
Today the Railtie prints to `$stdout` unconditionally:

```
[LogNorth] after_initialize running
[LogNorth] not configured: url=... api_key=...
```

New behavior:
- Use `Rails.logger.debug` (not `$stdout.puts`) for routine init messages
- "not configured" warning prints to `Rails.logger.warn` only when `enabled = true` and credentials are missing (i.e. user expected it to work)
- When `enabled = false`, the Railtie does nothing visible — no logs, no middleware insertion

### What "disabled" means
- Middleware not inserted (no request events)
- Error subscriber not registered (no exceptions sent)
- ActiveJob subscriber not attached (no job events)
- Direct API calls (`LogNorth.log`, `LogNorth.report_error`) become no-ops
- `LogNorth.config(url, key)` still works for explicit setup, but client.send becomes a no-op when disabled

## Acceptance
- [ ] Default `enabled` is `Rails.env.production?`
- [ ] No `$stdout.puts` from the Railtie in any env
- [ ] Test suite of host app produces zero LogNorth output by default
- [ ] When disabled, `LogNorth.log("x")` returns silently (no error, no HTTP call)
- [ ] Explicit `enabled = true` in test env still works for SDK testing
- [ ] CHANGELOG note for upgraders: previously-on-in-dev users need `enabled = true`

## Notes
- Touches `lib/lognorth/railtie.rb` (defaults + log calls) and `lib/lognorth/client.rb` (no-op gating).
- Breaking change for anyone relying on dev-mode events. Worth a major version bump.
