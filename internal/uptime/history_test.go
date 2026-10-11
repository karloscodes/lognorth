package uptime

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestHistoryFor(t *testing.T) {
	// pingsFrom records one ping a minute from `from`: + is up, - is down.
	pingsFrom := func(db *gorm.DB, appID uint, from time.Time, pattern string) {
		for i, ch := range pattern {
			db.Create(&Check{APIKeyID: appID, CheckedAt: from.Add(time.Duration(i) * time.Minute), Up: ch == '+'})
		}
	}

	t.Run("each UTC day counts its pings, failures, and minutes down", func(t *testing.T) {
		db := newTestDB(t)
		// Down from 23:58 on Jan 14 until the good ping at 00:01 on Jan 15.
		pingsFrom(db, 1, time.Date(2026, 1, 14, 23, 57, 0, 0, time.UTC), "+---+")
		// One failed ping is a blip: it counts as a failure, not as time down.
		pingsFrom(db, 1, time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC), "+-+")

		h := HistoryFor(db, 1, now, 3)

		want := []Day{
			{APIKeyID: 1, Date: "2026-01-13"},
			{APIKeyID: 1, Date: "2026-01-14", Checks: 3, Fails: 2, DownMinutes: 2},
			{APIKeyID: 1, Date: "2026-01-15", Checks: 5, Fails: 2, DownMinutes: 1},
		}
		if len(h.Days) != len(want) {
			t.Fatalf("expected %d days, got %+v", len(want), h.Days)
		}
		for i := range want {
			if h.Days[i] != want[i] {
				t.Errorf("day %d: expected %+v, got %+v", i, want[i], h.Days[i])
			}
		}
		if len(h.Outages) != 1 || !h.Outages[0].Start.Equal(time.Date(2026, 1, 14, 23, 58, 0, 0, time.UTC)) {
			t.Errorf("expected one outage from 23:58, got %+v", h.Outages)
		}
	})

	t.Run("uptime is the percent of pings that were up, and absent without pings", func(t *testing.T) {
		db := newTestDB(t)
		pingsFrom(db, 1, now.Add(-time.Hour), "++-+")

		pinged := HistoryFor(db, 1, now, 30)
		silent := HistoryFor(db, 2, now, 30)

		if pinged.Uptime == nil || *pinged.Uptime != 75 {
			t.Errorf("expected 75%% uptime, got %v", pinged.Uptime)
		}
		if silent.Uptime != nil {
			t.Errorf("expected no uptime for an app without pings, got %v", *silent.Uptime)
		}
	})

	t.Run("days older than the pings come from the rolled-up counts", func(t *testing.T) {
		db := newTestDB(t)
		pingsFrom(db, 1, time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), "+---++")
		if err := RollUp(db, 1, now); err != nil {
			t.Fatalf("roll up: %v", err)
		}
		db.Where("1 = 1").Delete(&Check{}) // the pings age out
		later := now.AddDate(0, 0, 40)

		h := HistoryFor(db, 1, later, 90)

		jan14 := h.Days[len(h.Days)-1-40-1]
		if jan14 != (Day{APIKeyID: 1, Date: "2026-01-14", Checks: 6, Fails: 3, DownMinutes: 3}) {
			t.Errorf("expected Jan 14 from the stored counts, got %+v", jan14)
		}
		if h.Uptime == nil || *h.Uptime != 50 {
			t.Errorf("expected 50%% uptime from the stored counts, got %v", h.Uptime)
		}
	})

	t.Run("rolling up again stores each day once", func(t *testing.T) {
		db := newTestDB(t)
		pingsFrom(db, 1, time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), "+-+")

		first := RollUp(db, 1, now)
		second := RollUp(db, 1, now)

		var days []Day
		db.Where("checks > 0").Find(&days)
		var total int64
		db.Model(&Day{}).Count(&total)
		if first != nil || second != nil || total != 29 || len(days) != 1 || days[0].Date != "2026-01-14" || days[0].Fails != 1 {
			t.Errorf("expected 29 finished days with one pinged, got %d, %+v (errors %v, %v)", total, days, first, second)
		}
	})

	t.Run("pings before the first day do not count", func(t *testing.T) {
		db := newTestDB(t)
		pingsFrom(db, 1, time.Date(2026, 1, 12, 23, 0, 0, 0, time.UTC), "---")
		pingsFrom(db, 1, time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC), "+")

		h := HistoryFor(db, 1, now, 3)

		if h.Days[0].Checks != 1 || h.Days[0].Fails != 0 || len(h.Outages) != 0 {
			t.Errorf("expected only the one good ping on Jan 13, got %+v and outages %+v", h.Days[0], h.Outages)
		}
	})
}
