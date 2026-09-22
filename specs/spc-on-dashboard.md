# Feature: SPC severity drives dashboard urgency

## Goal
The home dashboard urgency should reflect what's actually wrong right now, not just "you have errors." SPC already detects spikes, drift, and silence. The home page doesn't surface any of it.

## Behavior

### Today's gap
- `internal/events/spc_detection.go` implements `SPCIsSpike`, `SPCIsDrift`, and `IsSilence`.
- These detect: error rate spike (last 5 min), drift (last hour), and volume drop (silence).
- The live state is in `SpikeState` and `SilenceState`, and `events.ListAlerts` already reads it (the MCP `list_alerts` tool uses it) — but the home dashboard doesn't.
- Result: a steady 5-error-a-day app shows the same red urgency as an app currently spiking.

### The fix
The home dashboard's per-app card uses **maximum current SPC severity** to drive its visual state, alongside active issue count.

### Severity → visual mapping

| SPC state | Card color | Label |
|---|---|---|
| `critical` (active spike, silence, or unrecovered drift) | Red | "Spike: /api/orders 30× normal" or similar specific reason |
| `warning` (lower-severity SPC signal) | Amber | "Drifting: /api/checkout" |
| Active issues but no SPC signal | Neutral with subtle indicator | "{N} active" |
| No active issues, no SPC signal | Quiet/calm | "Quiet" or empty state |

The card includes a one-line reason naming the specific endpoint and signal type when SPC fires. "11 active issues" is no longer the headline — "spike: /api/orders 30× baseline" is.

### Specific reason line
Pull from the highest-severity active SPC alert for that app:
- Spike: `"/api/orders error rate 8× baseline"`
- Drift: `"/api/checkout error rate trending up over 4h"`
- Silence: `"/api/webhooks no traffic for 35 min (typical: 1.2k/h)"`

If multiple signals fire, show the worst one and a `+N more` suffix.

### Recovery
When SPC marks recovered, the card visibly de-escalates. Show a transient "recovered" pill for ~1 hour after recovery so the user sees that it cleared.

### Issues page connection
On the Issues page, each issue row that's part of an SPC spike gets a small severity chip (red/amber). Hovering or clicking reveals the SPC context (z-score, baseline, current rate). This is secondary to the home card change but flows from the same data.

## Acceptance
- [ ] Home dashboard reads current SPC state per app
- [ ] Card color reflects worst active SPC severity, not just issue count
- [ ] Card shows specific reason line (endpoint + signal type) when SPC fires
- [ ] Recovery state visible for ~1h after SPC marks recovered
- [ ] Apps with active issues but no SPC signal use neutral state, not red
- [ ] Apps with no active issues and no SPC signal show "Quiet" state
- [ ] Issue rows linked to active SPC spikes show a severity chip on the Issues page

## Notes
- The SPC severity is on `SpikeState.Severity` and `SilenceState.Severity` (warning / critical), and `events.ListAlerts` returns it with a one-line summary. This spec is about displaying it, not detecting it.
- This spec depends on `dashboard-active-count.md` for the underlying "what counts as active" definition. Build that first; SPC severity layers on top.
- Cardinality consideration: a noisy app could have many concurrent SPC alerts. Show only the worst on the home card; full list on the Issues page.
