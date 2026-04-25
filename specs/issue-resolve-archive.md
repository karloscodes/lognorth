# Feature: Resolve and Archive issues

## Goal
Give users a real triage flow. Today the only state changes are Mute (suppress alerts) and Mark Seen (clear unseen). There's no way to say "this is fixed" or "I don't want to look at this anymore."

## Behavior

### Three actions on an issue

| Action | Effect on view | Effect on alerts | Auto-reopen |
|---|---|---|---|
| **Mute** | Stays in list | No alerts | n/a — mute persists until unmuted |
| **Resolve** | Hidden from default | Alerts on next occurrence | Yes — next occurrence reopens it |
| **Archive** | Hidden from default | Alerts on next occurrence | No — stays archived until unarchived |

Mute stays as-is. Resolve and Archive are new.

### Resolve
The user fixed the bug. They click **Resolve** on the issue.
- Issue moves to the `Resolved` filter on the Issues page.
- Next time the same error occurs, the issue **auto-reopens** (state flips back to active) and a new-error notification fires once.
- Auto-reopen also triggers a regression alert (separate beat from a brand-new error).

### Archive
The user knows about it but doesn't care to fix it (third-party flake, deprecated endpoint, accepted behavior). They click **Archive**.
- Issue moves to the `Archived` filter.
- New occurrences keep being recorded but **don't move it back** to active.
- No alerts fire while archived.
- User must explicitly **Unarchive** to bring it back.

### Filter tabs on Issues page
Replace the current implicit "active" view with explicit tabs:
- `Active` — default. Issues that fired in the active window and aren't muted/resolved/archived.
- `Resolved` — manually resolved, awaiting potential auto-reopen.
- `Muted` — alerts off, but visible.
- `Archived` — manual close, no alerts, no auto-reopen.
- `All` — everything.

Counts shown next to each tab.

### Schema additions
Extend `IssueSeen` in `internal/events/issue.go` with three new columns:
- `resolved_at` (timestamp, nullable) — when user resolved
- `archived_at` (timestamp, nullable) — when user archived
- `reopened_at` (timestamp, nullable) — last time auto-reopen fired (for regression-alert dedup)

Auto-reopen logic: on event ingest, if `error_hash` matches an issue with `resolved_at IS NOT NULL` and the new event timestamp > `resolved_at`, clear `resolved_at`, set `reopened_at`, and fire a regression alert (deduped against `last_notified_at`).

## Acceptance
- [ ] Issue detail page has Resolve, Archive, and Mute buttons
- [ ] Resolved issues hidden from default Issues view
- [ ] Resolved issues auto-reopen on next occurrence with a regression alert
- [ ] Archived issues hidden from default and do not auto-reopen
- [ ] Archived issues still record occurrences (data retained) but fire no alerts
- [ ] Issues page has filter tabs: Active / Resolved / Muted / Archived / All, each with count
- [ ] Unresolve and Unarchive actions available on detail page
- [ ] `IssueSeen` schema migrated with new columns and indexes
- [ ] Existing Mute behavior unchanged

## Notes
- "Resolved" is the right verb (matches Sentry, Help Scout, GitHub Issues — universal language).
- Auto-reopen on resolved must be deduped: one regression alert per reopen, not one per occurrence.
- Archive has no auto-reopen on purpose. The user said "I don't care" — respect that until they unarchive.
- This spec is a prerequisite for `dashboard-active-count.md`'s archived-state filtering.
