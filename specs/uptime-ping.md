# Feature: Uptime Ping (active HTTP probe)

## Goal
Tell the user when their app is unreachable, with one piece of config: the URL.

## Behavior

### Setup
The user enters the app URL when creating the app (optional field on the create form). They can also add or change it later via `[edit]` on the apps page. Apps without a URL are skipped — no monitoring, no nagging.

We tried auto-discovery from `context.host` on the most recent event. It didn't work: none of the SDKs (Ruby, Go, JS) populate `context.host`, so the field was always empty. Dropped in v0.7.1.

### Defaults (no other knobs)
| Decision | Default |
|---|---|
| Frequency | Every 1 minute |
| Timeout | 5 seconds |
| HTTP method | GET |
| Failure threshold | 3 consecutive fails = down |
| Acceptable status | 2xx / 3xx / 4xx = up; 5xx or timeout = down |
| Bootstrap grace | 10 minutes after app creation, no alerts |

### Probe
Every minute, for each active app where `ping_disabled = false` and `ping_url != ''`:
- HTTP GET the URL with 5s timeout.
- 2xx/3xx/4xx → up. Reset `consecutive_fails`.
- 5xx or transport error → fail. `consecutive_fails++`.
- 3 consecutive fails AND was up → mark down, fire one notification.
- Recovers from down → fire one recovery notification, clear state.

### User overrides
- Edit `ping_url` to add, change, or remove the URL.
- Toggle `ping_disabled` to opt out entirely (e.g., background-only worker with no public URL).

### Notifications
Reuse existing `settings.SendNotification` channel (email / webhook / whatever the user has configured).

## Acceptance
- [x] Background sweeper runs every minute
- [x] URL captured on app creation (optional field)
- [x] HTTP probe with 5s timeout
- [x] 3 consecutive fails before transitioning to down
- [x] Down → one notification, not repeated per probe
- [x] Recovery → one notification
- [x] Bootstrap grace (10 min) prevents alerts on freshly-created apps
- [x] User can edit ping URL via UI
- [x] User can disable / re-enable the check
- [x] Status indicator on apps page (up / down / disabled / no URL)

## Notes
- Schema lives on `api_keys` (not a separate table) since it's per-app and tightly coupled to app lifecycle.
- The previous passive-sweep design was replaced because passive depends on the SDK still working — and the SDK breaks during exactly the failure modes (server down, network split) we want to detect.
- TCP probes are intentionally **not** in scope. HTTP covers virtually every web-app failure case. Add TCP if a real customer asks.
- Mode B (active heartbeat from SDK) was dropped — active HTTP probe from LogNorth is more reliable and requires no SDK change.
- Auto-discovery (v0.7.0): the sweeper queried `json_extract(context, '$.host')` from recent events. None of the SDKs ever sent that field. Removed in v0.7.1; user provides the URL on create.
