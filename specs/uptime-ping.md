# Feature: Uptime Ping (active HTTP probe, CoC)

## Goal
Tell the user when their app is unreachable, with zero configuration in the happy path.

## Behavior

### Convention over Configuration
No knobs. Everything has a sensible default that applies to every app:

| Decision | Convention |
|---|---|
| URL to ping | Auto-discovered from `context.host` of the most recent event |
| Frequency | Every 1 minute |
| Timeout | 5 seconds |
| HTTP method | GET |
| Failure threshold | 3 consecutive fails = down |
| Acceptable status | 2xx / 3xx / 4xx = up; 5xx or timeout = down |
| HTTPS by default | Auto-discovery prepends `https://` to the host |
| Bootstrap grace | 10 minutes after app creation, no alerts |
| Internal hosts | Skipped during auto-discovery (localhost, RFC1918, .local, .internal) |

### Auto-discovery
1. Sweeper sees an app without `ping_url`.
2. Queries the most recent event's `context.host` for that app.
3. If the host is public (not internal/loopback), saves it as `ping_url` with `https://` prefix.
4. Next sweep iteration probes it.

### Probe
Every minute, for each active app where `ping_disabled = false` and `ping_url != ''`:
- HTTP GET the URL with 5s timeout.
- 2xx/3xx/4xx → up. Reset `consecutive_fails`.
- 5xx or transport error → fail. `consecutive_fails++`.
- 3 consecutive fails AND was up → mark down, fire one notification.
- Recovers from down → fire one recovery notification, clear state.

### User overrides (rare)
- Edit `ping_url` if auto-detection got the wrong host (e.g., picked an internal hostname).
- Toggle `ping_disabled` to opt out entirely (e.g., background-only worker with no public URL).

### Notifications
Reuse existing `settings.SendNotification` channel (email / webhook / whatever the user has configured).

## Acceptance
- [x] Background sweeper runs every minute
- [x] Auto-discovers URL from `context.host` on apps without one
- [x] Skips internal hosts (localhost, RFC1918, .local, .internal)
- [x] HTTP probe with 5s timeout
- [x] 3 consecutive fails before transitioning to down
- [x] Down → one notification, not repeated per probe
- [x] Recovery → one notification
- [x] Bootstrap grace (10 min) prevents alerts on freshly-created apps
- [x] User can edit ping URL via UI
- [x] User can disable / re-enable the check
- [x] Status indicator on apps page (up / down / disabled / detecting)

## Notes
- Schema lives on `api_keys` (not a separate table) since it's per-app and tightly coupled to app lifecycle.
- The previous passive-sweep design was replaced because passive depends on the SDK still working — and the SDK breaks during exactly the failure modes (server down, network split) we want to detect.
- TCP probes are intentionally **not** in scope. HTTP covers virtually every web-app failure case. Add TCP if a real customer asks.
- Mode B (active heartbeat from SDK) was dropped — active HTTP probe from LogNorth is more reliable and requires no SDK change.
