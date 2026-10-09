package events

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestSurgedDuring(t *testing.T) {
	// fail records errors of one issue, one at each time.
	fail := func(t *testing.T, db *gorm.DB, message string, at ...time.Time) {
		t.Helper()
		for _, ts := range at {
			ctx := EventContext{ErrorClass: "Redis::CannotConnectError", Error: message, ErrorFile: "store.rb", ErrorLine: 18}
			event := &Event{APIKeyID: 1, Timestamp: ts, IsError: true, Message: message}
			if err := event.SetContext(&ctx); err != nil {
				t.Fatalf("SetContext: %v", err)
			}
			PopulateErrorFields(event, &ctx)
			if err := Create(db, event); err != nil {
				t.Fatalf("Create event: %v", err)
			}
		}
	}
	// every returns one time each `step` from `from` up to `to`.
	every := func(from, to time.Time, step time.Duration) []time.Time {
		var at []time.Time
		for ts := from; ts.Before(to); ts = ts.Add(step) {
			at = append(at, ts)
		}
		return at
	}
	start := time.Now().UTC().Add(-2 * time.Hour)
	end := start.Add(10 * time.Minute)

	t.Run("an issue that wakes up with the outage surged", func(t *testing.T) {
		db := newTestDB(t)
		fail(t, db, "connection refused", start.Add(-2*time.Minute), start, start.Add(time.Minute), start.Add(4*time.Minute))

		surges, err := SurgedDuring(db, 1, start, end)

		if err != nil {
			t.Fatalf("SurgedDuring: %v", err)
		}
		if len(surges) != 1 || surges[0].Count != 4 || surges[0].Error != "connection refused" {
			t.Fatalf("expected the issue with 4 errors, got %+v", surges)
		}
	})

	t.Run("an issue that errs all day did not surge", func(t *testing.T) {
		db := newTestDB(t)
		fail(t, db, "card declined", every(start.Add(-24*time.Hour), end, 5*time.Minute)...)

		surges, _ := SurgedDuring(db, 1, start, end)

		if len(surges) != 0 {
			t.Fatalf("expected no surge from a steady issue, got %+v", surges)
		}
	})

	t.Run("two errors are too few to say", func(t *testing.T) {
		db := newTestDB(t)
		fail(t, db, "connection refused", start, start.Add(time.Minute))

		surges, _ := SurgedDuring(db, 1, start, end)

		if len(surges) != 0 {
			t.Fatalf("expected no surge from two errors, got %+v", surges)
		}
	})

	t.Run("errors well before the outage do not count", func(t *testing.T) {
		db := newTestDB(t)
		fail(t, db, "connection refused", start.Add(-30*time.Minute), start.Add(-29*time.Minute), start.Add(-28*time.Minute))

		surges, _ := SurgedDuring(db, 1, start, end)

		if len(surges) != 0 {
			t.Fatalf("expected no surge, got %+v", surges)
		}
	})
}
