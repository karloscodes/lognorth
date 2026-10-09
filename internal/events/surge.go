package events

import (
	"sort"
	"time"

	"gorm.io/gorm"
)

// An issue surged during an outage when its errors jump next to its own
// normal rate. Errors that happen all day do not count just because they
// also happened during the outage: only the ones that woke up with it.
const (
	SurgeLead     = 5 * time.Minute // errors this long before an outage count
	surgeBaseline = 24 * time.Hour  // the issue's own rate is measured over this
	surgeMinCount = 3               // fewer errors than this is too little to say
	surgeFactor   = 5               // the window's rate must be this many times the normal one
)

// Surge is an issue whose errors surged in a window, with its count there.
type Surge struct {
	Hash       string `json:"hash"`
	Error      string `json:"error"`
	ErrorClass string `json:"error_class,omitempty"`
	Count      int64  `json:"count"`
}

// SurgedDuring returns the app's issues that surged from SurgeLead before
// `from` to `to`, most errors first. An issue surged when it has at least
// surgeMinCount errors in that window, at surgeFactor times the rate it had
// over the day before. The day's count gets one added, so an issue that never
// erred before still needs the minimum count, and one that errs at a steady
// rate never qualifies.
func SurgedDuring(db *gorm.DB, appID uint, from, to time.Time) ([]Surge, error) {
	windowStart := from.Add(-SurgeLead).UTC()
	windowEnd := to.UTC()
	baselineStart := windowStart.Add(-surgeBaseline)

	var rows []struct {
		Surge
		Baseline int64
	}
	err := db.Raw(`
		SELECT issue_hash AS hash, MAX(error_message) AS error, MAX(error_class) AS error_class,
			SUM(CASE WHEN timestamp >= ? THEN 1 ELSE 0 END) AS count,
			SUM(CASE WHEN timestamp < ? THEN 1 ELSE 0 END) AS baseline
		FROM events
		WHERE api_key_id = ? AND is_error = 1 AND issue_hash != ''
			AND timestamp >= ? AND timestamp <= ?
		GROUP BY issue_hash
	`, windowStart, windowStart, appID, baselineStart, windowEnd).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	window := windowEnd.Sub(windowStart).Minutes()
	baseline := surgeBaseline.Minutes()
	surges := []Surge{}
	for _, r := range rows {
		if r.Count < surgeMinCount || window <= 0 {
			continue
		}
		if float64(r.Count)/window >= surgeFactor*float64(r.Baseline+1)/baseline {
			surges = append(surges, r.Surge)
		}
	}
	sort.SliceStable(surges, func(i, j int) bool { return surges[i].Count > surges[j].Count })
	return surges, nil
}
