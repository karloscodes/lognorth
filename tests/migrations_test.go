package tests

import (
	"strings"
	"testing"
	"time"

	"lognorth/internal/database"
	"lognorth/internal/events"
)

func TestDataMigrations(t *testing.T) {
	t.Run("each migration runs once", func(t *testing.T) {
		db := setupTestDB(t)
		var before int64
		db.Table("schema_migrations").Count(&before)

		if err := database.Migrate(db); err != nil {
			t.Fatalf("second Migrate failed: %v", err)
		}

		var after int64
		db.Table("schema_migrations").Count(&after)
		if before == 0 || after != before {
			t.Errorf("schema_migrations went from %d to %d, want the same non-zero count", before, after)
		}
	})

	t.Run("upgrading an install from before issue_hash and UTC", func(t *testing.T) {
		db := setupTestDB(t)

		// State an older version left behind.
		db.Exec(`INSERT INTO events (api_key_id, timestamp, message, is_error, context, error_hash_1, issue_hash)
			VALUES (1, '2026-09-22 10:00:00+02:00', 'POST /pay 500', 1, '{"error":"card declined","error_class":"PaymentError"}', '', '')`)
		db.Exec(`ALTER TABLE ingested_events ADD COLUMN processed integer DEFAULT 0`)
		db.Exec(`INSERT INTO ingested_events (api_key_id, message, processed) VALUES (1, 'already moved', 1), (1, 'waiting', 0)`)
		db.Exec(`DELETE FROM schema_migrations`)

		if err := database.Migrate(db); err != nil {
			t.Fatalf("Migrate failed: %v", err)
		}

		var event events.Event
		db.First(&event)
		if event.IssueHash == "" || event.ErrorClass != "PaymentError" {
			t.Errorf("event = %+v, want error fields and issue_hash backfilled", event)
		}
		if want := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC); !event.Timestamp.Equal(want) {
			t.Errorf("timestamp = %v, want %v", event.Timestamp, want)
		}
		var stored string
		db.Raw("SELECT CAST(timestamp AS TEXT) FROM events").Scan(&stored)
		if !strings.HasSuffix(stored, "+00:00") {
			t.Errorf("stored timestamp = %q, want it rewritten in UTC", stored)
		}

		var seen events.IssueSeen
		if err := db.Where("error_hash = ?", event.IssueHash).First(&seen).Error; err != nil || seen.LastOccurrenceAt.IsZero() {
			t.Errorf("seen = %+v, err = %v: want alert state seeded, so the old issue does not alert as new", seen, err)
		}

		var messages []string
		db.Table("ingested_events").Pluck("message", &messages)
		if len(messages) != 1 || messages[0] != "waiting" {
			t.Errorf("ingested_events = %v, want only the waiting row", messages)
		}
	})
}
