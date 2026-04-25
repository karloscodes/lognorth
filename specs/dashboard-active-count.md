# Feature: Dashboard counts and urgency reflect active issues

## Goal
Stop dashboard urgency from being driven by stale issues that haven't fired in weeks.

## Behavior

### The bug
Today the home dashboard shows `11 issues — suppyhq-prod` in red even when 9 of those 11 haven't fired in 30 days. The number conflates "has ever happened" with "is happening now," which makes the dashboard noise rather than signal.

### The fix
The home dashboard counts and urgency derive from **active issues**, defined as issues with `last_seen` within the active window (default: 7 days, configurable per app).

### Per-app card
- **Primary count** = active issues only.
- Secondary line shows the total in muted text: `"11 active · 47 total"` (or the equivalent when total > active). Hide the total line when active and total are equal.
- Red urgency state only when active count > 0. No active issues = quiet/neutral state, regardless of historical total.
- "X NEW" badge counts only issues marked unseen AND active. Stale unseen issues don't count as new.

### Issues page
The existing `ACTIVE Last 24 hours` section already exists and is correct. Make the home dashboard agree with it. The Issues page's `All` view should still show every issue including stale ones — the home is the only place that filters.

### Active window
- Default: 7 days.
- Per-app setting overrides default: `1d`, `7d`, `30d`, or custom hours.
- Setting lives at: `Settings → Issues → Active window`.

### Auto-archive (companion behavior)
Issues with `last_seen` older than `archive_after_days` (default: 90 days) auto-archive — they stop appearing anywhere except the explicit `Archived` filter on the Issues page. This bounds dashboard noise on long-running apps.

## Acceptance
- [ ] Home dashboard count for each app = issues with `last_seen` within the active window
- [ ] Total count shown as secondary, muted text when different from active
- [ ] App card is only red when active > 0
- [ ] "X NEW" badge counts only active+unseen issues
- [ ] Active window setting available per-app, defaults to 7 days
- [ ] Issues older than `archive_after_days` (default 90) move to Archived bucket automatically
- [ ] Archived issues do not appear on home dashboard or default Issues view
- [ ] Archived count visible as a small "X archived" link to the Archived filter

## Notes
- `IssueWithState.Active` already exists in the data layer (`internal/events/issue.go`) — it's set from `last_seen > now - 24h`. This spec generalizes that to a configurable window and uses it on the home page.
- Auto-archive is a background sweep, similar to the cleanup job pattern.
- This spec depends on the issue resolve/archive spec for the storage of "archived" state.
