# Feature: SDK Ignored Paths

## Goal
Stop ingesting noisy traffic (health checks, uptime probes) without users having to opt in.

## Behavior

### Defaults
All SDKs ship with a default ignore list:

- `/up`           (Rails 7.1+ default health check)
- `/healthz`
- `/health`
- `/_health`
- `/ping`
- `/ready`
- `/live`

Match is exact-path or `path/...` prefix (so `/health/db` is also ignored).

### User override
Users can replace or extend the list:

```ruby
# Rails — replaces defaults
config.lognorth.ignored_paths = ["/up", "/internal/probe"]
```

```go
// Go
lognorth.New(lognorth.Config{
    IgnoredPaths: []string{"/up", "/internal/probe"},
})
```

```ts
// TS
new LogNorth({ ignoredPaths: ["/up", "/internal/probe"] })
```

When `IgnoredPaths`/`ignored_paths` is set, it **replaces** the default list — no merging. Empty array `[]` disables ignoring entirely.

### What gets skipped
- HTTP request logging (the middleware never sends the event)
- Errors raised inside ignored paths still flow through the error subscriber (errors are precious; not silenced by path filter)

## Acceptance
- [ ] Rails SDK: defaults applied when `ignored_paths` not configured
- [ ] Rails SDK: explicit `ignored_paths = [...]` replaces defaults
- [ ] Go SDK: same defaults + override behavior
- [ ] TS/Node SDK: same defaults + override behavior
- [ ] Errors raised on an ignored path still report
- [ ] README in each SDK documents the defaults and how to override

## Notes
- Rails SDK middleware already supports `ignored_paths` via `Rails.application.config.lognorth.ignored_paths` (`lib/lognorth/middleware.rb:63`). Just needs defaults populated in the Railtie.
