// Package events provides per-error alerting (new errors + regressions).
// Rate-based spike and silence detection lives in the SPC path — see
// jobs/anomaly_checker.go and events/spc_detection.go.
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
	RegressionQuietPeriod = 24 * time.Hour // How long error must be quiet to trigger regression
)

// =============================================================================
// DETECTION RULES - Pure functions, no side effects
// =============================================================================

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
		e.sendAlert(alertNewError, appName, errorClass, errorMsg, errorFile, errorLine, hash, 0)
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

	// Check REGRESSION - alert if quiet period passed.
	// Zero LastOccurrenceAt means legacy row without tracked occurrences; backfill
	// silently rather than claiming a 292-year regression (int64 duration overflow).
	if !seen.LastOccurrenceAt.IsZero() && IsRegression(seen.LastOccurrenceAt) {
		quietPeriod := time.Since(seen.LastOccurrenceAt)
		e.sendAlert(alertRegression, appName, errorClass, errorMsg, errorFile, errorLine, hash, quietPeriod)
		seen.LastNotifiedAt = now
	}

	seen.LastOccurrenceAt = now
	e.db.Save(&seen)
}

// =============================================================================
// ALERT SENDING
// =============================================================================

type alertType int

const (
	alertNewError alertType = iota
	alertRegression
)

func (e *AlertEngine) sendAlert(typ alertType, appName, errorClass, errorMsg, errorFile string, errorLine int, hash string, duration time.Duration) {
	headline := formatHeadline(errorClass, errorMsg)

	var subject string
	switch typ {
	case alertNewError:
		subject = fmt.Sprintf("%s — new: %s", appName, headline)
	case alertRegression:
		if dur := formatDuration(duration); dur != "" {
			subject = fmt.Sprintf("%s — back after %s: %s", appName, dur, headline)
		} else {
			subject = fmt.Sprintf("%s — back: %s", appName, headline)
		}
	}

	body := headline
	if loc := formatLocation(errorFile, errorLine); loc != "" {
		body += "\nat " + loc
	}

	if e.settings.BaseURL != "" {
		body += fmt.Sprintf("\n\n%s/issues/%s", e.settings.BaseURL, hash)
	}

	e.settings.SendEmail(subject, body)
}

// formatHeadline combines class and message into a single human line.
// "TypeError: Not Found" beats "Not Found" alone — users see the category
// without opening the link.
func formatHeadline(errorClass, errorMsg string) string {
	errorClass = strings.TrimSpace(errorClass)
	errorMsg = strings.TrimSpace(errorMsg)
	switch {
	case errorClass != "" && errorMsg != "":
		return errorClass + ": " + errorMsg
	case errorClass != "":
		return errorClass
	case errorMsg != "":
		return errorMsg
	default:
		return "unknown error"
	}
}

func formatLocation(errorFile string, errorLine int) string {
	errorFile = strings.TrimSpace(errorFile)
	if errorFile == "" {
		return ""
	}
	if errorLine > 0 {
		return fmt.Sprintf("%s:%d", errorFile, errorLine)
	}
	return errorFile
}

// =============================================================================
// HELPERS
// =============================================================================

// formatDuration renders a duration as a compact "15m" / "2h" / "3d" string.
// Returns "" for non-positive or absurd values (e.g. int64 overflow from a
// zero-valued timestamp, which lands at ~292 years).
func formatDuration(d time.Duration) string {
	if d <= 0 || d > 365*24*time.Hour {
		return ""
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dd", hours/24)
}
