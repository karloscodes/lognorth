# Feature: SDK Ignored Paths

## Goal
Let users skip noisy traffic (health checks, uptime probes) from the log feed.

## Behavior

### Defaults (intentionally small)
- **Rails SDK**: defaults to `["/up"]` only. Rails 7.1 auto-generates `/up`; nothing in user code should ever pick that name.
- **Go SDK**: no default. Users call `lognorth.IgnorePaths("/healthz", ...)` if they want it.
- **TS/Express SDK**: no default. Users pass `ignorePaths` to `middleware({ ignorePaths: [...] })`.

Why not more? `/health`, `/ping`, `/ready`, `/live` are all real endpoint names in real apps. Silent drops of user traffic are the worst outcome, so the defaults stay conservative. The docs show the common paths so users can opt in knowingly.

Matching is exact path or `path/…` prefix (so `/up` also catches `/up/detail`).

### User override

```ruby
# Rails — replaces defaults
config.lognorth.ignored_paths = ["/up", "/internal/probe"]
# Disable ignoring entirely
config.lognorth.ignored_paths = []
```

```go
lognorth.IgnorePaths("/healthz", "/internal/probe")
```

```ts
app.use(middleware({ ignorePaths: ["/healthz", "/internal/probe"] }))
```

Explicit configuration fully replaces defaults — no merging.

### What gets skipped
- HTTP request logging (the middleware never sends the event).
- Errors raised inside ignored paths **still flow** through the error subscriber. Errors are precious; a path filter should not silence them.

## Acceptance
- [x] Rails SDK: `["/up"]` default, overridable
- [x] Go SDK: capability + docs
- [x] TS/Node SDK: capability + docs
- [x] Errors raised on an ignored path still report
- [x] README in each SDK explains the option and lists common paths to opt in
