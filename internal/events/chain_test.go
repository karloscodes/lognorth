package events

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestChains(t *testing.T) {
	now := time.Now().UTC()
	// fail records one error of an issue in a request.
	fail := func(t *testing.T, db *gorm.DB, class, trace string, at time.Time) string {
		t.Helper()
		ctx := EventContext{ErrorClass: class, Error: class + " happened", ErrorFile: "app.rb", ErrorLine: 1}
		event := &Event{APIKeyID: 1, Timestamp: at, IsError: true, Message: class, TraceID: trace}
		event.SetContext(&ctx)
		PopulateErrorFields(event, &ctx)
		if err := Create(db, event); err != nil {
			t.Fatalf("Create event: %v", err)
		}
		return event.IssueHash
	}
	// requests records how many requests the app served in the window.
	requests := func(db *gorm.DB, n int64) {
		db.Create(&EndpointStat{APIKeyID: 1, Path: "/", Bucket: now.Add(-time.Hour), Total: n})
	}

	t.Run("an issue that fails first in the same requests is the cause", func(t *testing.T) {
		db := newTestDB(t)
		requests(db, 1000)
		var redis, view string
		for i := range 6 {
			at := now.Add(-time.Duration(i+1) * time.Hour)
			redis = fail(t, db, "Redis::TimeoutError", fmt.Sprint("t", i), at)
			view = fail(t, db, "NoMethodError", fmt.Sprint("t", i), at.Add(time.Millisecond))
		}

		ofView, err := ChainsOf(db, 1, view, now)
		ofRedis, _ := ChainsOf(db, 1, redis, now)

		if err != nil {
			t.Fatalf("ChainsOf: %v", err)
		}
		if len(ofView.Causes) != 1 || ofView.Causes[0].Hash != redis || ofView.Causes[0].Traces != 6 || ofView.Causes[0].Of != 6 || ofView.Causes[0].ErrorClass != "Redis::TimeoutError" {
			t.Errorf("expected Redis as the cause in 6 of 6 requests, got %+v", ofView.Causes)
		}
		if len(ofRedis.Symptoms) != 1 || ofRedis.Symptoms[0].Hash != view {
			t.Errorf("expected NoMethodError as Redis's symptom, got %+v", ofRedis.Symptoms)
		}
	})

	t.Run("an error in almost every request is not a cause", func(t *testing.T) {
		db := newTestDB(t)
		requests(db, 8)
		var view string
		for i := range 8 {
			at := now.Add(-time.Duration(i+1) * time.Hour)
			fail(t, db, "Cache::MissError", fmt.Sprint("t", i), at)
			if i < 6 {
				view = fail(t, db, "NoMethodError", fmt.Sprint("t", i), at.Add(time.Millisecond))
			}
		}

		chains, _ := ChainsOf(db, 1, view, now)

		if len(chains.Causes) != 0 {
			t.Errorf("expected no cause, got %+v", chains.Causes)
		}
	})

	t.Run("four requests are too few to say", func(t *testing.T) {
		db := newTestDB(t)
		requests(db, 1000)
		var view string
		for i := range 4 {
			at := now.Add(-time.Duration(i+1) * time.Hour)
			fail(t, db, "Redis::TimeoutError", fmt.Sprint("t", i), at)
			view = fail(t, db, "NoMethodError", fmt.Sprint("t", i), at.Add(time.Millisecond))
		}

		if chains, _ := ChainsOf(db, 1, view, now); len(chains.Causes) != 0 {
			t.Errorf("expected no cause, got %+v", chains.Causes)
		}
	})

	t.Run("an issue that fails after is not the cause", func(t *testing.T) {
		db := newTestDB(t)
		requests(db, 1000)
		var view string
		for i := range 6 {
			at := now.Add(-time.Duration(i+1) * time.Hour)
			view = fail(t, db, "NoMethodError", fmt.Sprint("t", i), at)
			fail(t, db, "Redis::TimeoutError", fmt.Sprint("t", i), at.Add(time.Millisecond))
		}

		if chains, _ := ChainsOf(db, 1, view, now); len(chains.Causes) != 0 {
			t.Errorf("expected no cause, got %+v", chains.Causes)
		}
	})
}
