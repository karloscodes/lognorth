# SPC Alerting Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace fixed-threshold alerting with Statistical Process Control that learns normal patterns and only alerts on statistically significant anomalies.

**Architecture:** Add baseline learning (hour-of-week patterns), Control Charts (spike detection via z-score), and CUSUM (drift detection). Critical alerts email immediately with `[CRITICAL]` prefix; warnings batch into daily 9am digest.

**Tech Stack:** Go, GORM, net/smtp (existing), Cartridge JobDispatcher

---

## Task 1: Add SPC Models

**Files:**
- Create: `internal/events/spc_models.go`
- Modify: `internal/database/database.go` (add migrations)

**Step 1: Write the model file**

Create `/Users/karloscodes/Code/fizzlog/lognorth/internal/events/spc_models.go`:

```go
package events

import (
	"time"
)

// BaselineStat stores learned normal behavior per app/endpoint/hour-of-week.
// Hour-of-week ranges from 0-167 (Monday 00:00 = 0, Sunday 23:00 = 167).
type BaselineStat struct {
	ID          uint      `gorm:"primaryKey"`
	APIKeyID    uint      `gorm:"index:idx_baseline_lookup"`
	Path        string    `gorm:"size:500;index:idx_baseline_lookup"`
	HourOfWeek  int       `gorm:"index:idx_baseline_lookup"`
	Mean        float64   `gorm:"default:0"`
	StdDev      float64   `gorm:"default:0"`
	SampleCount int       `gorm:"default:0"`
	UpdatedAt   time.Time
}

// CUSUMState tracks cumulative deviation for drift detection.
type CUSUMState struct {
	ID         uint      `gorm:"primaryKey"`
	APIKeyID   uint      `gorm:"uniqueIndex:idx_cusum_lookup"`
	Path       string    `gorm:"size:500;uniqueIndex:idx_cusum_lookup"`
	Cumulative float64   `gorm:"default:0"`
	StartedAt  time.Time
}

// AlertState tracks current alert severity per endpoint.
type AlertState struct {
	ID         uint      `gorm:"primaryKey"`
	APIKeyID   uint      `gorm:"uniqueIndex:idx_alert_state_lookup"`
	Path       string    `gorm:"size:500;uniqueIndex:idx_alert_state_lookup"`
	Severity   string    `gorm:"size:20"` // "warning", "critical", or ""
	DetectedAt time.Time
	NotifiedAt time.Time
	ErrorCount int `gorm:"default:0"`
}

// DigestItem stores warnings for the daily digest email.
type DigestItem struct {
	ID        uint      `gorm:"primaryKey"`
	APIKeyID  uint      `gorm:"index"`
	Path      string    `gorm:"size:500"`
	Message   string    `gorm:"type:text"`
	Severity  string    `gorm:"size:20"`
	Resolved  bool      `gorm:"default:false"`
	CreatedAt time.Time
}

// AutoMigrateSPC runs migrations for SPC tables.
func AutoMigrateSPC(db interface{ AutoMigrate(dst ...interface{}) error }) error {
	return db.AutoMigrate(
		&BaselineStat{},
		&CUSUMState{},
		&AlertState{},
		&DigestItem{},
	)
}
```

**Step 2: Add migration call**

In `/Users/karloscodes/Code/fizzlog/lognorth/internal/database/database.go`, find the `RunMigrations` function and add after existing migrations:

```go
// Add after existing migrations
if err := events.AutoMigrateSPC(db); err != nil {
	return fmt.Errorf("failed to migrate SPC tables: %w", err)
}
```

**Step 3: Run to verify migrations work**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go build ./...
```

Expected: No errors

**Step 4: Commit**

```bash
git add internal/events/spc_models.go internal/database/database.go
git commit -m "feat: add SPC data models for baseline, CUSUM, alerts, digest"
```

---

## Task 2: Add SPC Configuration Constants

**Files:**
- Create: `internal/events/spc_config.go`

**Step 1: Write the config file**

Create `/Users/karloscodes/Code/fizzlog/lognorth/internal/events/spc_config.go`:

```go
package events

import "time"

// SPC Configuration Constants
const (
	// Baseline learning
	BaselineSmoothingFactor = 0.1  // EMA weight for new data (10%)
	MinSamplesForBaseline   = 20   // Samples needed before trusting baseline
	DefaultMean             = 2.0  // Default error rate % during cold start
	DefaultStdDev           = 2.0  // Conservative stddev during cold start

	// Control Chart thresholds (z-scores)
	WarningSigma  = 2.0 // 95% confidence
	CriticalSigma = 3.0 // 99.7% confidence

	// CUSUM thresholds
	CUSUMSlack             = 0.5 // Acceptable drift before accumulating
	CUSUMWarningThreshold  = 2.5 // Accumulated deviation for warning
	CUSUMCriticalThreshold = 5.0 // Accumulated deviation for critical

	// Escalation rules
	EscalationTime       = 10 * time.Minute // Warning → Critical if persists
	EscalationErrorCount = 20               // Warning → Critical if errors exceed

	// Recovery
	RecoverySigma    = 1.0               // Below mean + 1σ to recover
	RecoveryDuration = 10 * time.Minute  // Must stay recovered this long

	// Jobs
	BaselineUpdateInterval = 5 * time.Minute
	AnomalyCheckInterval   = 5 * time.Minute
	DefaultDigestHour      = 9 // 9am local time
)

// HourOfWeek returns 0-167 for the current hour-of-week.
// Monday 00:00 = 0, Sunday 23:00 = 167.
func HourOfWeek(t time.Time) int {
	weekday := int(t.Weekday())
	// Convert Sunday=0 to Monday=0 based week
	if weekday == 0 {
		weekday = 6 // Sunday becomes 6
	} else {
		weekday-- // Mon=0, Tue=1, etc.
	}
	return weekday*24 + t.Hour()
}
```

**Step 2: Verify it compiles**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go build ./...
```

**Step 3: Commit**

```bash
git add internal/events/spc_config.go
git commit -m "feat: add SPC configuration constants"
```

---

## Task 3: Add SPC Detection Functions

**Files:**
- Create: `internal/events/spc_detection.go`
- Create: `tests/spc_detection_test.go`

**Step 1: Write failing tests**

Create `/Users/karloscodes/Code/fizzlog/lognorth/tests/spc_detection_test.go`:

```go
package tests

import (
	"testing"

	"lognorth/internal/events"
)

func TestIsSpike(t *testing.T) {
	t.Run("returns critical when above 3 sigma", func(t *testing.T) {
		isSpike, severity := events.IsSpike(10.0, 2.0, 2.0) // z = (10-2)/2 = 4

		if !isSpike {
			t.Error("expected spike to be detected")
		}
		if severity != "critical" {
			t.Errorf("expected critical, got %s", severity)
		}
	})

	t.Run("returns warning when between 2 and 3 sigma", func(t *testing.T) {
		isSpike, severity := events.IsSpike(6.5, 2.0, 2.0) // z = (6.5-2)/2 = 2.25

		if !isSpike {
			t.Error("expected spike to be detected")
		}
		if severity != "warning" {
			t.Errorf("expected warning, got %s", severity)
		}
	})

	t.Run("returns false when within normal range", func(t *testing.T) {
		isSpike, _ := events.IsSpike(3.0, 2.0, 2.0) // z = (3-2)/2 = 0.5

		if isSpike {
			t.Error("expected no spike")
		}
	})

	t.Run("handles zero stddev", func(t *testing.T) {
		isSpike, severity := events.IsSpike(5.0, 2.0, 0.0) // Should use 1.0

		if !isSpike {
			t.Error("expected spike with zero stddev fallback")
		}
		if severity != "critical" {
			t.Errorf("expected critical, got %s", severity)
		}
	})
}

func TestUpdateCUSUM(t *testing.T) {
	t.Run("accumulates positive deviation", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 0}

		events.UpdateCUSUM(state, 3.0, 2.0) // deviation = 3 - 2 - 0.5 = 0.5

		if state.Cumulative != 0.5 {
			t.Errorf("expected 0.5, got %f", state.Cumulative)
		}
	})

	t.Run("does not go below zero", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 0}

		events.UpdateCUSUM(state, 1.0, 2.0) // deviation = 1 - 2 - 0.5 = -1.5

		if state.Cumulative != 0 {
			t.Errorf("expected 0, got %f", state.Cumulative)
		}
	})

	t.Run("returns critical when threshold exceeded", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 4.6}

		detected, severity := events.UpdateCUSUM(state, 3.0, 2.0) // adds 0.5

		if !detected {
			t.Error("expected drift detected")
		}
		if severity != "critical" {
			t.Errorf("expected critical, got %s", severity)
		}
	})

	t.Run("returns warning when above warning threshold", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 2.1}

		detected, severity := events.UpdateCUSUM(state, 3.0, 2.0) // adds 0.5 = 2.6

		if !detected {
			t.Error("expected drift detected")
		}
		if severity != "warning" {
			t.Errorf("expected warning, got %s", severity)
		}
	})
}

func TestHourOfWeek(t *testing.T) {
	t.Run("monday midnight is 0", func(t *testing.T) {
		// 2026-02-09 is a Monday
		monday := time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC)

		if got := events.HourOfWeek(monday); got != 0 {
			t.Errorf("expected 0, got %d", got)
		}
	})

	t.Run("sunday 23:00 is 167", func(t *testing.T) {
		// 2026-02-15 is a Sunday
		sunday := time.Date(2026, 2, 15, 23, 0, 0, 0, time.UTC)

		if got := events.HourOfWeek(sunday); got != 167 {
			t.Errorf("expected 167, got %d", got)
		}
	})
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go test ./tests/... -run TestIsSpike -v
```

Expected: FAIL (functions don't exist yet)

**Step 3: Write the implementation**

Create `/Users/karloscodes/Code/fizzlog/lognorth/internal/events/spc_detection.go`:

```go
package events

// IsSpike checks if current error rate is a statistical spike.
// Returns (isSpike, severity) where severity is "warning", "critical", or "".
func IsSpike(current, mean, stddev float64) (bool, string) {
	if stddev == 0 {
		stddev = 1.0 // Prevent division by zero
	}

	zScore := (current - mean) / stddev

	if zScore > CriticalSigma {
		return true, "critical"
	}
	if zScore > WarningSigma {
		return true, "warning"
	}
	return false, ""
}

// UpdateCUSUM updates the cumulative sum and checks for drift.
// Returns (detected, severity) where severity is "warning", "critical", or "".
func UpdateCUSUM(state *CUSUMState, current, mean float64) (bool, string) {
	deviation := current - mean - CUSUMSlack
	state.Cumulative = max(0, state.Cumulative+deviation)

	if state.Cumulative > CUSUMCriticalThreshold {
		return true, "critical"
	}
	if state.Cumulative > CUSUMWarningThreshold {
		return true, "warning"
	}
	return false, ""
}

// MaxSeverity returns the higher severity level.
func MaxSeverity(a, b string) string {
	if a == "critical" || b == "critical" {
		return "critical"
	}
	if a == "warning" || b == "warning" {
		return "warning"
	}
	return ""
}

// IsRecovered checks if error rate is back to normal.
func IsRecovered(current, mean, stddev float64) bool {
	if stddev == 0 {
		stddev = 1.0
	}
	threshold := mean + RecoverySigma*stddev
	return current <= threshold
}
```

**Step 4: Run tests to verify they pass**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go test ./tests/... -run "TestIsSpike|TestUpdateCUSUM|TestHourOfWeek" -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/spc_detection.go tests/spc_detection_test.go
git commit -m "feat: add SPC detection functions with tests"
```

---

## Task 4: Add Baseline Learning Functions

**Files:**
- Create: `internal/events/spc_baseline.go`
- Add to: `tests/spc_detection_test.go`

**Step 1: Write failing tests**

Add to `/Users/karloscodes/Code/fizzlog/lognorth/tests/spc_detection_test.go`:

```go
func TestGetOrCreateBaseline(t *testing.T) {
	db := setupTestDB(t)
	events.AutoMigrateSPC(db)

	t.Run("creates new baseline with defaults for cold start", func(t *testing.T) {
		baseline := events.GetOrCreateBaseline(db, 1, "/api/test", 42)

		if baseline.SampleCount != 0 {
			t.Errorf("expected 0 samples, got %d", baseline.SampleCount)
		}
		if baseline.Mean != events.DefaultMean {
			t.Errorf("expected default mean %f, got %f", events.DefaultMean, baseline.Mean)
		}
	})

	t.Run("returns existing baseline", func(t *testing.T) {
		// Create baseline
		events.GetOrCreateBaseline(db, 1, "/api/existing", 10)

		// Update it
		db.Model(&events.BaselineStat{}).
			Where("api_key_id = ? AND path = ? AND hour_of_week = ?", 1, "/api/existing", 10).
			Update("sample_count", 50)

		// Fetch again
		baseline := events.GetOrCreateBaseline(db, 1, "/api/existing", 10)

		if baseline.SampleCount != 50 {
			t.Errorf("expected 50 samples, got %d", baseline.SampleCount)
		}
	})
}

func TestUpdateBaseline(t *testing.T) {
	db := setupTestDB(t)
	events.AutoMigrateSPC(db)

	t.Run("updates mean with EMA", func(t *testing.T) {
		baseline := &events.BaselineStat{
			APIKeyID:    1,
			Path:        "/api/test",
			HourOfWeek:  0,
			Mean:        10.0,
			StdDev:      2.0,
			SampleCount: 20,
		}
		db.Create(baseline)

		events.UpdateBaseline(db, baseline, 20.0) // New observation

		var updated events.BaselineStat
		db.First(&updated, baseline.ID)

		// EMA: new_mean = 0.9 * 10 + 0.1 * 20 = 11
		expectedMean := 11.0
		if updated.Mean != expectedMean {
			t.Errorf("expected mean %f, got %f", expectedMean, updated.Mean)
		}
		if updated.SampleCount != 21 {
			t.Errorf("expected 21 samples, got %d", updated.SampleCount)
		}
	})
}

func TestGetEffectiveBaseline(t *testing.T) {
	db := setupTestDB(t)
	events.AutoMigrateSPC(db)

	t.Run("returns defaults for cold start", func(t *testing.T) {
		baseline := events.GetOrCreateBaseline(db, 1, "/api/cold", 0)
		mean, stddev := events.GetEffectiveBaseline(baseline)

		if mean != events.DefaultMean {
			t.Errorf("expected default mean, got %f", mean)
		}
		if stddev != events.DefaultStdDev {
			t.Errorf("expected default stddev, got %f", stddev)
		}
	})

	t.Run("returns learned values after enough samples", func(t *testing.T) {
		baseline := &events.BaselineStat{
			APIKeyID:    1,
			Path:        "/api/warm",
			HourOfWeek:  0,
			Mean:        5.0,
			StdDev:      1.5,
			SampleCount: 25,
		}
		db.Create(baseline)

		mean, stddev := events.GetEffectiveBaseline(baseline)

		if mean != 5.0 {
			t.Errorf("expected 5.0, got %f", mean)
		}
		if stddev != 1.5 {
			t.Errorf("expected 1.5, got %f", stddev)
		}
	})
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go test ./tests/... -run "TestGetOrCreateBaseline|TestUpdateBaseline|TestGetEffectiveBaseline" -v
```

Expected: FAIL

**Step 3: Write the implementation**

Create `/Users/karloscodes/Code/fizzlog/lognorth/internal/events/spc_baseline.go`:

```go
package events

import (
	"math"
	"time"

	"gorm.io/gorm"
)

// GetOrCreateBaseline retrieves or creates a baseline for the given key.
func GetOrCreateBaseline(db *gorm.DB, apiKeyID uint, path string, hourOfWeek int) *BaselineStat {
	var baseline BaselineStat
	result := db.Where("api_key_id = ? AND path = ? AND hour_of_week = ?",
		apiKeyID, path, hourOfWeek).First(&baseline)

	if result.Error != nil {
		baseline = BaselineStat{
			APIKeyID:    apiKeyID,
			Path:        path,
			HourOfWeek:  hourOfWeek,
			Mean:        DefaultMean,
			StdDev:      DefaultStdDev,
			SampleCount: 0,
			UpdatedAt:   time.Now(),
		}
		db.Create(&baseline)
	}

	return &baseline
}

// UpdateBaseline updates the baseline with a new observation using EMA.
func UpdateBaseline(db *gorm.DB, baseline *BaselineStat, newValue float64) {
	// Exponential moving average for mean
	baseline.Mean = (1-BaselineSmoothingFactor)*baseline.Mean + BaselineSmoothingFactor*newValue

	// Welford's online algorithm for stddev (simplified EMA version)
	diff := newValue - baseline.Mean
	baseline.StdDev = math.Sqrt(
		(1-BaselineSmoothingFactor)*baseline.StdDev*baseline.StdDev +
			BaselineSmoothingFactor*diff*diff,
	)

	baseline.SampleCount++
	baseline.UpdatedAt = time.Now()

	db.Save(baseline)
}

// GetEffectiveBaseline returns the mean and stddev to use.
// Uses defaults during cold start (insufficient samples).
func GetEffectiveBaseline(baseline *BaselineStat) (mean, stddev float64) {
	if baseline.SampleCount < MinSamplesForBaseline {
		return DefaultMean, DefaultStdDev
	}
	return baseline.Mean, baseline.StdDev
}

// GetCurrentErrorRate calculates error rate for the current 5-min bucket.
func GetCurrentErrorRate(db *gorm.DB, apiKeyID uint, path string) float64 {
	bucket := TruncateTo5Min(time.Now())

	var stat EndpointStat
	result := db.Where("api_key_id = ? AND path = ? AND bucket = ?",
		apiKeyID, path, bucket).First(&stat)

	if result.Error != nil || stat.Total == 0 {
		return 0
	}

	return float64(stat.Errors) / float64(stat.Total) * 100
}

// GetActiveEndpoints returns all endpoints with recent activity.
func GetActiveEndpoints(db *gorm.DB, apiKeyID uint) []string {
	since := time.Now().Add(-1 * time.Hour)

	var paths []string
	db.Model(&EndpointStat{}).
		Where("api_key_id = ? AND bucket > ?", apiKeyID, since).
		Distinct("path").
		Pluck("path", &paths)

	return paths
}
```

**Step 4: Run tests to verify they pass**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go test ./tests/... -run "TestGetOrCreateBaseline|TestUpdateBaseline|TestGetEffectiveBaseline" -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/spc_baseline.go tests/spc_detection_test.go
git commit -m "feat: add baseline learning functions with tests"
```

---

## Task 5: Add Alert State Management

**Files:**
- Create: `internal/events/spc_alerts.go`
- Add to: `tests/spc_detection_test.go`

**Step 1: Write failing tests**

Add to `/Users/karloscodes/Code/fizzlog/lognorth/tests/spc_detection_test.go`:

```go
func TestAlertStateTransitions(t *testing.T) {
	db := setupTestDB(t)
	events.AutoMigrateSPC(db)

	t.Run("escalates warning to critical after timeout", func(t *testing.T) {
		state := &events.AlertState{
			APIKeyID:   1,
			Path:       "/api/test",
			Severity:   "warning",
			DetectedAt: time.Now().Add(-15 * time.Minute), // 15 min ago
			ErrorCount: 5,
		}
		db.Create(state)

		events.UpdateAlertSeverity(db, state, "warning", 10)

		var updated events.AlertState
		db.First(&updated, state.ID)

		if updated.Severity != "critical" {
			t.Errorf("expected critical after timeout, got %s", updated.Severity)
		}
	})

	t.Run("escalates warning to critical after error count", func(t *testing.T) {
		state := &events.AlertState{
			APIKeyID:   1,
			Path:       "/api/errors",
			Severity:   "warning",
			DetectedAt: time.Now().Add(-2 * time.Minute), // Recent
			ErrorCount: 15,
		}
		db.Create(state)

		events.UpdateAlertSeverity(db, state, "warning", 10) // +10 = 25 errors

		var updated events.AlertState
		db.First(&updated, state.ID)

		if updated.Severity != "critical" {
			t.Errorf("expected critical after error count, got %s", updated.Severity)
		}
	})

	t.Run("clears state on recovery", func(t *testing.T) {
		state := &events.AlertState{
			APIKeyID:   1,
			Path:       "/api/recover",
			Severity:   "critical",
			DetectedAt: time.Now().Add(-30 * time.Minute),
			ErrorCount: 50,
		}
		db.Create(state)

		events.ClearAlertState(db, state)

		var updated events.AlertState
		db.First(&updated, state.ID)

		if updated.Severity != "" {
			t.Errorf("expected empty severity, got %s", updated.Severity)
		}
	})
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go test ./tests/... -run TestAlertStateTransitions -v
```

Expected: FAIL

**Step 3: Write the implementation**

Create `/Users/karloscodes/Code/fizzlog/lognorth/internal/events/spc_alerts.go`:

```go
package events

import (
	"time"

	"gorm.io/gorm"
)

// GetOrCreateAlertState retrieves or creates an alert state for the endpoint.
func GetOrCreateAlertState(db *gorm.DB, apiKeyID uint, path string) *AlertState {
	var state AlertState
	result := db.Where("api_key_id = ? AND path = ?", apiKeyID, path).First(&state)

	if result.Error != nil {
		state = AlertState{
			APIKeyID: apiKeyID,
			Path:     path,
		}
		db.Create(&state)
	}

	return &state
}

// UpdateAlertSeverity updates the alert state with new severity and error count.
// Handles escalation from warning to critical based on time and error count.
func UpdateAlertSeverity(db *gorm.DB, state *AlertState, newSeverity string, errorsDelta int) {
	state.ErrorCount += errorsDelta

	// Check for escalation from warning to critical
	if state.Severity == "warning" && newSeverity != "critical" {
		timeSinceDetection := time.Since(state.DetectedAt)

		if timeSinceDetection > EscalationTime || state.ErrorCount > EscalationErrorCount {
			newSeverity = "critical"
		}
	}

	// First detection
	if state.Severity == "" && newSeverity != "" {
		state.DetectedAt = time.Now()
	}

	// Upgrade to critical
	if newSeverity == "critical" && state.Severity != "critical" {
		state.Severity = "critical"
	} else if newSeverity == "warning" && state.Severity == "" {
		state.Severity = "warning"
	}

	db.Save(state)
}

// ClearAlertState resets the alert state after recovery.
func ClearAlertState(db *gorm.DB, state *AlertState) {
	state.Severity = ""
	state.ErrorCount = 0
	state.DetectedAt = time.Time{}
	db.Save(state)
}

// GetUnnotifiedCriticalAlerts returns critical alerts that haven't been notified.
func GetUnnotifiedCriticalAlerts(db *gorm.DB) []AlertState {
	var alerts []AlertState
	db.Where("severity = ? AND (notified_at IS NULL OR notified_at < detected_at)",
		"critical").Find(&alerts)
	return alerts
}

// MarkAlertNotified marks the alert as notified.
func MarkAlertNotified(db *gorm.DB, state *AlertState) {
	state.NotifiedAt = time.Now()
	db.Save(state)
}

// QueueDigestItem adds a warning to the daily digest queue.
func QueueDigestItem(db *gorm.DB, apiKeyID uint, path, message, severity string) {
	item := DigestItem{
		APIKeyID:  apiKeyID,
		Path:      path,
		Message:   message,
		Severity:  severity,
		CreatedAt: time.Now(),
	}
	db.Create(&item)
}

// GetPendingDigestItems returns all unresolved digest items.
func GetPendingDigestItems(db *gorm.DB) []DigestItem {
	var items []DigestItem
	db.Where("resolved = ?", false).Order("created_at desc").Find(&items)
	return items
}

// MarkDigestItemsResolved marks digest items as resolved.
func MarkDigestItemsResolved(db *gorm.DB, items []DigestItem) {
	for _, item := range items {
		db.Model(&item).Update("resolved", true)
	}
}
```

**Step 4: Run tests to verify they pass**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go test ./tests/... -run TestAlertStateTransitions -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/events/spc_alerts.go tests/spc_detection_test.go
git commit -m "feat: add alert state management with escalation"
```

---

## Task 6: Add Settings for Digest

**Files:**
- Modify: `internal/settings/settings.go`

**Step 1: Add fields to Settings struct**

In `/Users/karloscodes/Code/fizzlog/lognorth/internal/settings/settings.go`, add to the Settings struct:

```go
// Add these fields to the Settings struct
DigestHour     int    `gorm:"default:9"`       // Hour to send digest (0-23)
DigestTimezone string `gorm:"size:50;default:'UTC'"` // IANA timezone
```

**Step 2: Verify it compiles**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go build ./...
```

**Step 3: Commit**

```bash
git add internal/settings/settings.go
git commit -m "feat: add digest hour and timezone settings"
```

---

## Task 7: Add Baseline Updater Job

**Files:**
- Create: `internal/jobs/baseline_updater.go`

**Step 1: Write the job**

Create `/Users/karloscodes/Code/fizzlog/lognorth/internal/jobs/baseline_updater.go`:

```go
package jobs

import (
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"time"

	"github.com/karloscodes/cartridge"
)

type BaselineUpdater struct{}

func NewBaselineUpdater() *BaselineUpdater {
	return &BaselineUpdater{}
}

func (j *BaselineUpdater) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	// Get all active API keys
	var keys []apikeys.APIKey
	db.Where("active = ?", true).Find(&keys)

	hourOfWeek := events.HourOfWeek(time.Now())

	for _, key := range keys {
		endpoints := events.GetActiveEndpoints(db, key.ID)

		for _, path := range endpoints {
			// Get current error rate
			errorRate := events.GetCurrentErrorRate(db, key.ID, path)

			// Get or create baseline for this hour-of-week
			baseline := events.GetOrCreateBaseline(db, key.ID, path, hourOfWeek)

			// Update baseline with new observation
			events.UpdateBaseline(db, baseline, errorRate)
		}
	}

	return nil
}
```

**Step 2: Verify it compiles**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go build ./...
```

**Step 3: Commit**

```bash
git add internal/jobs/baseline_updater.go
git commit -m "feat: add baseline updater background job"
```

---

## Task 8: Add Anomaly Checker Job

**Files:**
- Create: `internal/jobs/anomaly_checker.go`

**Step 1: Write the job**

Create `/Users/karloscodes/Code/fizzlog/lognorth/internal/jobs/anomaly_checker.go`:

```go
package jobs

import (
	"fmt"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"time"

	"github.com/karloscodes/cartridge"
)

type AnomalyChecker struct{}

func NewAnomalyChecker() *AnomalyChecker {
	return &AnomalyChecker{}
}

func (j *AnomalyChecker) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	// Get all active API keys
	var keys []apikeys.APIKey
	db.Where("active = ?", true).Find(&keys)

	hourOfWeek := events.HourOfWeek(time.Now())

	for _, key := range keys {
		endpoints := events.GetActiveEndpoints(db, key.ID)

		for _, path := range endpoints {
			j.checkEndpoint(ctx, key.ID, key.Name, path, hourOfWeek)
		}
	}

	// Send critical alerts
	j.sendCriticalAlerts(ctx)

	return nil
}

func (j *AnomalyChecker) checkEndpoint(ctx *cartridge.JobContext, apiKeyID uint, appName, path string, hourOfWeek int) {
	db := ctx.DB

	// Get current error rate
	currentRate := events.GetCurrentErrorRate(db, apiKeyID, path)

	// Get baseline
	baseline := events.GetOrCreateBaseline(db, apiKeyID, path, hourOfWeek)
	mean, stddev := events.GetEffectiveBaseline(baseline)

	// Control Chart check (spikes)
	_, spikeSeverity := events.IsSpike(currentRate, mean, stddev)

	// CUSUM check (drift)
	cusum := j.getOrCreateCUSUM(db, apiKeyID, path)
	_, driftSeverity := events.UpdateCUSUM(cusum, currentRate, mean)
	db.Save(cusum)

	// Take worst severity
	severity := events.MaxSeverity(spikeSeverity, driftSeverity)

	// Get alert state
	alertState := events.GetOrCreateAlertState(db, apiKeyID, path)

	// Check for recovery
	if alertState.Severity != "" && severity == "" {
		if events.IsRecovered(currentRate, mean, stddev) {
			j.handleRecovery(ctx, alertState, appName, path, currentRate, mean)
			return
		}
	}

	// Update alert state
	if severity != "" {
		// Estimate errors in this bucket
		errorsDelta := int(currentRate / 100 * 10) // Rough estimate

		previousSeverity := alertState.Severity
		events.UpdateAlertSeverity(db, alertState, severity, errorsDelta)

		// Queue warning for digest if new warning
		if previousSeverity == "" && alertState.Severity == "warning" {
			msg := fmt.Sprintf("%s: %s error rate elevated to %.1f%% (normal: %.1f%%)",
				appName, path, currentRate, mean)
			events.QueueDigestItem(db, apiKeyID, path, msg, "warning")
		}
	}
}

func (j *AnomalyChecker) getOrCreateCUSUM(db interface {
	Where(query interface{}, args ...interface{}) *gorm.DB
	Create(value interface{}) *gorm.DB
	First(dest interface{}, conds ...interface{}) *gorm.DB
}, apiKeyID uint, path string) *events.CUSUMState {
	var state events.CUSUMState
	result := db.Where("api_key_id = ? AND path = ?", apiKeyID, path).First(&state)

	if result.Error != nil {
		state = events.CUSUMState{
			APIKeyID:  apiKeyID,
			Path:      path,
			StartedAt: time.Now(),
		}
		db.Create(&state)
	}

	return &state
}

func (j *AnomalyChecker) handleRecovery(ctx *cartridge.JobContext, state *events.AlertState, appName, path string, currentRate, mean float64) {
	db := ctx.DB
	s := settings.Get(db)

	wasСritical := state.Severity == "critical"
	events.ClearAlertState(db, state)

	// Reset CUSUM
	var cusum events.CUSUMState
	if db.Where("api_key_id = ? AND path = ?", state.APIKeyID, path).First(&cusum).Error == nil {
		cusum.Cumulative = 0
		db.Save(&cusum)
	}

	// Send recovery email for critical
	if wasСritical && s.CanNotify() {
		subject := fmt.Sprintf("[RECOVERED] %s: Error rate back to normal", appName)
		body := fmt.Sprintf(`%s has recovered.

- Endpoint: %s
- Duration: %s
- Current error rate: %.1f%%
- Normal error rate: %.1f%%

View details: %s/apps/%d/issues`,
			path,
			path,
			time.Since(state.DetectedAt).Round(time.Minute),
			currentRate,
			mean,
			s.BaseURL,
			state.APIKeyID,
		)
		s.SendEmail(subject, body)
	}
}

func (j *AnomalyChecker) sendCriticalAlerts(ctx *cartridge.JobContext) {
	db := ctx.DB
	s := settings.Get(db)

	if !s.CanNotify() {
		return
	}

	alerts := events.GetUnnotifiedCriticalAlerts(db)

	for _, alert := range alerts {
		// Get app name
		var key apikeys.APIKey
		db.First(&key, alert.APIKeyID)

		// Get baseline for context
		hourOfWeek := events.HourOfWeek(time.Now())
		baseline := events.GetOrCreateBaseline(db, alert.APIKeyID, alert.Path, hourOfWeek)
		mean, stddev := events.GetEffectiveBaseline(baseline)

		currentRate := events.GetCurrentErrorRate(db, alert.APIKeyID, alert.Path)
		zScore := (currentRate - mean) / stddev

		subject := fmt.Sprintf("[CRITICAL] %s: Error rate spike detected", key.Name)
		body := fmt.Sprintf(`%s is experiencing abnormal error rates.

What's happening:
- Endpoint: %s
- Current error rate: %.1f%% (normal: %.1f%%)
- Started: %s ago
- Errors so far: %d

This is %.1f standard deviations above normal for this time of day.

View details: %s/apps/%d/issues`,
			key.Name,
			alert.Path,
			currentRate,
			mean,
			time.Since(alert.DetectedAt).Round(time.Minute),
			alert.ErrorCount,
			zScore,
			s.BaseURL,
			alert.APIKeyID,
		)

		if err := s.SendEmail(subject, body); err == nil {
			events.MarkAlertNotified(db, &alert)
		}
	}
}
```

**Step 2: Fix import (add gorm)**

The file needs the gorm import. Update the imports:

```go
import (
	"fmt"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"
)
```

**Step 3: Verify it compiles**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go build ./...
```

**Step 4: Commit**

```bash
git add internal/jobs/anomaly_checker.go
git commit -m "feat: add anomaly checker job with SPC detection"
```

---

## Task 9: Add Digest Sender Job

**Files:**
- Create: `internal/jobs/digest_sender.go`

**Step 1: Write the job**

Create `/Users/karloscodes/Code/fizzlog/lognorth/internal/jobs/digest_sender.go`:

```go
package jobs

import (
	"fmt"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"strings"
	"time"

	"github.com/karloscodes/cartridge"
)

type DigestSender struct {
	lastSentDate string
}

func NewDigestSender() *DigestSender {
	return &DigestSender{}
}

func (j *DigestSender) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB
	s := settings.Get(db)

	if !s.CanNotify() {
		return nil
	}

	// Get timezone
	loc, err := time.LoadLocation(s.DigestTimezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	today := now.Format("2006-01-02")

	// Only send once per day at the configured hour
	if j.lastSentDate == today {
		return nil
	}

	if now.Hour() != s.DigestHour {
		return nil
	}

	// Get pending digest items
	items := events.GetPendingDigestItems(db)
	if len(items) == 0 {
		j.lastSentDate = today
		return nil
	}

	// Build digest email
	subject := fmt.Sprintf("LogNorth Daily Digest - %s", now.Format("Jan 2"))
	body := j.buildDigestBody(db, items, s)

	if err := s.SendEmail(subject, body); err != nil {
		return err
	}

	// Mark items as resolved
	events.MarkDigestItemsResolved(db, items)
	j.lastSentDate = today

	return nil
}

func (j *DigestSender) buildDigestBody(db interface {
	Where(query interface{}, args ...interface{}) interface{ Find(dest interface{}) interface{} }
	Find(dest interface{}) interface{ Error() error }
}, items []events.DigestItem, s *settings.Settings) string {
	var sb strings.Builder

	sb.WriteString("Yesterday's summary across your apps:\n\n")

	// Group by resolved/unresolved
	var warnings []string
	for _, item := range items {
		warnings = append(warnings, fmt.Sprintf("- %s", item.Message))
	}

	if len(warnings) > 0 {
		sb.WriteString("WARNINGS (resolved automatically)\n")
		sb.WriteString(strings.Join(warnings, "\n"))
		sb.WriteString("\n\n")
	}

	// Add stats summary
	sb.WriteString("STATS\n")

	var keys []apikeys.APIKey
	db.Where("active = ?", true).Find(&keys)

	for _, key := range keys {
		// Get yesterday's stats
		yesterday := time.Now().AddDate(0, 0, -1)
		var totalEvents, totalErrors int64

		db.Model(&events.Event{}).
			Where("api_key_id = ? AND timestamp > ?", key.ID, yesterday).
			Count(&totalEvents)

		db.Model(&events.Event{}).
			Where("api_key_id = ? AND timestamp > ? AND is_error = ?", key.ID, yesterday, true).
			Count(&totalErrors)

		if totalEvents > 0 {
			errorRate := float64(totalErrors) / float64(totalEvents) * 100
			sb.WriteString(fmt.Sprintf("- %s: %d events, %d errors (%.2f%%)\n",
				key.Name, totalEvents, totalErrors, errorRate))
		}
	}

	sb.WriteString("\nNo action needed - just keeping you informed.\n")
	sb.WriteString(fmt.Sprintf("\nView all: %s\n", s.BaseURL))

	return sb.String()
}
```

**Step 2: Fix interface usage**

The db interface is complex. Simplify by using concrete type:

Replace the function signature:

```go
func (j *DigestSender) buildDigestBody(db *gorm.DB, items []events.DigestItem, s *settings.Settings) string {
```

And add the import:

```go
import (
	"fmt"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"strings"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"
)
```

**Step 3: Verify it compiles**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go build ./...
```

**Step 4: Commit**

```bash
git add internal/jobs/digest_sender.go
git commit -m "feat: add daily digest sender job"
```

---

## Task 10: Register New Jobs in App

**Files:**
- Modify: `internal/app.go`

**Step 1: Add job dispatchers**

In `/Users/karloscodes/Code/fizzlog/lognorth/internal/app.go`, find where jobs are registered (around line 58-61) and add:

```go
// After existing job dispatchers
baselineUpdater := cartridge.NewJobDispatcher(logger, dbManager, 5*time.Minute, jobs.NewBaselineUpdater())
anomalyChecker := cartridge.NewJobDispatcher(logger, dbManager, 5*time.Minute, jobs.NewAnomalyChecker())
digestSender := cartridge.NewJobDispatcher(logger, dbManager, 1*time.Minute, jobs.NewDigestSender())
```

And add to the Start() calls (find where `eventProcessor.Start()` etc. are called):

```go
baselineUpdater.Start()
anomalyChecker.Start()
digestSender.Start()
```

**Step 2: Verify it compiles**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go build ./...
```

**Step 3: Commit**

```bash
git add internal/app.go
git commit -m "feat: register SPC jobs in application startup"
```

---

## Task 11: Run Full Test Suite

**Step 1: Run all tests**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && go test ./... -v
```

Expected: All tests pass

**Step 2: Manual verification**

```bash
cd /Users/karloscodes/Code/fizzlog/lognorth && make dev
```

Verify:
- Application starts without errors
- Check logs for new job registrations
- Create some test events
- Verify baseline_stats table populates

---

## Summary

**Files created:**
- `internal/events/spc_models.go` - Data models
- `internal/events/spc_config.go` - Constants
- `internal/events/spc_detection.go` - Detection algorithms
- `internal/events/spc_baseline.go` - Baseline learning
- `internal/events/spc_alerts.go` - Alert state management
- `internal/jobs/baseline_updater.go` - Background job
- `internal/jobs/anomaly_checker.go` - Background job
- `internal/jobs/digest_sender.go` - Background job
- `tests/spc_detection_test.go` - Tests

**Files modified:**
- `internal/database/database.go` - Add migrations
- `internal/settings/settings.go` - Add digest fields
- `internal/app.go` - Register jobs

**Total commits:** 11
