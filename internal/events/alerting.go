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

// An ErrorAlert is an issue the owner hears about: a new one, or one back
// after RegressionQuietPeriod.
type ErrorAlert struct {
	Event *Event
	// QuietFor is how long a regression was quiet; 0 for a new issue.
	QuietFor time.Duration
}

// RecordOccurrence records an occurrence of the event's issue, and returns
// an alert when the issue is new, or back after RegressionQuietPeriod. It
// keys on the same issue_hash the issue list, mute, and seen state use. The
// state is tracked even without a notification channel, so setting one up
// later does not alert on every existing issue. SendErrorAlerts sends the
// alerts.
func (e *AlertEngine) RecordOccurrence(event *Event) (*ErrorAlert, error) {
	hash := event.IssueHash
	if hash == "" {
		return nil, nil
	}
	now := time.Now()

	var seen IssueSeen
	result := e.db.Where("error_hash = ?", hash).First(&seen)

	if result.Error == gorm.ErrRecordNotFound {
		if err := e.db.Create(&IssueSeen{ErrorHash: hash, LastOccurrenceAt: now}).Error; err != nil {
			return nil, err
		}
		return &ErrorAlert{Event: event}, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}

	// Zero LastOccurrenceAt means a row with no tracked occurrence yet; record
	// it silently rather than claiming a 292-year regression.
	var alert *ErrorAlert
	if !seen.Muted && !seen.LastOccurrenceAt.IsZero() && IsRegression(seen.LastOccurrenceAt) {
		alert = &ErrorAlert{Event: event, QuietFor: now.Sub(seen.LastOccurrenceAt)}
	}

	// A regression needs a quiet day, so a minute of precision is plenty. An
	// issue that fails on every request then costs one write a minute, not
	// one per error, while ingest waits for the same write lock.
	if now.Sub(seen.LastOccurrenceAt) < occurrenceWriteInterval {
		return alert, nil
	}
	if err := e.db.Model(&seen).Update("last_occurrence_at", now).Error; err != nil {
		return alert, err
	}
	return alert, nil
}

// =============================================================================
// ALERT SENDING
// =============================================================================

// maxListedAlerts caps the issues one email lists. The rest are a link away.
const maxListedAlerts = 10

// SendErrorAlerts sends one notification for an app's new and regressed
// issues of one processor run, so a deploy that breaks 20 things sends one
// email, not 20. One issue gets the same subject and body as always.
func (e *AlertEngine) SendErrorAlerts(appID uint, appName string, alerts []ErrorAlert) error {
	if len(alerts) == 0 || !e.CanNotify() {
		return nil
	}
	if len(alerts) == 1 {
		return e.settings.SendNotification(alertSubject(appName, alerts[0]), alertBody(alerts[0].Event, e.settings.BaseURL))
	}
	return e.settings.SendNotification(groupSubject(appName, alerts), groupBody(appID, alerts, e.settings.BaseURL))
}

// alertSubject names one issue: new, or back after how long.
func alertSubject(appName string, alert ErrorAlert) string {
	short := firstLine(FormatHeadline(alert.Event.ErrorClass, alert.Event.ErrorMessage), 120)
	if alert.QuietFor == 0 {
		return fmt.Sprintf("%s — new: %s", appName, short)
	}
	if dur := FormatDuration(alert.QuietFor); dur != "" {
		return fmt.Sprintf("%s — back after %s: %s", appName, dur, short)
	}
	return fmt.Sprintf("%s — back: %s", appName, short)
}

// groupSubject counts the issues: "shop — 3 new errors", "shop — 2 errors
// back", or "shop — 2 new errors, 1 back".
func groupSubject(appName string, alerts []ErrorAlert) string {
	newCount := 0
	for _, a := range alerts {
		if a.QuietFor == 0 {
			newCount++
		}
	}
	back := len(alerts) - newCount
	switch {
	case back == 0:
		return fmt.Sprintf("%s — %d new errors", appName, newCount)
	case newCount == 0:
		return fmt.Sprintf("%s — %d errors back", appName, back)
	}
	return fmt.Sprintf("%s — %d new %s, %d back", appName, newCount, plural(newCount, "error", "errors"), back)
}

// groupBody lists each issue as alertBody names one: the headline, where it
// is, the release, and its link. It ends with the agent command for the
// first.
func groupBody(appID uint, alerts []ErrorAlert, baseURL string) string {
	var body strings.Builder
	for i, a := range alerts[:min(len(alerts), maxListedAlerts)] {
		if i > 0 {
			body.WriteString("\n\n")
		}
		label := "new"
		if a.QuietFor > 0 {
			label = "back"
			if dur := FormatDuration(a.QuietFor); dur != "" {
				label = "back after " + dur
			}
		}
		body.WriteString(label + ": " + firstLine(FormatHeadline(a.Event.ErrorClass, a.Event.ErrorMessage), 200))
		if loc := formatLocation(a.Event.ErrorFile, a.Event.ErrorLine); loc != "" {
			body.WriteString("\n  at " + loc)
		}
		if ctx, err := a.Event.GetContext(); err == nil && ctx.Release != "" {
			body.WriteString("\n  in release " + ctx.Release)
		}
		if baseURL != "" {
			body.WriteString(fmt.Sprintf("\n  %s/issues/%s", baseURL, a.Event.IssueHash))
		}
	}
	if more := len(alerts) - maxListedAlerts; more > 0 {
		body.WriteString(fmt.Sprintf("\n\nAnd %d more", more))
		if baseURL != "" {
			body.WriteString(fmt.Sprintf(": %s/apps/%d/issues", baseURL, appID))
		}
		body.WriteString(".")
	}
	body.WriteString("\n\nAsk your agent: /lognorth:investigate " + alerts[0].Event.IssueHash)
	return body.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
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
