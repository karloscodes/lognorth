package uptime

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestOutages(t *testing.T) {
	// pings records one ping a minute from `from`: + is up, - is down.
	pings := func(db *gorm.DB, appID uint, from time.Time, pattern string) {
		for i, ch := range pattern {
			db.Create(&Check{APIKeyID: appID, CheckedAt: from.Add(time.Duration(i) * time.Minute), Up: ch == '+'})
		}
	}
	from := now.Add(-time.Hour)

	t.Run("a run of three or more failures is an outage", func(t *testing.T) {
		db := newTestDB(t)
		pings(db, 1, from, "++---++")

		outages := Outages(db, 1, from)

		if len(outages) != 1 || !outages[0].Start.Equal(from.Add(2*time.Minute)) || !outages[0].End.Equal(from.Add(5*time.Minute)) || outages[0].Ongoing {
			t.Fatalf("expected one outage from minute 2 to 5, got %+v", outages)
		}
	})

	t.Run("one or two failures are a blip, not an outage", func(t *testing.T) {
		db := newTestDB(t)
		pings(db, 1, from, "+-+--+")

		if outages := Outages(db, 1, from); len(outages) != 0 {
			t.Fatalf("expected no outage, got %+v", outages)
		}
	})

	t.Run("an outage still running ends at its last failure", func(t *testing.T) {
		db := newTestDB(t)
		pings(db, 1, from, "++----")

		outages := Outages(db, 1, from)

		if len(outages) != 1 || !outages[0].Ongoing || !outages[0].End.Equal(from.Add(5*time.Minute)) {
			t.Fatalf("expected one ongoing outage, got %+v", outages)
		}
	})

	t.Run("another app's pings do not count", func(t *testing.T) {
		db := newTestDB(t)
		pings(db, 2, from, "----")

		if outages := Outages(db, 1, from); len(outages) != 0 {
			t.Fatalf("expected no outage, got %+v", outages)
		}
	})
}
