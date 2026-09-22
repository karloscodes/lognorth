// Package uptime keeps the result of every uptime ping so the app can show
// whether an app's URL is up, how fast it answers, and when it failed.
// The uptime sweeper (internal/jobs) probes each URL once a minute and
// records one Check per probe.
package uptime

import (
	"slices"
	"strconv"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/apikeys"
)

// Retention is how long checks are kept. At one a minute that is about
// 43,000 rows per app.
const Retention = 30 * 24 * time.Hour

// Check is one probe of an app's URL.
type Check struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	APIKeyID  uint      `gorm:"index:idx_uptime_app_time,priority:1" json:"-"`
	CheckedAt time.Time `gorm:"index:idx_uptime_app_time,priority:2" json:"checked_at"`
	Up        bool      `json:"up"`
	Status    int       `json:"status"`     // HTTP status; 0 when no response came back
	LatencyMS int       `json:"latency_ms"` // time to the response, or to the failure
	Error     string    `gorm:"size:200" json:"error,omitempty"`
}

func (Check) TableName() string { return "uptime_checks" }

// Record stores one probe result.
func Record(db *gorm.DB, check *Check) error {
	return db.Create(check).Error
}

// DeleteOlderThan removes checks older than the cutoff.
func DeleteOlderThan(db *gorm.DB, cutoff time.Time) (int64, error) {
	result := db.Where("checked_at < ?", cutoff).Delete(&Check{})
	return result.RowsAffected, result.Error
}

// State is an app's uptime at a glance, for the status bar and app lists.
type State struct {
	// "up", "failing" (a probe failed, not yet 3 in a row), "down",
	// "pending" (URL set, no probe yet), "off" (checks turned off), or "none" (no URL).
	Status    string     `json:"status"`
	URL       string     `json:"url,omitempty"`
	LatencyMS int        `json:"latency_ms,omitempty"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	DownSince *time.Time `json:"down_since,omitempty"`
	Uptime24h *float64   `json:"uptime_24h,omitempty"` // percent of the last day's checks that were up
}

// States returns the uptime state of each app, keyed by app ID. It runs two
// queries whatever the number of apps.
func States(db *gorm.DB, keys []apikeys.APIKey, now time.Time) map[uint]State {
	ids := make([]uint, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, k.ID)
	}
	latest := latestChecks(db, ids)
	daily := uptimeSince(db, ids, now.Add(-24*time.Hour))

	states := make(map[uint]State, len(keys))
	for _, k := range keys {
		states[k.ID] = stateOf(k, latest[k.ID], daily[k.ID])
	}
	return states
}

func stateOf(k apikeys.APIKey, last *Check, uptime24h *float64) State {
	s := State{URL: k.PingURL, Uptime24h: uptime24h}
	if last != nil {
		checkedAt := last.CheckedAt
		s.CheckedAt = &checkedAt
		s.LatencyMS = last.LatencyMS
	}
	switch {
	case k.PingURL == "":
		s.Status = "none"
	case k.PingDisabled:
		s.Status = "off"
	case k.IsDown:
		s.Status = "down"
		downSince := k.DownSince
		s.DownSince = &downSince
	case last == nil:
		s.Status = "pending"
	case k.ConsecutiveFails > 0:
		s.Status = "failing"
	default:
		s.Status = "up"
	}
	return s
}

// latestChecks returns each app's most recent check.
func latestChecks(db *gorm.DB, ids []uint) map[uint]*Check {
	checks := map[uint]*Check{}
	if len(ids) == 0 {
		return checks
	}
	var rows []Check
	db.Raw(`
		SELECT c.* FROM uptime_checks c
		JOIN (
			SELECT api_key_id, MAX(checked_at) AS latest FROM uptime_checks
			WHERE api_key_id IN ? GROUP BY api_key_id
		) l ON c.api_key_id = l.api_key_id AND c.checked_at = l.latest
	`, ids).Scan(&rows)
	for i := range rows {
		checks[rows[i].APIKeyID] = &rows[i]
	}
	return checks
}

// uptimeSince returns the percent of each app's checks since the cutoff that were up.
func uptimeSince(db *gorm.DB, ids []uint, since time.Time) map[uint]*float64 {
	result := map[uint]*float64{}
	if len(ids) == 0 {
		return result
	}
	var rows []struct {
		APIKeyID uint
		Total    int64
		Ups      int64
	}
	db.Model(&Check{}).
		Select("api_key_id, COUNT(*) AS total, SUM(CASE WHEN up THEN 1 ELSE 0 END) AS ups").
		Where("api_key_id IN ? AND checked_at >= ?", ids, since).
		Group("api_key_id").
		Scan(&rows)
	for _, r := range rows {
		if r.Total == 0 {
			continue
		}
		pct := 100 * float64(r.Ups) / float64(r.Total)
		result[r.APIKeyID] = &pct
	}
	return result
}

// Bar is one slice of the uptime strip.
type Bar struct {
	Start     time.Time `json:"start"`
	Checks    int       `json:"checks"`
	Fails     int       `json:"fails"`
	LatencyMS int       `json:"latency_ms,omitempty"` // average of the successful pings
	Failures  []Reason  `json:"failures,omitempty"`   // why pings failed, most frequent first
}

// Reason is one way pings failed in a bar: an HTTP status ("503") or the
// transport error ("no answer within 5s"), and how often.
type Reason struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// Report is an app's uptime over a window, for the uptime pane.
type Report struct {
	State
	Window      string `json:"window"`                 // "24h"
	Bars        []Bar  `json:"bars"`                   // oldest first
	LastFailure *Check `json:"last_failure,omitempty"` // the most recent failed probe in the window
}

// ReportFor builds the uptime strip for one app: the window split into `bars`
// equal slices, each counting its checks and failures.
func ReportFor(db *gorm.DB, key apikeys.APIKey, now time.Time, window time.Duration, bars int) Report {
	report := Report{
		State:  States(db, []apikeys.APIKey{key}, now)[key.ID],
		Window: "24h",
		Bars:   make([]Bar, bars),
	}
	start := now.Add(-window)
	size := window / time.Duration(bars)
	for i := range report.Bars {
		report.Bars[i].Start = start.Add(time.Duration(i) * size)
	}

	var checks []Check
	db.Where("api_key_id = ? AND checked_at >= ?", key.ID, start).Order("checked_at").Find(&checks)
	latencySum := make([]int, bars)
	for i := range checks {
		c := checks[i]
		idx := int(c.CheckedAt.Sub(start) / size)
		if idx < 0 || idx >= bars {
			continue
		}
		bar := &report.Bars[idx]
		bar.Checks++
		if c.Up {
			latencySum[idx] += c.LatencyMS
			continue
		}
		bar.Fails++
		bar.Failures = addReason(bar.Failures, failureLabel(c))
		report.LastFailure = &checks[i]
	}
	for i := range report.Bars {
		bar := &report.Bars[i]
		if ok := bar.Checks - bar.Fails; ok > 0 {
			bar.LatencyMS = latencySum[i] / ok
		}
		slices.SortStableFunc(bar.Failures, func(a, b Reason) int { return b.Count - a.Count })
	}
	return report
}

// failureLabel names a failed ping: its HTTP status, or the error when no
// response came back.
func failureLabel(c Check) string {
	if c.Status > 0 {
		return strconv.Itoa(c.Status)
	}
	if c.Error != "" {
		return c.Error
	}
	return "no answer"
}

func addReason(reasons []Reason, label string) []Reason {
	for i := range reasons {
		if reasons[i].Label == label {
			reasons[i].Count++
			return reasons
		}
	}
	return append(reasons, Reason{Label: label, Count: 1})
}
