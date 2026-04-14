# Feature: Uptime Ping (Heartbeat)

## Goal
Detect when an app stops sending events (crash, deploy gone wrong, network split) and notify before users do. Today LogNorth only knows about errors that *arrive* — silence is invisible.

## Behavior

### Two modes — pick the simpler that works

**Mode A — Passive (no SDK change)**
- "App is silent" = no event from this `api_key_id` for N minutes (default 5).
- Background sweeper runs every minute. For each app with notification enabled:
  - Compute `last_event_at` from events table
  - If `now - last_event_at > threshold` and not already notified → send "Silent" alert
  - When events resume, send "Recovered" alert
- Per-app threshold setting (default 5 min, off by default for new apps to avoid noise on quiet apps).

**Mode B — Active heartbeat (opt-in)**
- SDK sends `POST /api/v1/heartbeat` every 60s with no payload
- Server tracks `last_heartbeat_at` per api_key
- Same down/up notification logic, but threshold can be tighter (e.g. 2 min) since heartbeats are predictable

### Recommendation
Start with Mode A. Most apps with the SDK installed will be sending events constantly; passive detection costs zero SDK work. Add Mode B later for low-traffic apps where errors-only detection is too lossy.

### UI
- Apps page shows a green/red dot per app indicating "live" / "silent"
- Settings: per-app toggle "Alert me when this app goes silent for [N] minutes"
- Notification channels reuse existing email / webhook config

### Anti-noise
- Don't alert during the first 10 minutes after enabling (avoid bootstrap false-positives)
- One alert per silent-period; recovery alert clears it
- Suppress for muted apps

## Acceptance
- [ ] Background sweeper runs every minute
- [ ] Silent → notification sent once, not repeatedly
- [ ] Recovery → notification sent once
- [ ] Per-app threshold setting in UI, default 5 min
- [ ] Off by default for new apps
- [ ] Apps page shows live/silent indicator
- [ ] Quiet test app (no events) doesn't trigger if threshold disabled

## Notes
- Adds modest schema: `api_keys.silence_threshold_minutes`, `api_keys.silence_alert_at` (nullable).
- The sweeper can piggyback on the existing background goroutine if there is one; otherwise a tiny ticker.
- Out of scope: external HTTP uptime probing of the user's own services. That's a different product.
