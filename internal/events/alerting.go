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

	// occurrenceWriteInterval is the most often an issue's last occurrence
	// is written.
	occurrenceWriteInterval = time.Minute
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
	ID           uint   `gorm:"primaryKey"`
	APIKeyID     uint   `gorm:"uniqueIndex:idx_spike_state_lookup"`
	Path         string `gorm:"size:500;uniqueIndex:idx_spike_state_lookup"`
	LastAlertAt  time.Time
	SpikeStartAt time.Time
	Severity     string `gorm:"size:20"` // "warning", "critical", or ""
	ErrorCount   int    `gorm:"default:0"`
	CleanChecks  int    `gorm:"default:0"` // Clean checks in a row since the last anomaly
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

// CheckNewErrorOrRegression records an occurrence of the event's issue and
// alerts when the issue is new, or back after RegressionQuietPeriod. It keys on
// the same issue_hash the issue list, mute, and seen state use. The state is
// tracked even without a notification channel, so setting one up later does
// not alert on every existing issue.
func (e *AlertEngine) CheckNewErrorOrRegression(event *Event, appName string) error {
	hash := event.IssueHash
	if hash == "" {
		return nil
	}
	now := time.Now()

	var seen IssueSeen
	result := e.db.Where("error_hash = ?", hash).First(&seen)

	if result.Error == gorm.ErrRecordNotFound {
		if err := e.db.Create(&IssueSeen{ErrorHash: hash, LastOccurrenceAt: now}).Error; err != nil {
			return err
		}
		return e.sendAlert(alertNewError, appName, event, 0)
	}
	if result.Error != nil {
		return result.Error
	}

	// Zero LastOccurrenceAt means a row with no tracked occurrence yet; record
	// it silently rather than claiming a 292-year regression.
	var sendErr error
	if !seen.Muted && !seen.LastOccurrenceAt.IsZero() && IsRegression(seen.LastOccurrenceAt) {
		sendErr = e.sendAlert(alertRegression, appName, event, time.Since(seen.LastOccurrenceAt))
	}

	// A regression needs a quiet day, so a minute of precision is plenty. An
	// issue that fails on every request then costs one write a minute, not
	// one per error, while ingest waits for the same write lock.
	if now.Sub(seen.LastOccurrenceAt) < occurrenceWriteInterval {
		return sendErr
	}
	if err := e.db.Model(&seen).Update("last_occurrence_at", now).Error; err != nil {
		return err
	}
	return sendErr
}

// =============================================================================
// ALERT SENDING
// =============================================================================

type alertType int

const (
	alertNewError alertType = iota
	alertRegression
)

func (e *AlertEngine) sendAlert(typ alertType, appName string, event *Event, duration time.Duration) error {
	if !e.CanNotify() {
		return nil
	}

	headline := FormatHeadline(event.ErrorClass, event.ErrorMessage)
	short := firstLine(headline, 120)

	var subject string
	switch typ {
	case alertNewError:
		subject = fmt.Sprintf("%s — new: %s", appName, short)
	case alertRegression:
		if dur := FormatDuration(duration); dur != "" {
			subject = fmt.Sprintf("%s — back after %s: %s", appName, dur, short)
		} else {
			subject = fmt.Sprintf("%s — back: %s", appName, short)
		}
	}

	return e.settings.SendNotification(subject, alertBody(event, e.settings.BaseURL))
}

// alertBody names the error, where it is, the release it came in when the
// SDK sent one, and the links to read more.
func alertBody(event *Event, baseURL string) string {
	body := FormatHeadline(event.ErrorClass, event.ErrorMessage)
	if loc := formatLocation(event.ErrorFile, event.ErrorLine); loc != "" {
		body += "\nat " + loc
	}
	if ctx, err := event.GetContext(); err == nil && ctx.Release != "" {
		body += "\nin release " + ctx.Release
	}
	if baseURL != "" {
		body += fmt.Sprintf("\n\n%s/issues/%s", baseURL, event.IssueHash)
	}
	return body + "\n\nAsk your agent: /lognorth:investigate " + event.IssueHash
}

// firstLine returns the first line of s, cut to max characters. Subjects and
// titles must be one line: ntfy rejects a newline in a header, and SMTP would
// read the rest as extra headers.
func firstLine(s string, max int) string {
	s, _, _ = strings.Cut(s, "\n")
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

// FormatHeadline combines class and message into a single human line.
// "TypeError: Not Found" beats "Not Found" alone — users see the category
// without opening the link.
func FormatHeadline(errorClass, errorMsg string) string {
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

// FormatDuration renders a duration as a compact "15m" / "2h" / "3d" string.
// Returns "" for non-positive or absurd values (e.g. int64 overflow from a
// zero-valued timestamp, which lands at ~292 years).
func FormatDuration(d time.Duration) string {
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
