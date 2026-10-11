package uptime

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"lognorth/internal/apikeys"
)

// DayRetention is how long the daily counts are kept: the 90 days the status
// page shows. The pings behind them go after Retention.
const DayRetention = 90 * 24 * time.Hour

// pingDays is how many UTC days, today included, the pings cover in full.
const pingDays = int(Retention / (24 * time.Hour))

// Day is one UTC day of an app's pings. RollUp stores each finished day, so
// its counts outlive its pings.
type Day struct {
	APIKeyID    uint   `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Date        string `gorm:"primaryKey;size:10" json:"date"` // "2026-10-11"
	Checks      int    `json:"checks"`
	Fails       int    `json:"fails"`
	DownMinutes int    `json:"down_minutes"` // time inside outages; blips do not count
}

func (Day) TableName() string { return "uptime_days" }

// History is an app's pings over whole UTC days, for the status page.
type History struct {
	Days    []Day    // oldest first; the last one is today
	Outages []Outage // oldest first, from the pings still kept
	Uptime  *float64 // percent of the pings that were up; nil without pings
}

// HistoryFor reads the app's last `days` UTC days, today included. The days
// the pings cover come from the pings, in one query. Older days come from
// the stored counts, up to DayRetention.
func HistoryFor(db *gorm.DB, appID uint, now time.Time, days int) History {
	h := fromPings(db, appID, now, min(days, pingDays))
	if days > pingDays {
		h.Days = append(storedDays(db, appID, firstDay(now, days), days-pingDays), h.Days...)
	}
	h.Uptime = uptimeOf(h.Days)
	return h
}

// RollUp stores each finished day the pings cover that is not stored yet.
// A stored day never changes, because all its pings are in, so running it
// again stores nothing new.
func RollUp(db *gorm.DB, appID uint, now time.Time) error {
	var stored int64
	db.Model(&Day{}).Where("api_key_id = ? AND date = ?", appID, firstDay(now, 2).Format(time.DateOnly)).Count(&stored)
	if stored > 0 {
		return nil // yesterday is in, and so is every day before it
	}
	finished := fromPings(db, appID, now, pingDays).Days[:pingDays-1]
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&finished).Error
}

// RollUpAll rolls up every app's pings.
func RollUpAll(db *gorm.DB, now time.Time) error {
	var ids []uint
	if err := db.Model(&apikeys.APIKey{}).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := RollUp(db, id, now); err != nil {
			return err
		}
	}
	return nil
}

// DeleteDaysOlderThan removes the daily counts of days before the cutoff.
func DeleteDaysOlderThan(db *gorm.DB, cutoff time.Time) (int64, error) {
	result := db.Where("date < ?", cutoff.UTC().Format(time.DateOnly)).Delete(&Day{})
	return result.RowsAffected, result.Error
}

// firstDay is the start of the oldest of the last `days` UTC days.
func firstDay(now time.Time, days int) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1-days)
}

// fromPings counts the last `days` UTC days from the pings, and finds their
// outages. Keep `days` within pingDays: older pings are gone.
func fromPings(db *gorm.DB, appID uint, now time.Time, days int) History {
	from := firstDay(now, days)
	checks := pingsSince(db, appID, from)

	h := History{Days: make([]Day, days), Outages: outagesIn(checks)}
	for i := range h.Days {
		h.Days[i] = Day{APIKeyID: appID, Date: from.AddDate(0, 0, i).Format(time.DateOnly)}
	}
	for _, c := range checks {
		i := int(c.CheckedAt.Sub(from) / (24 * time.Hour))
		if i < 0 || i >= days {
			continue
		}
		h.Days[i].Checks++
		if !c.Up {
			h.Days[i].Fails++
		}
	}
	for _, o := range h.Outages {
		for i := range h.Days {
			dayStart := from.AddDate(0, 0, i)
			h.Days[i].DownMinutes += minutesInside(o, dayStart, dayStart.AddDate(0, 0, 1))
		}
	}
	return h
}

// storedDays reads `n` days of stored counts from `from`. A day with no
// stored counts has no pings.
func storedDays(db *gorm.DB, appID uint, from time.Time, n int) []Day {
	var rows []Day
	db.Where("api_key_id = ? AND date >= ? AND date < ?", appID, from.Format(time.DateOnly), from.AddDate(0, 0, n).Format(time.DateOnly)).Find(&rows)
	byDate := make(map[string]Day, len(rows))
	for _, d := range rows {
		byDate[d.Date] = d
	}

	days := make([]Day, n)
	for i := range days {
		date := from.AddDate(0, 0, i).Format(time.DateOnly)
		days[i] = Day{APIKeyID: appID, Date: date}
		if d, ok := byDate[date]; ok {
			days[i] = d
		}
	}
	return days
}

func uptimeOf(days []Day) *float64 {
	checks, fails := 0, 0
	for _, d := range days {
		checks += d.Checks
		fails += d.Fails
	}
	if checks == 0 {
		return nil
	}
	pct := 100 * float64(checks-fails) / float64(checks)
	return &pct
}

// minutesInside is how much of the outage falls between start and end,
// rounded up to whole minutes, so a day an outage touches never shows 0.
func minutesInside(o Outage, start, end time.Time) int {
	from := o.Start
	if start.After(from) {
		from = start
	}
	to := o.End
	if end.Before(to) {
		to = end
	}
	if !to.After(from) {
		return 0
	}
	return int((to.Sub(from) + time.Minute - 1) / time.Minute)
}
