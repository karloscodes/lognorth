package events

import (
	"testing"
	"time"
)

func TestCreateIngested(t *testing.T) {
	t.Run("stores the events with UTC timestamps", func(t *testing.T) {
		db := newTestDB(t)
		offset := time.FixedZone("CEST", 2*3600)
		event := &IngestedEvent{APIKeyID: 1, Message: "GET /users", Timestamp: time.Date(2026, 9, 22, 10, 0, 0, 0, offset)}

		err := CreateIngested(db, []*IngestedEvent{event})

		if err != nil {
			t.Fatalf("CreateIngested failed: %v", err)
		}
		stored, _ := NextIngested(db, 0, 10)
		if len(stored) != 1 {
			t.Fatalf("stored %d events, want 1", len(stored))
		}
		if want := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC); !stored[0].Timestamp.Equal(want) || stored[0].Timestamp.Location() != time.UTC {
			t.Errorf("timestamp = %v, want %v in UTC", stored[0].Timestamp, want)
		}
	})

	t.Run("fills a missing timestamp with now", func(t *testing.T) {
		db := newTestDB(t)
		event := &IngestedEvent{APIKeyID: 1, Message: "no timestamp"}

		CreateIngested(db, []*IngestedEvent{event})

		if time.Since(event.Timestamp) > time.Minute {
			t.Errorf("timestamp = %v, want about now", event.Timestamp)
		}
	})
}

func TestNextIngested(t *testing.T) {
	t.Run("returns events after the cursor, oldest first, up to the limit", func(t *testing.T) {
		db := newTestDB(t)
		batch := []*IngestedEvent{}
		for _, m := range []string{"first", "second", "third", "fourth"} {
			batch = append(batch, &IngestedEvent{APIKeyID: 1, Message: m})
		}
		CreateIngested(db, batch)

		page, _ := NextIngested(db, batch[0].ID, 2)

		if len(page) != 2 || page[0].Message != "second" || page[1].Message != "third" {
			t.Errorf("page = %+v, want second and third", page)
		}
	})
}

func TestDeleteIngestedOlderThan(t *testing.T) {
	t.Run("deletes only the old backlog", func(t *testing.T) {
		db := newTestDB(t)
		CreateIngested(db, []*IngestedEvent{{APIKeyID: 1, Message: "old"}, {APIKeyID: 1, Message: "new"}})
		db.Model(&IngestedEvent{}).Where("message = ?", "old").Update("created_at", time.Now().UTC().Add(-48*time.Hour))

		deleted, err := DeleteIngestedOlderThan(db, 24*time.Hour)

		if err != nil || deleted != 1 {
			t.Fatalf("deleted = %d, err = %v, want 1 old event deleted", deleted, err)
		}
		left, _ := NextIngested(db, 0, 10)
		if len(left) != 1 || left[0].Message != "new" {
			t.Errorf("left = %+v, want only the new event", left)
		}
	})
}
