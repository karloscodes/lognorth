package alerts

// New errors and regressions: the issue of an error event is new, or back
// after a quiet day.

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/events"
)

const (
	// RegressionQuietPeriod is how long an issue must be quiet before its
	// return alerts again.
	RegressionQuietPeriod = 24 * time.Hour

	// occurrenceWriteInterval is the most often an issue's last occurrence
	// is written.
	occurrenceWriteInterval = time.Minute
)

// IsRegression reports whether an issue last seen at lastOccurrence has been
// quiet long enough to alert when it comes back at now.
func IsRegression(lastOccurrence, now time.Time) bool {
	return now.Sub(lastOccurrence) >= RegressionQuietPeriod
}

// CheckError records an occurrence of the event's issue, and returns the
// alert to record when the issue is new, or back after RegressionQuietPeriod.
// It keys on the same issue_hash the issue list, mute, and seen state use. It
// keeps the state with or without a channel, so setting one up later does not
// alert on every existing issue.
func CheckError(db *gorm.DB, event *events.Event, baseURL string, now time.Time) (*Alert, error) {
	hash := event.IssueHash
	if hash == "" {
		return nil, nil
	}

	var seen events.IssueSeen
	result := db.Where("error_hash = ?", hash).Limit(1).Find(&seen)
	if result.Error != nil {
		return nil, result.Error
	}
	if seen.ID == 0 {
		if err := db.Create(&events.IssueSeen{ErrorHash: hash, LastOccurrenceAt: now}).Error; err != nil {
			return nil, err
		}
		return errorAlert(event, 0, baseURL), nil
	}

	// Zero LastOccurrenceAt means a row with no tracked occurrence yet; record
	// it silently rather than claiming a 292-year regression.
	var alert *Alert
	if !seen.Muted && !seen.LastOccurrenceAt.IsZero() && IsRegression(seen.LastOccurrenceAt, now) {
		alert = errorAlert(event, now.Sub(seen.LastOccurrenceAt), baseURL)
	}

	// A regression needs a quiet day, so a minute of precision is plenty. An
	// issue that fails on every request then costs one write a minute, not
	// one per error, while ingest waits for the same write lock.
	if now.Sub(seen.LastOccurrenceAt) < occurrenceWriteInterval {
		return alert, nil
	}
	if err := db.Model(&seen).Update("last_occurrence_at", now).Error; err != nil {
		return alert, err
	}
	return alert, nil
}

// errorAlert is the alert of a new issue (quietFor 0), or of one back after
// quietFor.
func errorAlert(event *events.Event, quietFor time.Duration, baseURL string) *Alert {
	short := firstLine(FormatHeadline(event.ErrorClass, event.ErrorMessage), 120)
	a := &Alert{APIKeyID: event.APIKeyID, Kind: KindNewError, Title: "new: " + short, Body: alertBody(event, baseURL)}
	if quietFor > 0 {
		a.Kind = KindRegression
		a.Title = "back: " + short
		if dur := FormatDuration(quietFor); dur != "" {
			a.Title = fmt.Sprintf("back after %s: %s", dur, short)
		}
	}
	return a
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// alertBody names the error, where it is, the release it came in when the
// SDK sent one, and the links to read more.
func alertBody(event *events.Event, baseURL string) string {
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
