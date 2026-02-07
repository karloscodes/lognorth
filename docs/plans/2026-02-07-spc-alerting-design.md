# Statistical Process Control Alerting

**Date:** 2026-02-07
**Status:** Draft
**Goal:** Notify but not overwhelm. Amplify the signal.

## Problem

Current alerting uses fixed thresholds (3x baseline, +15%). This causes:
- Too many alerts during natural traffic variation
- Arbitrary thresholds that don't adapt to each app
- No distinction between urgent and informational

Target users (solos/SMBs) want to sleep through the night but wake up if something's on fire.

## Solution

Statistical Process Control (SPC) to distinguish normal variation from real problems.

### Key Decisions

| Decision | Choice |
|----------|--------|
| Configuration | Fully automatic - system learns what's normal |
| Detection | Hybrid: Control Charts (spikes) + CUSUM (drift) |
| Severity | Combined: 3σ AND (persists >10min OR >20 errors) = critical |
| Delivery | Email only. Critical = immediate, Warning = daily digest |
| Digest time | 9am local time |

## Data Model

### baseline_stats

Learned normal behavior per app/endpoint/hour-of-week (168 buckets).

```sql
CREATE TABLE baseline_stats (
    id INTEGER PRIMARY KEY,
    api_key_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    hour_of_week INTEGER NOT NULL,  -- 0-167
    mean REAL DEFAULT 0,
    std_dev REAL DEFAULT 0,
    sample_count INTEGER DEFAULT 0,
    updated_at DATETIME,
    UNIQUE(api_key_id, path, hour_of_week)
);
```

**Learning algorithm:**
- Exponential moving average: `new_mean = 0.9 * old_mean + 0.1 * current`
- StdDev via Welford's online algorithm
- Cold start: use conservative defaults until 20+ samples

### cusum_states

CUSUM state for drift detection.

```sql
CREATE TABLE cusum_states (
    id INTEGER PRIMARY KEY,
    api_key_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    cumulative REAL DEFAULT 0,
    started_at DATETIME,
    UNIQUE(api_key_id, path)
);
```

### alert_states

Current alert state per endpoint.

```sql
CREATE TABLE alert_states (
    id INTEGER PRIMARY KEY,
    api_key_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    severity TEXT,  -- 'warning', 'critical', NULL
    detected_at DATETIME,
    notified_at DATETIME,
    error_count INTEGER DEFAULT 0,
    UNIQUE(api_key_id, path)
);
```

### digest_queue

Queued warnings for daily digest.

```sql
CREATE TABLE digest_queue (
    id INTEGER PRIMARY KEY,
    api_key_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    message TEXT,
    severity TEXT,
    resolved BOOLEAN DEFAULT FALSE,
    created_at DATETIME
);
```

### Settings additions

```go
DigestHour     int    // Hour to send digest (default: 9)
DigestTimezone string // User's timezone (default: "UTC")
```

## Detection Algorithms

### Control Charts (Spike Detection)

```go
func IsSpike(current, mean, stddev float64) (bool, string) {
    if stddev == 0 {
        stddev = 1.0
    }

    zScore := (current - mean) / stddev

    if zScore > 3.0 {
        return true, "critical"  // 99.7% confidence
    }
    if zScore > 2.0 {
        return true, "warning"   // 95% confidence
    }
    return false, ""
}
```

### CUSUM (Drift Detection)

```go
func UpdateCUSUM(state *CUSUMState, current, mean, slack float64) (bool, string) {
    deviation := current - mean - slack
    state.Cumulative = max(0, state.Cumulative + deviation)

    if state.Cumulative > 5.0 {
        return true, "critical"
    }
    if state.Cumulative > 2.5 {
        return true, "warning"
    }
    return false, ""
}
```

CUSUM resets when error rate drops below mean.

## Alert State Machine

```
Normal → Warning   : 2σ breach (CUSUM or Control Chart)
Warning → Critical : 3σ AND (>10min OR >20 errors)
Critical → Normal  : Error rate < mean + 1σ for 10 min
Warning → Normal   : Same recovery condition
```

**Email behavior:**

| Transition | Action |
|------------|--------|
| → Critical | Send immediately with `[CRITICAL]` subject |
| → Warning | Queue for daily digest |
| Critical → Normal | Send recovery email |
| Warning → Normal | Mention in next digest as resolved |

## Background Jobs

| Job | Frequency | Purpose |
|-----|-----------|---------|
| `BaselineUpdater` | 5 min | Update baseline stats from recent data |
| `AnomalyChecker` | 5 min | Run Control Chart + CUSUM, manage alert states |
| `DigestSender` | Daily 9am | Send warning digest if any queued |

### AnomalyChecker Flow

```go
func (j *AnomalyChecker) Run(db *gorm.DB) {
    apps := GetAllAPIKeys(db)

    for _, app := range apps {
        endpoints := GetActiveEndpoints(db, app.ID)

        for _, path := range endpoints {
            current := GetCurrentErrorRate(db, app.ID, path)
            baseline := GetBaseline(db, app.ID, path, currentHourOfWeek())

            // Control Chart check
            spikeDetected, spikeSeverity := IsSpike(current, baseline.Mean, baseline.StdDev)

            // CUSUM check
            cusum := GetCUSUMState(db, app.ID, path)
            driftDetected, driftSeverity := UpdateCUSUM(cusum, current, baseline.Mean, 0.5)

            // Take worst severity
            severity := maxSeverity(spikeSeverity, driftSeverity)

            UpdateAlertState(db, app.ID, path, severity)
        }
    }

    SendCriticalAlerts(db)
}
```

## Email Templates

### Critical Alert

```
Subject: [CRITICAL] AppName: Error rate spike detected

AppName is experiencing abnormal error rates.

What's happening:
- Endpoint: POST /api/checkout
- Current error rate: 12.3% (normal: 2.1%)
- Started: 10 minutes ago
- Errors so far: 47

This is 4.8 standard deviations above normal for this time of day.

View details: https://lognorth.example.com/apps/123/issues
```

### Recovery

```
Subject: [RECOVERED] AppName: Error rate back to normal

POST /api/checkout has recovered.

- Duration: 23 minutes
- Peak error rate: 12.3%
- Total errors during incident: 142
- Current error rate: 1.8%
```

### Daily Digest

```
Subject: LogNorth Daily Digest - Feb 7

Yesterday's summary across your apps:

WARNINGS (resolved automatically)
- AppName: POST /api/users had elevated errors 2:00-2:15am (resolved)

STATS
- AppName: 12,847 events, 23 errors (0.18%)
- OtherApp: 3,201 events, 2 errors (0.06%)

No action needed - just keeping you informed.
```

## Constants

```go
// internal/events/spc_config.go

const (
    // Baseline learning
    BaselineSmoothingFactor = 0.1
    MinSamplesForBaseline   = 20
    DefaultMean             = 2.0
    DefaultStdDev           = 2.0

    // Control Chart thresholds
    WarningSigma            = 2.0
    CriticalSigma           = 3.0

    // CUSUM thresholds
    CUSUMSlack              = 0.5
    CUSUMWarningThreshold   = 2.5
    CUSUMCriticalThreshold  = 5.0

    // Escalation
    EscalationTime          = 10 * time.Minute
    EscalationErrorCount    = 20

    // Recovery
    RecoverySigma           = 1.0
    RecoveryDuration        = 10 * time.Minute

    // Jobs
    BaselineUpdateInterval  = 5 * time.Minute
    AnomalyCheckInterval    = 5 * time.Minute
    DefaultDigestHour       = 9
)
```

## What We're NOT Building

- Webhooks (add later if requested)
- PagerDuty/Slack integrations
- User-configurable thresholds
- Complex severity tiers

## Estimated Scope

- 4 new files
- ~500 lines of Go
- 3 new background jobs
- 4 new database tables

## Escape Hatches

- If SPC proves too sensitive: adjust sigma thresholds
- If users want webhooks: add single webhook URL to settings
- If digest timing matters: already have DigestHour/DigestTimezone
