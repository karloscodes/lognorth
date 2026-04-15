// Package events provides alerting logic for LogNorth
//
// ALL ALERT RULES ARE DEFINED HERE
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ ALERT TYPE    │ TRIGGER                      │ COOLDOWN                 │
// ├─────────────────────────────────────────────────────────────────────────┤
// │ NEW ERROR     │ First occurrence ever        │ None (fires once)        │
// │ REGRESSION    │ Error quiet 24h+, returns    │ None (quiet = cooldown)  │
// │ SPIKE         │ Error rate above baseline    │ 1 hour per endpoint      │
// │ RECOVERY      │ Spike ended, back to normal  │ Once per spike           │
// └─────────────────────────────────────────────────────────────────────────┘
//
// DETECTION FORMULAS
//
// Spike detected when:
//   - current_rate >= baseline * 3  (3x normal)
//   - OR current_rate >= baseline + 15%  (absolute jump)
//   - AND total requests >= 10  (enough traffic)
//
// Recovery detected when:
//   - current_rate <= baseline + 5%  (back to normal)
//   - OR current_rate < 5% if no baseline
//
package events

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/settings"
)

// =============================================================================
// ALERT THRESHOLDS - Tune these values to adjust sensitivity
// =============================================================================

const (
	// Error alerts
	RegressionQuietPeriod = 24 * time.Hour // How long error must be quiet to trigger regression

	// Spike detection
	SpikeMultiplier      = 3.0  // Alert when error rate >= baseline * this
	SpikeAbsoluteJump    = 15.0 // Alert when error rate >= baseline + this %
	MinTrafficForSpike   = 10   // Minimum requests to consider endpoint
	DefaultSpikeThreshold = 10  // Default % threshold when no baseline exists

	// Recovery detection
	RecoveryMargin          = 5.0 // Consider recovered when within this % of baseline
	RecoveryAbsoluteMax     = 5.0 // Consider recovered when below this % (no baseline)

	// Cooldowns
	SpikeCooldown = 1 * time.Hour // Don't re-alert same endpoint within this period
)

// =============================================================================
// DETECTION RULES - Pure functions, no side effects
// =============================================================================

// IsSpike returns true if current error rate is significantly above baseline
func IsSpike(currentRate, baseline float64, threshold int) bool {
	if baseline == 0 {
		// No baseline: use absolute threshold
		return currentRate >= float64(threshold)
	}
	// Above 3x baseline OR +15% absolute jump
	return currentRate >= baseline*SpikeMultiplier || currentRate >= baseline+SpikeAbsoluteJump
}

// IsRecovered returns true if endpoint has returned to normal after a spike
func IsRecovered(currentRate, baseline float64) bool {
	if baseline == 0 {
		return currentRate < RecoveryAbsoluteMax
	}
	return currentRate <= baseline+RecoveryMargin
}

// IsRegression returns true if error has been quiet long enough to be a regression
func IsRegression(lastOccurrence time.Time) bool {
	return time.Since(lastOccurrence) >= RegressionQuietPeriod
}

// =============================================================================
// STATE TRACKING
// =============================================================================

// SpikeState tracks spike alerting state per endpoint
type SpikeState struct {
	ID           uint      `gorm:"primaryKey"`
	APIKeyID     uint      `gorm:"uniqueIndex:idx_spike_state_lookup"`
	Path         string    `gorm:"size:500;uniqueIndex:idx_spike_state_lookup"`
	IsSpiking    bool      `gorm:"default:false"`
	LastAlertAt  time.Time
	SpikeStartAt time.Time
	// SPC fields for alert state tracking
	Severity   string `gorm:"size:20"` // "warning", "critical", or ""
	ErrorCount int    `gorm:"default:0"`
}

func (SpikeState) TableName() string {
	return "spike_states"
}

// =============================================================================
// ALERT ENGINE
// =============================================================================

// AlertEngine handles alert detection and sending
type AlertEngine struct {
	db       *gorm.DB
	settings *settings.Settings
}

// NewAlertEngine creates a new alert engine
func NewAlertEngine(db *gorm.DB) (*AlertEngine, error) {
	s, err := settings.Get(db)
	if err != nil {
		return nil, err
	}
	return &AlertEngine{db: db, settings: s}, nil
}

// CanNotify returns true if notifications are configured
func (e *AlertEngine) CanNotify() bool {
	return e.settings.CanNotify()
}

// =============================================================================
// ALERT CHECKS
// =============================================================================

// CheckNewErrorOrRegression checks if we should alert for this error
func (e *AlertEngine) CheckNewErrorOrRegression(ctx map[string]any, apiKeyID uint, appName string) {
	if !e.CanNotify() {
		return
	}

	errorMsg, _ := ctx["error"].(string)

	// Prefer new three-tier hashing when structured fields are available
	errorClass, _ := ctx["error_class"].(string)
	if errorClass == "" {
		errorClass, _ = ctx["error_type"].(string)
	}
	errorFile, _ := ctx["error_file"].(string)
	errorLine := 0
	if el, ok := ctx["error_line"].(int); ok {
		errorLine = el
	} else if el, ok := ctx["error_line"].(float64); ok {
		errorLine = int(el)
	}

	var hash string
	if errorClass != "" || errorMsg != "" {
		h1, h2, h3 := computeErrorHashes(errorClass, errorMsg, errorFile, errorLine, apiKeyID)
		if h3 != "" {
			hash = h3
		} else if h2 != "" {
			hash = h2
		} else {
			hash = h1
		}
	}
	// Fallback to legacy hash if new hashing produced nothing
	if hash == "" {
		hash = hashError(ctx, apiKeyID)
	}

	now := time.Now()

	var seen IssueSeen
	result := e.db.Where("error_hash = ?", hash).First(&seen)

	if result.Error == gorm.ErrRecordNotFound {
		// NEW ERROR - always alert
		e.sendAlert(alertNewError, appName, errorMsg, hash, 0)
		e.db.Create(&IssueSeen{
			ErrorHash:        hash,
			SeenAt:           now,
			LastOccurrenceAt: now,
			LastNotifiedAt:   now,
		})
		return
	}

	// Skip if muted - still track occurrence but no alert
	if seen.Muted {
		seen.LastOccurrenceAt = now
		e.db.Save(&seen)
		return
	}

	// Check REGRESSION - always alert if quiet period passed
	if IsRegression(seen.LastOccurrenceAt) {
		quietPeriod := time.Since(seen.LastOccurrenceAt)
		e.sendAlert(alertRegression, appName, errorMsg, hash, quietPeriod)
		seen.LastNotifiedAt = now
	}

	seen.LastOccurrenceAt = now
	e.db.Save(&seen)
}

// CheckSpikes detects spikes, sends grouped alerts
// Recovery notifications are intentionally skipped - no one needs to be told things are fine
func (e *AlertEngine) CheckSpikes() {
	if !e.CanNotify() {
		return
	}

	spikes, _ := e.detectSpikeChanges(DefaultSpikeThreshold)

	if len(spikes) > 0 {
		e.sendGroupedSpikeAlert(spikes)
	}
}

// =============================================================================
// SPIKE DETECTION
// =============================================================================

type endpointStatus struct {
	apiKeyID    uint
	path        string
	currentRate float64
	baseline    float64
	duration    time.Duration
}

func (e *AlertEngine) detectSpikeChanges(threshold int) (spikes, recoveries []endpointStatus) {
	now := time.Now()
	bucket := TruncateTo5Min(now)

	var stats []EndpointStat
	e.db.Where("bucket = ? AND total >= ?", bucket, MinTrafficForSpike).Find(&stats)

	for _, stat := range stats {
		rate := float64(stat.Errors) / float64(stat.Total) * 100
		baseline, _ := GetBaselineErrorRate(e.db, stat.APIKeyID, stat.Path)

		spiking := IsSpike(rate, baseline, threshold)
		recovered := IsRecovered(rate, baseline)

		state := e.getOrCreateSpikeState(stat.APIKeyID, stat.Path)

		switch {
		case spiking && !state.IsSpiking:
			// NEW SPIKE
			state.IsSpiking = true
			state.SpikeStartAt = now
			state.LastAlertAt = now
			e.db.Save(&state)
			spikes = append(spikes, endpointStatus{stat.APIKeyID, stat.Path, rate, baseline, 0})

		case spiking && state.IsSpiking && time.Since(state.LastAlertAt) >= SpikeCooldown:
			// ONGOING SPIKE (cooldown passed)
			state.LastAlertAt = now
			e.db.Save(&state)
			spikes = append(spikes, endpointStatus{stat.APIKeyID, stat.Path, rate, baseline, 0})

		case !spiking && state.IsSpiking && recovered:
			// RECOVERED
			state.IsSpiking = false
			e.db.Save(&state)
			recoveries = append(recoveries, endpointStatus{stat.APIKeyID, stat.Path, rate, baseline, time.Since(state.SpikeStartAt)})
		}
	}

	return spikes, recoveries
}

func (e *AlertEngine) getOrCreateSpikeState(apiKeyID uint, path string) SpikeState {
	var state SpikeState
	e.db.Where("api_key_id = ? AND path = ?", apiKeyID, path).FirstOrCreate(&state, SpikeState{
		APIKeyID: apiKeyID,
		Path:     path,
	})
	return state
}

// =============================================================================
// ALERT SENDING
// =============================================================================

type alertType int

const (
	alertNewError alertType = iota
	alertRegression
)

func (e *AlertEngine) sendAlert(typ alertType, appName, errorMsg, hash string, duration time.Duration) {
	var subject, body string

	switch typ {
	case alertNewError:
		subject = fmt.Sprintf("%s: new error", appName)
		body = errorMsg

	case alertRegression:
		subject = fmt.Sprintf("%s: error returned after %s", appName, formatDuration(duration))
		body = errorMsg
	}

	if e.settings.BaseURL != "" {
		body += fmt.Sprintf("\n\n%s/issues/%s", e.settings.BaseURL, hash)
	}

	e.settings.SendEmail(subject, body)
}

func (e *AlertEngine) sendGroupedSpikeAlert(spikes []endpointStatus) {
	var subject, body string
	if len(spikes) == 1 {
		s := spikes[0]
		subject = fmt.Sprintf("error spike on %s", s.path)
		body = fmt.Sprintf("%s is at %.1f%% errors — normally %s.", s.path, s.currentRate, formatBaseline(s.baseline))
	} else {
		subject = fmt.Sprintf("%d endpoints spiking", len(spikes))
		lines := make([]string, len(spikes))
		for i, s := range spikes {
			lines[i] = fmt.Sprintf("• %s: %.1f%% (normally %s)", s.path, s.currentRate, formatBaseline(s.baseline))
		}
		body = strings.Join(lines, "\n")
	}

	if e.settings.BaseURL != "" {
		body += fmt.Sprintf("\n\n%s/endpoints", e.settings.BaseURL)
	}
	e.settings.SendEmail(subject, body)
}

// =============================================================================
// HELPERS
// =============================================================================

func formatBaseline(baseline float64) string {
	if baseline == 0 {
		return "no baseline"
	}
	return fmt.Sprintf("%.1f%%", baseline)
}

// formatDuration renders a duration as a compact "15m" / "2h" / "3d" string.
// Used in alert subjects where screen real estate is tight and users already
// understand the unit abbreviations from log UIs.
func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dd", hours/24)
}
