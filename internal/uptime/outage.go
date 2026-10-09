package uptime

import (
	"time"

	"gorm.io/gorm"
)

// FailuresToDown is how many failed pings in a row make an app down. The
// sweeper alerts at this count, and an outage is a run at least this long.
const FailuresToDown = 3

// Outage is a run of FailuresToDown or more failed pings in a row. Start is
// the first failed ping. End is the first good ping after the run, or the
// last failed ping when the app is still down.
type Outage struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Ongoing bool      `json:"ongoing,omitempty"`
}

// Outages returns the app's outages since the cutoff, oldest first. Shorter
// runs of failures are blips, not outages, as they are for the alert.
func Outages(db *gorm.DB, appID uint, since time.Time) []Outage {
	var checks []Check
	db.Select("checked_at", "up").
		Where("api_key_id = ? AND checked_at >= ?", appID, since.UTC()).
		Order("checked_at").
		Find(&checks)

	var outages []Outage
	fails := 0
	var start, last time.Time
	for _, c := range checks {
		if !c.Up {
			if fails == 0 {
				start = c.CheckedAt
			}
			fails++
			last = c.CheckedAt
			continue
		}
		if fails >= FailuresToDown {
			outages = append(outages, Outage{Start: start, End: c.CheckedAt})
		}
		fails = 0
	}
	if fails >= FailuresToDown {
		outages = append(outages, Outage{Start: start, End: last, Ongoing: true})
	}
	return outages
}
