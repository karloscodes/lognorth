package events

import (
	"testing"
	"time"
)

func TestAppHours(t *testing.T) {
	t.Run("a batch adds to the hours it touches, one row per app and hour", func(t *testing.T) {
		db := setupTestDB(t)
		noon := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
		first := HourBatch{}
		first.Add(1, false, noon.Add(5*time.Minute))
		first.Add(1, true, noon.Add(59*time.Minute))
		first.Add(2, false, noon.Add(time.Minute))
		first.Flush(db)
		second := HourBatch{}
		second.Add(1, true, noon.Add(30*time.Minute))
		second.Add(1, false, noon.Add(time.Hour))

		err := second.Flush(db)

		if err != nil {
			t.Fatalf("Flush: %v", err)
		}
		var hours []AppHour
		db.Order("api_key_id, hour").Find(&hours)
		want := []AppHour{
			{APIKeyID: 1, Hour: noon, Total: 3, Errors: 2},
			{APIKeyID: 1, Hour: noon.Add(time.Hour), Total: 1},
			{APIKeyID: 2, Hour: noon, Total: 1},
		}
		if len(hours) != len(want) {
			t.Fatalf("hours = %+v, want %+v", hours, want)
		}
		for i, w := range want {
			h := hours[i]
			if h.APIKeyID != w.APIKeyID || !h.Hour.Equal(w.Hour) || h.Total != w.Total || h.Errors != w.Errors {
				t.Errorf("hour %d = %+v, want %+v", i, h, w)
			}
		}
	})

	t.Run("a recount from an hour on keeps the hours before it", func(t *testing.T) {
		db := setupTestDB(t)
		now := time.Now().UTC()
		Create(db, &Event{APIKeyID: 1, Message: "old", Timestamp: now.Add(-5 * time.Hour)})
		Create(db, &Event{APIKeyID: 1, Message: "new", IsError: true, Timestamp: now})
		countHours(t, db)
		Create(db, &Event{APIKeyID: 1, Message: "old, not counted", Timestamp: now.Add(-5 * time.Hour)})
		Create(db, &Event{APIKeyID: 1, Message: "new, not counted", Timestamp: now})

		err := RecountAppHours(db, now.Add(-2*time.Hour))

		if err != nil {
			t.Fatalf("RecountAppHours: %v", err)
		}
		stats := GetStats(db, 1)
		if stats.Total != 3 || stats.Errors != 1 {
			t.Errorf("total %d, errors %d; want 3 and 1: the old hour keeps its count", stats.Total, stats.Errors)
		}
	})

	t.Run("retention deletes whole hours, so the counts still match the events", func(t *testing.T) {
		db := setupTestDB(t)
		boundary := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
		Create(db, &Event{APIKeyID: 1, Message: "before the boundary hour", Timestamp: boundary.Add(-time.Second)})
		Create(db, &Event{APIKeyID: 1, Message: "first second of the boundary hour", Timestamp: boundary})
		Create(db, &Event{APIKeyID: 1, Message: "now", IsError: true, Timestamp: time.Now()})
		countHours(t, db)

		DeleteOlderThan(db, 24*time.Hour)

		var left int64
		db.Model(&Event{}).Count(&left)
		stats := GetStats(db, 1)
		if left != 2 || stats.Total != left || stats.Errors != 1 {
			t.Errorf("%d events left, counted %d (%d errors); want 2 events, both counted", left, stats.Total, stats.Errors)
		}
	})
}
