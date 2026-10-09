package uptime

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/events"
)

func TestRecentOutages(t *testing.T) {
	fail := func(t *testing.T, db *gorm.DB, message string, at time.Time) {
		t.Helper()
		ctx := events.EventContext{Error: message, ErrorFile: "app.rb", ErrorLine: 1}
		event := &events.Event{APIKeyID: 1, Timestamp: at, IsError: true, Message: message}
		event.SetContext(&ctx)
		events.PopulateErrorFields(event, &ctx)
		if err := events.Create(db, event); err != nil {
			t.Fatalf("Create event: %v", err)
		}
	}
	db := newTestDB(t)
	if err := db.AutoMigrate(&events.Event{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Minute)
	for i, up := range []bool{true, false, false, false, false, true} {
		db.Create(&Check{APIKeyID: 1, CheckedAt: start.Add(time.Duration(i) * time.Minute), Up: up})
	}
	for i := range 4 {
		fail(t, db, "redis refused", start.Add(time.Duration(i+1)*time.Minute))
	}
	for ts := start.Add(-24 * time.Hour); ts.Before(start.Add(6 * time.Minute)); ts = ts.Add(10 * time.Minute) {
		fail(t, db, "card declined", ts)
	}

	outages, err := RecentOutages(db, 1, start.Add(-time.Hour), 10)

	if err != nil {
		t.Fatalf("RecentOutages: %v", err)
	}
	if len(outages) != 1 || len(outages[0].Issues) != 1 || outages[0].Issues[0].Error != "redis refused" {
		t.Fatalf("expected one outage with only the redis issue, got %+v", outages)
	}
	if hit := SurgedIn(outages, outages[0].Issues[0].Hash); len(hit) != 1 || hit[0].Count != 4 {
		t.Errorf("expected the redis issue to have surged in the outage, got %+v", hit)
	}
	if hit := SurgedIn(outages, "some-other-hash"); len(hit) != 0 {
		t.Errorf("expected no outage for another issue, got %+v", hit)
	}
}
